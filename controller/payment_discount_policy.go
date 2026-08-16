package controller

import (
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

type amountDiscountPolicyUpdateRequest struct {
	AmountDiscount map[int]float64 `json:"amount_discount"`
	EligibleGroups []string        `json:"eligible_groups"`
}

func GetAmountDiscountGroups(c *gin.Context) {
	groups, err := model.GetOccupiedUserGroups()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"groups": groups})
}

func UpdateAmountDiscountPolicy(c *gin.Context) {
	var request amountDiscountPolicyUpdateRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "invalid amount discount policy")
		return
	}

	discounts, groups, err := operation_setting.NormalizeAmountDiscountPolicy(
		request.AmountDiscount,
		request.EligibleGroups,
	)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	occupiedGroups, err := model.GetOccupiedUserGroups()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	allowedGroups := make(map[string]struct{}, len(occupiedGroups))
	for _, group := range occupiedGroups {
		allowedGroups[group] = struct{}{}
	}
	for _, group := range operation_setting.GetAmountDiscountPolicy().EligibleGroups {
		allowedGroups[group] = struct{}{}
	}
	for _, group := range groups {
		if _, ok := allowedGroups[group]; !ok {
			common.ApiErrorMsg(c, fmt.Sprintf("group %q has no users and was not previously selected", group))
			return
		}
	}

	discountJSON, err := common.Marshal(discounts)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	groupsJSON, err := common.Marshal(groups)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.UpdateOptionsBulk(map[string]string{
		operation_setting.AmountDiscountOptionKey:               string(discountJSON),
		operation_setting.AmountDiscountEligibleGroupsOptionKey: string(groupsJSON),
	}); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := operation_setting.RefreshAmountDiscountPolicy(); err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"amount_discount": discounts,
			"eligible_groups": groups,
		},
	})
}
