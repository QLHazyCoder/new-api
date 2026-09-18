package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestWalletQuotaSupportsLargeBalancesAndInt64Boundary(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)

	user := createReserveTestUser(t, 1_131_876_424)
	require.NoError(t, IncreaseUserQuota(user.Id, 1_675_000_000, true))
	require.Equal(t, int64(2_806_876_424), getUserQuotaFromDB(t, user.Id))

	require.NoError(t, IncreaseUserQuota(user.Id, 50_000_000_000, true))
	require.Equal(t, int64(52_806_876_424), getUserQuotaFromDB(t, user.Id))
	require.NoError(t, DecreaseUserQuota(user.Id, 2_806_876_424, true))
	require.Equal(t, int64(50_000_000_000), getUserQuotaFromDB(t, user.Id))

	require.NoError(t, OverrideUserQuota(user.Id, common.MaxWalletQuota-1))
	require.NoError(t, IncreaseUserQuota(user.Id, 1, true))
	require.Equal(t, common.MaxWalletQuota, getUserQuotaFromDB(t, user.Id))
	require.ErrorIs(t, IncreaseUserQuota(user.Id, 1, true), common.ErrWalletQuotaOverflow)
	require.Equal(t, common.MaxWalletQuota, getUserQuotaFromDB(t, user.Id))

	token := createReserveTestToken(t, 50_000_000_000)
	reserved, err := TryReserveTokenQuota(token.Id, token.Key, 2_000_000_000, false)
	require.NoError(t, err)
	require.True(t, reserved)
	reloaded := getTokenFromDB(t, token.Id)
	require.Equal(t, int64(48_000_000_000), reloaded.RemainQuota)
	require.Equal(t, int64(2_000_000_000), reloaded.UsedQuota)

	require.NoError(t, increaseTokenQuota(token.Id, 50_000_000_000))
	reloaded = getTokenFromDB(t, token.Id)
	require.Equal(t, int64(98_000_000_000), reloaded.RemainQuota)
	require.Equal(t, int64(-48_000_000_000), reloaded.UsedQuota)
}

func TestWalletQuotaSchemaUsesSigned64Columns(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(
		&Channel{}, &Token{}, &User{}, &Redemption{}, &Checkin{}, &QuotaData{},
		&AffiliateRewardEvent{}, &SensitiveWordAuditEvent{},
	))
	require.NoError(t, validateWalletQuotaSchema())
}
