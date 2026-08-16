package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
)

type topUpPricing struct {
	PayMoney float64
	Snapshot model.TopUpPricingSnapshot
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
		PayMoney: payMoney.InexactFloat64(),
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
