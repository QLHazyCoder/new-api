package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createEpayV2OrderForTest(t *testing.T, tradeNo string, userID int, creditedQuota, moneyMinor, rewardRateBps int64) *TopUp {
	t.Helper()
	snapshot := &TopUpPricingSnapshot{
		Version:              TopUpPricingSnapshotVersion,
		PaymentProvider:      PaymentProviderEpay,
		RequestedAmount:      2,
		StoredAmount:         2,
		QuotaPerUnit:         common.QuotaPerUnit,
		CreditedQuota:        creditedQuota,
		QuotedMoneyMinor:     moneyMinor,
		RewardRateBps:        rewardRateBps,
		InviteRewardEligible: rewardRateBps > 0,
	}
	raw, err := snapshot.Marshal()
	require.NoError(t, err)
	order := &TopUp{
		UserId:              userID,
		Amount:              2,
		Money:               decimal.NewFromInt(moneyMinor).Div(decimal.NewFromInt(100)).InexactFloat64(),
		TradeNo:             tradeNo,
		PaymentMethod:       "alipay",
		PaymentProvider:     PaymentProviderEpay,
		CreditedQuota:       creditedQuota,
		QuotedMoneyMinor:    moneyMinor,
		InviteRewardRateBps: rewardRateBps,
		SettlementVersion:   TopUpPricingSnapshotVersion,
		PricingSnapshot:     raw,
		CreateTime:          common.GetTimestamp(),
		Status:              common.TopUpStatusPending,
	}
	require.NoError(t, order.Insert())
	return order
}

func TestRechargeEpayV2UsesPersistedQuotaAfterQuotaPerUnitChanges(t *testing.T) {
	truncateTables(t)
	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

	user := insertUserForPaymentGuardTest(t, 901, 0)
	order := createEpayV2OrderForTest(t, "EPAYV2FIXEDQUOTA", user.Id, 1_000_000, 1000, 0)
	common.QuotaPerUnit = 1_000_000

	alreadyDone, err := RechargeEpayWithMoney(order.TradeNo, "alipay", "10.00", "127.0.0.1")
	require.NoError(t, err)
	assert.False(t, alreadyDone)
	assert.EqualValues(t, 1_000_000, getUserQuotaForPaymentGuardTest(t, user.Id))
}

func TestRechargeEpayV2RejectsCallbackPaymentMismatch(t *testing.T) {
	truncateTables(t)
	user := insertUserForPaymentGuardTest(t, 902, 0)
	order := createEpayV2OrderForTest(t, "EPAYV2MONEY", user.Id, 1_000_000, 1000, 0)

	_, err := RechargeEpayWithMoney(order.TradeNo, "alipay", "9.99", "127.0.0.1")
	require.ErrorIs(t, err, ErrTopUpPaymentMismatch)
	assert.EqualValues(t, 0, getUserQuotaForPaymentGuardTest(t, user.Id))
	assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, order.TradeNo))
}

func TestRechargeEpayV2RejectsCallbackMoneyBeyondCurrencyPrecision(t *testing.T) {
	truncateTables(t)
	user := insertUserForPaymentGuardTest(t, 906, 0)
	order := createEpayV2OrderForTest(t, "EPAYV2PRECISION", user.Id, 1_000_000, 1000, 0)

	_, err := RechargeEpayWithMoney(order.TradeNo, "alipay", "10.001", "127.0.0.1")
	require.ErrorIs(t, err, ErrTopUpPaymentMismatch)
	assert.EqualValues(t, 0, getUserQuotaForPaymentGuardTest(t, user.Id))
	assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, order.TradeNo))
}

func TestRechargeEpayV2RejectsMismatchedStoredSnapshot(t *testing.T) {
	truncateTables(t)
	user := insertUserForPaymentGuardTest(t, 905, 0)
	order := createEpayV2OrderForTest(t, "EPAYV2SNAPSHOT", user.Id, 1_000_000, 1000, 0)
	order.QuotedMoneyMinor = 999
	require.NoError(t, DB.Model(&TopUp{}).Where("id = ?", order.Id).Update("quoted_money_minor", order.QuotedMoneyMinor).Error)

	_, err := RechargeEpayWithMoney(order.TradeNo, "alipay", "10.00", "127.0.0.1")
	require.ErrorIs(t, err, ErrTopUpPricingMismatch)
	assert.EqualValues(t, 0, getUserQuotaForPaymentGuardTest(t, user.Id))
	assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, order.TradeNo))
}

func TestRechargeEpayV2UsesSnapshotInviteRewardRate(t *testing.T) {
	truncateTables(t)
	setTopUpInviteRewardForPaymentGuardTest(t, 50, true)

	inviter := insertUserForPaymentGuardTest(t, 903, 0)
	invitee := insertUserForPaymentGuardTest(t, 904, 0, inviter.Id)
	order := createEpayV2OrderForTest(t, "EPAYV2REWARD", invitee.Id, 1_000_000, 1000, 200)

	_, err := RechargeEpayWithMoney(order.TradeNo, "alipay", "10.00", "127.0.0.1")
	require.NoError(t, err)
	affQuota, affHistory := getUserAffiliateQuotaForPaymentGuardTest(t, inviter.Id)
	assert.EqualValues(t, 20_000, affQuota)
	assert.EqualValues(t, 20_000, affHistory)

	var event AffiliateRewardEvent
	require.NoError(t, DB.Where("source_id = ?", order.TradeNo).First(&event).Error)
	assert.EqualValues(t, 200, event.RewardRateBps)
	assert.Equal(t, "2", event.RewardPercent)
}
