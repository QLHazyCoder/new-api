package model

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/require"
)

func setupSensitiveWordTest(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(
		&User{},
		&UserSession{},
		&Option{},
		&Log{},
		&SensitiveWordRule{},
		&SensitiveWordRuleWord{},
		&SensitiveWordRuleGroup{},
		&SensitiveWordWhitelist{},
		&SensitiveWordAuditEvent{},
	))

	oldWords := append([]string(nil), setting.SensitiveWords...)
	oldCheckEnabled := setting.CheckSensitiveEnabled
	oldCheckPromptEnabled := setting.CheckSensitiveOnPromptEnabled
	oldRedisEnabled := common.RedisEnabled
	setting.SensitiveWords = nil
	setting.CheckSensitiveEnabled = true
	setting.CheckSensitiveOnPromptEnabled = true
	common.RedisEnabled = false

	require.NoError(t, DB.Exec("DELETE FROM sensitive_word_audit_events").Error)
	require.NoError(t, DB.Exec("DELETE FROM sensitive_word_rule_words").Error)
	require.NoError(t, DB.Exec("DELETE FROM sensitive_word_rule_groups").Error)
	require.NoError(t, DB.Exec("DELETE FROM sensitive_word_whitelists").Error)
	require.NoError(t, DB.Exec("DELETE FROM sensitive_word_rules").Error)
	require.NoError(t, DB.Where(&Option{Key: "SensitiveWordConfig"}).Delete(&Option{}).Error)
	require.NoError(t, DB.Where(&Option{Key: sensitiveWordMigrationKey}).Delete(&Option{}).Error)
	require.NoError(t, LOG_DB.Where("type = ?", LogTypeSensitiveWordBlock).Delete(&Log{}).Error)
	invalidateSensitiveWordRuntime()

	t.Cleanup(func() {
		_ = DB.Exec("DELETE FROM sensitive_word_audit_events").Error
		_ = DB.Exec("DELETE FROM sensitive_word_rule_words").Error
		_ = DB.Exec("DELETE FROM sensitive_word_rule_groups").Error
		_ = DB.Exec("DELETE FROM sensitive_word_whitelists").Error
		_ = DB.Exec("DELETE FROM sensitive_word_rules").Error
		_ = DB.Where(&Option{Key: "SensitiveWordConfig"}).Delete(&Option{}).Error
		_ = DB.Where(&Option{Key: sensitiveWordMigrationKey}).Delete(&Option{}).Error
		_ = LOG_DB.Where("type = ?", LogTypeSensitiveWordBlock).Delete(&Log{}).Error
		setting.SensitiveWords = oldWords
		setting.CheckSensitiveEnabled = oldCheckEnabled
		setting.CheckSensitiveOnPromptEnabled = oldCheckPromptEnabled
		common.RedisEnabled = oldRedisEnabled
		invalidateSensitiveWordRuntime()
	})
}

func saveSensitiveWordTestConfig(t *testing.T, mode string, auditEnabled bool, threshold int) {
	t.Helper()
	require.NoError(t, SaveSensitiveWordConfig(SensitiveWordConfig{
		Enabled:                 true,
		CheckPrompt:             true,
		Mode:                    mode,
		AuditEnabled:            auditEnabled,
		BlockMessage:            sensitiveWordBlockMessage,
		BanThreshold:            threshold,
		FullPromptRetentionDays: 180,
		MaxPromptRunes:          SensitiveWordMaxPromptRunes,
	}))
}

func createSensitiveWordTestUser(t *testing.T, quota int, whitelisted bool) *User {
	t.Helper()
	id := time.Now().UnixNano()
	user := &User{
		Username:               fmt.Sprintf("sensitive-test-%d", id),
		Password:               "unused-password",
		Status:                 common.UserStatusEnabled,
		Role:                   common.RoleCommonUser,
		Group:                  "default",
		Quota:                  quota,
		AffCode:                fmt.Sprintf("sw-%d", id),
		AuthVersion:            1,
		SensitiveWordWhitelist: whitelisted,
	}
	require.NoError(t, DB.Create(user).Error)
	t.Cleanup(func() {
		_ = DB.Where("user_id = ?", user.Id).Delete(&UserSession{}).Error
		_ = DB.Delete(&User{}, user.Id).Error
	})
	return user
}

func addSensitiveWordTestRule(t *testing.T, name string, words []string, scope string, groups []string) *SensitiveWordRuleDetail {
	t.Helper()
	detail, err := UpsertSensitiveWordRule(0, name, words, scope, groups, 1, nil)
	require.NoError(t, err)
	return detail
}

func sensitiveWordTestInput(user *User, requestID, prompt string) SensitiveCheckInput {
	return SensitiveCheckInput{
		RequestID: requestID,
		UserID:    user.Id,
		Username:  user.Username,
		TokenID:   99,
		TokenName: "sensitive-test-key",
		GroupName: "default",
		ModelName: "gpt-test",
		Endpoint:  "/v1/chat/completions",
		Protocol:  "openai",
		Prompt:    prompt,
	}
}

func getSensitiveWordTestAudit(t *testing.T, requestID string) SensitiveWordAuditEvent {
	t.Helper()
	var event SensitiveWordAuditEvent
	require.NoError(t, DB.Where("request_id = ?", requestID).First(&event).Error)
	return event
}

func getSensitiveWordTestLog(t *testing.T, requestID string) Log {
	t.Helper()
	var log Log
	require.NoError(t, LOG_DB.Where("request_id = ? AND type = ?", requestID, LogTypeSensitiveWordBlock).First(&log).Error)
	return log
}

func TestNormalizeSensitiveWord(t *testing.T) {
	if _, err := normalizeSensitiveWord("  禁词  "); err != nil {
		t.Fatal(err)
	}
	if _, err := normalizeSensitiveWord(""); err == nil {
		t.Fatal("expected empty word error")
	}
}

func TestTruncateSensitivePrompt(t *testing.T) {
	if got := truncateSensitivePrompt("abcdef", 3); got != "abc…" {
		t.Fatalf("got %q", got)
	}
	if got := truncateSensitivePrompt("a\x00b", 10); got != "ab" {
		t.Fatalf("got %q", got)
	}
}

func TestMigrateSensitiveWordDataImportsPersistedLegacyOptionOnce(t *testing.T) {
	setupSensitiveWordTest(t)
	var previous Option
	previousExists := DB.Where(&Option{Key: "SensitiveWords"}).First(&previous).Error == nil
	t.Cleanup(func() {
		if previousExists {
			_ = DB.Save(&previous).Error
			return
		}
		_ = DB.Where(&Option{Key: "SensitiveWords"}).Delete(&Option{}).Error
	})

	setting.SensitiveWords = []string{"memory-only-word"}
	require.NoError(t, DB.Save(&Option{Key: "SensitiveWords", Value: "legacy-one\nlegacy-two"}).Error)
	require.NoError(t, MigrateSensitiveWordData())

	rules, err := ListSensitiveWordRules()
	require.NoError(t, err)
	require.Len(t, rules, 1)
	require.Equal(t, 2, rules[0].WordCount)
	detail, err := GetSensitiveWordRuleDetail(rules[0].ID)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"legacy-one", "legacy-two"}, detail.Words)
	require.True(t, isSensitiveWordMigrationComplete())

	require.NoError(t, DeleteSensitiveWordRule(detail.ID))
	user := createSensitiveWordTestUser(t, 6800000, false)
	result, err := CheckSensitiveRequest(sensitiveWordTestInput(user, "legacy-after-delete", "legacy-one"))
	require.NoError(t, err)
	require.False(t, result.Matched, "新规则删除后不得回退到旧 SensitiveWords 配置")
}

func TestMigrateSensitiveWordDataKeepsInvalidLegacyOptionActive(t *testing.T) {
	setupSensitiveWordTest(t)
	var previous Option
	previousExists := DB.Where(&Option{Key: "SensitiveWords"}).First(&previous).Error == nil
	t.Cleanup(func() {
		if previousExists {
			_ = DB.Save(&previous).Error
			return
		}
		_ = DB.Where(&Option{Key: "SensitiveWords"}).Delete(&Option{}).Error
	})
	legacyWord := strings.Repeat("超长旧词", 80)
	require.Greater(t, len([]rune(legacyWord)), SensitiveWordMaxRunes)
	require.NoError(t, DB.Save(&Option{Key: "SensitiveWords", Value: legacyWord}).Error)

	require.NoError(t, MigrateSensitiveWordData())
	var markerCount int64
	require.NoError(t, DB.Model(&Option{}).Where(&Option{Key: sensitiveWordMigrationKey}).Count(&markerCount).Error)
	require.Zero(t, markerCount, "未能安全导入的旧词不得标记为迁移完成")

	user := createSensitiveWordTestUser(t, 42000000, false)
	result, err := CheckSensitiveRequest(sensitiveWordTestInput(user, "legacy-invalid-word", "前缀"+legacyWord+"后缀"))
	require.NoError(t, err)
	require.True(t, result.Matched, "不符合新编辑器限制的旧词仍必须保持拦截")
	require.True(t, result.Blocked)
}

func TestSaveSensitiveWordConfigInvalidatesLocalSnapshot(t *testing.T) {
	setupSensitiveWordTest(t)
	initial := GetSensitiveWordConfig()
	require.Equal(t, "block", initial.Mode)
	require.NoError(t, SaveSensitiveWordConfig(SensitiveWordConfig{
		Enabled:                 true,
		CheckPrompt:             true,
		Mode:                    "observe",
		AuditEnabled:            false,
		BlockMessage:            sensitiveWordBlockMessage,
		BanThreshold:            7,
		FullPromptRetentionDays: 30,
		MaxPromptRunes:          4096,
	}))
	updated := GetSensitiveWordConfig()
	require.Equal(t, "observe", updated.Mode)
	require.False(t, updated.AuditEnabled)
	require.Equal(t, 7, updated.BanThreshold)
	require.Equal(t, 4096, updated.MaxPromptRunes)
	// Exercise the cached read path as well as the post-save reload above.
	require.Equal(t, updated, GetSensitiveWordConfig())
}

func TestSensitiveWordRulesApplyGlobalAndCandidateGroupOnce(t *testing.T) {
	setupSensitiveWordTest(t)
	saveSensitiveWordTestConfig(t, "block", true, SensitiveWordBanThreshold)
	globalRule := addSensitiveWordTestRule(t, "全局规则", []string{"全局禁词"}, SensitiveWordScopeGlobal, nil)
	groupRule := addSensitiveWordTestRule(t, "VIP 规则", []string{"局部禁词"}, SensitiveWordScopeGroup, []string{"vip"})
	user := createSensitiveWordTestUser(t, 73100000, false)

	result, err := CheckSensitiveRequestForGroups(
		sensitiveWordTestInput(user, "global-and-local", "全局禁词和局部禁词同时出现"),
		[]string{"default", "vip"},
	)
	require.NoError(t, err)
	require.True(t, result.Matched)
	require.True(t, result.Blocked)
	require.Equal(t, 1, result.ViolationCount, "同一请求命中多个词只能计数一次")
	require.ElementsMatch(t, []int64{globalRule.ID, groupRule.ID}, result.MatchedRuleIDs)
	require.Equal(t, "vip", getSensitiveWordTestAudit(t, "global-and-local").GroupName, "自动分组候选应记录实际命中的局部分组")

	result, err = CheckSensitiveRequestForGroups(
		sensitiveWordTestInput(user, "local-wrong-group", "局部禁词"),
		[]string{"default"},
	)
	require.NoError(t, err)
	require.False(t, result.Matched, "局部规则不得作用于未绑定分组")

	result, err = CheckSensitiveRequestForGroups(
		sensitiveWordTestInput(user, "global-only", "全局禁词"),
		[]string{"default"},
	)
	require.NoError(t, err)
	require.True(t, result.Blocked)
	require.Equal(t, 2, result.ViolationCount)

	groups := ListSensitiveWordGroups()
	require.NotContains(t, groups, "auto")
	_, err = validSensitiveGroups([]string{"auto"})
	require.Error(t, err)
	_, err = validSensitiveGroups([]string{"not-priced"})
	require.Error(t, err)
}

func TestSensitiveWordWhitelistAndObserveModeRecordWithoutCounting(t *testing.T) {
	setupSensitiveWordTest(t)
	addSensitiveWordTestRule(t, "审计规则", []string{"命中词"}, SensitiveWordScopeGlobal, nil)
	whitelisted := createSensitiveWordTestUser(t, 19000000, true)
	saveSensitiveWordTestConfig(t, "block", true, SensitiveWordBanThreshold)

	result, err := CheckSensitiveRequest(sensitiveWordTestInput(whitelisted, "whitelist-request", "命中词需要审计"))
	require.NoError(t, err)
	require.True(t, result.Matched)
	require.True(t, result.WhitelistBypassed)
	require.False(t, result.Blocked)
	require.Zero(t, result.ViolationCount)
	event := getSensitiveWordTestAudit(t, "whitelist-request")
	require.True(t, event.WhitelistBypassed)
	require.False(t, event.Blocked)
	require.NotEmpty(t, event.FullPrompt)
	whitelistLog := getSensitiveWordTestLog(t, "whitelist-request")
	require.Contains(t, whitelistLog.Other, "whitelist_bypass")
	require.Contains(t, whitelistLog.Other, fmt.Sprintf("\"audit_id\":%d", event.ID))

	observed := createSensitiveWordTestUser(t, 23000000, false)
	saveSensitiveWordTestConfig(t, "observe", false, SensitiveWordBanThreshold)
	result, err = CheckSensitiveRequest(sensitiveWordTestInput(observed, "observe-request", "命中词只观察"))
	require.NoError(t, err)
	require.True(t, result.Matched)
	require.True(t, result.ObserveOnly)
	require.False(t, result.Blocked)
	require.Zero(t, result.ViolationCount)
	event = getSensitiveWordTestAudit(t, "observe-request")
	require.True(t, event.ObserveOnly)
	require.Empty(t, event.RedactedPreview)
	require.Empty(t, event.FullPrompt)
	require.Equal(t, "[]", event.MatchedSnippets, "关闭审计证据时不得保留匹配上下文")
	require.Contains(t, getSensitiveWordTestLog(t, "observe-request").Other, "\"action\":\"observe\"")
}

func TestSensitiveWordUserWhitelistFieldOverridesLegacyCompatibilityRow(t *testing.T) {
	setupSensitiveWordTest(t)
	saveSensitiveWordTestConfig(t, "block", true, SensitiveWordBanThreshold)
	addSensitiveWordTestRule(t, "白名单一致性规则", []string{"一致性命中词"}, SensitiveWordScopeGlobal, nil)
	user := createSensitiveWordTestUser(t, 12000000, false)
	require.NoError(t, DB.Create(&SensitiveWordWhitelist{UserID: user.Id, Enabled: true, CreatedBy: 1}).Error)

	result, err := CheckSensitiveRequest(sensitiveWordTestInput(user, "legacy-row-disabled", "一致性命中词"))
	require.NoError(t, err)
	require.True(t, result.Blocked, "用户抽屉关闭白名单后，旧兼容记录不得继续放行")
	require.False(t, result.WhitelistBypassed)
}

func TestSensitiveWordFifthViolationBansWithoutChangingBalance(t *testing.T) {
	setupSensitiveWordTest(t)
	saveSensitiveWordTestConfig(t, "block", true, 5)
	addSensitiveWordTestRule(t, "封禁规则", []string{"违规词", "第二个违规词"}, SensitiveWordScopeGlobal, nil)
	user := createSensitiveWordTestUser(t, 88800000, false)
	now := time.Now().Unix()
	session := UserSession{
		SID:             fmt.Sprintf("sensitive-session-%d", user.Id),
		UserID:          user.Id,
		Version:         1,
		UserAuthVersion: 1,
		Status:          UserSessionStatusActive,
		RefreshHash:     "sensitive-test-refresh-hash",
		LoginMethod:     "password",
		IP:              "127.0.0.1",
		UserAgent:       "sensitive-test",
		CreatedAt:       now,
		LastActiveAt:    now,
		ExpiresAt:       now + 3600,
	}
	require.NoError(t, DB.Create(&session).Error)

	for count := 1; count <= 6; count++ {
		requestID := fmt.Sprintf("violation-%d", count)
		result, err := CheckSensitiveRequest(sensitiveWordTestInput(user, requestID, "违规词与第二个违规词"))
		require.NoError(t, err)
		require.True(t, result.Blocked)
		require.Equal(t, count, result.ViolationCount)
		if count < 5 {
			require.False(t, result.AutoBanned)
		}
		if count == 5 {
			require.True(t, result.AutoBanned)
		}
		if count == 6 {
			require.False(t, result.AutoBanned, "已经封禁的用户不得重复执行封禁")
		}
	}

	var persisted User
	require.NoError(t, DB.First(&persisted, user.Id).Error)
	require.Equal(t, common.UserStatusDisabled, persisted.Status)
	require.Equal(t, 6, persisted.SensitiveWordViolationCount)
	require.Equal(t, 88800000, persisted.Quota, "敏感词封禁绝不能清理用户余额或内部额度")
	require.Equal(t, int64(2), persisted.AuthVersion)

	var persistedSession UserSession
	require.NoError(t, DB.First(&persistedSession, "sid = ?", session.SID).Error)
	require.Equal(t, UserSessionStatusRevoked, persistedSession.Status)
	require.Equal(t, "sensitive_word_threshold", persistedSession.RevokedReason)

	var eventCount, logCount int64
	require.NoError(t, DB.Model(&SensitiveWordAuditEvent{}).Where("user_id = ?", user.Id).Count(&eventCount).Error)
	require.NoError(t, LOG_DB.Model(&Log{}).Where("user_id = ? AND type = ?", user.Id, LogTypeSensitiveWordBlock).Count(&logCount).Error)
	require.Equal(t, int64(6), eventCount)
	require.Equal(t, int64(6), logCount)
	fifthEvent := getSensitiveWordTestAudit(t, "violation-5")
	require.True(t, fifthEvent.AutoBanned)
	require.Equal(t, 88800000, fifthEvent.QuotaBefore)
	require.Equal(t, 88800000, fifthEvent.QuotaAfter)
	fifthLog := getSensitiveWordTestLog(t, "violation-5")
	require.Contains(t, fifthLog.Other, fmt.Sprintf("\"audit_id\":%d", fifthEvent.ID))
	require.Contains(t, fifthLog.Other, "\"balance_changed\":false")
}

func TestSensitiveWordRuntimeRefreshAndUserLogRedaction(t *testing.T) {
	setupSensitiveWordTest(t)
	saveSensitiveWordTestConfig(t, "block", true, SensitiveWordBanThreshold)
	user := createSensitiveWordTestUser(t, 3000000, false)

	result, err := CheckSensitiveRequest(sensitiveWordTestInput(user, "before-rule", "运行时新词"))
	require.NoError(t, err)
	require.False(t, result.Matched)
	addSensitiveWordTestRule(t, "热更新规则", []string{"运行时新词"}, SensitiveWordScopeGlobal, nil)
	result, err = CheckSensitiveRequest(sensitiveWordTestInput(user, "after-rule", "运行时新词"))
	require.NoError(t, err)
	require.True(t, result.Blocked, "规则保存后应立即刷新匹配器，无需重启服务")

	log := getSensitiveWordTestLog(t, "after-rule")
	logs := []*Log{&log}
	formatUserLogs(logs, 0)
	other, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	require.NotContains(t, other, "audit_id")
	filter, ok := other["keyword_filter"].(map[string]interface{})
	require.True(t, ok)
	require.NotContains(t, filter, "matched_words")
	require.NotContains(t, filter, "rule_names")
	require.NotContains(t, filter, "prompt_hash")
	require.Equal(t, false, filter["balance_changed"])
}
