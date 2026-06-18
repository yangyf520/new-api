package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type TokenBudgetPolicy struct {
	Id           int     `json:"id"`
	ScopeType    string  `json:"scope_type" gorm:"type:varchar(16);uniqueIndex:idx_budget_policies_scope,priority:1"`
	ScopeCode    string  `json:"scope_code" gorm:"type:varchar(64);uniqueIndex:idx_budget_policies_scope,priority:2"`
	TokenType    string  `json:"token_type" gorm:"type:varchar(16);default:'user';uniqueIndex:idx_budget_policies_scope,priority:3"`
	TotalAmount  float64 `json:"total_amount" gorm:"type:decimal(12,4);default:0"`
	Currency     string  `json:"currency" gorm:"type:varchar(8);default:'CNY'"`
	ParentId     *int    `json:"parent_id" gorm:"index"`
	TokenApplyId          int     `json:"token_apply_id" gorm:"index;default:0"`
	Enabled               bool    `json:"enabled" gorm:"default:true"`
	CreatedAt             int64   `json:"created_at" gorm:"bigint;index;default:0"`
	UpdatedAt             int64   `json:"updated_at" gorm:"bigint;index;default:0"`
}

func (TokenBudgetPolicy) TableName() string {
	return "token_budget_policies"
}

var (
	errTokenBudgetPolicyParentNotFound = errors.New("父策略不存在")
	errTokenBudgetPolicySelfParent     = errors.New("parent_id 不能指向自身")
	errTokenBudgetPolicyCycle          = errors.New("parent_id 存在循环引用")
)

var allowedTokenBudgetScopeTypes = map[string]struct{}{
	"company": {},
	"org":     {},
	"team":    {},
	"project": {},
}

func (p *TokenBudgetPolicy) BeforeSave(tx *gorm.DB) error {
	return validateTokenBudgetPolicy(tx, p)
}

// ValidateAllTokenBudgetPolicies checks every row; used after migration to reject invalid data.
func ValidateAllTokenBudgetPolicies() error {
	var policies []TokenBudgetPolicy
	if err := DB.Find(&policies).Error; err != nil {
		return err
	}
	for i := range policies {
		policy := policies[i]
		if err := validateTokenBudgetPolicy(DB, &policy); err != nil {
			return fmt.Errorf("token_budget_policies id=%d (%s %s): %w",
				policy.Id, policy.ScopeType, policy.ScopeCode, err)
		}
	}
	return nil
}

func validateTokenBudgetPolicy(tx *gorm.DB, p *TokenBudgetPolicy) error {
	if p == nil {
		return errors.New("策略不能为空")
	}
	p.ScopeType = strings.TrimSpace(strings.ToLower(p.ScopeType))
	p.ScopeCode = strings.TrimSpace(p.ScopeCode)
	p.TokenType = strings.TrimSpace(strings.ToLower(p.TokenType))
	p.Currency = common.NormalizeCurrency(p.Currency)

	p.TotalAmount = common.RoundDecimal(p.TotalAmount)

	if p.ScopeCode == "" {
		return errors.New("scope_code 不能为空")
	}
	if p.TokenType == "" {
		p.TokenType = TokenApplyTypeUser
	}
	if _, ok := allowedTokenBudgetScopeTypes[p.ScopeType]; !ok {
		return fmt.Errorf("不支持的 scope_type: %s", p.ScopeType)
	}
	if common.DecimalLT(p.TotalAmount, 0) {
		return errors.New("total_amount 不能为负数")
	}
	if p.Enabled && !common.DecimalGT(p.TotalAmount, 0) {
		return errors.New("启用策略时 total_amount 必须大于 0")
	}

	if p.ParentId != nil {
		if *p.ParentId <= 0 {
			return errors.New("parent_id 无效")
		}
		if p.Id > 0 && *p.ParentId == p.Id {
			return errTokenBudgetPolicySelfParent
		}
		parent, err := getTokenBudgetPolicyByID(tx, *p.ParentId)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errTokenBudgetPolicyParentNotFound
			}
			return err
		}
		if err := ensureNoTokenBudgetPolicyCycle(tx, p.Id, *p.ParentId); err != nil {
			return err
		}
		if p.TokenType != parent.TokenType {
			return errors.New("token_type 必须与父策略一致")
		}
		if !common.CurrencyEqual(p.Currency, parent.Currency) {
			return errors.New("currency 必须与父策略一致")
		}
		siblingSum, err := sumTokenBudgetPolicySiblingCaps(tx, *p.ParentId, p.Id)
		if err != nil {
			return err
		}
		if common.DecimalGT(siblingSum+p.TotalAmount, parent.TotalAmount) {
			return fmt.Errorf("子策略上限之和 %v 超过父策略 %s 上限 %v",
				common.RoundDecimal(siblingSum+p.TotalAmount), parent.ScopeCode, common.RoundDecimal(parent.TotalAmount))
		}
	}

	if p.Id > 0 {
		childSum, err := sumTokenBudgetPolicyChildCaps(tx, p.Id)
		if err != nil {
			return err
		}
		if common.DecimalGT(childSum, p.TotalAmount) {
			return fmt.Errorf("子策略上限之和 %v 超过本策略上限 %v",
				common.RoundDecimal(childSum), common.RoundDecimal(p.TotalAmount))
		}
	}
	return nil
}

func getTokenBudgetPolicyByID(tx *gorm.DB, id int) (*TokenBudgetPolicy, error) {
	policy := &TokenBudgetPolicy{}
	err := tx.Where("id = ?", id).First(policy).Error
	return policy, err
}

func ensureNoTokenBudgetPolicyCycle(tx *gorm.DB, policyID int, parentID int) error {
	visited := map[int]struct{}{}
	if policyID > 0 {
		visited[policyID] = struct{}{}
	}
	current := parentID
	for current > 0 {
		if _, ok := visited[current]; ok {
			return errTokenBudgetPolicyCycle
		}
		visited[current] = struct{}{}
		row := &TokenBudgetPolicy{}
		if err := tx.Select("id", "parent_id").Where("id = ?", current).First(row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errTokenBudgetPolicyParentNotFound
			}
			return err
		}
		if row.ParentId == nil || *row.ParentId <= 0 {
			return nil
		}
		current = *row.ParentId
	}
	return nil
}

func sumTokenBudgetPolicySiblingCaps(tx *gorm.DB, parentID int, excludeID int) (float64, error) {
	q := tx.Model(&TokenBudgetPolicy{}).Where("parent_id = ?", parentID)
	if excludeID > 0 {
		q = q.Where("id <> ?", excludeID)
	}
	var total float64
	if err := q.Select("COALESCE(SUM(total_amount), 0)").Scan(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func sumTokenBudgetPolicyChildCaps(tx *gorm.DB, policyID int) (float64, error) {
	var total float64
	err := tx.Model(&TokenBudgetPolicy{}).
		Where("parent_id = ?", policyID).
		Select("COALESCE(SUM(total_amount), 0)").
		Scan(&total).Error
	return total, err
}

func ListTokenBudgetPolicies(scopeType, scopeCode, tokenType string) ([]TokenBudgetPolicy, error) {
	q := DB.Model(&TokenBudgetPolicy{})
	if s := strings.TrimSpace(strings.ToLower(scopeType)); s != "" {
		q = q.Where("scope_type = ?", s)
	}
	if s := strings.TrimSpace(scopeCode); s != "" {
		q = q.Where("scope_code = ?", s)
	}
	if s := strings.TrimSpace(tokenType); s != "" {
		q = q.Where("token_type = ?", normalizeTokenApplyType(s))
	}
	var policies []TokenBudgetPolicy
	err := q.Order("id asc").Find(&policies).Error
	return policies, err
}

func getTokenBudgetPolicyByScope(tx *gorm.DB, scopeType, scopeCode, tokenType string) (*TokenBudgetPolicy, error) {
	policy := &TokenBudgetPolicy{}
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

type tokenBudgetPolicySyncSpec struct {
	ScopeType       string
	ScopeCode       string
	TokenType       string
	TotalAmount     float64
	Currency        string
	ParentScopeType string
	ParentScopeCode string
	TokenApplyId    int
}

func normalizeTokenBudgetScopeType(scopeType string) string {
	scopeType = strings.TrimSpace(strings.ToLower(scopeType))
	if scopeType == "" {
		return "team"
	}
	return scopeType
}

func syncTokenBudgetPoliciesFromIssue(tx *gorm.DB, req *IssueTokenRequest, tokenType string, tokenApplyId int) error {
	if req == nil {
		return nil
	}
	currency := common.NormalizeCurrency(req.Currency)
	scopeType := normalizeTokenBudgetScopeType(req.ScopeType)
	parentScopeType := normalizeTokenBudgetScopeType(req.ParentScopeType)
	if strings.TrimSpace(req.ParentScopeType) == "" {
		parentScopeType = "org"
	}

	specs := make([]tokenBudgetPolicySyncSpec, 0, 3)

	parentCode := strings.TrimSpace(req.ParentOrgCode)
	parentBudget := common.RoundDecimal(req.ParentOrgBudget)
	if parentCode != "" && parentBudget > 0 {
		specs = append(specs, tokenBudgetPolicySyncSpec{
			ScopeType:    parentScopeType,
			ScopeCode:    parentCode,
			TokenType:    tokenType,
			TotalAmount:  parentBudget,
			Currency:     currency,
			TokenApplyId: tokenApplyId,
		})
	}

	orgBudget := common.RoundDecimal(req.OrgBudget)
	if orgBudget > 0 {
		spec := tokenBudgetPolicySyncSpec{
			ScopeType:    scopeType,
			ScopeCode:    strings.TrimSpace(req.OrgCode),
			TokenType:    tokenType,
			TotalAmount:  orgBudget,
			Currency:     currency,
			TokenApplyId: tokenApplyId,
		}
		if parentCode != "" {
			spec.ParentScopeType = parentScopeType
			spec.ParentScopeCode = parentCode
		}
		specs = append(specs, spec)
	}

	projectCode := strings.TrimSpace(req.ProjectCode)
	projectBudget := common.RoundDecimal(req.ProjectBudget)
	if projectCode != "" && projectBudget > 0 {
		specs = append(specs, tokenBudgetPolicySyncSpec{
			ScopeType:       "project",
			ScopeCode:       projectCode,
			TokenType:       tokenType,
			TotalAmount:     projectBudget,
			Currency:        currency,
			ParentScopeType: scopeType,
			ParentScopeCode: strings.TrimSpace(req.OrgCode),
			TokenApplyId:    tokenApplyId,
		})
	}

	for _, spec := range specs {
		if err := upsertTokenBudgetPolicyFromIssue(tx, spec); err != nil {
			return err
		}
	}
	return nil
}

func syncTokenBudgetPoliciesFromUpdate(tx *gorm.DB, app *TokenApplyRecord, req *UpdateTokenRequest) error {
	if app == nil || req == nil {
		return nil
	}
	if common.RoundDecimal(req.OrgBudget) <= 0 &&
		common.RoundDecimal(req.ProjectBudget) <= 0 &&
		common.RoundDecimal(req.ParentOrgBudget) <= 0 {
		return nil
	}
	issueReq := &IssueTokenRequest{
		OrgCode:          app.OrgCode,
		OrgBudget:        req.OrgBudget,
		ProjectCode:      app.ProjectCode,
		ProjectBudget:    req.ProjectBudget,
		Currency:         firstNonEmpty(req.Currency, app.Currency),
		TokenType:        app.TokenType,
		ScopeType:        req.ScopeType,
		ParentOrgCode:    req.ParentOrgCode,
		ParentOrgBudget:  req.ParentOrgBudget,
		ParentScopeType:  req.ParentScopeType,
	}
	return syncTokenBudgetPoliciesFromIssue(tx, issueReq, app.TokenType, app.Id)
}

func upsertTokenBudgetPolicyFromIssue(tx *gorm.DB, spec tokenBudgetPolicySyncSpec) error {
	spec.ScopeType = strings.TrimSpace(strings.ToLower(spec.ScopeType))
	spec.ScopeCode = strings.TrimSpace(spec.ScopeCode)
	spec.TokenType = normalizeTokenApplyType(spec.TokenType)
	spec.Currency = common.NormalizeCurrency(spec.Currency)
	spec.TotalAmount = common.RoundDecimal(spec.TotalAmount)
	if spec.ScopeCode == "" || spec.TotalAmount <= 0 {
		return nil
	}

	existing, err := getTokenBudgetPolicyByScope(tx, spec.ScopeType, spec.ScopeCode, spec.TokenType)
	notFound := errors.Is(err, gorm.ErrRecordNotFound)
	if err != nil && !notFound {
		return err
	}

	if existing != nil && common.DecimalLT(spec.TotalAmount, existing.TotalAmount) {
		allPolicies, err := loadEnabledTokenBudgetPolicies(tx)
		if err != nil {
			return err
		}
		used, err := sumApprovedAmount(tx, existing, 0, allPolicies)
		if err != nil {
			return err
		}
		if common.DecimalGT(used, spec.TotalAmount) {
			return fmt.Errorf("总包不能低于累计已批 %v", common.RoundDecimal(used))
		}
		childSum, err := sumTokenBudgetPolicyChildCaps(tx, existing.Id)
		if err != nil {
			return err
		}
		if common.DecimalGT(childSum, spec.TotalAmount) {
			return fmt.Errorf("总包不能低于子策略上限之和 %v", common.RoundDecimal(childSum))
		}
	}

	policy := &TokenBudgetPolicy{
		ScopeType:    spec.ScopeType,
		ScopeCode:    spec.ScopeCode,
		TokenType:    spec.TokenType,
		TotalAmount:  spec.TotalAmount,
		Currency:     spec.Currency,
		TokenApplyId: spec.TokenApplyId,
		Enabled:      true,
		UpdatedAt:    common.GetTimestamp(),
	}
	if existing == nil {
		policy.CreatedAt = policy.UpdatedAt
	}
	if existing != nil {
		policy.Id = existing.Id
		if existing.ParentId != nil {
			policy.ParentId = existing.ParentId
		}
		if policy.TokenApplyId <= 0 {
			policy.TokenApplyId = existing.TokenApplyId
		}
	}

	parentScopeCode := strings.TrimSpace(spec.ParentScopeCode)
	if parentScopeCode != "" {
		parentScopeType := spec.ParentScopeType
		if parentScopeType == "" {
			parentScopeType = "org"
		}
		parent, err := getTokenBudgetPolicyByScope(tx, parentScopeType, parentScopeCode, spec.TokenType)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("父部门总包不存在: %s", parentScopeCode)
			}
			return err
		}
		policy.ParentId = &parent.Id
	}

	return tx.Save(policy).Error
}
