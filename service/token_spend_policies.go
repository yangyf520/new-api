package service

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
)

func HasConsumptionPoliciesForRelay(relayInfo *relaycommon.RelayInfo) (bool, error) {
	if relayInfo == nil || relayInfo.IsPlayground {
		return false, nil
	}
	policies, _, _, err := loadTokenSpendPoliciesForRelay(relayInfo)
	if err != nil {
		return false, err
	}
	return len(policies) > 0, nil
}

func ReserveConsumption(relayInfo *relaycommon.RelayInfo, preConsumedQuota int) *types.NewAPIError {
	if relayInfo == nil || relayInfo.IsPlayground || preConsumedQuota <= 0 {
		return nil
	}
	policies, defaultCurrency, tokenApplyId, err := loadTokenSpendPoliciesForRelay(relayInfo)
	if err != nil {
		return types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
	}
	if len(policies) == 0 {
		return nil
	}
	if err := model.ReserveTokenSpendWithPolicies(policies, preConsumedQuota, defaultCurrency, tokenApplyId); err != nil {
		return tokenSpendPolicyAPIError(err)
	}
	return nil
}

func AdjustConsumption(relayInfo *relaycommon.RelayInfo, preConsumedQuota, actualQuota int) error {
	if relayInfo == nil || relayInfo.IsPlayground {
		return nil
	}
	if preConsumedQuota == actualQuota {
		return nil
	}
	policies, defaultCurrency, tokenApplyId, err := loadTokenSpendPoliciesForRelay(relayInfo)
	if err != nil {
		return err
	}
	if len(policies) == 0 {
		return nil
	}
	return model.AdjustTokenSpendWithPolicies(policies, preConsumedQuota, actualQuota, defaultCurrency, tokenApplyId)
}

func ReleaseConsumption(relayInfo *relaycommon.RelayInfo, preConsumedQuota int) error {
	if relayInfo == nil || relayInfo.IsPlayground || preConsumedQuota <= 0 {
		return nil
	}
	policies, defaultCurrency, tokenApplyId, err := loadTokenSpendPoliciesForRelay(relayInfo)
	if err != nil {
		return err
	}
	if len(policies) == 0 {
		return nil
	}
	return model.ReleaseTokenSpendWithPolicies(policies, preConsumedQuota, defaultCurrency, tokenApplyId)
}

func tokenSpendPolicyAPIError(err error) *types.NewAPIError {
	return types.NewErrorWithStatusCode(
		err,
		types.ErrorCodeInsufficientUserQuota,
		http.StatusForbidden,
		types.ErrOptionWithSkipRetry(),
		types.ErrOptionWithNoRecordErrorLog(),
	)
}

func loadTokenSpendPoliciesForRelay(relayInfo *relaycommon.RelayInfo) ([]*model.TokenSpendPolicy, string, int, error) {
	orgCode := strings.TrimSpace(relayInfo.TokenGroup)
	if orgCode == "" {
		orgCode = strings.TrimSpace(relayInfo.UsingGroup)
	}
	tokenType := model.ResolveTokenApplyTypeForToken(relayInfo.TokenId)
	tokenApplyId := model.ResolveTokenApplyIdByTokenId(relayInfo.TokenId)
	defaultCurrency := "CNY"
	if app, err := model.GetTokenApplyRecordByTokenId(relayInfo.TokenId); err == nil {
		if strings.TrimSpace(app.Currency) != "" {
			defaultCurrency = app.Currency
		}
		if org := strings.TrimSpace(app.OrgCode); org != "" {
			orgCode = org
		}
	}
	policies, err := model.LoadTokenSpendPolicyChain(model.DB, relayInfo.TokenId, orgCode, tokenType)
	if err != nil {
		return nil, "", 0, fmt.Errorf("加载消耗策略失败: %w", err)
	}
	return policies, defaultCurrency, tokenApplyId, nil
}
