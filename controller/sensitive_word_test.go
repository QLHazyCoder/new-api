package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSensitiveWordAuditListRedactsFullPrompt(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SensitiveWordAuditEvent{}))
	event := model.SensitiveWordAuditEvent{
		RequestID:        "sensitive-audit-list-test",
		UserID:           1,
		UsernameSnapshot: "audit-user",
		GroupName:        "default",
		ModelName:        "gpt-test",
		PromptHash:       "test-hash",
		RedactedPreview:  "脱敏摘要",
		FullPrompt:       "管理员详情才能读取的完整提示词",
		MatchedWords:     `["测试词"]`,
		MatchedScope:     "global",
		Blocked:          true,
		CreatedAt:        time.Now(),
	}
	require.NoError(t, db.Create(&event).Error)

	gin.SetMode(gin.TestMode)
	listRecorder := httptest.NewRecorder()
	listContext, _ := gin.CreateTestContext(listRecorder)
	listContext.Request = httptest.NewRequest(http.MethodGet, "/api/sensitive-words/audits", nil)
	GetSensitiveWordAudits(listContext)

	require.Equal(t, http.StatusOK, listRecorder.Code, listRecorder.Body.String())
	require.NotContains(t, listRecorder.Body.String(), event.FullPrompt)
	var listResponse struct {
		Success bool `json:"success"`
		Data    struct {
			Items []model.SensitiveWordAuditEvent `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(listRecorder.Body.Bytes(), &listResponse))
	require.True(t, listResponse.Success)
	require.Len(t, listResponse.Data.Items, 1)
	require.Empty(t, listResponse.Data.Items[0].FullPrompt)

	// The router protects this detail handler with AdminAuth. Its payload must
	// retain the evidence for the authorized administrator review flow.
	detailRecorder := httptest.NewRecorder()
	detailContext, _ := gin.CreateTestContext(detailRecorder)
	detailContext.Request = httptest.NewRequest(http.MethodGet, "/api/sensitive-words/audits/"+strconv.FormatInt(event.ID, 10), nil)
	detailContext.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(event.ID, 10)}}
	GetSensitiveWordAudit(detailContext)

	require.Equal(t, http.StatusOK, detailRecorder.Code, detailRecorder.Body.String())
	require.Contains(t, detailRecorder.Body.String(), event.FullPrompt)
}

func TestRelaySensitiveWordBlockPrecedesBillingAndRedactsEndpointQuery(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.Option{},
		&model.SensitiveWordRule{},
		&model.SensitiveWordRuleWord{},
		&model.SensitiveWordRuleGroup{},
		&model.SensitiveWordAuditEvent{},
	))

	user := model.User{
		Username:    "relay-sensitive-user",
		Password:    "password",
		DisplayName: "relay-sensitive-user",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		Quota:       4567890,
		AffCode:     "relay-sensitive-aff",
		AuthVersion: 1,
	}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, model.SaveSensitiveWordConfig(model.SensitiveWordConfig{
		Enabled:                 true,
		CheckPrompt:             true,
		Mode:                    "block",
		AuditEnabled:            true,
		BanThreshold:            5,
		FullPromptRetentionDays: 180,
		MaxPromptRunes:          model.SensitiveWordMaxPromptRunes,
	}))
	_, err := model.UpsertSensitiveWordRule(
		0,
		"Relay 拦截测试",
		[]string{"relay-sensitive-marker"},
		model.SensitiveWordScopeGlobal,
		nil,
		1,
		nil,
	)
	require.NoError(t, err)
	// Invalidate the process-local snapshot before the temporary test database
	// is released by setupManageUserTestDB.
	t.Cleanup(func() {
		_ = model.SaveSensitiveWordConfig(model.SensitiveWordConfig{
			Enabled:                 false,
			CheckPrompt:             true,
			Mode:                    "off",
			AuditEnabled:            false,
			BanThreshold:            5,
			FullPromptRetentionDays: 180,
			MaxPromptRunes:          model.SensitiveWordMaxPromptRunes,
		})
	})

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/chat/completions?caller_secret=must-not-be-audited",
		strings.NewReader(`{"model":"relay-test-model","messages":[{"role":"user","content":"relay-sensitive-marker"}]}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set(common.RequestIdKey, "relay-sensitive-request")
	ctx.Set("username", user.Username)
	ctx.Set("token_name", "relay-sensitive-token")
	common.SetContextKey(ctx, constant.ContextKeyUserId, user.Id)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 101)
	common.SetContextKey(ctx, constant.ContextKeyOriginalModel, "relay-test-model")
	common.SetContextKey(ctx, constant.ContextKeyUserQuota, user.Quota)

	Relay(ctx, types.RelayFormatOpenAI)

	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "sensitive_words_detected")
	require.Contains(t, recorder.Body.String(), "攻击破解别人网站")

	var event model.SensitiveWordAuditEvent
	require.NoError(t, db.Where("request_id = ?", "relay-sensitive-request").First(&event).Error)
	require.Equal(t, "/v1/chat/completions", event.Endpoint)
	require.NotContains(t, event.Endpoint, "caller_secret")
	require.Contains(t, event.FullPrompt, "relay-sensitive-marker")

	var stored model.User
	require.NoError(t, db.First(&stored, user.Id).Error)
	require.Equal(t, 4567890, stored.Quota)
	require.Equal(t, 1, stored.SensitiveWordViolationCount)
	var consumeCount int64
	require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeConsume).Count(&consumeCount).Error)
	require.Zero(t, consumeCount, "敏感词拦截必须发生在预扣费和正常消费日志之前")
}
