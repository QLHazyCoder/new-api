package model

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

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
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		common.OptionMap = originalOptionMap
		require.NoError(t, ratio_setting.UpdateGroupTypesByJSONString(originalGroupTypes))
		require.NoError(t, ratio_setting.UpdateGroupTypeMapByJSONString(originalGroupTypeMap))
	})

	require.NoError(t, UpdateOption("group_ratio_setting.group_types", `{"vendor":{"name":"Vendor","icon":"OpenAI","order":1}}`))
	require.NoError(t, UpdateOption("group_ratio_setting.group_type_map", `{"default":"vendor"}`))

	var options []Option
	require.NoError(t, db.Find(&options).Error)
	stored := make(map[string]string, len(options))
	for _, option := range options {
		stored[option.Key] = option.Value
	}
	require.Equal(t, `{"vendor":{"name":"Vendor","icon":"OpenAI","order":1}}`, stored["group_ratio_setting.group_types"])
	require.Equal(t, `{"default":"vendor"}`, stored["group_ratio_setting.group_type_map"])

	require.NoError(t, ratio_setting.UpdateGroupTypesByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateGroupTypeMapByJSONString(`{}`))
	require.NoError(t, config.GlobalConfig.LoadFromDB(stored))
	require.Equal(t, "Vendor", ratio_setting.GetGroupTypesCopy()["vendor"].Name)
	require.Equal(t, "vendor", ratio_setting.GetGroupTypeMapCopy()["default"])
}
