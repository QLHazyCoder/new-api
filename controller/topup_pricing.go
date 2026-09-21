package controller

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
)

type topUpPricing struct {
	PayMoney        float64
	PayMoneyDecimal decimal.Decimal
	Snapshot        model.TopUpPricingSnapshot
}

func calculateTopUpPricing(requestedAmount int64, storedAmount int64, group string, provider string, unitPrice float64) topUpPricing {
	dAmount := decimal.NewFromInt(requestedAmount)
	quotaDisplayType := operation_setting.GetQuotaDisplayType()
	if quotaDisplayType == operation_setting.QuotaDisplayTypeTokens {
		dAmount = dAmount.Div(decimal.NewFromFloat(common.QuotaPerUnit))
	}

	topupGroupRatio := common.GetTopupGroupRatio(group)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}
	discountRate, discountEligible, discountApplied := operation_setting.GetAmountDiscountRate(requestedAmount, group)
	payMoney := dAmount.
		Mul(decimal.NewFromFloat(unitPrice)).
		Mul(decimal.NewFromFloat(topupGroupRatio)).
		Mul(decimal.NewFromFloat(discountRate))

	return topUpPricing{
		PayMoney:        payMoney.InexactFloat64(),
		PayMoneyDecimal: payMoney,
		Snapshot: model.TopUpPricingSnapshot{
			Version:                model.TopUpPricingSnapshotVersion,
			PaymentProvider:        provider,
			UserGroup:              group,
			QuotaDisplayType:       quotaDisplayType,
			RequestedAmount:        requestedAmount,
			NormalizedAmount:       dAmount.InexactFloat64(),
			StoredAmount:           storedAmount,
			QuotaPerUnit:           common.QuotaPerUnit,
			UnitPrice:              unitPrice,
			TopupGroupRatio:        topupGroupRatio,
			AmountDiscountEligible: discountEligible,
			AmountDiscountApplied:  discountApplied,
			AmountDiscountRate:     discountRate,
			PayMoney:               payMoney.InexactFloat64(),
		},
	}
}

// finalizeEpayPricing rounds the provider-facing amount once to its smallest
// currency unit and records that exact integer in the snapshot. The float is
// retained only for legacy display fields and logging.
func finalizeEpayPricing(pricing *topUpPricing, creditedQuota int64, storedAmount int64) error {
	if pricing == nil {
		return fmt.Errorf("epay pricing is nil")
	}
	minor, err := common.WalletQuotaFromDecimal(
		pricing.PayMoneyDecimal.Mul(decimal.NewFromInt(100)).Round(0),
	)
	if err != nil || minor <= 0 {
		return fmt.Errorf("epay payment amount is outside supported range")
	}
	pricing.PayMoneyDecimal = decimal.NewFromInt(minor).Div(decimal.NewFromInt(100))
	pricing.PayMoney = pricing.PayMoneyDecimal.InexactFloat64()
	pricing.Snapshot.StoredAmount = storedAmount
	pricing.Snapshot.CreditedQuota = creditedQuota
	pricing.Snapshot.QuotedMoneyMinor = minor
	pricing.Snapshot.PayMoney = pricing.PayMoney
	return nil
}

func marshalTopUpPricingSnapshot(pricing topUpPricing) (string, error) {
	return pricing.Snapshot.Marshal()
}

func getEpayTopUpPricing(requestedAmount int64, group string) topUpPricing {
	return calculateTopUpPricing(requestedAmount, requestedAmount, group, model.PaymentProviderEpay, operation_setting.Price)
}

func getWaffoTopUpPricing(requestedAmount int64, storedAmount int64, group string) topUpPricing {
	return calculateTopUpPricing(requestedAmount, storedAmount, group, model.PaymentProviderWaffo, setting.WaffoUnitPrice)
}

func getWaffoPancakeTopUpPricing(requestedAmount int64, storedAmount int64, group string) topUpPricing {
	return calculateTopUpPricing(requestedAmount, storedAmount, group, model.PaymentProviderWaffoPancake, setting.WaffoPancakeUnitPrice)
}
