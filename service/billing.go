package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const (
	BillingSourceWallet       = "wallet"
	BillingSourceSubscription = "subscription"
)

func skipTokenQuotaDeduction(relayInfo *relaycommon.RelayInfo) bool {
	if relayInfo == nil || relayInfo.IsPlayground {
		return true
	}
	// token-apply 总包始终由 remain_quota 限制，忽略 UnlimitedQuota 标记（兼容历史数据）
	if model.IsTokenApplyToken(relayInfo.TokenId) {
		return false
	}
	return relayInfo.TokenUnlimited
}

// PreConsumeBilling 根据用户计费偏好创建 BillingSession 并执行预扣费。
// 会话存储在 relayInfo.Billing 上，供后续 Settle / Refund 使用。
func PreConsumeBilling(c *gin.Context, preConsumedQuota int, relayInfo *relaycommon.RelayInfo) *types.NewAPIError {
	if apiErr := ReserveConsumption(relayInfo, preConsumedQuota); apiErr != nil {
		return apiErr
	}
	session, apiErr := NewBillingSession(c, relayInfo, preConsumedQuota)
	if apiErr != nil {
		_ = ReleaseConsumption(relayInfo, preConsumedQuota)
		return apiErr
	}
	session.consumptionReserved = preConsumedQuota
	if relayInfo != nil {
		relayInfo.ConsumptionReservedQuota = preConsumedQuota
	}
	relayInfo.Billing = session
	return nil
}

// PreConsumeConsumptionOnly 免费模型等跳过钱包/token 预扣时，仍对 ③ counter 预占。
func PreConsumeConsumptionOnly(relayInfo *relaycommon.RelayInfo, estimatedQuota int) *types.NewAPIError {
	if apiErr := ReserveConsumption(relayInfo, estimatedQuota); apiErr != nil {
		return apiErr
	}
	if relayInfo != nil {
		relayInfo.ConsumptionReservedQuota = estimatedQuota
	}
	return nil
}

// ReleasePreConsumedConsumption 请求失败且未创建 BillingSession 时释放 counter 预占。
func ReleasePreConsumedConsumption(relayInfo *relaycommon.RelayInfo) {
	if relayInfo == nil || relayInfo.ConsumptionReservedQuota <= 0 {
		return
	}
	reserved := relayInfo.ConsumptionReservedQuota
	relayInfo.ConsumptionReservedQuota = 0
	if err := ReleaseConsumption(relayInfo, reserved); err != nil {
		common.SysLog("error releasing consumption reservation after request failure: " + err.Error())
	}
}

// ---------------------------------------------------------------------------
// SettleBilling — 后结算辅助函数
// ---------------------------------------------------------------------------

// SettleBilling 执行计费结算。如果 RelayInfo 上有 BillingSession 则通过 session 结算，
// 否则回退到旧的 PostConsumeQuota 路径（兼容按次计费等场景）。
func SettleBilling(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, actualQuota int) error {
	if relayInfo.Billing != nil {
		preConsumed := relayInfo.Billing.GetPreConsumedQuota()
		consumptionReserved := getConsumptionReserved(relayInfo)
		delta := actualQuota - preConsumed

		if delta > 0 {
			logger.LogInfo(ctx, fmt.Sprintf("预扣费后补扣费：%s（实际消耗：%s，预扣费：%s）",
				logger.FormatQuota(delta),
				logger.FormatQuota(actualQuota),
				logger.FormatQuota(preConsumed),
			))
		} else if delta < 0 {
			logger.LogInfo(ctx, fmt.Sprintf("预扣费后返还扣费：%s（实际消耗：%s，预扣费：%s）",
				logger.FormatQuota(-delta),
				logger.FormatQuota(actualQuota),
				logger.FormatQuota(preConsumed),
			))
		} else {
			logger.LogInfo(ctx, fmt.Sprintf("预扣费与实际消耗一致，无需调整：%s（按次计费）",
				logger.FormatQuota(actualQuota),
			))
		}

		if err := AdjustConsumption(relayInfo, consumptionReserved, actualQuota); err != nil {
			logBillingSettleFailure(ctx, relayInfo, actualQuota, "adjust consumption", err)
			return err
		}
		if err := relayInfo.Billing.Settle(actualQuota); err != nil {
			if rollbackErr := AdjustConsumption(relayInfo, actualQuota, consumptionReserved); rollbackErr != nil {
				logBillingSettleFailure(ctx, relayInfo, actualQuota, "rollback consumption after settle failed", rollbackErr)
			}
			logBillingSettleFailure(ctx, relayInfo, actualQuota, "settle billing session", err)
			return err
		}
		relayInfo.ConsumptionReservedQuota = 0

		if actualQuota != 0 {
			if relayInfo.BillingSource == BillingSourceSubscription {
				checkAndSendSubscriptionQuotaNotify(relayInfo)
			} else {
				checkAndSendQuotaNotify(relayInfo, actualQuota-preConsumed, preConsumed)
			}
		}
		return nil
	}

	// 回退：无 BillingSession 时使用旧路径
	consumptionReserved := getConsumptionReserved(relayInfo)
	if err := AdjustConsumption(relayInfo, consumptionReserved, actualQuota); err != nil {
		logBillingSettleFailure(ctx, relayInfo, actualQuota, "adjust consumption (legacy)", err)
		return err
	}

	quotaDelta := actualQuota - relayInfo.FinalPreConsumedQuota
	if quotaDelta != 0 {
		if err := PostConsumeQuota(relayInfo, quotaDelta, relayInfo.FinalPreConsumedQuota, true); err != nil {
			if rollbackErr := AdjustConsumption(relayInfo, actualQuota, consumptionReserved); rollbackErr != nil {
				logBillingSettleFailure(ctx, relayInfo, actualQuota, "rollback consumption after legacy post-consume failed", rollbackErr)
			}
			logBillingSettleFailure(ctx, relayInfo, actualQuota, "legacy post-consume quota", err)
			return err
		}
	}
	relayInfo.ConsumptionReservedQuota = 0
	return nil
}

func getConsumptionReserved(relayInfo *relaycommon.RelayInfo) int {
	if relayInfo == nil {
		return 0
	}
	if relayInfo.ConsumptionReservedQuota > 0 {
		return relayInfo.ConsumptionReservedQuota
	}
	if relayInfo.Billing != nil {
		if session, ok := relayInfo.Billing.(*BillingSession); ok && session.consumptionReserved > 0 {
			return session.consumptionReserved
		}
		return relayInfo.Billing.GetPreConsumedQuota()
	}
	return relayInfo.FinalPreConsumedQuota
}

func logBillingSettleFailure(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, actualQuota int, stage string, err error) {
	tokenId := 0
	userId := 0
	if relayInfo != nil {
		tokenId = relayInfo.TokenId
		userId = relayInfo.UserId
	}
	msg := fmt.Sprintf("billing settle failed (%s): userId=%d tokenId=%d actualQuota=%s err=%v",
		stage, userId, tokenId, logger.FormatQuota(actualQuota), err)
	logger.LogError(ctx, msg)
	common.SysLog(msg)
}
