package controller

import (
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func IssueToken(c *gin.Context) {
	req := &model.IssueTokenRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		common.ApiError(c, err)
		return
	}
	result, err := model.IssueTokenApplication(req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func UpdateIssuedToken(c *gin.Context) {
	tokenApplyId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	req := &model.UpdateTokenRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		common.ApiError(c, err)
		return
	}
	result, err := model.UpdateTokenApplication(tokenApplyId, req)
	if err != nil {
		if errors.Is(err, model.ErrTokenApplyNotFound) {
			common.ApiErrorMsg(c, err.Error())
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func tokenApplyPortalScope(c *gin.Context) model.TokenApplyPortalScope {
	return model.TokenApplyPortalScope{
		UserId:  c.GetInt("id"),
		OrgCode: c.GetString("group"),
		IsAdmin: c.GetInt("role") >= common.RoleAdminUser,
	}
}

func PortalListTokenApplications(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	scope := tokenApplyPortalScope(c)
	items, total, err := model.ListTokenApplicationsPortal(scope, c.Query("keyword"), pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

func PortalGetTokenApplication(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	detail, err := model.GetTokenApplyRecordDetailPortal(tokenApplyPortalScope(c), id)
	if err != nil {
		if errors.Is(err, model.ErrTokenApplyNotFound) {
			common.ApiErrorMsg(c, err.Error())
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, detail)
}

func PortalGetApplicationToken(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	tokenView, err := model.GetTokenApplyTokenPortal(tokenApplyPortalScope(c), id)
	if err != nil {
		if errors.Is(err, model.ErrTokenApplyNotFound) {
			common.ApiErrorMsg(c, err.Error())
			return
		}
		common.ApiErrorMsg(c, err.Error())
		return
	}
	common.ApiSuccess(c, tokenView)
}

func PortalListBudgetPolicies(c *gin.Context) {
	scope := tokenApplyPortalScope(c)
	policies, err := model.ListTokenBudgetPoliciesPortal(scope, c.Query("scope_type"), c.Query("scope_code"), c.Query("token_type"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, policies)
}

func PortalListTokenSpendPolicies(c *gin.Context) {
	scope := tokenApplyPortalScope(c)
	policies, err := model.ListTokenSpendPolicyViewsPortal(scope, c.Query("scope_type"), c.Query("scope_code"), c.Query("token_type"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, policies)
}
