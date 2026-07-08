package model

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/go-redis/redis/v8"
)

const (
	tokenSpendRedisPrefix     = "new-api:token_spend:v1:"
	tokenSpendRedisDirtyKey   = tokenSpendRedisPrefix + "dirty"
	// Single-key script: Redis Cluster requires all KEYS in one slot; dirty tracking is done in Go.
	tokenSpendRedisIncrScript = `
local used = tonumber(redis.call('GET', KEYS[1]) or '0')
local delta = tonumber(ARGV[1])
local cap = tonumber(ARGV[2])
if ARGV[3] == '1' and cap > 0 and used + delta > cap + 0.0001 then
  return -1
end
local newUsed = used + delta
if newUsed < 0 then newUsed = 0 end
redis.call('SET', KEYS[1], string.format('%.4f', newUsed))
redis.call('EXPIRE', KEYS[1], tonumber(ARGV[4]))
return 1
`
)

var tokenSpendRedisIncrSHA string

func tokenSpendRedisOn() bool {
	return common.RedisEnabled && common.RDB != nil
}

func ensureTokenSpendRedisReady() error {
	if !common.RedisEnabled || common.RDB == nil {
		return errors.New("消耗限额依赖 Redis，当前 Redis 不可用")
	}
	if tokenSpendRedisIncrSHA == "" {
		return errors.New("消耗限额 Redis 脚本未初始化")
	}
	return nil
}

func tokenSpendRedisKey(policyID int, periodKey string) string {
	return fmt.Sprintf("%s%d:%s", tokenSpendRedisPrefix, policyID, periodKey)
}

func markTokenSpendRedisDirty(ctx context.Context, spendKey string) {
	if spendKey == "" || spendKey == tokenSpendRedisDirtyKey {
		return
	}
	_ = common.RDB.SAdd(ctx, tokenSpendRedisDirtyKey, spendKey).Err()
}

func tokenSpendRedisTTL(periodType string) time.Duration {
	switch strings.TrimSpace(strings.ToLower(periodType)) {
	case "day":
		return 48 * time.Hour
	case "week":
		return 8 * 24 * time.Hour
	default:
		return 35 * 24 * time.Hour
	}
}

// InitTokenSpendRedis loads Lua script and starts periodic DB flush.
func InitTokenSpendRedis() {
	if !tokenSpendRedisOn() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sha, err := common.RDB.ScriptLoad(ctx, tokenSpendRedisIncrScript).Result()
	if err != nil {
		common.SysLog("token spend redis script load failed: " + err.Error())
		return
	}
	tokenSpendRedisIncrSHA = sha
	interval := common.TokenSpendRedisFlushSec
	if interval <= 0 {
		interval = 5
	}
	gopool.Go(func() {
		ticker := time.NewTicker(time.Duration(interval) * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			flushTokenSpendRedisToDB()
		}
	})
	common.SysLog("token spend redis counter enabled")
}

func applyTokenSpendQuotaDeltaRedis(policies []*TokenSpendPolicy, quotaDelta int, currency string, enforceCap bool, tokenApplyId int) error {
	if quotaDelta == 0 || len(policies) == 0 {
		return nil
	}
	if err := ensureTokenSpendRedisReady(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	now := time.Now()
	updatedAt := common.GetTimestamp()
	for _, orig := range policies {
		if orig == nil || orig.Id <= 0 {
			continue
		}
		row := *orig
		if !row.Enabled {
			continue
		}
		if strings.TrimSpace(row.PeriodType) == "" || row.PeriodType == TokenSpendPolicyPeriodNone {
			continue
		}
		if !common.DecimalGT(row.CapAmount, 0) {
			continue
		}
		if !common.CurrencyEqual(common.NormalizeCurrency(currency), row.Currency) {
			return fmt.Errorf("请求币种 %s 与策略 %s %s 币种 %s 不一致",
				common.NormalizeCurrency(currency), row.ScopeType, row.ScopeCode, common.NormalizeCurrency(row.Currency))
		}
		periodKey := common.PeriodKey(now, row.PeriodType)
		if periodKey == "" {
			continue
		}
		absQuota := quotaDelta
		if absQuota < 0 {
			absQuota = -absQuota
		}
		deltaAmount, err := QuotaToAmount(absQuota, currency)
		if err != nil {
			return err
		}
		if deltaAmount <= 0 {
			continue
		}
		if quotaDelta < 0 {
			deltaAmount = -deltaAmount
		}

		spendKey := tokenSpendRedisKey(row.Id, periodKey)
		if err := ensureTokenSpendRedisSeed(ctx, spendKey, &row, periodKey); err != nil {
			return err
		}

		enforce := "0"
		if enforceCap {
			enforce = "1"
		}
		res, err := common.RDB.EvalSha(ctx, tokenSpendRedisIncrSHA, []string{spendKey},
			fmt.Sprintf("%.4f", deltaAmount),
			fmt.Sprintf("%.4f", row.CapAmount),
			enforce,
			strconv.Itoa(int(tokenSpendRedisTTL(row.PeriodType).Seconds())),
		).Int64()
		if err != nil {
			return err
		}
		if res >= 0 {
			markTokenSpendRedisDirty(ctx, spendKey)
		}
		if res < 0 {
			periodLabel := "本周期"
			switch row.PeriodType {
			case "day":
				periodLabel = "今日"
			case "week":
				periodLabel = "本周"
			case "month":
				periodLabel = "本月"
			}
			used, _ := common.RDB.Get(ctx, spendKey).Float64()
			return fmt.Errorf("消耗封顶：%s %s %s已消耗 %v，本次 %v 将超出上限 %v",
				row.ScopeType, row.ScopeCode, periodLabel, common.RoundDecimal(used), common.RoundDecimal(deltaAmount), common.RoundDecimal(row.CapAmount))
		}
		_ = updatedAt
		_ = tokenApplyId
	}
	return nil
}

func ensureTokenSpendRedisSeed(ctx context.Context, spendKey string, row *TokenSpendPolicy, periodKey string) error {
	n, err := common.RDB.Exists(ctx, spendKey).Result()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	used := 0.0
	if row != nil && strings.TrimSpace(row.PeriodKey) == periodKey {
		used = common.RoundDecimal(row.UsedAmount)
	}
	ttl := tokenSpendRedisTTL(row.PeriodType)
	ok, err := common.RDB.SetNX(ctx, spendKey, fmt.Sprintf("%.4f", used), ttl).Result()
	if err != nil {
		return err
	}
	if ok {
		markTokenSpendRedisDirty(ctx, spendKey)
	}
	return nil
}

func flushTokenSpendRedisToDB() {
	if !tokenSpendRedisOn() || tokenSpendRedisIncrSHA == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	keys, err := common.RDB.SMembers(ctx, tokenSpendRedisDirtyKey).Result()
	if err != nil || len(keys) == 0 {
		return
	}

	updatedAt := common.GetTimestamp()
	for _, key := range keys {
		if !strings.HasPrefix(key, tokenSpendRedisPrefix) || key == tokenSpendRedisDirtyKey {
			_ = common.RDB.SRem(ctx, tokenSpendRedisDirtyKey, key).Err()
			continue
		}
		suffix := strings.TrimPrefix(key, tokenSpendRedisPrefix)
		parts := strings.SplitN(suffix, ":", 2)
		if len(parts) != 2 {
			_ = common.RDB.SRem(ctx, tokenSpendRedisDirtyKey, key).Err()
			continue
		}
		policyID, err := strconv.Atoi(parts[0])
		if err != nil || policyID <= 0 {
			_ = common.RDB.SRem(ctx, tokenSpendRedisDirtyKey, key).Err()
			continue
		}
		periodKey := parts[1]
		val, err := common.RDB.Get(ctx, key).Result()
		if errors.Is(err, redis.Nil) {
			_ = common.RDB.SRem(ctx, tokenSpendRedisDirtyKey, key).Err()
			continue
		}
		if err != nil {
			common.SysLog("token spend redis flush get failed: " + err.Error())
			continue
		}
		used, err := strconv.ParseFloat(val, 64)
		if err != nil {
			_ = common.RDB.SRem(ctx, tokenSpendRedisDirtyKey, key).Err()
			continue
		}
		if used < 0 {
			used = 0
		}
		updates := map[string]interface{}{
			"used_amount": common.RoundDecimal(used),
			"period_key":  periodKey,
			"updated_at":  updatedAt,
		}
		if err := DB.Model(&TokenSpendPolicy{}).Where("id = ?", policyID).Updates(updates).Error; err != nil {
			common.SysLog("token spend redis flush db failed: " + err.Error())
			continue
		}
		_ = common.RDB.SRem(ctx, tokenSpendRedisDirtyKey, key).Err()
	}
}
