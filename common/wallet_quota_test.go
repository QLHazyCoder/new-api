package common

import (
	"math/big"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestWalletQuotaCheckedArithmetic(t *testing.T) {
	value, err := AddWalletQuota(10, 5)
	require.NoError(t, err)
	require.Equal(t, int64(15), value)

	value, err = AddWalletQuota(MaxWalletQuota, 1)
	require.ErrorIs(t, err, ErrWalletQuotaOverflow)
	require.Zero(t, value)

	value, err = AddWalletQuota(MinWalletQuota, -1)
	require.ErrorIs(t, err, ErrWalletQuotaOverflow)
	require.Zero(t, value)

	value, err = SubWalletQuota(MinWalletQuota, 1)
	require.ErrorIs(t, err, ErrWalletQuotaOverflow)
	require.Zero(t, value)

	value, err = SubWalletQuota(MaxWalletQuota, -1)
	require.ErrorIs(t, err, ErrWalletQuotaOverflow)
	require.Zero(t, value)
}

func TestWalletQuotaDecimalAndBigIntInputs(t *testing.T) {
	value, err := WalletQuotaFromDecimal(decimal.RequireFromString("50000000000.4"))
	require.NoError(t, err)
	require.Equal(t, int64(50000000000), value)

	value, err = WalletQuotaFromDecimal(decimal.NewFromBigInt(big.NewInt(MaxWalletQuota), 0))
	require.NoError(t, err)
	require.Equal(t, MaxWalletQuota, value)

	value, err = WalletQuotaFromBigInt(new(big.Int).Add(big.NewInt(MaxWalletQuota), big.NewInt(1)))
	require.ErrorIs(t, err, ErrWalletQuotaOverflow)
	require.Zero(t, value)
}
