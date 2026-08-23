package model

import (
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertUserForPaymentGuardTest(t *testing.T, id int, quota int, inviterId ...int) *User {
	t.Helper()
	inviter := 0
	if len(inviterId) > 0 {
		inviter = inviterId[0]
	}
	user := &User{
		Id:        id,
		Username:  "payment_guard_user_" + strconv.Itoa(id),
		Status:    common.UserStatusEnabled,
		Quota:     quota,
		AffCode:   "aff_" + strconv.Itoa(id),
		InviterId: inviter,
	}
	require.NoError(t, DB.Create(user).Error)
	return user
}

func insertSubscriptionPlanForPaymentGuardTest(t *testing.T, id int) *SubscriptionPlan {
	t.Helper()
	plan := &SubscriptionPlan{
		Id:            id,
		Title:         "Guard Plan",
		PriceAmount:   9.99,
		Currency:      "USD",
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		Enabled:       true,
		TotalAmount:   1000,
	}
	require.NoError(t, DB.Create(plan).Error)
	return plan
}

func insertSubscriptionOrderForPaymentGuardTest(t *testing.T, tradeNo string, userID int, planID int, paymentProvider string) {
	t.Helper()
	order := &SubscriptionOrder{
		UserId:          userID,
		PlanId:          planID,
		Money:           9.99,
		TradeNo:         tradeNo,
		PaymentMethod:   paymentProvider,
		PaymentProvider: paymentProvider,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, order.Insert())
}

func insertTopUpForPaymentGuardTest(t *testing.T, tradeNo string, userID int, paymentProvider string) {
	t.Helper()
	topUp := &TopUp{
		UserId:          userID,
		Amount:          2,
		Money:           9.99,
		TradeNo:         tradeNo,
		PaymentMethod:   paymentProvider,
		PaymentProvider: paymentProvider,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())
}

func getTopUpStatusForPaymentGuardTest(t *testing.T, tradeNo string) string {
	t.Helper()
	topUp := GetTopUpByTradeNo(tradeNo)
	require.NotNil(t, topUp)
	return topUp.Status
}

func countUserSubscriptionsForPaymentGuardTest(t *testing.T, userID int) int64 {
	t.Helper()
	var count int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ?", userID).Count(&count).Error)
	return count
}

func getUserQuotaForPaymentGuardTest(t *testing.T, userID int) int {
	t.Helper()
	var user User
	require.NoError(t, DB.Select("quota").Where("id = ?", userID).First(&user).Error)
	return user.Quota
}

func getUserAffiliateQuotaForPaymentGuardTest(t *testing.T, userID int) (int, int) {
	t.Helper()
	var user User
	require.NoError(t, DB.Select("aff_quota", "aff_history").Where("id = ?", userID).First(&user).Error)
	return user.AffQuota, user.AffHistoryQuota
}

func setTopUpInviteRewardForPaymentGuardTest(t *testing.T, percent float64, complianceConfirmed bool) {
	t.Helper()
	paymentSetting := operation_setting.GetPaymentSetting()
	oldPercent := common.TopUpInviteRewardPercent
	oldConfirmed := paymentSetting.ComplianceConfirmed
	oldTermsVersion := paymentSetting.ComplianceTermsVersion

	common.TopUpInviteRewardPercent = percent
	paymentSetting.ComplianceConfirmed = complianceConfirmed
	if complianceConfirmed {
		paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	} else {
		paymentSetting.ComplianceTermsVersion = ""
	}

	t.Cleanup(func() {
		common.TopUpInviteRewardPercent = oldPercent
		paymentSetting.ComplianceConfirmed = oldConfirmed
		paymentSetting.ComplianceTermsVersion = oldTermsVersion
	})
}

func TestCompleteTopUp_GrantsInviteRewardOnce(t *testing.T) {
	truncateTables(t)
	setTopUpInviteRewardForPaymentGuardTest(t, 10, true)

	insertUserForPaymentGuardTest(t, 401, 0)
	insertUserForPaymentGuardTest(t, 402, 0, 401)
	insertTopUpForPaymentGuardTest(t, "invite-reward-once", 402, PaymentProviderEpay)

	result, err := CompleteTopUp(CompleteTopUpOptions{
		TradeNo:                 "invite-reward-once",
		ExpectedPaymentProvider: PaymentProviderEpay,
		CallbackPaymentMethod:   "alipay",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.AlreadyCompleted)

	expectedQuota := int(2 * common.QuotaPerUnit)
	expectedReward := expectedQuota / 10
	assert.Equal(t, expectedQuota, result.QuotaToAdd)
	assert.Equal(t, expectedReward, result.InviteRewardQuota)
	assert.Equal(t, expectedQuota, getUserQuotaForPaymentGuardTest(t, 402))
	affQuota, affHistory := getUserAffiliateQuotaForPaymentGuardTest(t, 401)
	assert.Equal(t, expectedReward, affQuota)
	assert.Equal(t, expectedReward, affHistory)
	events := findAffiliateRewardEventsForTest(t)
	require.Len(t, events, 1)
	assert.Equal(t, 401, events[0].InviterId)
	assert.Equal(t, 402, events[0].InviteeId)
	assert.Equal(t, AffiliateRewardEventTypeTopUp, events[0].EventType)
	assert.Equal(t, "invite-reward-once", events[0].SourceId)
	assert.Equal(t, int64(expectedQuota), events[0].BaseQuota)
	assert.Equal(t, "10", events[0].RewardPercent)
	assert.Equal(t, int64(expectedReward), events[0].RewardQuota)
	assert.Equal(t, int64(expectedReward), events[0].AffQuotaDelta)
	require.NotNil(t, events[0].IdempotencyKey)
	assert.Equal(t, affiliateRewardIdempotencyKeyForTest("topup", "invite-reward-once"), *events[0].IdempotencyKey)

	result, err = CompleteTopUp(CompleteTopUpOptions{
		TradeNo:                 "invite-reward-once",
		ExpectedPaymentProvider: PaymentProviderEpay,
		CallbackPaymentMethod:   "alipay",
	})
	require.NoError(t, err)
	require.True(t, result.AlreadyCompleted)
	assert.Equal(t, expectedQuota, getUserQuotaForPaymentGuardTest(t, 402))
	affQuota, affHistory = getUserAffiliateQuotaForPaymentGuardTest(t, 401)
	assert.Equal(t, expectedReward, affQuota)
	assert.Equal(t, expectedReward, affHistory)
	assert.Len(t, findAffiliateRewardEventsForTest(t), 1)
}

func TestCompleteTopUp_DoesNotGrantInviteRewardWhenPercentZero(t *testing.T) {
	truncateTables(t)
	setTopUpInviteRewardForPaymentGuardTest(t, 0, true)

	insertUserForPaymentGuardTest(t, 411, 0)
	insertUserForPaymentGuardTest(t, 412, 0, 411)
	insertTopUpForPaymentGuardTest(t, "invite-reward-zero", 412, PaymentProviderEpay)

	result, err := CompleteTopUp(CompleteTopUpOptions{
		TradeNo:                 "invite-reward-zero",
		ExpectedPaymentProvider: PaymentProviderEpay,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Zero(t, result.InviteRewardQuota)
	assert.Equal(t, int(2*common.QuotaPerUnit), getUserQuotaForPaymentGuardTest(t, 412))
	affQuota, affHistory := getUserAffiliateQuotaForPaymentGuardTest(t, 411)
	assert.Zero(t, affQuota)
	assert.Zero(t, affHistory)
}

func TestCompleteTopUp_DoesNotGrantInviteRewardWithoutCompliance(t *testing.T) {
	truncateTables(t)
	setTopUpInviteRewardForPaymentGuardTest(t, 10, false)

	insertUserForPaymentGuardTest(t, 421, 0)
	insertUserForPaymentGuardTest(t, 422, 0, 421)
	insertTopUpForPaymentGuardTest(t, "invite-reward-compliance", 422, PaymentProviderEpay)

	result, err := CompleteTopUp(CompleteTopUpOptions{
		TradeNo:                 "invite-reward-compliance",
		ExpectedPaymentProvider: PaymentProviderEpay,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Zero(t, result.InviteRewardQuota)
	assert.Equal(t, int(2*common.QuotaPerUnit), getUserQuotaForPaymentGuardTest(t, 422))
	affQuota, affHistory := getUserAffiliateQuotaForPaymentGuardTest(t, 421)
	assert.Zero(t, affQuota)
	assert.Zero(t, affHistory)
}

func TestCompleteTopUp_RejectsMismatchedPaymentProviderWithoutReward(t *testing.T) {
	truncateTables(t)
	setTopUpInviteRewardForPaymentGuardTest(t, 10, true)

	insertUserForPaymentGuardTest(t, 431, 0)
	insertUserForPaymentGuardTest(t, 432, 0, 431)
	insertTopUpForPaymentGuardTest(t, "invite-reward-mismatch", 432, PaymentProviderEpay)

	result, err := CompleteTopUp(CompleteTopUpOptions{
		TradeNo:                 "invite-reward-mismatch",
		ExpectedPaymentProvider: PaymentProviderStripe,
	})
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)
	assert.Nil(t, result)
	assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, "invite-reward-mismatch"))
	assert.Zero(t, getUserQuotaForPaymentGuardTest(t, 432))
	affQuota, affHistory := getUserAffiliateQuotaForPaymentGuardTest(t, 431)
	assert.Zero(t, affQuota)
	assert.Zero(t, affHistory)
}

func TestUpdateOption_RejectsInvalidTopUpInviteRewardPercent(t *testing.T) {
	oldPercent := common.TopUpInviteRewardPercent
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	oldOptionValue, hadOptionValue := common.OptionMap["TopUpInviteRewardPercent"]
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.TopUpInviteRewardPercent = oldPercent
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		if !hadOptionValue {
			delete(common.OptionMap, "TopUpInviteRewardPercent")
		} else {
			common.OptionMap["TopUpInviteRewardPercent"] = oldOptionValue
		}
	})

	common.TopUpInviteRewardPercent = 3
	err := updateOptionMap("TopUpInviteRewardPercent", "NaN")
	require.Error(t, err)
	assert.Equal(t, 3.0, common.TopUpInviteRewardPercent)

	err = updateOptionMap("TopUpInviteRewardPercent", "12.5")
	require.NoError(t, err)
	assert.Equal(t, 12.5, common.TopUpInviteRewardPercent)
	assert.Equal(t, "12.5", common.OptionMap["TopUpInviteRewardPercent"])
}

func TestRechargeWaffoPancake_RejectsMismatchedPaymentMethod(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 101, 0)
	insertTopUpForPaymentGuardTest(t, "waffo-pancake-guard", 101, PaymentProviderStripe)

	err := RechargeWaffoPancake("waffo-pancake-guard")
	require.Error(t, err)

	topUp := GetTopUpByTradeNo("waffo-pancake-guard")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
	assert.Equal(t, 0, getUserQuotaForPaymentGuardTest(t, 101))
}

func TestUpdatePendingTopUpStatus_RejectsMismatchedPaymentProvider(t *testing.T) {
	testCases := []struct {
		name                    string
		tradeNo                 string
		storedPaymentProvider   string
		expectedPaymentProvider string
		targetStatus            string
	}{
		{
			name:                    "stripe expire",
			tradeNo:                 "stripe-expire-guard",
			storedPaymentProvider:   PaymentProviderCreem,
			expectedPaymentProvider: PaymentProviderStripe,
			targetStatus:            common.TopUpStatusExpired,
		},
		{
			name:                    "waffo failed",
			tradeNo:                 "waffo-failed-guard",
			storedPaymentProvider:   PaymentProviderStripe,
			expectedPaymentProvider: PaymentProviderWaffo,
			targetStatus:            common.TopUpStatusFailed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			insertUserForPaymentGuardTest(t, 150, 0)
			insertTopUpForPaymentGuardTest(t, tc.tradeNo, 150, tc.storedPaymentProvider)

			err := UpdatePendingTopUpStatus(tc.tradeNo, tc.expectedPaymentProvider, tc.targetStatus)
			require.ErrorIs(t, err, ErrPaymentMethodMismatch)
			assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, tc.tradeNo))
		})
	}
}

func TestCompleteSubscriptionOrder_RejectsMismatchedPaymentProvider(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 202, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 301)
	insertSubscriptionOrderForPaymentGuardTest(t, "sub-guard-order", 202, plan.Id, PaymentProviderStripe)

	err := CompleteSubscriptionOrder("sub-guard-order", `{"provider":"epay"}`, PaymentProviderEpay, "alipay")
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)

	order := GetSubscriptionOrderByTradeNo("sub-guard-order")
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	assert.Zero(t, countUserSubscriptionsForPaymentGuardTest(t, 202))

	topUp := GetTopUpByTradeNo("sub-guard-order")
	assert.Nil(t, topUp)
}

func TestExpireSubscriptionOrder_RejectsMismatchedPaymentProvider(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 303, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 401)
	insertSubscriptionOrderForPaymentGuardTest(t, "sub-expire-guard", 303, plan.Id, PaymentProviderStripe)

	err := ExpireSubscriptionOrder("sub-expire-guard", PaymentProviderCreem)
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)

	order := GetSubscriptionOrderByTradeNo("sub-expire-guard")
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
}

func createEpayTestOrder(t *testing.T, userId int, tradeNo string, provider string, status string) TopUp {
	t.Helper()
	topUp := TopUp{
		UserId:          userId,
		Amount:          2,
		Money:           10.0,
		TradeNo:         tradeNo,
		PaymentMethod:   "alipay",
		PaymentProvider: provider,
		CreateTime:      common.GetTimestamp(),
		Status:          status,
	}
	require.NoError(t, DB.Create(&topUp).Error)
	return topUp
}

func TestRechargeEpayCreditsQuotaExactlyOnce(t *testing.T) {
	truncateTables(t)

	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

	user := insertUserForPaymentGuardTest(t, 501, 0)
	order := createEpayTestOrder(t, user.Id, "EPAYTESTONCE", PaymentProviderEpay, common.TopUpStatusPending)

	alreadyDone, err := RechargeEpay(order.TradeNo, "alipay", "127.0.0.1")
	require.NoError(t, err)
	assert.False(t, alreadyDone)
	assert.Equal(t, 2*500000, getUserQuotaForPaymentGuardTest(t, user.Id))

	reloaded := GetTopUpByTradeNo(order.TradeNo)
	require.NotNil(t, reloaded)
	assert.Equal(t, common.TopUpStatusSuccess, reloaded.Status)
	assert.NotZero(t, reloaded.CompleteTime)

	alreadyDone, err = RechargeEpay(order.TradeNo, "alipay", "127.0.0.1")
	require.NoError(t, err)
	assert.True(t, alreadyDone)
	assert.Equal(t, 2*500000, getUserQuotaForPaymentGuardTest(t, user.Id))
}

func TestRechargeEpayKeepsRedisAndDatabaseCreditInSync(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)

	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 5
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

	user := insertUserForPaymentGuardTest(t, 502, 7)
	require.NoError(t, populateUserCache(*user))
	order := createEpayTestOrder(t, user.Id, "EPAYTESTREDISSYNC", PaymentProviderEpay, common.TopUpStatusPending)

	alreadyDone, err := RechargeEpay(order.TradeNo, "alipay", "127.0.0.1")
	require.NoError(t, err)
	assert.False(t, alreadyDone)
	assert.Equal(t, 17, getUserQuotaForPaymentGuardTest(t, user.Id))
	cached, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 17, cached.Quota)

	alreadyDone, err = RechargeEpay(order.TradeNo, "alipay", "127.0.0.1")
	require.NoError(t, err)
	assert.True(t, alreadyDone)
	cached, err = cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 17, cached.Quota)
}

func TestRechargeEpayUpdatesPaymentMethodToActual(t *testing.T) {
	truncateTables(t)

	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

	user := insertUserForPaymentGuardTest(t, 503, 0)
	order := createEpayTestOrder(t, user.Id, "EPAYTESTMETHOD", PaymentProviderEpay, common.TopUpStatusPending)

	alreadyDone, err := RechargeEpay(order.TradeNo, "wxpay", "127.0.0.1")
	require.NoError(t, err)
	assert.False(t, alreadyDone)

	reloaded := GetTopUpByTradeNo(order.TradeNo)
	require.NotNil(t, reloaded)
	assert.Equal(t, "wxpay", reloaded.PaymentMethod)
	assert.Equal(t, 2*500000, getUserQuotaForPaymentGuardTest(t, user.Id))
}

func TestRechargeEpayRejectsForeignAndNonPendingOrders(t *testing.T) {
	truncateTables(t)

	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

	user := insertUserForPaymentGuardTest(t, 504, 7)

	t.Run("order from another payment provider", func(t *testing.T) {
		order := createEpayTestOrder(t, user.Id, "EPAYTESTSTRIPE", PaymentProviderStripe, common.TopUpStatusPending)
		_, err := RechargeEpay(order.TradeNo, "alipay", "127.0.0.1")
		assert.ErrorIs(t, err, ErrPaymentMethodMismatch)
		assert.Equal(t, 7, getUserQuotaForPaymentGuardTest(t, user.Id))
	})

	t.Run("order that is not pending", func(t *testing.T) {
		order := createEpayTestOrder(t, user.Id, "EPAYTESTEXPIRED", PaymentProviderEpay, common.TopUpStatusExpired)
		_, err := RechargeEpay(order.TradeNo, "alipay", "127.0.0.1")
		assert.ErrorIs(t, err, ErrTopUpStatusInvalid)
		assert.Equal(t, 7, getUserQuotaForPaymentGuardTest(t, user.Id))
	})

	t.Run("missing order", func(t *testing.T) {
		_, err := RechargeEpay("EPAYTESTMISSING", "alipay", "127.0.0.1")
		assert.ErrorIs(t, err, ErrTopUpNotFound)
	})
}

func TestRechargeEpayRejectsQuotaOverflowBeforeCompletingOrder(t *testing.T) {
	truncateTables(t)

	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = float64(common.MaxQuota)
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

	user := insertUserForPaymentGuardTest(t, 505, 3)
	order := createEpayTestOrder(t, user.Id, "EPAYTESTOVERFLOW", PaymentProviderEpay, common.TopUpStatusPending)

	_, err := RechargeEpay(order.TradeNo, "alipay", "127.0.0.1")
	require.Error(t, err)
	assert.Equal(t, 3, getUserQuotaForPaymentGuardTest(t, user.Id))
	assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, order.TradeNo))
}

func TestRechargeEpayEnforcesFinalWalletQuotaLimit(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

	testCases := []struct {
		name         string
		currentQuota int
		wantErr      bool
		wantQuota    int
		wantStatus   string
	}{
		{
			name:         "allows exact highest representable wallet balance",
			currentQuota: common.MaxQuota - 1 - 1_000_000,
			wantQuota:    common.MaxQuota - 1,
			wantStatus:   common.TopUpStatusSuccess,
		},
		{
			name:         "rejects balance above int32 quota domain",
			currentQuota: common.MaxQuota - 1_000_000,
			wantErr:      true,
			wantQuota:    common.MaxQuota - 1_000_000,
			wantStatus:   common.TopUpStatusPending,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			user := insertUserForPaymentGuardTest(t, 506, tc.currentQuota)
			order := createEpayTestOrder(t, user.Id, "EPAYTESTWALLETLIMIT", PaymentProviderEpay, common.TopUpStatusPending)

			_, err := RechargeEpay(order.TradeNo, "alipay", "127.0.0.1")
			if tc.wantErr {
				require.ErrorIs(t, err, ErrTopUpQuotaLimitExceeded)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantQuota, getUserQuotaForPaymentGuardTest(t, user.Id))
			assert.Equal(t, tc.wantStatus, getTopUpStatusForPaymentGuardTest(t, order.TradeNo))
		})
	}
}
