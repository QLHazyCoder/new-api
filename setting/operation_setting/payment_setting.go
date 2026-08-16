package operation_setting

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

const AmountDiscountEligibleGroupsOptionKey = "payment_setting.amount_discount_eligible_groups"

const AmountDiscountOptionKey = "payment_setting.amount_discount"

type PaymentSetting struct {
	AmountOptions                []int           `json:"amount_options"`
	AmountDiscount               map[int]float64 `json:"amount_discount"`
	AmountDiscountEligibleGroups []string        `json:"amount_discount_eligible_groups"`
	DefaultTopUpAmount           int             `json:"default_topup_amount"`
	ComplianceConfirmed          bool            `json:"compliance_confirmed"`
	ComplianceTermsVersion       string          `json:"compliance_terms_version"`
	ComplianceConfirmedAt        int64           `json:"compliance_confirmed_at"`
	ComplianceConfirmedBy        int             `json:"compliance_confirmed_by"`
	ComplianceConfirmedIP        string          `json:"compliance_confirmed_ip"`
}

type AmountDiscountPolicy struct {
	Discounts      map[int]float64
	EligibleGroups []string
	eligibleSet    map[string]struct{}
}

const CurrentComplianceTermsVersion = "v1"

var paymentSetting = PaymentSetting{
	AmountOptions:                []int{10, 20, 50, 100, 200, 500},
	AmountDiscount:               map[int]float64{},
	AmountDiscountEligibleGroups: []string{},
	DefaultTopUpAmount:           100,
}

var amountDiscountPolicy atomic.Pointer[AmountDiscountPolicy]

func init() {
	config.GlobalConfig.Register("payment_setting", &paymentSetting)
	_ = RefreshAmountDiscountPolicy()
}

func GetPaymentSetting() *PaymentSetting {
	return &paymentSetting
}

func NormalizeAmountDiscountPolicy(discounts map[int]float64, groups []string) (map[int]float64, []string, error) {
	normalizedDiscounts := make(map[int]float64, len(discounts))
	for amount, rate := range discounts {
		if amount <= 0 {
			return nil, nil, fmt.Errorf("discount amount must be a positive integer")
		}
		if math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 || rate > 1 {
			return nil, nil, fmt.Errorf("discount rate for amount %d must be greater than 0 and at most 1", amount)
		}
		normalizedDiscounts[amount] = rate
	}

	groupSet := make(map[string]struct{}, len(groups))
	for _, rawGroup := range groups {
		group := strings.TrimSpace(rawGroup)
		if group == "" {
			continue
		}
		if utf8.RuneCountInString(group) > 64 {
			return nil, nil, fmt.Errorf("group %q exceeds 64 characters", group)
		}
		groupSet[group] = struct{}{}
	}
	normalizedGroups := make([]string, 0, len(groupSet))
	for group := range groupSet {
		normalizedGroups = append(normalizedGroups, group)
	}
	sort.Strings(normalizedGroups)

	return normalizedDiscounts, normalizedGroups, nil
}

func ValidateAmountDiscountJSON(value string) error {
	var discounts map[int]float64
	if err := common.UnmarshalJsonStr(value, &discounts); err != nil {
		return fmt.Errorf("invalid amount discount JSON: %w", err)
	}
	_, _, err := NormalizeAmountDiscountPolicy(discounts, nil)
	return err
}

func ValidateAmountDiscountEligibleGroupsJSON(value string) error {
	var groups []string
	if err := common.UnmarshalJsonStr(value, &groups); err != nil {
		return fmt.Errorf("invalid amount discount eligible groups JSON: %w", err)
	}
	_, _, err := NormalizeAmountDiscountPolicy(nil, groups)
	return err
}

func RefreshAmountDiscountPolicy() error {
	discounts, groups, err := NormalizeAmountDiscountPolicy(
		paymentSetting.AmountDiscount,
		paymentSetting.AmountDiscountEligibleGroups,
	)
	if err != nil {
		return err
	}

	eligibleSet := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		eligibleSet[group] = struct{}{}
	}
	paymentSetting.AmountDiscount = cloneAmountDiscounts(discounts)
	paymentSetting.AmountDiscountEligibleGroups = append([]string(nil), groups...)
	amountDiscountPolicy.Store(&AmountDiscountPolicy{
		Discounts:      discounts,
		EligibleGroups: groups,
		eligibleSet:    eligibleSet,
	})
	return nil
}

func GetAmountDiscountPolicy() AmountDiscountPolicy {
	policy := amountDiscountPolicy.Load()
	if policy == nil {
		return AmountDiscountPolicy{
			Discounts:      map[int]float64{},
			EligibleGroups: []string{},
			eligibleSet:    map[string]struct{}{},
		}
	}
	eligibleSet := make(map[string]struct{}, len(policy.eligibleSet))
	for group := range policy.eligibleSet {
		eligibleSet[group] = struct{}{}
	}
	return AmountDiscountPolicy{
		Discounts:      cloneAmountDiscounts(policy.Discounts),
		EligibleGroups: append([]string(nil), policy.EligibleGroups...),
		eligibleSet:    eligibleSet,
	}
}

func GetAmountDiscountRate(amount int64, group string) (rate float64, eligible bool, applied bool) {
	policy := amountDiscountPolicy.Load()
	if policy == nil {
		return 1, false, false
	}
	group = strings.TrimSpace(group)
	if _, ok := policy.eligibleSet[group]; !ok {
		return 1, false, false
	}
	rate, ok := policy.Discounts[int(amount)]
	if !ok {
		return 1, true, false
	}
	return rate, true, true
}

func GetAmountDiscountsForGroup(group string) map[int]float64 {
	policy := amountDiscountPolicy.Load()
	if policy == nil {
		return map[int]float64{}
	}
	if _, ok := policy.eligibleSet[strings.TrimSpace(group)]; !ok {
		return map[int]float64{}
	}
	return cloneAmountDiscounts(policy.Discounts)
}

func cloneAmountDiscounts(discounts map[int]float64) map[int]float64 {
	cloned := make(map[int]float64, len(discounts))
	for amount, rate := range discounts {
		cloned[amount] = rate
	}
	return cloned
}

func IsPaymentComplianceConfirmed() bool {
	return paymentSetting.ComplianceConfirmed &&
		paymentSetting.ComplianceTermsVersion == CurrentComplianceTermsVersion
}
