package model

import (
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestLogOtherScopesAndMerges(t *testing.T) {
	var other LogOther

	assert.True(t, other.SetPublic("request_path", "/v1/chat/completions"))
	other.MergePublic(map[string]any{
		"zero": 0,
	})
	assert.True(t, other.SetAdmin("use_channel", []string{"channel-a"}))
	other.MergeAdmin(map[string]any{
		"rejected": false,
	})
	assert.True(t, other.SetRoot("upstream_request_id", "upstream-private"))
	other.MergeRoot(map[string]any{
		"generation": 0,
	})
	assert.True(t, other.SetAudit("method", "POST"))
	other.MergeAudit(map[string]any{
		"success": false,
	})

	require.JSONEq(t, `{
		"request_path": "/v1/chat/completions",
		"zero": 0,
		"admin_info": {
			"use_channel": ["channel-a"],
			"rejected": false
		},
		"root_info": {
			"upstream_request_id": "upstream-private",
			"generation": 0
		},
		"audit_info": {
			"method": "POST",
			"success": false
		}
	}`, other.JSONString())
}

func TestLogOtherRejectsSensitivePublicFields(t *testing.T) {
	other := NewLogOther()

	for _, key := range []string{
		"admin_info",
		"root_info",
		"audit_info",
		"channel_id",
		"channel_name",
		"channel_type",
		"reject_reason",
		"is_model_mapped",
		"upstream_model_name",
		"response_model",
	} {
		assert.False(t, other.SetPublic(key, "must-not-leak"), key)
	}
	other.MergePublic(map[string]any{
		"request_path": "/v1/responses",
		"channel_name": "still-must-not-leak",
		"admin_info":   map[string]any{"secret": true},
	})

	require.JSONEq(t, `{"request_path":"/v1/responses"}`, other.JSONString())
	require.JSONEq(t, `{}`, NewLogOther().JSONString())
}

func TestLogOtherJSONStringDoesNotMutateReceiver(t *testing.T) {
	other := NewLogOther()
	require.True(t, other.SetPublic("request_path", "/v1/chat/completions"))
	require.True(t, other.SetAdmin("rejected", false))

	before := other.Snapshot()
	first := other.JSONString()
	after := other.Snapshot()
	second := other.JSONString()

	require.Equal(t, before, after)
	require.Equal(t, first, second)
}

func openLogModelVisibilityMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&Log{}))
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	return db
}

func TestMigrateLogModelVisibility(t *testing.T) {
	t.Run("dry run apply idempotency and scoped values win", func(t *testing.T) {
		db := openLogModelVisibilityMigrationTestDB(t)
		logs := []*Log{
			{
				Id:    1,
				Other: `{"public_id":9007199254740993,"is_model_mapped":true,"upstream_model_name":"legacy-upstream","response_model":{"requested_model":"requested","upstream_model":"legacy-upstream","returned_model":"legacy-returned"}}`,
			},
			{
				Id:    2,
				Other: `{"upstream_model_name":"legacy-upstream","response_model":{"requested_model":"requested","upstream_model":"legacy-upstream","returned_model":"legacy-returned"},"admin_info":{"upstream_model_name":"scoped-upstream","response_model":{"requested_model":"requested","upstream_model":"scoped-upstream","returned_model":"scoped-returned"},"existing":"preserved"}}`,
			},
			{Id: 3, Other: `{"model_price":0.004}`},
		}
		for _, entry := range logs {
			require.NoError(t, db.Create(entry).Error)
		}

		dryRun, err := migrateLogModelVisibility(db, false, 1)
		require.NoError(t, err)
		assert.Equal(t, LogModelVisibilityMigrationReport{Scanned: 3, Candidates: 2, Remaining: 2}, dryRun)

		var before Log
		require.NoError(t, db.First(&before, 1).Error)
		assert.Contains(t, before.Other, `"response_model"`)

		applied, err := migrateLogModelVisibility(db, true, 1)
		require.NoError(t, err)
		assert.Equal(t, LogModelVisibilityMigrationReport{Scanned: 3, Candidates: 2, Migrated: 2}, applied)

		var migratedFirst, migratedSecond Log
		require.NoError(t, db.First(&migratedFirst, 1).Error)
		require.NoError(t, db.First(&migratedSecond, 2).Error)
		assert.Contains(t, migratedFirst.Other, `"public_id":9007199254740993`)
		for _, entry := range []Log{migratedFirst, migratedSecond} {
			var values map[string]common.RawMessage
			require.NoError(t, common.UnmarshalJsonStr(entry.Other, &values))
			for _, key := range modelDiagnosticLogOtherKeys {
				assert.NotContains(t, values, key)
			}
		}

		var firstAdmin, secondAdmin map[string]common.RawMessage
		var firstValues, secondValues map[string]common.RawMessage
		require.NoError(t, common.UnmarshalJsonStr(migratedFirst.Other, &firstValues))
		require.NoError(t, common.UnmarshalJsonStr(migratedSecond.Other, &secondValues))
		require.NoError(t, common.Unmarshal(firstValues[logOtherAdminInfoKey], &firstAdmin))
		require.NoError(t, common.Unmarshal(secondValues[logOtherAdminInfoKey], &secondAdmin))
		assert.JSONEq(t, `true`, string(firstAdmin["is_model_mapped"]))
		assert.JSONEq(t, `"legacy-upstream"`, string(firstAdmin["upstream_model_name"]))
		assert.JSONEq(t, `"scoped-upstream"`, string(secondAdmin["upstream_model_name"]))
		assert.JSONEq(t, `{"requested_model":"requested","upstream_model":"scoped-upstream","returned_model":"scoped-returned"}`, string(secondAdmin["response_model"]))
		assert.JSONEq(t, `"preserved"`, string(secondAdmin["existing"]))

		idempotent, err := migrateLogModelVisibility(db, true, 1)
		require.NoError(t, err)
		assert.Equal(t, LogModelVisibilityMigrationReport{Scanned: 3}, idempotent)
	})

	t.Run("invalid JSON leaves the row unchanged", func(t *testing.T) {
		db := openLogModelVisibilityMigrationTestDB(t)
		require.NoError(t, db.Create(&Log{Id: 1, Other: `{"response_model":`}).Error)

		_, err := migrateLogModelVisibility(db, false, 10)
		require.ErrorContains(t, err, "decode other JSON")
		var stored Log
		require.NoError(t, db.First(&stored, 1).Error)
		assert.Equal(t, `{"response_model":`, stored.Other)
	})

	t.Run("a failed batch rolls back every update", func(t *testing.T) {
		db := openLogModelVisibilityMigrationTestDB(t)
		require.NoError(t, db.Create(&Log{Id: 1, Other: `{"upstream_model_name":"first"}`}).Error)
		require.NoError(t, db.Create(&Log{Id: 2, Other: `{"upstream_model_name":"second"}`}).Error)
		require.NoError(t, db.Exec(`
			CREATE TRIGGER reject_second_log_visibility_update
			BEFORE UPDATE ON logs
			WHEN NEW.id = 2
			BEGIN
				SELECT RAISE(ABORT, 'reject second update');
			END;
		`).Error)

		report, err := migrateLogModelVisibility(db, true, 10)
		require.ErrorContains(t, err, "apply log batch")
		assert.Equal(t, int64(0), report.Migrated)
		var first Log
		require.NoError(t, db.First(&first, 1).Error)
		assert.Contains(t, first.Other, `"upstream_model_name":"first"`)
		assert.NotContains(t, first.Other, `"admin_info"`)
	})
}

func TestInitLogDBForMaintenanceUsesOnlyLogDatabase(t *testing.T) {
	previousDB := DB
	previousLogDB := LOG_DB
	previousLogDatabaseType := common.LogDatabaseType()
	previousSQLitePath := common.SQLitePath
	common.SQLitePath = filepath.Join(t.TempDir(), "maintenance-logs.db")
	t.Setenv("LOG_SQL_DSN", "")
	t.Setenv("SQL_DSN", "local")
	t.Cleanup(func() {
		if LOG_DB != nil && LOG_DB != previousLogDB {
			require.NoError(t, CloseLogDBForMaintenance())
		}
		LOG_DB = previousLogDB
		common.SetLogDatabaseType(previousLogDatabaseType)
		common.SQLitePath = previousSQLitePath
	})

	require.NoError(t, InitLogDBForMaintenance())
	require.NotNil(t, LOG_DB)
	assert.Equal(t, common.DatabaseTypeSQLite, common.LogDatabaseType())
	assert.Same(t, previousDB, DB)
}
