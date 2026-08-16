package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
)

const TopUpPricingSnapshotVersion = 1

type TopUpPricingSnapshot struct {
	Version                int     `json:"version"`
	PaymentProvider        string  `json:"payment_provider"`
	UserGroup              string  `json:"user_group"`
	QuotaDisplayType       string  `json:"quota_display_type"`
	RequestedAmount        int64   `json:"requested_amount"`
	NormalizedAmount       float64 `json:"normalized_amount"`
	StoredAmount           int64   `json:"stored_amount"`
	QuotaPerUnit           float64 `json:"quota_per_unit"`
	UnitPrice              float64 `json:"unit_price"`
	TopupGroupRatio        float64 `json:"topup_group_ratio"`
	AmountDiscountEligible bool    `json:"amount_discount_eligible"`
	AmountDiscountApplied  bool    `json:"amount_discount_applied"`
	AmountDiscountRate     float64 `json:"amount_discount_rate"`
	PayMoney               float64 `json:"pay_money"`
}

func (snapshot *TopUpPricingSnapshot) Marshal() (string, error) {
	if snapshot == nil {
		return "", errors.New("pricing snapshot is nil")
	}
	data, err := common.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func ParseTopUpPricingSnapshot(raw string) (*TopUpPricingSnapshot, error) {
	if raw == "" {
		return nil, nil
	}
	var snapshot TopUpPricingSnapshot
	if err := common.UnmarshalJsonStr(raw, &snapshot); err != nil {
		return nil, err
	}
	if snapshot.Version <= 0 {
		return nil, errors.New("pricing snapshot version is invalid")
	}
	return &snapshot, nil
}
