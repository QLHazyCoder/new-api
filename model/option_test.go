package model

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
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
