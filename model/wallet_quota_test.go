package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
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
	require.EqualValues(t, common.MaxWalletQuota, getUserQuotaFromDB(t, user.Id))
	require.ErrorIs(t, IncreaseUserQuota(user.Id, 1, true), common.ErrWalletQuotaOverflow)
	require.EqualValues(t, common.MaxWalletQuota, getUserQuotaFromDB(t, user.Id))

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

func TestWalletQuotaSchemaRecognizesDialectTypeNames(t *testing.T) {
	previous := DB
	t.Cleanup(func() { DB = previous })
	for _, test := range []struct {
		dialect gorm.Dialector
		valid   []string
		invalid []string
	}{
		{postgres.New(postgres.Config{}), []string{"bigint", "int8", " INT8 "}, []string{"int4", "integer", "numeric", "bigint unsigned"}},
		{mysql.New(mysql.Config{SkipInitializeWithVersion: true}), []string{"bigint", "bigint(20)"}, []string{"int8", "int", "bigint unsigned"}},
		{sqlite.Open(":memory:"), []string{"integer", "bigint"}, []string{"real", "text", "bigint unsigned"}},
	} {
		t.Run(test.dialect.Name(), func(t *testing.T) {
			DB = &gorm.DB{Config: &gorm.Config{Dialector: test.dialect}}
			for _, name := range test.valid {
				require.True(t, isSignedBigIntType(name), name)
			}
			for _, name := range test.invalid {
				require.False(t, isSignedBigIntType(name), name)
			}
		})
	}
}
