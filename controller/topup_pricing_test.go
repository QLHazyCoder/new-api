package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateTopUpPricingUsesEligibleExactAmountAndPersistsAuditInputs(t *testing.T) {
	originalPrice := operation_setting.Price
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalDiscounts := operation_setting.GetAmountDiscountPolicy().Discounts
	originalGroups := operation_setting.GetAmountDiscountPolicy().EligibleGroups
	originalGroupRatio := common.TopupGroupRatio2JSONString()
	t.Cleanup(func() {
		operation_setting.Price = originalPrice
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
		operation_setting.GetPaymentSetting().AmountDiscount = originalDiscounts
		operation_setting.GetPaymentSetting().AmountDiscountEligibleGroups = originalGroups
		require.NoError(t, operation_setting.RefreshAmountDiscountPolicy())
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(originalGroupRatio))
	})

	operation_setting.Price = 1.5
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{200: 0.9}
	operation_setting.GetPaymentSetting().AmountDiscountEligibleGroups = []string{"vip"}
	require.NoError(t, operation_setting.RefreshAmountDiscountPolicy())
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1,"vip":2}`))

	pricing := getEpayTopUpPricing(200, "vip")
	assert.InDelta(t, 540, pricing.PayMoney, 0.000001)
	assert.Equal(t, model.PaymentProviderEpay, pricing.Snapshot.PaymentProvider)
	assert.Equal(t, "vip", pricing.Snapshot.UserGroup)
	assert.Equal(t, int64(200), pricing.Snapshot.RequestedAmount)
	assert.Equal(t, int64(200), pricing.Snapshot.StoredAmount)
	assert.Equal(t, 1.5, pricing.Snapshot.UnitPrice)
	assert.Equal(t, 2.0, pricing.Snapshot.TopupGroupRatio)
	assert.True(t, pricing.Snapshot.AmountDiscountEligible)
	assert.True(t, pricing.Snapshot.AmountDiscountApplied)
	assert.Equal(t, 0.9, pricing.Snapshot.AmountDiscountRate)

	assert.InDelta(t, 603, getEpayTopUpPricing(201, "vip").PayMoney, 0.000001)
	assert.InDelta(t, 300, getEpayTopUpPricing(200, "default").PayMoney, 0.000001)
}

func TestCalculateTopUpPricingConvertsTokensBeforeApplyingDiscount(t *testing.T) {
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalDiscounts := operation_setting.GetAmountDiscountPolicy().Discounts
	originalGroups := operation_setting.GetAmountDiscountPolicy().EligibleGroups
	originalGroupRatio := common.TopupGroupRatio2JSONString()
	t.Cleanup(func() {
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
		operation_setting.GetPaymentSetting().AmountDiscount = originalDiscounts
		operation_setting.GetPaymentSetting().AmountDiscountEligibleGroups = originalGroups
		require.NoError(t, operation_setting.RefreshAmountDiscountPolicy())
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(originalGroupRatio))
	})

	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
	requestedAmount := int64(common.QuotaPerUnit * 3)
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{int(requestedAmount): 0.5}
	operation_setting.GetPaymentSetting().AmountDiscountEligibleGroups = []string{"default"}
	require.NoError(t, operation_setting.RefreshAmountDiscountPolicy())
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1}`))

	pricing := calculateTopUpPricing(requestedAmount, 3, "default", model.PaymentProviderWaffo, 2)
	assert.InDelta(t, 3, pricing.PayMoney, 0.000001)
	assert.InDelta(t, 3, pricing.Snapshot.NormalizedAmount, 0.000001)
	assert.Equal(t, int64(3), pricing.Snapshot.StoredAmount)
	assert.Equal(t, operation_setting.QuotaDisplayTypeTokens, pricing.Snapshot.QuotaDisplayType)
}

func TestCalculateTopUpPricingPreservesCNYAmountAndGatewayUnitPrice(t *testing.T) {
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalDiscounts := operation_setting.GetAmountDiscountPolicy().Discounts
	originalGroups := operation_setting.GetAmountDiscountPolicy().EligibleGroups
	originalGroupRatio := common.TopupGroupRatio2JSONString()
	t.Cleanup(func() {
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
		operation_setting.GetPaymentSetting().AmountDiscount = originalDiscounts
		operation_setting.GetPaymentSetting().AmountDiscountEligibleGroups = originalGroups
		require.NoError(t, operation_setting.RefreshAmountDiscountPolicy())
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(originalGroupRatio))
	})

	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeCNY
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{500: 0.95}
	operation_setting.GetPaymentSetting().AmountDiscountEligibleGroups = []string{"vip"}
	require.NoError(t, operation_setting.RefreshAmountDiscountPolicy())
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"vip":0.8}`))

	pricing := calculateTopUpPricing(500, 500, "vip", model.PaymentProviderWaffoPancake, 0.14)
	assert.InDelta(t, 53.2, pricing.PayMoney, 0.000001)
	assert.Equal(t, operation_setting.QuotaDisplayTypeCNY, pricing.Snapshot.QuotaDisplayType)
	assert.InDelta(t, 500, pricing.Snapshot.NormalizedAmount, 0.000001)
	assert.Equal(t, 0.14, pricing.Snapshot.UnitPrice)
	assert.Equal(t, model.PaymentProviderWaffoPancake, pricing.Snapshot.PaymentProvider)
}
