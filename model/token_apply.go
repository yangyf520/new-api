package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	TokenApplyStatusIssued = "issued"

	TokenApplyTypeUser = "user"
	TokenApplyTypeApp  = "app"

	TokenApplyQuotaModeFixed     = "fixed"
	TokenApplyQuotaModeUnlimited = "unlimited"

	TokenApplyLogActionIssue     = "issue"
	TokenApplyLogActionIncrease  = "increase"
	TokenApplyLogActionDecrease  = "decrease"
	TokenApplyLogActionAdjust    = "adjust"

	maxIssueTokenNameLen = 50
)

var (
	ErrTokenApplyNotFound       = errors.New("申请单不存在")
	ErrTokenApplyDuplicateLog   = errors.New("变更工单已处理")
	ErrTokenApplyBudgetExceeded = errors.New("预算不足")
)

// TokenApplyRecord maps to token_apply_records.
type TokenApplyRecord struct {
	Id          int     `json:"id"`
	TicketNo    string  `json:"ticket_no" gorm:"type:varchar(64);uniqueIndex"`
	RecordId    string  `json:"record_id" gorm:"type:varchar(64);index;default:''"`
	UserId      int     `json:"user_id" gorm:"index;default:0"`
	TokenId     int     `json:"token_id" gorm:"index;default:0"`
	Amount      float64 `json:"amount" gorm:"type:decimal(12,4);default:0"`
	Quota       int     `json:"quota" gorm:"default:0"`
	TokenType   string  `json:"token_type" gorm:"type:varchar(16);default:'user'"`
	OrgCode     string  `json:"org_code" gorm:"type:varchar(64);index;default:''"`
	OrgName     string  `json:"org_name" gorm:"type:varchar(128);default:''"`
	ProjectCode string  `json:"project_code" gorm:"type:varchar(64);default:''"`
	Currency    string  `json:"currency" gorm:"type:varchar(8);default:'CNY'"`
	QuotaMode   string  `json:"quota_mode" gorm:"type:varchar(16);default:'fixed'"`
	UserName    string  `json:"user_name" gorm:"type:varchar(64);default:''"`
	WorkNo      string  `json:"work_no" gorm:"type:varchar(64);default:''"`
	TokenGroup  string  `json:"token_group" gorm:"type:varchar(64);default:''"`
	Remark      string  `json:"remark,omitempty" gorm:"type:text"`
	Status      string  `json:"status" gorm:"type:varchar(16);default:'issued'"`
	IssuedTime  int64   `json:"issued_time" gorm:"bigint;default:0"`
	CreatedAt   int64   `json:"created_at" gorm:"bigint;index;default:0"`
}

func (TokenApplyRecord) TableName() string {
	return "token_apply_records"
}

// TokenApplyLog maps to token_apply_logs.
type TokenApplyLog struct {
	Id                int     `json:"id"`
	TokenApplyId      int     `json:"token_apply_id" gorm:"index;uniqueIndex:idx_token_application_logs_app_change,priority:1"`
	Action            string  `json:"action" gorm:"type:varchar(16)"`
	ChangeTicketNo    string  `json:"change_ticket_no" gorm:"type:varchar(64);default:'';uniqueIndex:idx_token_application_logs_app_change,priority:2"`
	RecordId          string  `json:"record_id" gorm:"type:varchar(64);default:''"`
	AmountBefore      float64 `json:"amount_before" gorm:"type:decimal(12,4);default:0"`
	AmountAfter       float64 `json:"amount_after" gorm:"type:decimal(12,4);default:0"`
	QuotaBefore       int     `json:"quota_before" gorm:"default:0"`
	QuotaAfter        int     `json:"quota_after" gorm:"default:0"`
	BudgetDelta       float64 `json:"budget_delta" gorm:"type:decimal(12,4);default:0"`
	TokenUsedAtChange int     `json:"token_used_at_change" gorm:"default:0"`
	Remark            string  `json:"remark,omitempty" gorm:"type:text"`
	CreatedAt         int64   `json:"created_at" gorm:"bigint;index;default:0"`
}

func (TokenApplyLog) TableName() string {
	return "token_apply_logs"
}

type legacyTokenApplyLog struct {
	ApplicationId int `gorm:"column:application_id"`
}

func (legacyTokenApplyLog) TableName() string { return "token_apply_logs" }

// migrateTokenApplyLogApplicationIdColumn renames token_apply_logs.application_id → token_apply_id.
func migrateTokenApplyLogApplicationIdColumn() error {
	if !DB.Migrator().HasTable(&TokenApplyLog{}) {
		return nil
	}
	if !DB.Migrator().HasColumn(&legacyTokenApplyLog{}, "ApplicationId") {
		return nil
	}
	if DB.Migrator().HasColumn(&TokenApplyLog{}, "TokenApplyId") {
		return nil
	}
	return DB.Migrator().RenameColumn(&legacyTokenApplyLog{}, "ApplicationId", "token_apply_id")
}

func GetTokenApplyRecordByTokenId(tokenId int) (*TokenApplyRecord, error) {
	if tokenId <= 0 {
		return nil, gorm.ErrRecordNotFound
	}
	app := &TokenApplyRecord{}
	err := DB.Where("token_id = ?", tokenId).First(app).Error
	return app, err
}

// IsTokenApplyToken reports whether the token was issued via /api/token-apply (Relay debits the key, not users.quota).
func IsTokenApplyToken(tokenId int) bool {
	if tokenId <= 0 {
		return false
	}
	var count int64
	err := DB.Model(&TokenApplyRecord{}).Where("token_id = ?", tokenId).Count(&count).Error
	return err == nil && count > 0
}

func ResolveTokenApplyTypeForToken(tokenId int) string {
	app, err := GetTokenApplyRecordByTokenId(tokenId)
	if err != nil {
		return TokenApplyTypeUser
	}
	return normalizeTokenApplyType(app.TokenType)
}

func ResolveTokenApplyIdByTokenId(tokenId int) int {
	if tokenId <= 0 {
		return 0
	}
	app, err := GetTokenApplyRecordByTokenId(tokenId)
	if err != nil {
		return 0
	}
	return app.Id
}

type IssueTokenRequest struct {
	TicketNo          string  `json:"ticket_no"`
	RecordId          string  `json:"record_id"`
	Email             string  `json:"email"`
	Amount            float64 `json:"amount"`
	Currency          string  `json:"currency"`
	OrgCode           string  `json:"org_code"`
	OrgName           string  `json:"org_name"`
	OrgBudget         float64 `json:"org_budget"`
	OrgConsumptionCap float64 `json:"org_consumption_cap"`
	ConsumePeriodType string  `json:"consume_period_type"`
	ProjectCode       string  `json:"project_code"`
	ProjectBudget     float64 `json:"project_budget"`
	TokenType         string  `json:"token_type"`
	QuotaMode         string  `json:"quota_mode"`
	ScopeType         string  `json:"scope_type"`
	ParentOrgCode     string  `json:"parent_org_code"`
	ParentOrgBudget   float64 `json:"parent_org_budget"`
	ParentScopeType   string  `json:"parent_scope_type"`
	UserName          string  `json:"user_name"`
	WorkNo            string  `json:"work_no"`
	TokenName         string  `json:"token_name"`
	TokenGroup        string  `json:"token_group"`
	Remark            string  `json:"remark"`
}

type UpdateTokenRequest struct {
	ChangeTicketNo   string  `json:"change_ticket_no"`
	RecordId         string  `json:"record_id"`
	Amount           float64 `json:"amount"`
	Currency         string  `json:"currency"`
	OrgBudget        float64 `json:"org_budget"`
	OrgConsumptionCap float64 `json:"org_consumption_cap"`
	ConsumePeriodType string  `json:"consume_period_type"`
	ProjectBudget    float64 `json:"project_budget"`
	ScopeType        string  `json:"scope_type"`
	ParentOrgCode    string  `json:"parent_org_code"`
	ParentOrgBudget  float64 `json:"parent_org_budget"`
	ParentScopeType  string  `json:"parent_scope_type"`
	Remark           string  `json:"remark"`
}

type IssueTokenResult struct {
	TokenApplyId int    `json:"token_apply_id"`
	UserId       int    `json:"user_id"`
	TokenId       int    `json:"token_id"`
	TokenKey      string `json:"token_key"`
	TokenName     string `json:"token_name"`
	RemainQuota   int    `json:"remain_quota"`
}

type UpdateTokenResult struct {
	TokenApplyId int     `json:"token_apply_id"`
	TokenId      int     `json:"token_id"`
	Amount        float64 `json:"amount"`
	RemainQuota   int     `json:"remain_quota"`
}

func GetTokenApplyRecordByTicketNo(ticketNo string) (*TokenApplyRecord, error) {
	ticketNo = strings.TrimSpace(ticketNo)
	if ticketNo == "" {
		return nil, errors.New("ticket_no 不能为空")
	}
	app := &TokenApplyRecord{}
	err := DB.Where("ticket_no = ?", ticketNo).First(app).Error
	if err != nil {
		return nil, err
	}
	return app, nil
}

func GetTokenApplyRecordById(id int) (*TokenApplyRecord, error) {
	if id <= 0 {
		return nil, errors.New("token_apply_id 无效")
	}
	app := &TokenApplyRecord{}
	err := DB.Where("id = ?", id).First(app).Error
	if err != nil {
		return nil, err
	}
	return app, nil
}

func GetTokenApplyLogByChangeTicket(tokenApplyId int, changeTicketNo string) (*TokenApplyLog, error) {
	logRow := &TokenApplyLog{}
	err := DB.Where("token_apply_id = ? AND change_ticket_no = ?", tokenApplyId, changeTicketNo).First(logRow).Error
	if err != nil {
		return nil, err
	}
	return logRow, nil
}

func IssueTokenApplication(req *IssueTokenRequest) (*IssueTokenResult, error) {
	if req == nil {
		return nil, errors.New("请求不能为空")
	}
	req.TicketNo = strings.TrimSpace(req.TicketNo)
	req.Email = strings.TrimSpace(req.Email)
	req.OrgCode = strings.TrimSpace(req.OrgCode)
	if req.TicketNo == "" || req.Email == "" || req.OrgCode == "" {
		return nil, errors.New("ticket_no、email、org_code 为必填项")
	}

	tokenType := normalizeTokenApplyType(req.TokenType)
	quotaMode := normalizeQuotaMode(req.QuotaMode)
	if tokenType == TokenApplyTypeUser && strings.TrimSpace(req.WorkNo) == "" {
		return nil, errors.New("user 类型必须提供 work_no")
	}

	tokenName, err := parseIssueTokenName(req)
	if err != nil {
		return nil, err
	}
	tokenGroup := resolveIssueTokenGroup(req)

	if existing, err := GetTokenApplyRecordByTicketNo(req.TicketNo); err == nil {
		return replayIssueResult(existing)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	unlimited := quotaMode == TokenApplyQuotaModeUnlimited
	req.Amount = common.RoundDecimal(req.Amount)
	if req.Amount <= 0 {
		return nil, errors.New("amount 必须大于 0")
	}
	totalQuota, err := AmountToQuota(req.Amount, req.Currency)
	if err != nil {
		return nil, err
	}
	if totalQuota <= 0 {
		return nil, errors.New("换算后的额度无效")
	}
	// unlimited 仅表示不计入 ① 审批预算；② 总包仍由 amount 换算为 Key 额度。

	ownerEmail := req.Email
	if tokenType == TokenApplyTypeApp {
		appEmail := strings.TrimSpace(operation_setting.GetTokenApplySetting().AppUserEmail)
		if appEmail == "" {
			return nil, errors.New("未配置 app_user_email")
		}
		ownerEmail = appEmail
	}

	result := &IssueTokenResult{}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if !unlimited {
			if err := syncTokenBudgetPoliciesFromIssue(tx, req, tokenType, 0); err != nil {
				return err
			}
			if err := checkIssueBudget(tx, req, tokenType); err != nil {
				return err
			}
		}
		if err := syncTokenSpendPoliciesFromIssue(tx, req, tokenType, 0); err != nil {
			return err
		}

		user, err := findOrCreateIssueUser(tx, ownerEmail, req.UserName, req.OrgCode)
		if err != nil {
			return err
		}

		maxTokens := operation_setting.GetMaxUserTokens()
		count, err := CountUserTokens(user.Id)
		if err != nil {
			return err
		}
		if int(count)+1 > maxTokens {
			return fmt.Errorf("已达到最大令牌数量限制 (%d)", maxTokens)
		}

		now := common.GetTimestamp()
		key, err := common.GenerateKey()
		if err != nil {
			return err
		}
		token := &Token{
			UserId:         user.Id,
			Name:           tokenName,
			Key:            key,
			CreatedTime:    now,
			AccessedTime:   now,
			ExpiredTime:    -1,
			RemainQuota:    totalQuota,
			UnlimitedQuota: false,
			Group:          tokenGroup,
		}
		if err := tx.Create(token).Error; err != nil {
			return err
		}

		app := &TokenApplyRecord{
			TicketNo:    req.TicketNo,
			RecordId:    strings.TrimSpace(req.RecordId),
			UserId:      user.Id,
			TokenId:     token.Id,
			Amount:      req.Amount,
			Quota:       totalQuota,
			TokenType:   tokenType,
			OrgCode:     req.OrgCode,
			OrgName:     strings.TrimSpace(req.OrgName),
			ProjectCode: strings.TrimSpace(req.ProjectCode),
			Currency:    common.NormalizeCurrency(req.Currency),
			QuotaMode:   quotaMode,
			UserName:    strings.TrimSpace(req.UserName),
			WorkNo:      strings.TrimSpace(req.WorkNo),
			TokenGroup:  tokenGroup,
			Remark:      strings.TrimSpace(req.Remark),
			Status:     TokenApplyStatusIssued,
			IssuedTime: now,
			CreatedAt:  now,
		}
		if err := tx.Create(app).Error; err != nil {
			return err
		}

		if !unlimited {
			if err := syncTokenBudgetPoliciesFromIssue(tx, req, tokenType, app.Id); err != nil {
				return err
			}
		}
		if err := syncTokenSpendPoliciesFromIssue(tx, req, tokenType, app.Id); err != nil {
			return err
		}

		budgetDelta := 0.0
		if !unlimited {
			budgetDelta = req.Amount
		}
		logRow := &TokenApplyLog{
			TokenApplyId:      app.Id,
			Action:            TokenApplyLogActionIssue,
			ChangeTicketNo:    "",
			RecordId:          strings.TrimSpace(req.RecordId),
			AmountBefore:      0,
			AmountAfter:       req.Amount,
			QuotaBefore:       0,
			QuotaAfter:        totalQuota,
			BudgetDelta:       budgetDelta,
			TokenUsedAtChange: 0,
			Remark:            strings.TrimSpace(req.Remark),
			CreatedAt:         now,
		}
		if err := tx.Create(logRow).Error; err != nil {
			return err
		}

		result.TokenApplyId = app.Id
		result.UserId = user.Id
		result.TokenId = token.Id
		result.TokenKey = formatTokenKey(key)
		result.TokenName = tokenName
		result.RemainQuota = totalQuota
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func UpdateTokenApplication(tokenApplyId int, req *UpdateTokenRequest) (*UpdateTokenResult, error) {
	if req == nil {
		return nil, errors.New("请求不能为空")
	}
	req.ChangeTicketNo = strings.TrimSpace(req.ChangeTicketNo)
	if tokenApplyId <= 0 {
		return nil, errors.New("token_apply_id 无效")
	}
	if req.ChangeTicketNo == "" {
		return nil, errors.New("change_ticket_no 为必填项")
	}
	req.Amount = common.RoundDecimal(req.Amount)
	if req.Amount < 0 {
		return nil, errors.New("amount 不能为负数")
	}

	if _, err := GetTokenApplyLogByChangeTicket(tokenApplyId, req.ChangeTicketNo); err == nil {
		return replayUpdateResult(tokenApplyId)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	result := &UpdateTokenResult{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		app := &TokenApplyRecord{}
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("id = ?", tokenApplyId).First(app).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTokenApplyNotFound
			}
			return err
		}

		if app.TokenId <= 0 {
			return errors.New("申请单未关联令牌")
		}

		unlimited := app.QuotaMode == TokenApplyQuotaModeUnlimited
		req.Amount = common.RoundDecimal(req.Amount)
		if req.Amount <= 0 {
			return errors.New("amount 必须大于 0")
		}
		newQuota, err := AmountToQuota(req.Amount, firstNonEmpty(req.Currency, app.Currency))
		if err != nil {
			return err
		}

		token := &Token{}
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("id = ?", app.TokenId).First(token).Error; err != nil {
			return err
		}
		totalUsed := token.UsedQuota
		if newQuota < totalUsed {
			return fmt.Errorf("减额后总可用额度 %d 低于已消耗 %d", newQuota-totalUsed, totalUsed)
		}
		newRemain := newQuota - totalUsed

		oldAmount := common.RoundDecimal(app.Amount)
		oldQuota := app.Quota
		action := TokenApplyLogActionAdjust
		budgetDelta := 0.0
		if req.Amount > oldAmount {
			action = TokenApplyLogActionIncrease
			if !unlimited {
				if err := syncTokenBudgetPoliciesFromUpdate(tx, app, req); err != nil {
					return err
				}
				budgetDelta = common.SubtractDecimal(req.Amount, oldAmount)
				if budgetDelta > 0 {
					if err := checkBudgetDelta(tx, app, budgetDelta); err != nil {
						return err
					}
				}
			}
		} else if req.Amount < oldAmount {
			action = TokenApplyLogActionDecrease
		}
		if err := syncTokenSpendPoliciesFromUpdate(tx, app, req); err != nil {
			return err
		}

		now := common.GetTimestamp()
		if err := tx.Model(token).Updates(map[string]interface{}{
			"remain_quota":    newRemain,
			"unlimited_quota": false,
			"accessed_time":   now,
		}).Error; err != nil {
			return err
		}

		if err := tx.Model(app).Updates(map[string]interface{}{
			"amount":     req.Amount,
			"quota":      newQuota,
			"record_id":  strings.TrimSpace(req.RecordId),
			"remark":     strings.TrimSpace(req.Remark),
		}).Error; err != nil {
			return err
		}

		logRow := &TokenApplyLog{
			TokenApplyId:      app.Id,
			Action:            action,
			ChangeTicketNo:    req.ChangeTicketNo,
			RecordId:          strings.TrimSpace(req.RecordId),
			AmountBefore:      oldAmount,
			AmountAfter:       req.Amount,
			QuotaBefore:       oldQuota,
			QuotaAfter:        newQuota,
			BudgetDelta:       budgetDelta,
			TokenUsedAtChange: totalUsed,
			Remark:            strings.TrimSpace(req.Remark),
			CreatedAt:         now,
		}
		if err := tx.Create(logRow).Error; err != nil {
			return err
		}

		result.TokenApplyId = app.Id
		result.TokenId = app.TokenId
		result.Amount = req.Amount
		result.RemainQuota = newRemain
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func replayIssueResult(app *TokenApplyRecord) (*IssueTokenResult, error) {
	if app.TokenId <= 0 {
		return nil, errors.New("申请单未关联令牌")
	}
	token, err := GetTokenById(app.TokenId)
	if err != nil {
		return nil, err
	}
	return &IssueTokenResult{
		TokenApplyId: app.Id,
		UserId:        app.UserId,
		TokenId:       token.Id,
		TokenKey:      formatTokenKey(token.Key),
		TokenName:     token.Name,
		RemainQuota:   token.RemainQuota,
	}, nil
}

func replayUpdateResult(tokenApplyId int) (*UpdateTokenResult, error) {
	app, err := GetTokenApplyRecordById(tokenApplyId)
	if err != nil {
		return nil, err
	}
	token, err := GetTokenById(app.TokenId)
	if err != nil {
		return nil, err
	}
	return &UpdateTokenResult{
		TokenApplyId: app.Id,
		TokenId:       app.TokenId,
		Amount:        app.Amount,
		RemainQuota:   token.RemainQuota,
	}, nil
}

func QuotaToAmount(quota int, currency string) (float64, error) {
	if quota <= 0 {
		return 0, nil
	}
	dAmount := decimal.NewFromInt(int64(quota)).Div(decimal.NewFromFloat(common.QuotaPerUnit))
	switch common.NormalizeCurrency(currency) {
	case "CNY":
		rate := decimal.NewFromFloat(operation_setting.USDExchangeRate)
		if rate.IsZero() {
			return 0, errors.New("USDExchangeRate 未配置")
		}
		amount, _ := dAmount.Mul(rate).Round(4).Float64()
		return amount, nil
	case "USD":
		amount, _ := dAmount.Round(4).Float64()
		return amount, nil
	default:
		return 0, fmt.Errorf("不支持的币种: %s", currency)
	}
}

func AmountToQuota(amount float64, currency string) (int, error) {
	amount = common.RoundDecimal(amount)
	if amount <= 0 {
		return 0, errors.New("amount 必须大于 0")
	}
	dAmount := decimal.NewFromFloat(amount)
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "", "CNY", "RMB":
		rate := decimal.NewFromFloat(operation_setting.USDExchangeRate)
		if rate.IsZero() {
			return 0, errors.New("USDExchangeRate 未配置")
		}
		dAmount = dAmount.Div(rate)
	case "USD":
	default:
		return 0, fmt.Errorf("不支持的币种: %s", currency)
	}
	quota := dAmount.Mul(decimal.NewFromFloat(common.QuotaPerUnit))
	q := int(quota.IntPart())
	if q <= 0 {
		return 0, errors.New("换算后的额度无效")
	}
	return q, nil
}

func checkIssueBudget(tx *gorm.DB, req *IssueTokenRequest, tokenType string) error {
	if req.Amount <= 0 {
		return errors.New("fixed 模式下 amount 必须大于 0")
	}
	return checkBudgetDelta(tx, &TokenApplyRecord{
		OrgCode:     req.OrgCode,
		ProjectCode: strings.TrimSpace(req.ProjectCode),
		TokenType:   tokenType,
		Currency:    common.NormalizeCurrency(req.Currency),
	}, req.Amount)
}

func checkBudgetDelta(tx *gorm.DB, app *TokenApplyRecord, delta float64) error {
	delta = common.RoundDecimal(delta)
	if delta <= 0 || app == nil {
		return nil
	}
	if tx == nil {
		return errors.New("预算校验必须在事务内执行")
	}
	policies, err := loadTokenBudgetPolicyChain(tx, app)
	if err != nil {
		return err
	}
	if len(policies) == 0 {
		return nil
	}
	if err := lockTokenBudgetPolicyChain(tx, policies); err != nil {
		return err
	}
	allPolicies, err := loadEnabledTokenBudgetPolicies(tx)
	if err != nil {
		return err
	}
	appCurrency := common.NormalizeCurrency(app.Currency)
	for _, p := range policies {
		if !p.Enabled {
			continue
		}
		if !common.CurrencyEqual(appCurrency, p.Currency) {
			return fmt.Errorf("申请币种 %s 与策略 %s %s 币种 %s 不一致",
				appCurrency, p.ScopeType, p.ScopeCode, common.NormalizeCurrency(p.Currency))
		}
		used, err := sumApprovedAmount(tx, p, 0, allPolicies)
		if err != nil {
			return err
		}
		if common.SumExceeds(used, delta, p.TotalAmount) {
			return fmt.Errorf("预算不足：%s %s 累计已批 %v，本次 %v 将超出上限 %v",
				p.ScopeType, p.ScopeCode, common.RoundDecimal(used), delta, common.RoundDecimal(p.TotalAmount))
		}
	}
	return nil
}

func lockTokenBudgetPolicyChain(tx *gorm.DB, policies []*TokenBudgetPolicy) error {
	if len(policies) == 0 {
		return nil
	}
	ids := make([]int, 0, len(policies))
	for _, p := range policies {
		ids = append(ids, p.Id)
	}
	var locked []TokenBudgetPolicy
	return tx.Set("gorm:query_option", "FOR UPDATE").Where("id IN ?", ids).Find(&locked).Error
}

func loadEnabledTokenBudgetPolicies(query *gorm.DB) ([]TokenBudgetPolicy, error) {
	var policies []TokenBudgetPolicy
	if err := query.Where("enabled = ?", true).Find(&policies).Error; err != nil {
		return nil, err
	}
	return policies, nil
}

func collectDescendantPolicies(rootId int, all []TokenBudgetPolicy) []TokenBudgetPolicy {
	children := make(map[int][]TokenBudgetPolicy)
	for _, p := range all {
		if p.ParentId != nil && *p.ParentId > 0 {
			children[*p.ParentId] = append(children[*p.ParentId], p)
		}
	}
	var result []TokenBudgetPolicy
	queue := append([]TokenBudgetPolicy{}, children[rootId]...)
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		result = append(result, p)
		queue = append(queue, children[p.Id]...)
	}
	return result
}

func budgetScopeFilters(policy *TokenBudgetPolicy, all []TokenBudgetPolicy) (orgCodes []string, projectCodes []string) {
	if policy == nil {
		return nil, nil
	}
	if policy.ScopeType == "project" {
		return nil, []string{policy.ScopeCode}
	}
	orgCodes = []string{policy.ScopeCode}
	for _, desc := range collectDescendantPolicies(policy.Id, all) {
		if desc.ScopeType == "project" {
			projectCodes = append(projectCodes, desc.ScopeCode)
		} else {
			orgCodes = append(orgCodes, desc.ScopeCode)
		}
	}
	return dedupeStrings(orgCodes), dedupeStrings(projectCodes)
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func loadTokenBudgetPolicyChain(db *gorm.DB, app *TokenApplyRecord) ([]*TokenBudgetPolicy, error) {
	if db == nil {
		db = DB
	}
	scopeCode := strings.TrimSpace(app.OrgCode)
	if scopeCode == "" {
		return nil, nil
	}
	policy := &TokenBudgetPolicy{}
	err := db.Where("scope_code = ? AND token_type = ? AND enabled = ? AND scope_type <> ?",
		scopeCode, app.TokenType, true, "project").First(policy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if projectCode := strings.TrimSpace(app.ProjectCode); projectCode != "" {
			err = db.Where("scope_type = ? AND scope_code = ? AND token_type = ? AND enabled = ?",
				"project", projectCode, app.TokenType, true).First(policy).Error
		}
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	chain := []*TokenBudgetPolicy{policy}
	for policy.ParentId != nil && *policy.ParentId > 0 {
		parent := &TokenBudgetPolicy{}
		if err := db.Where("id = ? AND enabled = ?", *policy.ParentId, true).First(parent).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				break
			}
			return nil, err
		}
		chain = append(chain, parent)
		policy = parent
	}
	return chain, nil
}

func sumApprovedAmount(query *gorm.DB, policy *TokenBudgetPolicy, periodStart int64, allPolicies []TokenBudgetPolicy) (float64, error) {
	if policy == nil {
		return 0, nil
	}
	q := query.Table("token_apply_logs AS l").
		Select("COALESCE(SUM(l.budget_delta), 0)").
		Joins("JOIN token_apply_records AS a ON a.id = l.token_apply_id").
		Where("a.token_type = ? AND l.created_at >= ?", policy.TokenType, periodStart)
	orgCodes, projectCodes := budgetScopeFilters(policy, allPolicies)
	switch {
	case len(orgCodes) > 0 && len(projectCodes) > 0:
		q = q.Where("a.org_code IN ? OR a.project_code IN ?", orgCodes, projectCodes)
	case len(orgCodes) > 0:
		q = q.Where("a.org_code IN ?", orgCodes)
	case len(projectCodes) > 0:
		q = q.Where("a.project_code IN ?", projectCodes)
	default:
		q = q.Where("a.org_code = ?", policy.ScopeCode)
	}
	var total float64
	if err := q.Scan(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func findOrCreateIssueUser(tx *gorm.DB, email, displayName, group string) (*User, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil, errors.New("email 不能为空")
	}
	user := &User{}
	err := tx.Where("email = ?", email).First(user).Error
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	username, err := buildIssueUsername(tx, email)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(displayName) == "" {
		displayName = username
	}
	groupName := strings.TrimSpace(group)
	if groupName == "" {
		groupName = "default"
	}
	user = &User{
		Username:    username,
		DisplayName: displayName,
		Email:       email,
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       groupName,
		Password:    common.GetRandomString(16),
	}
	if err := user.InsertWithTx(tx, 0); err != nil {
		return nil, err
	}
	return user, nil
}

func buildIssueUsername(tx *gorm.DB, email string) (string, error) {
	base := strings.Split(email, "@")[0]
	base = strings.TrimSpace(base)
	if base == "" {
		base = "user"
	}
	if len(base) > UserNameMaxLength {
		base = base[:UserNameMaxLength]
	}
	candidate := base
	for i := 0; i < 1000; i++ {
		var count int64
		if err := tx.Model(&User{}).Where("username = ?", candidate).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return candidate, nil
		}
		suffix := fmt.Sprintf("%d", i+1)
		maxLen := UserNameMaxLength - len(suffix)
		if maxLen < 1 {
			maxLen = 1
		}
		trimmed := base
		if len(trimmed) > maxLen {
			trimmed = trimmed[:maxLen]
		}
		candidate = trimmed + suffix
	}
	return "", errors.New("无法生成唯一用户名")
}

func parseIssueTokenName(req *IssueTokenRequest) (string, error) {
	if req == nil {
		return "", errors.New("请求不能为空")
	}
	name := strings.TrimSpace(req.TokenName)
	if name == "" {
		name = strings.TrimSpace(req.TicketNo)
	}
	if name == "" {
		return "", errors.New("token_name 为必填项（未传时默认使用 ticket_no）")
	}
	if len(name) > maxIssueTokenNameLen {
		return "", fmt.Errorf("令牌名称不能超过 %d 个字符", maxIssueTokenNameLen)
	}
	return name, nil
}

func resolveIssueTokenGroup(req *IssueTokenRequest) string {
	if req == nil {
		return ""
	}
	if group := strings.TrimSpace(req.TokenGroup); group != "" {
		return group
	}
	return strings.TrimSpace(req.OrgCode)
}

// NormalizeTokenApplyType normalizes token_type to user or app.
func NormalizeTokenApplyType(tokenType string) string {
	return normalizeTokenApplyType(tokenType)
}

func normalizeTokenApplyType(tokenType string) string {
	tokenType = strings.TrimSpace(strings.ToLower(tokenType))
	if tokenType == TokenApplyTypeApp {
		return TokenApplyTypeApp
	}
	return TokenApplyTypeUser
}

func normalizeQuotaMode(mode string) string {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == TokenApplyQuotaModeUnlimited {
		return TokenApplyQuotaModeUnlimited
	}
	return TokenApplyQuotaModeFixed
}

func formatTokenKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if strings.HasPrefix(key, "sk-") {
		return key
	}
	return "sk-" + key
}

// FirstNonEmpty returns the first non-empty trimmed string.
func FirstNonEmpty(values ...string) string {
	return firstNonEmpty(values...)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

type TokenApplyRecordListItem struct {
	TokenApplyRecord
	UserEmail    string  `json:"user_email"`
	RemainQuota  int     `json:"remain_quota"`
	UsedQuota    int     `json:"used_quota"`
	RemainAmount float64 `json:"remain_amount"`
	UsedAmount   float64 `json:"used_amount"`
}

type TokenApplyRecordDetail struct {
	TokenApplyRecordListItem
	Logs []TokenApplyLog `json:"logs"`
}

type TokenApplyTokenView struct {
	TokenId      int     `json:"token_id"`
	TokenName    string  `json:"token_name"`
	TokenKey     string  `json:"token_key"`
	RemainQuota  int     `json:"remain_quota"`
	UsedQuota    int     `json:"used_quota"`
	RemainAmount float64 `json:"remain_amount"`
	UsedAmount   float64 `json:"used_amount"`
	Status       int     `json:"status"`
	Currency     string  `json:"currency"`
}

type TokenBudgetPolicyAdminView struct {
	TokenBudgetPolicy
	PeriodKey       string  `json:"period_key"`
	ApprovedAmount  float64 `json:"approved_amount"`
	RemainingAmount float64 `json:"remaining_amount"`
}

func ListTokenApplicationsAdmin(keyword string, offset, limit int) ([]TokenApplyRecordListItem, int64, error) {
	q := DB.Model(&TokenApplyRecord{})
	keyword = strings.TrimSpace(keyword)
	if keyword != "" {
		if id, err := strconv.Atoi(keyword); err == nil {
			q = q.Where("id = ?", id)
		} else {
			like := "%" + keyword + "%"
			q = q.Where(
				"ticket_no LIKE ? OR org_code LIKE ? OR org_name LIKE ? OR work_no LIKE ? OR user_name LIKE ? OR record_id LIKE ?",
				like, like, like, like, like, like,
			)
		}
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var apps []TokenApplyRecord
	if err := q.Order("id desc").Offset(offset).Limit(limit).Find(&apps).Error; err != nil {
		return nil, 0, err
	}
	items, err := enrichTokenApplicationListItems(apps)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func enrichTokenApplicationListItems(apps []TokenApplyRecord) ([]TokenApplyRecordListItem, error) {
	if len(apps) == 0 {
		return []TokenApplyRecordListItem{}, nil
	}
	userIds := make([]int, 0, len(apps))
	tokenIds := make([]int, 0, len(apps))
	for _, app := range apps {
		if app.UserId > 0 {
			userIds = append(userIds, app.UserId)
		}
		if app.TokenId > 0 {
			tokenIds = append(tokenIds, app.TokenId)
		}
	}
	emailByUserId := map[int]string{}
	if len(userIds) > 0 {
		var users []User
		if err := DB.Select("id", "email").Where("id IN ?", userIds).Find(&users).Error; err != nil {
			return nil, err
		}
		for _, user := range users {
			emailByUserId[user.Id] = user.Email
		}
	}
	type tokenQuotaRow struct {
		Id          int
		RemainQuota int
		UsedQuota   int
	}
	quotaByTokenId := map[int]tokenQuotaRow{}
	if len(tokenIds) > 0 {
		var tokens []Token
		if err := DB.Select("id", "remain_quota", "used_quota").Where("id IN ?", tokenIds).Find(&tokens).Error; err != nil {
			return nil, err
		}
		for _, token := range tokens {
			quotaByTokenId[token.Id] = tokenQuotaRow{
				Id:          token.Id,
				RemainQuota: token.RemainQuota,
				UsedQuota:   token.UsedQuota,
			}
		}
	}
	items := make([]TokenApplyRecordListItem, 0, len(apps))
	for _, app := range apps {
		item := TokenApplyRecordListItem{TokenApplyRecord: app}
		item.UserEmail = emailByUserId[app.UserId]
		if row, ok := quotaByTokenId[app.TokenId]; ok {
			item.RemainQuota = row.RemainQuota
			item.UsedQuota = row.UsedQuota
			currency := app.Currency
			if remainAmt, err := QuotaToAmount(row.RemainQuota, currency); err == nil {
				item.RemainAmount = remainAmt
			}
			if usedAmt, err := QuotaToAmount(row.UsedQuota, currency); err == nil {
				item.UsedAmount = usedAmt
			}
		}
		items = append(items, item)
	}
	return items, nil
}

func GetTokenApplyRecordDetailAdmin(id int) (*TokenApplyRecordDetail, error) {
	if id <= 0 {
		return nil, ErrTokenApplyNotFound
	}
	app := &TokenApplyRecord{}
	if err := DB.Where("id = ?", id).First(app).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTokenApplyNotFound
		}
		return nil, err
	}
	items, err := enrichTokenApplicationListItems([]TokenApplyRecord{*app})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrTokenApplyNotFound
	}
	var logs []TokenApplyLog
	if err := DB.Where("token_apply_id = ?", id).Order("id asc").Find(&logs).Error; err != nil {
		return nil, err
	}
	return &TokenApplyRecordDetail{
		TokenApplyRecordListItem: items[0],
		Logs:                     logs,
	}, nil
}

func ListTokenBudgetPoliciesAdmin(scopeType, scopeCode, tokenType string) ([]TokenBudgetPolicyAdminView, error) {
	policies, err := ListTokenBudgetPolicies(scopeType, scopeCode, tokenType)
	if err != nil {
		return nil, err
	}
	allPolicies, err := loadEnabledTokenBudgetPolicies(DB)
	if err != nil {
		return nil, err
	}
	views := make([]TokenBudgetPolicyAdminView, 0, len(policies))
	for i := range policies {
		policy := policies[i]
		view := TokenBudgetPolicyAdminView{TokenBudgetPolicy: policy}
		view.PeriodKey = "累计"
		approved, err := sumApprovedAmount(DB, &policy, 0, allPolicies)
		if err != nil {
			return nil, err
		}
		view.ApprovedAmount = common.RoundDecimal(approved)
		remaining := policy.TotalAmount - view.ApprovedAmount
		if common.DecimalLT(remaining, 0) {
			remaining = 0
		}
		view.RemainingAmount = common.RoundDecimal(remaining)
		views = append(views, view)
	}
	return views, nil
}

// PolicyInDepartment reports whether scopeCode belongs to orgCode or its subtree.
func PolicyInDepartment(scopeCode, orgCode string) bool {
	return policyInDepartment(scopeCode, orgCode)
}

func policyInDepartment(scopeCode, orgCode string) bool {
	orgCode = strings.TrimSpace(orgCode)
	scopeCode = strings.TrimSpace(scopeCode)
	if orgCode == "" || scopeCode == "" {
		return false
	}
	if scopeCode == orgCode {
		return true
	}
	return strings.HasPrefix(scopeCode, orgCode+"-")
}

// TokenApplyPortalScope scopes read-only portal data: admins see all, others see their department (users.group / org_code).
type TokenApplyPortalScope struct {
	UserId  int
	OrgCode string
	IsAdmin bool
}

func (s TokenApplyPortalScope) canAccessOrg(orgCode string) bool {
	if s.IsAdmin {
		return true
	}
	org := strings.TrimSpace(s.OrgCode)
	if org == "" {
		return false
	}
	return strings.TrimSpace(orgCode) == org
}

func ListTokenApplicationsPortal(scope TokenApplyPortalScope, keyword string, offset, limit int) ([]TokenApplyRecordListItem, int64, error) {
	if scope.IsAdmin {
		return ListTokenApplicationsAdmin(keyword, offset, limit)
	}
	q := DB.Model(&TokenApplyRecord{})
	org := strings.TrimSpace(scope.OrgCode)
	if org == "" {
		return []TokenApplyRecordListItem{}, 0, nil
	}
	q = q.Where("org_code = ?", org)
	keyword = strings.TrimSpace(keyword)
	if keyword != "" {
		if id, err := strconv.Atoi(keyword); err == nil {
			q = q.Where("id = ?", id)
		} else {
			like := "%" + keyword + "%"
			q = q.Where(
				"ticket_no LIKE ? OR org_name LIKE ? OR work_no LIKE ? OR user_name LIKE ? OR record_id LIKE ?",
				like, like, like, like, like,
			)
		}
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var apps []TokenApplyRecord
	if err := q.Order("id desc").Offset(offset).Limit(limit).Find(&apps).Error; err != nil {
		return nil, 0, err
	}
	items, err := enrichTokenApplicationListItems(apps)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func GetTokenApplyRecordDetailPortal(scope TokenApplyPortalScope, id int) (*TokenApplyRecordDetail, error) {
	detail, err := GetTokenApplyRecordDetailAdmin(id)
	if err != nil {
		return nil, err
	}
	if !scope.canAccessOrg(detail.OrgCode) {
		return nil, ErrTokenApplyNotFound
	}
	return detail, nil
}

func tokenApplyVisibleTokenIDs(scope TokenApplyPortalScope) ([]int, error) {
	q := DB.Model(&TokenApplyRecord{}).Where("token_id > 0")
	if !scope.IsAdmin {
		org := strings.TrimSpace(scope.OrgCode)
		if org == "" {
			return []int{}, nil
		}
		q = q.Where("org_code = ?", org)
	}
	var ids []int
	if err := q.Pluck("token_id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func isTokenApplyVisible(scope TokenApplyPortalScope, tokenId int) (bool, error) {
	if tokenId <= 0 {
		return false, nil
	}
	q := DB.Model(&TokenApplyRecord{}).Where("token_id = ?", tokenId)
	if !scope.IsAdmin {
		org := strings.TrimSpace(scope.OrgCode)
		if org == "" {
			return false, nil
		}
		q = q.Where("org_code = ?", org)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func GetTokenByIdIfTokenApplyVisible(scope TokenApplyPortalScope, tokenId int) (*Token, error) {
	ok, err := isTokenApplyVisible(scope, tokenId)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return GetTokenById(tokenId)
}

func GetAllUserTokensAccessible(userId int, scope TokenApplyPortalScope, startIdx, num int) ([]*Token, int64, error) {
	applyIDs, err := tokenApplyVisibleTokenIDs(scope)
	if err != nil {
		return nil, 0, err
	}
	q := DB.Model(&Token{})
	if len(applyIDs) == 0 {
		q = q.Where("user_id = ?", userId)
	} else {
		q = q.Where("user_id = ? OR id IN ?", userId, applyIDs)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var tokens []*Token
	err = q.Order("id desc").Limit(num).Offset(startIdx).Find(&tokens).Error
	return tokens, total, err
}

func GetTokenApplyTokenPortal(scope TokenApplyPortalScope, id int) (*TokenApplyTokenView, error) {
	detail, err := GetTokenApplyRecordDetailPortal(scope, id)
	if err != nil {
		return nil, err
	}
	if detail.TokenId <= 0 {
		return nil, errors.New("申请单未关联令牌")
	}
	token, err := GetTokenById(detail.TokenId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTokenApplyNotFound
		}
		return nil, err
	}
	currency := detail.Currency
	remainAmount, _ := QuotaToAmount(token.RemainQuota, currency)
	usedAmount, _ := QuotaToAmount(token.UsedQuota, currency)
	return &TokenApplyTokenView{
		TokenId:      token.Id,
		TokenName:    token.Name,
		TokenKey:     formatTokenKey(token.Key),
		RemainQuota:  token.RemainQuota,
		UsedQuota:    token.UsedQuota,
		RemainAmount: remainAmount,
		UsedAmount:   usedAmount,
		Status:       token.Status,
		Currency:     currency,
	}, nil
}

func ListTokenBudgetPoliciesPortal(scope TokenApplyPortalScope, scopeType, scopeCode, tokenType string) ([]TokenBudgetPolicyAdminView, error) {
	views, err := ListTokenBudgetPoliciesAdmin(scopeType, scopeCode, tokenType)
	if err != nil || scope.IsAdmin {
		return views, err
	}
	org := strings.TrimSpace(scope.OrgCode)
	if org == "" {
		return []TokenBudgetPolicyAdminView{}, nil
	}
	filtered := make([]TokenBudgetPolicyAdminView, 0, len(views))
	for _, view := range views {
		if policyInDepartment(view.ScopeCode, org) {
			filtered = append(filtered, view)
		}
	}
	return filtered, nil
}

func ListTokenSpendPolicyViewsPortal(scope TokenApplyPortalScope, scopeType, scopeCode, tokenType string) ([]TokenSpendPolicyView, error) {
	views, err := ListTokenSpendPolicyViews(scopeType, scopeCode, tokenType)
	if err != nil || scope.IsAdmin {
		return views, err
	}
	org := strings.TrimSpace(scope.OrgCode)
	if org == "" {
		return []TokenSpendPolicyView{}, nil
	}
	filtered := make([]TokenSpendPolicyView, 0, len(views))
	for _, view := range views {
		if policyInDepartment(view.ScopeCode, org) {
			filtered = append(filtered, view)
		}
	}
	return filtered, nil
}
