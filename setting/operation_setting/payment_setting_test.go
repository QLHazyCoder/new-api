package operation_setting

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func preserveAmountDiscountPolicy(t *testing.T) {
	t.Helper()
	originalDiscounts := cloneAmountDiscounts(paymentSetting.AmountDiscount)
	originalGroups := append([]string(nil), paymentSetting.AmountDiscountEligibleGroups...)
	t.Cleanup(func() {
		paymentSetting.AmountDiscount = originalDiscounts
		paymentSetting.AmountDiscountEligibleGroups = originalGroups
		require.NoError(t, RefreshAmountDiscountPolicy())
	})
}

func TestNormalizeAmountDiscountPolicy(t *testing.T) {
	discounts, groups, err := NormalizeAmountDiscountPolicy(
		map[int]float64{200: 0.99, 500: 0.98},
		[]string{" vip ", "default", "vip", ""},
	)
	require.NoError(t, err)
	assert.Equal(t, map[int]float64{200: 0.99, 500: 0.98}, discounts)
	assert.Equal(t, []string{"default", "vip"}, groups)

	invalidCases := []struct {
		name      string
		discounts map[int]float64
	}{
		{name: "non-positive amount", discounts: map[int]float64{0: 0.9}},
		{name: "zero rate", discounts: map[int]float64{100: 0}},
		{name: "rate above one", discounts: map[int]float64{100: 1.01}},
		{name: "nan rate", discounts: map[int]float64{100: math.NaN()}},
	}
	for _, testCase := range invalidCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, _, err := NormalizeAmountDiscountPolicy(testCase.discounts, nil)
			assert.Error(t, err)
		})
	}
}

func TestAmountDiscountPolicyRequiresEligibleExactMatch(t *testing.T) {
	preserveAmountDiscountPolicy(t)
	paymentSetting.AmountDiscount = map[int]float64{200: 0.9}
	paymentSetting.AmountDiscountEligibleGroups = []string{"vip"}
	require.NoError(t, RefreshAmountDiscountPolicy())

	rate, eligible, applied := GetAmountDiscountRate(200, "vip")
	assert.Equal(t, 0.9, rate)
	assert.True(t, eligible)
	assert.True(t, applied)

	rate, eligible, applied = GetAmountDiscountRate(201, "vip")
	assert.Equal(t, 1.0, rate)
	assert.True(t, eligible)
	assert.False(t, applied)

	rate, eligible, applied = GetAmountDiscountRate(200, "default")
	assert.Equal(t, 1.0, rate)
	assert.False(t, eligible)
	assert.False(t, applied)
	assert.Empty(t, GetAmountDiscountsForGroup("default"))
	assert.Equal(t, map[int]float64{200: 0.9}, GetAmountDiscountsForGroup("vip"))
}

func TestAmountDiscountPolicyEmptyGroupsDisableConfiguredDiscounts(t *testing.T) {
	preserveAmountDiscountPolicy(t)
	paymentSetting.AmountDiscount = map[int]float64{200: 0.9}
	paymentSetting.AmountDiscountEligibleGroups = nil
	require.NoError(t, RefreshAmountDiscountPolicy())

	rate, eligible, applied := GetAmountDiscountRate(200, "default")
	assert.Equal(t, 1.0, rate)
	assert.False(t, eligible)
	assert.False(t, applied)

	serializedGroups, err := common.Marshal(paymentSetting.AmountDiscountEligibleGroups)
	require.NoError(t, err)
	assert.JSONEq(t, `[]`, string(serializedGroups))
	assert.NotNil(t, GetAmountDiscountPolicy().EligibleGroups)
}
