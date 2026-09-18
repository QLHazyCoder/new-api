package common

import (
	"errors"
	"math/big"

	"github.com/shopspring/decimal"
)

// Wallet quotas are signed int64 values. The bounds are technical storage
// limits, not product limits: wallet operations may use the complete signed
// range as long as the resulting value remains representable.
const (
	MaxWalletQuota int64 = 1<<63 - 1
	MinWalletQuota int64 = -1 << 63
)

var ErrWalletQuotaOverflow = errors.New("wallet quota exceeds int64 range")

// AddWalletQuota returns base+delta without allowing signed integer wraparound.
func AddWalletQuota(base, delta int64) (int64, error) {
	if delta > 0 && base > MaxWalletQuota-delta {
		return 0, ErrWalletQuotaOverflow
	}
	if delta < 0 && base < MinWalletQuota-delta {
		return 0, ErrWalletQuotaOverflow
	}
	return base + delta, nil
}

// SubWalletQuota returns base-delta without allowing signed integer wraparound.
func SubWalletQuota(base, delta int64) (int64, error) {
	if delta > 0 && base < MinWalletQuota+delta {
		return 0, ErrWalletQuotaOverflow
	}
	if delta < 0 && base > MaxWalletQuota+delta {
		return 0, ErrWalletQuotaOverflow
	}
	return base - delta, nil
}

// WalletQuotaFromDecimal rounds a decimal raw-quota value to an int64 and
// rejects values that cannot be represented exactly in the wallet domain.
func WalletQuotaFromDecimal(value decimal.Decimal) (int64, error) {
	rounded := value.Round(0)
	integer := rounded.BigInt()
	if integer == nil || !integer.IsInt64() {
		return 0, ErrWalletQuotaOverflow
	}
	return integer.Int64(), nil
}

// WalletQuotaFromBigInt is useful when a caller already performed its
// calculation using arbitrary precision integers.
func WalletQuotaFromBigInt(value *big.Int) (int64, error) {
	if value == nil || !value.IsInt64() {
		return 0, ErrWalletQuotaOverflow
	}
	return value.Int64(), nil
}
