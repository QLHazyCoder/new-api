package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSumUsedQuotaHonorsRequestedLogType(t *testing.T) {
	previousLogDB, previousLogType := LOG_DB, common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&Log{}))
	LOG_DB = db
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		LOG_DB = previousLogDB
		common.SetLogDatabaseType(previousLogType)
		require.NoError(t, sqlDB.Close())
	})

	require.NoError(t, db.Create(&Log{
		UserId: 7, TokenId: 71, Username: "alice", TokenName: "daily-key",
		CreatedAt: 1_000, Type: LogTypeConsume, Quota: 100,
	}).Error)
	require.NoError(t, db.Create(&Log{
		UserId: 7, TokenId: 71, Username: "alice", TokenName: "daily-key",
		CreatedAt: 1_001, Type: LogTypeRefund, Quota: 40,
	}).Error)

	consume, err := SumUsedQuota(LogTypeConsume, 900, 1_100, "", "alice", "daily-key", 0, "")
	require.NoError(t, err)
	require.Equal(t, 100, consume.Quota)

	refund, err := SumUsedQuota(LogTypeRefund, 900, 1_100, "", "alice", "daily-key", 0, "")
	require.NoError(t, err)
	require.Equal(t, 40, refund.Quota)
}
