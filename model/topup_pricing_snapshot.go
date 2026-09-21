package model

import (
	"errors"
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

// Version 2 adds the exact Epay settlement fields. Version 1 snapshots are
// still readable because Waffo and historical orders may contain them.
const TopUpPricingSnapshotVersion = 2

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
	// The following fields are authoritative for Epay settlement. They are
	// optional so the same snapshot type remains compatible with other
	// providers and version-1 historical records.
	CreditedQuota        int64 `json:"credited_quota,omitempty,string"`
	QuotedMoneyMinor     int64 `json:"quoted_money_minor,omitempty,string"`
	RewardRateBps        int64 `json:"reward_rate_bps,omitempty,string"`
	InviteRewardEligible bool  `json:"invite_reward_eligible,omitempty"`
}

// RewardRateBpsFromPercent converts the administrator-facing percentage into
// an integer basis-point value. Keeping this conversion explicit prevents a
// float64 percentage from becoming an accounting input at settlement time.
func RewardRateBpsFromPercent(percent float64) (int64, error) {
	if math.IsNaN(percent) || math.IsInf(percent, 0) || percent < 0 || percent > 100 {
		return 0, errors.New("invite reward percentage must be between 0 and 100")
	}
	bps := decimal.NewFromFloat(percent).Mul(decimal.NewFromInt(100)).Round(0)
	value, err := common.WalletQuotaFromDecimal(bps)
	if err != nil || value < 0 || value > 10000 {
		return 0, errors.New("invite reward percentage is outside basis-point range")
	}
	return value, nil
}

func RewardPercentFromBps(bps int64) string {
	return decimal.NewFromInt(bps).Div(decimal.NewFromInt(100)).String()
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
