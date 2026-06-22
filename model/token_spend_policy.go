package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const TokenSpendPolicyPeriodNone = "none"

type TokenSpendPolicy struct {
	Id           int     `json:"id"`
	ScopeType    string  `json:"scope_type" gorm:"type:varchar(16);uniqueIndex:idx_spend_policies_scope,priority:1"`
	ScopeCode    string  `json:"scope_code" gorm:"type:varchar(64);uniqueIndex:idx_spend_policies_scope,priority:2"`
	TokenType    string  `json:"token_type" gorm:"type:varchar(16);default:'user';uniqueIndex:idx_spend_policies_scope,priority:3"`
	TokenId      int     `json:"token_id" gorm:"index;default:0"`
	CapAmount    float64 `json:"cap_amount" gorm:"type:decimal(12,4);default:0"`
	Currency     string  `json:"currency" gorm:"type:varchar(8);default:'CNY'"`
	PeriodType   string  `json:"period_type" gorm:"type:varchar(16);default:'month'"`
	UsedAmount   float64 `json:"used_amount" gorm:"type:decimal(12,4);default:0"`
	PeriodKey    string  `json:"period_key" gorm:"type:varchar(16);default:''"`
	ParentId     *int    `json:"parent_id" gorm:"index"`
	TokenApplyId int     `json:"token_apply_id" gorm:"index;default:0"`
	Enabled      bool    `json:"enabled" gorm:"default:true"`
	CreatedAt    int64   `json:"created_at" gorm:"bigint;index;default:0"`
	UpdatedAt    int64   `json:"updated_at" gorm:"bigint;index;default:0"`
}

func (TokenSpendPolicy) TableName() string {
	return "token_spend_policies"
}

var allowedTokenSpendScopeTypes = map[string]struct{}{
	"company": {},
	"org":     {},
	"team":    {},
	"project": {},
	"token":   {},
}

var allowedTokenSpendPeriodTypes = map[string]struct{}{
	"day":                     {},
	"week":                    {},
	"month":                   {},
	TokenSpendPolicyPeriodNone: {},
}

func ListTokenSpendPolicies(scopeType, scopeCode, tokenType string) ([]TokenSpendPolicy, error) {
	q := DB.Model(&TokenSpendPolicy{})
	if s := strings.TrimSpace(strings.ToLower(scopeType)); s != "" {
		q = q.Where("scope_type = ?", s)
	}
	if s := strings.TrimSpace(scopeCode); s != "" {
		q = q.Where("scope_code = ?", s)
	}
	if s := strings.TrimSpace(tokenType); s != "" {
		q = q.Where("token_type = ?", normalizeTokenApplyType(s))
	}
	var policies []TokenSpendPolicy
	err := q.Order("id asc").Find(&policies).Error
	return policies, err
}

func getTokenSpendPolicyByScope(tx *gorm.DB, scopeType, scopeCode, tokenType string) (*TokenSpendPolicy, error) {
	policy := &TokenSpendPolicy{}
	err := tx.Where("scope_type = ? AND scope_code = ? AND token_type = ?",
		strings.TrimSpace(strings.ToLower(scopeType)),
		strings.TrimSpace(scopeCode),
		normalizeTokenApplyType(tokenType),
	).First(policy).Error
	if err != nil {
		return nil, err
	}
	return policy, nil
}

func normalizeTokenSpendPeriodType(periodType string) string {
	periodType = strings.TrimSpace(strings.ToLower(periodType))
	if periodType == "" {
		return "month"
	}
	return periodType
}

type tokenSpendPolicySyncSpec struct {
	ScopeType    string
	ScopeCode    string
	TokenType    string
	CapAmount    float64
	Currency     string
	PeriodType   string
	TokenApplyId int
}

func syncTokenSpendPoliciesFromIssue(tx *gorm.DB, req *IssueTokenRequest, tokenType string, tokenApplyId int) error {
	if req == nil {
		return nil
	}
	capAmount := common.RoundDecimal(req.CapAmount)
	periodType := normalizeTokenSpendPeriodType(req.PeriodType)
	if capAmount <= 0 || periodType == TokenSpendPolicyPeriodNone {
		return nil
	}
	scopeType := normalizeTokenBudgetScopeType(req.ScopeType)
	scopeCode := strings.TrimSpace(req.OrgCode)
	if scopeCode == "" {
		return nil
	}
	return upsertTokenSpendPolicyFromIssue(tx, tokenSpendPolicySyncSpec{
		ScopeType:    scopeType,
		ScopeCode:    scopeCode,
		TokenType:    tokenType,
		CapAmount:    capAmount,
		Currency:     common.NormalizeCurrency(req.Currency),
		PeriodType:   periodType,
		TokenApplyId: tokenApplyId,
	})
}

func syncTokenSpendPoliciesFromUpdate(tx *gorm.DB, app *TokenApplyRecord, req *UpdateTokenRequest) error {
	if app == nil || req == nil {
		return nil
	}
	capAmount := common.RoundDecimal(req.CapAmount)
	periodType := normalizeTokenSpendPeriodType(req.PeriodType)
	if capAmount <= 0 || periodType == TokenSpendPolicyPeriodNone {
		return nil
	}
	issueReq := &IssueTokenRequest{
		OrgCode:           app.OrgCode,
		CapAmount:  capAmount,
		PeriodType:        periodType,
		Currency:          firstNonEmpty(req.Currency, app.Currency),
		TokenType:         app.TokenType,
		ScopeType:         req.ScopeType,
	}
	return syncTokenSpendPoliciesFromIssue(tx, issueReq, app.TokenType, app.Id)
}

func upsertTokenSpendPolicyFromIssue(tx *gorm.DB, spec tokenSpendPolicySyncSpec) error {
	spec.ScopeType = strings.TrimSpace(strings.ToLower(spec.ScopeType))
	spec.ScopeCode = strings.TrimSpace(spec.ScopeCode)
	spec.TokenType = normalizeTokenApplyType(spec.TokenType)
	spec.Currency = common.NormalizeCurrency(spec.Currency)
	spec.PeriodType = normalizeTokenSpendPeriodType(spec.PeriodType)
	spec.CapAmount = common.RoundDecimal(spec.CapAmount)
	if spec.ScopeCode == "" || spec.CapAmount <= 0 || spec.PeriodType == TokenSpendPolicyPeriodNone {
		return nil
	}
	if _, ok := allowedTokenSpendScopeTypes[spec.ScopeType]; !ok {
		return fmt.Errorf("不支持的 scope_type: %s", spec.ScopeType)
	}
	if _, ok := allowedTokenSpendPeriodTypes[spec.PeriodType]; !ok {
		return fmt.Errorf("不支持的 period_type: %s", spec.PeriodType)
	}

	existing, err := getTokenSpendPolicyByScope(tx, spec.ScopeType, spec.ScopeCode, spec.TokenType)
	notFound := errors.Is(err, gorm.ErrRecordNotFound)
	if err != nil && !notFound {
		return err
	}

	now := common.GetTimestamp()
	policy := &TokenSpendPolicy{
		ScopeType:    spec.ScopeType,
		ScopeCode:    spec.ScopeCode,
		TokenType:    spec.TokenType,
		CapAmount:    spec.CapAmount,
		Currency:     spec.Currency,
		PeriodType:   spec.PeriodType,
		TokenApplyId: spec.TokenApplyId,
		Enabled:      true,
		UpdatedAt:    now,
	}
	if existing == nil {
		policy.CreatedAt = now
	} else {
		policy.Id = existing.Id
		policy.UsedAmount = existing.UsedAmount
		policy.PeriodKey = existing.PeriodKey
		policy.ParentId = existing.ParentId
		if policy.TokenApplyId <= 0 {
			policy.TokenApplyId = existing.TokenApplyId
		}
	}
	return tx.Save(policy).Error
}

// LoadTokenSpendPolicyChain loads the policy chain used for consumption cap enforcement.
// Priority: token-level policy (token_id) overrides org/project policies.
func LoadTokenSpendPolicyChain(db *gorm.DB, tokenId int, orgCode, tokenType string) ([]*TokenSpendPolicy, error) {
	if db == nil {
		db = DB
	}
	tokenType = normalizeTokenApplyType(tokenType)
	orgCode = strings.TrimSpace(orgCode)

	if tokenId > 0 {
		tokenPolicy := &TokenSpendPolicy{}
		err := db.Where("token_id = ? AND token_type = ? AND enabled = ?", tokenId, tokenType, true).First(tokenPolicy).Error
		if err == nil {
			return loadTokenSpendPolicyParents(db, tokenPolicy)
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}

	if orgCode == "" {
		return nil, nil
	}
	orgPolicy := &TokenSpendPolicy{}
	err := db.Where("scope_code = ? AND token_type = ? AND enabled = ? AND scope_type <> ?",
		orgCode, tokenType, true, "project").First(orgPolicy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return loadTokenSpendPolicyParents(db, orgPolicy)
}

func loadTokenSpendPolicyParents(db *gorm.DB, policy *TokenSpendPolicy) ([]*TokenSpendPolicy, error) {
	if db == nil {
		db = DB
	}
	if policy == nil {
		return nil, nil
	}
	chain := []*TokenSpendPolicy{policy}
	cur := policy
	for cur.ParentId != nil && *cur.ParentId > 0 {
		parent := &TokenSpendPolicy{}
		if err := db.Where("id = ? AND enabled = ?", *cur.ParentId, true).First(parent).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				break
			}
			return nil, err
		}
		chain = append(chain, parent)
		cur = parent
	}
	return chain, nil
}

func ReserveTokenSpendWithPolicies(policies []*TokenSpendPolicy, quotaDelta int, currency string, tokenApplyId int) error {
	if quotaDelta <= 0 || len(policies) == 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		return applyTokenSpendQuotaDeltaInTx(tx, policies, quotaDelta, currency, true, tokenApplyId)
	})
}

func AdjustTokenSpendWithPolicies(policies []*TokenSpendPolicy, preConsumedQuota, actualQuota int, currency string, tokenApplyId int) error {
	if len(policies) == 0 || preConsumedQuota == actualQuota {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		return applyTokenSpendQuotaDeltaInTx(tx, policies, actualQuota-preConsumedQuota, currency, true, tokenApplyId)
	})
}

func ReleaseTokenSpendWithPolicies(policies []*TokenSpendPolicy, preConsumedQuota int, currency string, tokenApplyId int) error {
	if preConsumedQuota <= 0 || len(policies) == 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		return applyTokenSpendQuotaDeltaInTx(tx, policies, -preConsumedQuota, currency, false, tokenApplyId)
	})
}

func applyTokenSpendQuotaDeltaInTx(tx *gorm.DB, policies []*TokenSpendPolicy, quotaDelta int, currency string, enforceCap bool, tokenApplyId int) error {
	if tx == nil {
		return errors.New("消耗计数必须在事务内执行")
	}
	if quotaDelta == 0 || len(policies) == 0 {
		return nil
	}
	ids := make([]int, 0, len(policies))
	for _, p := range policies {
		if p != nil && p.Id > 0 {
			ids = append(ids, p.Id)
		}
	}
	if len(ids) == 0 {
		return nil
	}

	var locked []TokenSpendPolicy
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", ids).Find(&locked).Error; err != nil {
		return err
	}
	byID := make(map[int]TokenSpendPolicy, len(locked))
	for _, row := range locked {
		byID[row.Id] = row
	}

	now := time.Now()
	updatedAt := common.GetTimestamp()
	for _, orig := range policies {
		if orig == nil || orig.Id <= 0 {
			continue
		}
		row, ok := byID[orig.Id]
		if !ok || !row.Enabled {
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

		used := 0.0
		if strings.TrimSpace(row.PeriodKey) == periodKey {
			used = common.RoundDecimal(row.UsedAmount)
		}
		if deltaAmount > 0 && enforceCap && common.SumExceeds(used, deltaAmount, row.CapAmount) {
			periodLabel := "本周期"
			switch row.PeriodType {
			case "day":
				periodLabel = "今日"
			case "week":
				periodLabel = "本周"
			case "month":
				periodLabel = "本月"
			}
			return fmt.Errorf("消耗封顶：%s %s %s已消耗 %v，本次 %v 将超出上限 %v",
				row.ScopeType, row.ScopeCode, periodLabel, used, common.RoundDecimal(deltaAmount), common.RoundDecimal(row.CapAmount))
		}
		newUsed := common.RoundDecimal(used + deltaAmount)
		if newUsed < 0 {
			newUsed = 0
		}
		updates := map[string]interface{}{
			"used_amount": newUsed,
			"period_key":  periodKey,
			"updated_at":  updatedAt,
		}
		if tokenApplyId > 0 {
			updates["token_apply_id"] = tokenApplyId
		}
		if err := tx.Model(&TokenSpendPolicy{}).Where("id = ?", row.Id).Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}

// TokenSpendPolicyView enriches TokenSpendPolicy with current-period usage for portal/API read paths.
type TokenSpendPolicyView struct {
	Id              int     `json:"id"`
	ScopeType       string  `json:"scope_type"`
	ScopeCode       string  `json:"scope_code"`
	TokenType       string  `json:"token_type"`
	CapAmount       float64 `json:"cap_amount"`
	Currency        string  `json:"currency"`
	PeriodType      string  `json:"period_type"`
	ParentId        *int    `json:"parent_id"`
	Enabled         bool    `json:"enabled"`
	TokenApplyId    int     `json:"token_apply_id"`
	PeriodKey       string  `json:"period_key"`
	UsedAmount      float64 `json:"used_amount"`
	RemainingAmount float64 `json:"remaining_amount"`
}

func ListTokenSpendPolicyViews(scopeType, scopeCode, tokenType string) ([]TokenSpendPolicyView, error) {
	policies, err := ListTokenSpendPolicies(scopeType, scopeCode, tokenType)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	views := make([]TokenSpendPolicyView, 0, len(policies))
	for i := range policies {
		p := policies[i]
		if !p.Enabled {
			continue
		}
		if strings.TrimSpace(p.PeriodType) == "" || p.PeriodType == TokenSpendPolicyPeriodNone {
			continue
		}
		if !common.DecimalGT(p.CapAmount, 0) {
			continue
		}
		periodKey := common.PeriodKey(now, p.PeriodType)
		used := 0.0
		if strings.TrimSpace(p.PeriodKey) == periodKey {
			used = common.RoundDecimal(p.UsedAmount)
		}
		remaining := p.CapAmount - used
		if common.DecimalLT(remaining, 0) {
			remaining = 0
		}
		views = append(views, TokenSpendPolicyView{
			Id:              p.Id,
			ScopeType:       p.ScopeType,
			ScopeCode:       p.ScopeCode,
			TokenType:       p.TokenType,
			CapAmount:       common.RoundDecimal(p.CapAmount),
			Currency:        p.Currency,
			PeriodType:      p.PeriodType,
			ParentId:        p.ParentId,
			Enabled:         p.Enabled,
			TokenApplyId:    p.TokenApplyId,
			PeriodKey:       periodKey,
			UsedAmount:      used,
			RemainingAmount: common.RoundDecimal(remaining),
		})
	}
	return views, nil
}
