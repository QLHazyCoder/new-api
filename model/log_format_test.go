package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/require"
)

// TestFormatUserLogsStripsQuotaSaturation verifies the admin-only quota
// saturation marker (nested under other.admin_info) is removed for non-admin
// log views, since formatUserLogs strips the whole admin_info object.
func TestFormatUserLogsStripsQuotaSaturation(t *testing.T) {
	other := common.MapToJsonStr(map[string]interface{}{
		"model_price": 0.004,
		"admin_info": map[string]interface{}{
			"quota_saturation": map[string]interface{}{
				"op":      "QuotaFromDecimal",
				"kind":    "overflow",
				"clamped": common.MaxQuota,
			},
		},
	})
	logs := []*Log{{Other: other}}

	formatUserLogs(logs, 0)

	parsed, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	_, hasAdminInfo := parsed["admin_info"]
	require.False(t, hasAdminInfo, "admin_info (and nested quota_saturation) must be stripped for non-admin views")
	// Non-admin billing fields remain visible.
	require.Contains(t, parsed, "model_price")
}

func resetLogsForVisibilityTest(t *testing.T) {
	t.Helper()
	require.NoError(t, LOG_DB.Exec("DELETE FROM logs").Error)
	t.Cleanup(func() {
		require.NoError(t, LOG_DB.Exec("DELETE FROM logs").Error)
	})
}

func TestUserLogViewsHideErrorLogs(t *testing.T) {
	resetLogsForVisibilityTest(t)

	now := time.Now().Unix()
	records := []Log{
		{
			UserId:           1001,
			CreatedAt:        now - 2,
			Type:             LogTypeConsume,
			Username:         "visible-user",
			TokenName:        "visible-token",
			ModelName:        "gpt-visible",
			TokenId:          7001,
			RequestId:        "consume-request",
			PromptTokens:     10,
			CompletionTokens: 5,
			Group:            "default",
		},
		{
			UserId:    1001,
			CreatedAt: now - 1,
			Type:      LogTypeError,
			Username:  "visible-user",
			TokenName: "visible-token",
			ModelName: "gpt-visible",
			TokenId:   7001,
			RequestId: "error-request",
			Content:   "upstream failed",
			Group:     "default",
			Other: common.MapToJsonStr(map[string]interface{}{
				"admin_info": map[string]interface{}{
					"use_channel": []int{61},
				},
			}),
		},
	}
	require.NoError(t, LOG_DB.Create(&records).Error)

	userLogs, total, err := GetUserLogs(1001, LogTypeUnknown, 0, 0, "", "", 0, 20, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, userLogs, 1)
	require.Equal(t, LogTypeConsume, userLogs[0].Type)
	require.Equal(t, "consume-request", userLogs[0].RequestId)

	errorLogs, total, err := GetUserLogs(1001, LogTypeError, 0, 0, "", "", 0, 20, "", "", "")
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, errorLogs)

	tokenLogs, err := GetLogByTokenId(7001)
	require.NoError(t, err)
	require.Len(t, tokenLogs, 1)
	require.Equal(t, LogTypeConsume, tokenLogs[0].Type)

	adminLogs, total, err := GetAllLogs(LogTypeError, 0, 0, "", "", "", 0, 20, 0, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, adminLogs, 1)
	require.Equal(t, "error-request", adminLogs[0].RequestId)
	require.Contains(t, adminLogs[0].Other, "admin_info")
}
