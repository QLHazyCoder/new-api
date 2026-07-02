package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func seedSubscriptionPlanForApplicableGroupTest(t *testing.T, id int, applicableGroup string) {
	t.Helper()
	plan := &SubscriptionPlan{
		Id:              id,
		Title:           "group plan",
		PriceAmount:     1,
		Currency:        "USD",
		DurationUnit:    SubscriptionDurationMonth,
		DurationValue:   1,
		Enabled:         true,
		TotalAmount:     1000,
		ApplicableGroup: applicableGroup,
	}
	require.NoError(t, DB.Create(plan).Error)
}

func seedUserSubscriptionForApplicableGroupTest(t *testing.T, id int, userId int, planId int, applicableGroup string, amountTotal int64) {
	t.Helper()
	sub := &UserSubscription{
		Id:              id,
		UserId:          userId,
		PlanId:          planId,
		AmountTotal:     amountTotal,
		AmountUsed:      0,
		Status:          "active",
		StartTime:       time.Now().Add(-time.Hour).Unix(),
		EndTime:         time.Now().Add(24 * time.Hour).Unix(),
		ApplicableGroup: applicableGroup,
	}
	require.NoError(t, DB.Create(sub).Error)
}

func getUserSubscriptionAmountUsedForApplicableGroupTest(t *testing.T, id int) int64 {
	t.Helper()
	var sub UserSubscription
	require.NoError(t, DB.Select("amount_used").Where("id = ?", id).First(&sub).Error)
	return sub.AmountUsed
}

func TestPreConsumeUserSubscriptionApplicableGroup_AllGroupsCompatible(t *testing.T) {
	truncateTables(t)
	seedSubscriptionPlanForApplicableGroupTest(t, 101, "")
	seedUserSubscriptionForApplicableGroupTest(t, 201, 1, 101, "", 1000)

	res, err := PreConsumeUserSubscription("req-all-groups", 1, "gpt-test", 0, 100, "GPT-Pro-正价")

	require.NoError(t, err)
	require.Equal(t, 201, res.UserSubscriptionId)
	require.Equal(t, int64(100), getUserSubscriptionAmountUsedForApplicableGroupTest(t, 201))
}

func TestPreConsumeUserSubscriptionApplicableGroup_MatchingGroup(t *testing.T) {
	truncateTables(t)
	seedSubscriptionPlanForApplicableGroupTest(t, 102, "GPT-Pro-正价")
	seedUserSubscriptionForApplicableGroupTest(t, 202, 1, 102, "GPT-Pro-正价", 1000)

	res, err := PreConsumeUserSubscription("req-matching-group", 1, "gpt-test", 0, 100, "GPT-Pro-正价")

	require.NoError(t, err)
	require.Equal(t, 202, res.UserSubscriptionId)
	require.Equal(t, int64(100), getUserSubscriptionAmountUsedForApplicableGroupTest(t, 202))
}

func TestPreConsumeUserSubscriptionApplicableGroup_NonMatchingGroupSkipped(t *testing.T) {
	truncateTables(t)
	seedSubscriptionPlanForApplicableGroupTest(t, 103, "GPT-Pro-正价")
	seedUserSubscriptionForApplicableGroupTest(t, 203, 1, 103, "GPT-Pro-正价", 1000)

	_, err := PreConsumeUserSubscription("req-non-matching-group", 1, "gpt-test", 0, 100, "GPT-Plus")

	require.Error(t, err)
	require.Contains(t, err.Error(), "subscription quota insufficient")
	require.Equal(t, int64(0), getUserSubscriptionAmountUsedForApplicableGroupTest(t, 203))
}

func TestPreConsumeUserSubscriptionApplicableGroup_SelectsMatchingSubscription(t *testing.T) {
	truncateTables(t)
	seedSubscriptionPlanForApplicableGroupTest(t, 104, "GPT-Pro-正价")
	seedSubscriptionPlanForApplicableGroupTest(t, 105, "GPT-Plus")
	seedUserSubscriptionForApplicableGroupTest(t, 204, 1, 104, "GPT-Pro-正价", 1000)
	seedUserSubscriptionForApplicableGroupTest(t, 205, 1, 105, "GPT-Plus", 1000)

	res, err := PreConsumeUserSubscription("req-select-matching", 1, "gpt-test", 0, 100, "GPT-Plus")

	require.NoError(t, err)
	require.Equal(t, 205, res.UserSubscriptionId)
	require.Equal(t, int64(0), getUserSubscriptionAmountUsedForApplicableGroupTest(t, 204))
	require.Equal(t, int64(100), getUserSubscriptionAmountUsedForApplicableGroupTest(t, 205))
}

func TestUserActiveSubscriptionsAllowWalletOverflow_OnlyApplicableStrictSubscriptionBlocks(t *testing.T) {
	truncateTables(t)
	seedSubscriptionPlanForApplicableGroupTest(t, 106, "GPT-Pro-正价")
	seedUserSubscriptionForApplicableGroupTest(t, 206, 2, 106, "GPT-Pro-正价", 1000)

	allowOverflow, err := UserActiveSubscriptionsAllowWalletOverflow(2, "GPT-Plus")

	require.NoError(t, err)
	require.True(t, allowOverflow)

	allowOverflow, err = UserActiveSubscriptionsAllowWalletOverflow(2, "GPT-Pro-正价")

	require.NoError(t, err)
	require.False(t, allowOverflow)
}

func TestUserActiveSubscriptionsAllowWalletOverflow_AllGroupsStrictSubscriptionBlocks(t *testing.T) {
	truncateTables(t)
	seedSubscriptionPlanForApplicableGroupTest(t, 107, "")
	seedUserSubscriptionForApplicableGroupTest(t, 207, 3, 107, "", 1000)

	allowOverflow, err := UserActiveSubscriptionsAllowWalletOverflow(3, "GPT-Plus")

	require.NoError(t, err)
	require.False(t, allowOverflow)
}
