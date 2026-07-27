package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMigrateDBFastIncludesAuthorizationTables(t *testing.T) {
	originalDB := DB
	originalLogDB := LOG_DB
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	DB = db
	LOG_DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	initCol()
	t.Cleanup(func() {
		DB = originalDB
		LOG_DB = originalLogDB
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		initCol()
	})

	require.NoError(t, migrateDBFast())
	var requiredOption Option
	require.NoError(t, db.Where(&Option{Key: playgroundImageConcurrencyKey}).First(&requiredOption).Error)
	require.Equal(t, "0", requiredOption.Value)
	for _, table := range []struct {
		name  string
		model any
	}{
		{name: "user sessions", model: &UserSession{}},
		{name: "auth flows", model: &AuthFlow{}},
		{name: "external identity claims", model: &ExternalIdentityClaim{}},
		{name: "affiliate reward ledger", model: &AffiliateRewardEvent{}},
		{name: "performance metrics", model: &PerfMetric{}},
		{name: "system tasks", model: &SystemTask{}},
		{name: "playground image batches", model: &PlaygroundImageBatch{}},
		{name: "playground image tasks", model: &PlaygroundImageTask{}},
		{name: "casbin rules", model: &CasbinRule{}},
		{name: "authorization roles", model: &AuthzRole{}},
	} {
		t.Run(table.name, func(t *testing.T) {
			require.True(t, db.Migrator().HasTable(table.model))
		})
	}
}
