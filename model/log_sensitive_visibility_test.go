package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSensitiveWordLogVisibilityIsEnforcedBeforePagination(t *testing.T) {
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

	for _, entry := range []Log{
		{UserId: 7, TokenId: 71, Type: LogTypeConsume, RequestId: "visible-consume"},
		{UserId: 7, TokenId: 71, Type: LogTypeSensitiveWordBlock, RequestId: "private-audit", Other: `{"admin_info":{"keyword_filter":{"action":"blocked","audit_id":42}}}`},
		{UserId: 7, TokenId: 71, Type: LogTypeError, RequestId: "visible-error"},
		{UserId: 8, TokenId: 81, Type: LogTypeSensitiveWordBlock, RequestId: "other-user-audit"},
	} {
		require.NoError(t, db.Create(&entry).Error)
	}

	read := func(role, logType, offset, size int, requestID string) ([]*Log, int64) {
		t.Helper()
		logs, total, err := GetUserLogsForRole(7, role, logType, 0, 0, "", "", offset, size, "", requestID, "")
		require.NoError(t, err)
		return logs, total
	}

	first, total := read(common.RoleCommonUser, LogTypeUnknown, 0, 1, "")
	require.EqualValues(t, 2, total)
	require.Len(t, first, 1)
	require.Equal(t, "visible-error", first[0].RequestId)
	second, total := read(common.RoleCommonUser, LogTypeUnknown, 1, 1, "")
	require.EqualValues(t, 2, total)
	require.Len(t, second, 1)
	require.Equal(t, "visible-consume", second[0].RequestId)
	for _, filter := range []struct {
		logType   int
		requestID string
	}{{LogTypeSensitiveWordBlock, ""}, {LogTypeUnknown, "private-audit"}} {
		logs, total := read(common.RoleCommonUser, filter.logType, 0, 10, filter.requestID)
		require.Zero(t, total)
		require.Empty(t, logs)
	}

	for _, role := range []int{common.RoleAdminUser, common.RoleRootUser} {
		logs, total := read(role, LogTypeSensitiveWordBlock, 0, 10, "")
		require.EqualValues(t, 1, total)
		require.Len(t, logs, 1)
		require.Contains(t, logs[0].Other, `"audit_id":42`)
	}
	logs, total, err := GetUserLogs(7, LogTypeUnknown, 0, 0, "", "", 0, 10, "", "", "")
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, logs, 2)

	tokenLogs, err := GetLogByTokenId(71)
	require.NoError(t, err)
	require.Len(t, tokenLogs, 2)
	for _, entry := range tokenLogs {
		require.NotEqual(t, LogTypeSensitiveWordBlock, entry.Type)
	}
}
