package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetOccupiedUserGroups(t *testing.T) {
	originalDB := DB
	originalDatabaseType := common.MainDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.LogDatabaseType())
	InitColumnNamesForTest()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}))
	DB = db
	t.Cleanup(func() {
		DB = originalDB
		common.SetDatabaseTypes(originalDatabaseType, common.LogDatabaseType())
		InitColumnNamesForTest()
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	users := []User{
		{Username: "enabled", Password: "password", AffCode: "enabled-code", Group: "default", Status: common.UserStatusEnabled},
		{Username: "disabled", Password: "password", AffCode: "disabled-code", Group: "friend", Status: common.UserStatusDisabled},
		{Username: "trimmed", Password: "password", AffCode: "trimmed-code", Group: " vip ", Status: common.UserStatusEnabled},
		{Username: "blank", Password: "password", AffCode: "blank-code", Group: " ", Status: common.UserStatusEnabled},
		{Username: "deleted", Password: "password", AffCode: "deleted-code", Group: "deleted-group", Status: common.UserStatusEnabled},
	}
	for index := range users {
		require.NoError(t, db.Create(&users[index]).Error)
	}
	require.NoError(t, db.Delete(&users[4]).Error)

	groups, err := GetOccupiedUserGroups()
	require.NoError(t, err)
	assert.Equal(t, []string{"default", "friend", "vip"}, groups)
}
