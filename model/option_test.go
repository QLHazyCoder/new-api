package model

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

func TestGroupTypeOptionsReloadsLegacyAutomaticRoutingAssignment(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			dsn := os.Getenv("TEST_" + strings.ToUpper(dialect) + "_DSN")
			if dialect != "sqlite" && dsn == "" {
				t.Skip("database DSN not configured")
			}
			db := openSensitiveWordMatrixDB(t, dialect, dsn)
			previousDB := DB
			previousOptions := common.OptionMap
			previousPolicy := CurrentRequestPolicy()
			previousTypes, previousAssignments := ratio_setting.GroupTypeOptions2JSONString()
			DB = db
			common.OptionMap = map[string]string{}
			t.Cleanup(func() {
				DB = previousDB
				common.OptionMap = previousOptions
				requestPolicySnapshot.Store(previousPolicy)
				require.NoError(t, ratio_setting.UpdateGroupTypeOptionsByJSONString(previousTypes, previousAssignments))
			})
			require.NoError(t, db.AutoMigrate(&Option{}))
			typesJSON := `{"vendor":{"name":"标准类型","order":1}}`
			legacyMap := `{"auto":"vendor","default":"vendor","vip":"vendor"}`
			require.NoError(t, db.Create(&[]Option{{Key: groupTypesOptionKey, Value: typesJSON}, {Key: groupTypeMapOptionKey, Value: legacyMap}}).Error)
			for range 2 {
				require.NoError(t, ratio_setting.UpdateGroupTypeOptionsByJSONString(`{}`, `{}`))
				loadOptionsFromDatabase()
				require.Equal(t, "标准类型", ratio_setting.GetGroupTypesCopy()["vendor"].Name)
				require.Equal(t, map[string]string{"default": "vendor", "vip": "vendor"}, ratio_setting.GetGroupTypeMapCopy())
				require.JSONEq(t, `{"default":"vendor","vip":"vendor"}`, common.OptionMap[groupTypeMapOptionKey])
			}
			var stored Option
			require.NoError(t, db.Where(&Option{Key: groupTypeMapOptionKey}).First(&stored).Error)
			require.Equal(t, legacyMap, stored.Value, "startup must leave stored configuration intact for rollback")
			require.Error(t, UpdateGroupTypeOptions(typesJSON, legacyMap), "new writes must still reject auto assignments")
		})
	}
}

func TestNormalizeLegacyAudioCompletionRatioOptionValue(t *testing.T) {
	normalized, repaired := normalizeLegacyOptionValue("AudioCompletionRatio", " <nil> ")
	require.True(t, repaired)

	var ratios map[string]float64
	require.NoError(t, json.Unmarshal([]byte(normalized), &ratios))
	require.Equal(t, 2.0, ratios["gpt-4o-realtime"])
	require.Equal(t, 1.0, ratios["gpt-4o-mini-tts"])

	unchanged, repaired := normalizeLegacyOptionValue("AudioCompletionRatio", "{}")
	require.False(t, repaired)
	require.Equal(t, "{}", unchanged)
}

func TestUpdateOptionMapLogRetentionDays(t *testing.T) {
	originalRetentionDays := common.LogRetentionDays
	originalOptionMap := common.OptionMap
	t.Cleanup(func() {
		common.LogRetentionDays = originalRetentionDays
		common.OptionMap = originalOptionMap
	})

	common.OptionMap = map[string]string{"LogRetentionDays": "30"}
	common.LogRetentionDays = 30

	require.NoError(t, updateOptionMap("LogRetentionDays", " 60 "))
	require.Equal(t, 60, common.LogRetentionDays)
	require.Equal(t, "60", common.OptionMap["LogRetentionDays"])

	require.Error(t, updateOptionMap("LogRetentionDays", "-1"))
	require.Equal(t, 60, common.LogRetentionDays)
	require.Equal(t, "60", common.OptionMap["LogRetentionDays"])
}

func TestUpdateOptionMapPlaygroundImageMaxConcurrency(t *testing.T) {
	originalOptionMap := common.OptionMap
	t.Cleanup(func() {
		common.OptionMap = originalOptionMap
	})

	common.OptionMap = map[string]string{playgroundImageConcurrencyKey: "0"}
	require.NoError(t, updateOptionMap(playgroundImageConcurrencyKey, " 3 "))
	require.Equal(t, "3", common.OptionMap[playgroundImageConcurrencyKey])

	require.Error(t, updateOptionMap(playgroundImageConcurrencyKey, "-1"))
	require.Equal(t, "3", common.OptionMap[playgroundImageConcurrencyKey])
}

func TestGroupTypeOptionsPersistAndReload(t *testing.T) {
	db := setupRegistrationGroupPolicyTest(t)
	originalGroupTypes := ratio_setting.GroupTypes2JSONString()
	originalGroupTypeMap := ratio_setting.GroupTypeMap2JSONString()
	originalOptionMap := common.OptionMap
	originalPolicy := CurrentRequestPolicy()
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		common.OptionMap = originalOptionMap
		requestPolicySnapshot.Store(originalPolicy)
		require.NoError(t, ratio_setting.UpdateGroupTypeOptionsByJSONString(originalGroupTypes, originalGroupTypeMap))
	})

	require.Error(t, UpdateGroupTypeOptions(
		`{"vendor":{"name":"Vendor","order":1}}`,
		`{"retired":"vendor"}`,
	))
	var optionsBeforeSave []Option
	require.NoError(t, db.Find(&optionsBeforeSave).Error)
	require.Empty(t, optionsBeforeSave)

	require.NoError(t, UpdateGroupTypeOptions(`{"vendor":{"name":"Vendor","icon":"OpenAI","order":1}}`, `{"default":"vendor"}`))

	var options []Option
	require.NoError(t, db.Find(&options).Error)
	stored := make(map[string]string, len(options))
	for _, option := range options {
		stored[option.Key] = option.Value
	}
	require.Equal(t, `{"vendor":{"name":"Vendor","icon":"OpenAI","order":1}}`, stored["group_ratio_setting.group_types"])
	require.Equal(t, `{"default":"vendor"}`, stored["group_ratio_setting.group_type_map"])

	for range 2 {
		require.NoError(t, ratio_setting.UpdateGroupTypeOptionsByJSONString(`{}`, `{}`))
		loadOptionsFromDatabase()
		require.Equal(t, "Vendor", ratio_setting.GetGroupTypesCopy()["vendor"].Name)
		require.Equal(t, "vendor", ratio_setting.GetGroupTypeMapCopy()["default"])
	}

	// Retain the last valid runtime pair while exposing invalid stored values for repair.
	require.NoError(t, db.Model(&Option{}).Where(&Option{Key: groupTypeMapOptionKey}).Update("value", `{"default":"missing"}`).Error)
	loadOptionsFromDatabase()
	require.Equal(t, "vendor", ratio_setting.GetGroupTypeMapCopy()["default"])
	require.Equal(t, `{"default":"missing"}`, common.OptionMap[groupTypeMapOptionKey])

	require.Error(t, UpdateOption(groupTypesOptionKey, `{}`))
	require.Error(t, UpdateOptionsBulk(map[string]string{groupTypeMapOptionKey: `{}`}))
	require.NoError(t, UpdateOptionsBulk(map[string]string{groupTypesOptionKey: `{}`, groupTypeMapOptionKey: `{}`}))
	require.Empty(t, ratio_setting.GetGroupTypesCopy())
	require.Empty(t, ratio_setting.GetGroupTypeMapCopy())
}

func TestUpdateGroupTypeOptionsRollsBackBothOptionsOnDatabaseFailure(t *testing.T) {
	db := setupRegistrationGroupPolicyTest(t)
	originalGroupTypes := ratio_setting.GroupTypes2JSONString()
	originalGroupTypeMap := ratio_setting.GroupTypeMap2JSONString()
	originalOptionMap := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		common.OptionMap = originalOptionMap
		require.NoError(t, ratio_setting.UpdateGroupTypeOptionsByJSONString(originalGroupTypes, originalGroupTypeMap))
	})

	oldGroupTypes := `{"vendor":{"name":"Vendor","order":1}}`
	oldGroupTypeMap := `{"default":"vendor"}`
	require.NoError(t, ratio_setting.UpdateGroupTypeOptionsByJSONString(oldGroupTypes, oldGroupTypeMap))
	require.NoError(t, db.Create(&[]Option{
		{Key: groupTypesOptionKey, Value: oldGroupTypes},
		{Key: groupTypeMapOptionKey, Value: oldGroupTypeMap},
	}).Error)
	common.OptionMap[groupTypesOptionKey] = oldGroupTypes
	common.OptionMap[groupTypeMapOptionKey] = oldGroupTypeMap
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_group_type_map_update BEFORE UPDATE ON options WHEN NEW.key = 'group_ratio_setting.group_type_map' BEGIN SELECT RAISE(ABORT, 'injected write failure'); END`).Error)

	err := UpdateGroupTypeOptions(`{"domestic":{"name":"Domestic","order":1}}`, `{"vip":"domestic"}`)
	require.Error(t, err)

	var stored []Option
	require.NoError(t, db.Where("key IN ?", []string{groupTypesOptionKey, groupTypeMapOptionKey}).Find(&stored).Error)
	require.Len(t, stored, 2)
	storedValues := map[string]string{}
	for _, option := range stored {
		storedValues[option.Key] = option.Value
	}
	require.Equal(t, oldGroupTypes, storedValues[groupTypesOptionKey])
	require.Equal(t, oldGroupTypeMap, storedValues[groupTypeMapOptionKey])
	require.Equal(t, "Vendor", ratio_setting.GetGroupTypesCopy()["vendor"].Name)
	require.Equal(t, "vendor", ratio_setting.GetGroupTypeMapCopy()["default"])
	require.Equal(t, oldGroupTypes, common.OptionMap[groupTypesOptionKey])
	require.Equal(t, oldGroupTypeMap, common.OptionMap[groupTypeMapOptionKey])
}
