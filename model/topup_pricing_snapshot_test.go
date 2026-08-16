package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopUpPricingSnapshotRoundTrip(t *testing.T) {
	original := &TopUpPricingSnapshot{
		Version:                TopUpPricingSnapshotVersion,
		PaymentProvider:        PaymentProviderEpay,
		UserGroup:              "vip",
		QuotaDisplayType:       "USD",
		RequestedAmount:        200,
		NormalizedAmount:       200,
		StoredAmount:           200,
		QuotaPerUnit:           500000,
		UnitPrice:              1,
		TopupGroupRatio:        1.2,
		AmountDiscountEligible: true,
		AmountDiscountApplied:  true,
		AmountDiscountRate:     0.99,
		PayMoney:               23.76,
	}
	raw, err := original.Marshal()
	require.NoError(t, err)
	parsed, err := ParseTopUpPricingSnapshot(raw)
	require.NoError(t, err)
	require.NotNil(t, parsed)
	assert.Equal(t, original, parsed)

	empty, err := ParseTopUpPricingSnapshot("")
	require.NoError(t, err)
	assert.Nil(t, empty)
}

func TestTopUpPricingSnapshotRejectsInvalidVersion(t *testing.T) {
	parsed, err := ParseTopUpPricingSnapshot(`{"version":0}`)
	assert.Error(t, err)
	assert.Nil(t, parsed)
}
