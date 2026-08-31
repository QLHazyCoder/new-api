package model

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
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

func TestSensitiveWordAuditPromptUsesDialectSizedTypes(t *testing.T) {
	require.Equal(t, "mediumtext", sensitiveWordAuditPromptDBType(string(common.DatabaseTypeMySQL)))
	require.Equal(t, "text", sensitiveWordAuditPromptDBType(string(common.DatabaseTypePostgreSQL)))
	require.Equal(t, "text", sensitiveWordAuditPromptDBType(string(common.DatabaseTypeSQLite)))
}

func TestSensitiveWordAuditPromptGormTypeForSupportedDialects(t *testing.T) {
	cases := []struct {
		name string
		open func() (*gorm.DB, error)
		want string
	}{
		{
			name: "mysql",
			open: func() (*gorm.DB, error) {
				return gorm.Open(mysql.New(mysql.Config{
					DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8mb4&parseTime=True&loc=Local",
					SkipInitializeWithVersion: true,
				}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
			},
			want: "mediumtext",
		},
		{
			name: "postgres",
			open: func() (*gorm.DB, error) {
				return gorm.Open(postgres.New(postgres.Config{
					DSN:                  "host=localhost user=postgres dbname=postgres sslmode=disable",
					PreferSimpleProtocol: true,
				}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
			},
			want: "text",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, err := tc.open()
			require.NoError(t, err)
			statement := &gorm.Statement{DB: db}
			require.NoError(t, statement.Parse(&SensitiveWordAuditEvent{}))
			field := statement.Schema.LookUpField("FullPrompt")
			require.NotNil(t, field)
			require.Equal(t, tc.want, strings.ToLower(db.Migrator().FullDataTypeOf(field).SQL))
		})
	}
}

func TestTruncateSensitivePromptIsByteSafeForLargeUnicodeInput(t *testing.T) {
	value := truncateSensitivePrompt(strings.Repeat("界", 200000), 200000)
	require.True(t, utf8.ValidString(value))
	require.LessOrEqual(t, len([]byte(value)), SensitiveWordMaxPromptBytes)
	require.True(t, strings.HasSuffix(value, "…"))
}

func TestSensitiveWordObserveAuditPersistenceErrorIsTyped(t *testing.T) {
	setupSensitiveWordTest(t)
	saveSensitiveWordTestConfig(t, "observe", true, SensitiveWordBanThreshold)
	user := createSensitiveWordTestUser(t, 4200000, false)
	addSensitiveWordTestRule(t, "审计故障规则", []string{"审计故障命中词"}, SensitiveWordScopeGlobal, nil)

	triggerName := fmt.Sprintf("sensitive_audit_failure_%d", time.Now().UnixNano())
	triggerSQL := fmt.Sprintf(
		"CREATE TRIGGER %s BEFORE INSERT ON sensitive_word_audit_events BEGIN SELECT RAISE(ABORT, 'forced audit failure'); END",
		triggerName,
	)
	require.NoError(t, DB.Exec(triggerSQL).Error)
	t.Cleanup(func() { _ = DB.Exec("DROP TRIGGER " + triggerName).Error })

	result, err := CheckSensitiveRequest(sensitiveWordTestInput(user, "observe-audit-failure", "审计故障命中词"))
	require.ErrorIs(t, err, ErrSensitiveWordAuditPersistence)
	require.NotNil(t, result)
	require.True(t, result.Matched)
	require.True(t, result.ObserveOnly)
	require.False(t, result.Blocked)
	require.Zero(t, result.ViolationCount)

	var stored User
	require.NoError(t, DB.First(&stored, user.Id).Error)
	require.Zero(t, stored.SensitiveWordViolationCount)
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

func TestSensitiveWordEnableResetsCounterIdempotentlyAndPreservesEvidence(t *testing.T) {
	setupSensitiveWordTest(t)
	saveSensitiveWordTestConfig(t, "block", true, SensitiveWordBanThreshold)
	addSensitiveWordTestRule(t, "启用重置规则", []string{"启用重置命中词"}, SensitiveWordScopeGlobal, nil)
	user := createSensitiveWordTestUser(t, 91827364, false)
	user.UsedQuota = 123456
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("used_quota", user.UsedQuota).Error)

	for count := 1; count <= SensitiveWordBanThreshold; count++ {
		result, err := CheckSensitiveRequest(sensitiveWordTestInput(user, fmt.Sprintf("enable-reset-hit-%d", count), "启用重置命中词"))
		require.NoError(t, err)
		require.True(t, result.Blocked)
	}

	var before User
	require.NoError(t, DB.First(&before, user.Id).Error)
	require.Equal(t, common.UserStatusDisabled, before.Status)
	require.Equal(t, SensitiveWordBanThreshold, before.SensitiveWordViolationCount)
	require.Equal(t, 91827364, before.Quota)
	require.Equal(t, 123456, before.UsedQuota)
	var evidenceBefore int64
	require.NoError(t, DB.Model(&SensitiveWordAuditEvent{}).Where("user_id = ?", user.Id).Count(&evidenceBefore).Error)

	// A session created while the account is disabled still has to be revoked
	// when an administrator enables the account and advances auth_version.
	now := time.Now().Unix()
	staleSession := UserSession{
		SID: fmt.Sprintf("enable-reset-stale-%d", user.Id), UserID: user.Id, Version: 1,
		UserAuthVersion: before.AuthVersion, Status: UserSessionStatusActive,
		RefreshHash: "enable-reset-stale-refresh", LoginMethod: "password",
		CreatedAt: now, LastActiveAt: now, ExpiresAt: now + 3600,
	}
	require.NoError(t, DB.Create(&staleSession).Error)

	reset, err := EnableUserAndResetSensitiveWordViolations(user.Id)
	require.NoError(t, err)
	require.Equal(t, common.UserStatusDisabled, reset.StatusBefore)
	require.Equal(t, common.UserStatusEnabled, reset.StatusAfter)
	require.Equal(t, SensitiveWordBanThreshold, reset.ViolationCountBefore)
	require.Zero(t, reset.ViolationCountAfter)
	require.True(t, reset.StatusChanged)
	require.True(t, reset.ViolationCountReset)
	require.Equal(t, before.AuthVersion+1, reset.AuthVersionAfter)
	require.Equal(t, before.Quota, reset.QuotaAfter)
	require.Equal(t, before.UsedQuota, reset.UsedQuotaAfter)
	require.Equal(t, int64(1), reset.SessionsRevoked)

	var enabled User
	require.NoError(t, DB.First(&enabled, user.Id).Error)
	require.Equal(t, common.UserStatusEnabled, enabled.Status)
	require.Zero(t, enabled.SensitiveWordViolationCount)
	require.Equal(t, before.Quota, enabled.Quota)
	require.Equal(t, before.UsedQuota, enabled.UsedQuota)
	var staleAfter UserSession
	require.NoError(t, DB.First(&staleAfter, "sid = ?", staleSession.SID).Error)
	require.Equal(t, UserSessionStatusRevoked, staleAfter.Status)

	var evidenceAfter int64
	require.NoError(t, DB.Model(&SensitiveWordAuditEvent{}).Where("user_id = ?", user.Id).Count(&evidenceAfter).Error)
	require.Equal(t, evidenceBefore, evidenceAfter, "启用清零不得删除历史审计证据")
	historyEvent := getSensitiveWordTestAudit(t, "enable-reset-hit-1")
	require.Contains(t, historyEvent.FullPrompt, "启用重置命中词")

	// Repeated enable is idempotent: it must not advance auth_version or
	// revoke a session that was created after the first successful enable.
	var activeSession UserSession
	activeSession = UserSession{
		SID: fmt.Sprintf("enable-reset-active-%d", user.Id), UserID: user.Id, Version: 1,
		UserAuthVersion: enabled.AuthVersion, Status: UserSessionStatusActive,
		RefreshHash: "enable-reset-active-refresh", LoginMethod: "password",
		CreatedAt: now, LastActiveAt: now, ExpiresAt: now + 3600,
	}
	require.NoError(t, DB.Create(&activeSession).Error)
	second, err := EnableUserAndResetSensitiveWordViolations(user.Id)
	require.NoError(t, err)
	require.False(t, second.StatusChanged)
	require.False(t, second.ViolationCountReset)
	require.Equal(t, enabled.AuthVersion, second.AuthVersionAfter)
	require.Zero(t, second.SessionsRevoked)
	var activeAfter UserSession
	require.NoError(t, DB.First(&activeAfter, "sid = ?", activeSession.SID).Error)
	require.Equal(t, UserSessionStatusActive, activeAfter.Status)

	// The next effective hit starts a fresh sequence at one instead of
	// immediately re-banning the account.
	next, err := CheckSensitiveRequest(sensitiveWordTestInput(user, "enable-reset-next-hit", "启用重置命中词"))
	require.NoError(t, err)
	require.True(t, next.Blocked)
	require.Equal(t, 1, next.ViolationCount)
	require.False(t, next.AutoBanned)
	require.NoError(t, DB.First(&enabled, user.Id).Error)
	require.Equal(t, common.UserStatusEnabled, enabled.Status)
	require.Equal(t, 1, enabled.SensitiveWordViolationCount)
	require.Equal(t, before.Quota, enabled.Quota)
	require.Equal(t, before.UsedQuota, enabled.UsedQuota)
}

func TestSensitiveWordEnableResetsManuallyDisabledUserAndPreservesWhitelist(t *testing.T) {
	setupSensitiveWordTest(t)
	user := createSensitiveWordTestUser(t, 456789, true)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
		"status":                         common.UserStatusDisabled,
		"sensitive_word_violation_count": 2,
	}).Error)

	result, err := EnableUserAndResetSensitiveWordViolations(user.Id)
	require.NoError(t, err)
	require.True(t, result.StatusChanged)
	require.True(t, result.ViolationCountReset)
	var updated User
	require.NoError(t, DB.First(&updated, user.Id).Error)
	require.Equal(t, common.UserStatusEnabled, updated.Status)
	require.Zero(t, updated.SensitiveWordViolationCount)
	require.True(t, updated.SensitiveWordWhitelist, "启用操作不得改变白名单状态")
	require.Equal(t, 456789, updated.Quota)
}

func TestSensitiveWordEnableOnlyClearsAlreadyEnabledUserWithoutRevokingSession(t *testing.T) {
	setupSensitiveWordTest(t)
	user := createSensitiveWordTestUser(t, 13579, false)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("sensitive_word_violation_count", 3).Error)
	now := time.Now().Unix()
	require.NoError(t, DB.Create(&UserSession{
		SID: "enabled-reset-session", UserID: user.Id, Version: 1, UserAuthVersion: user.AuthVersion,
		Status: UserSessionStatusActive, RefreshHash: "enabled-reset-refresh", LoginMethod: "password",
		CreatedAt: now, LastActiveAt: now, ExpiresAt: now + 3600,
	}).Error)

	result, err := EnableUserAndResetSensitiveWordViolations(user.Id)
	require.NoError(t, err)
	require.False(t, result.StatusChanged)
	require.True(t, result.ViolationCountReset)
	require.Equal(t, int64(1), result.AuthVersionAfter)
	require.Zero(t, result.SessionsRevoked)

	var updated User
	require.NoError(t, DB.First(&updated, user.Id).Error)
	require.Equal(t, common.UserStatusEnabled, updated.Status)
	require.Zero(t, updated.SensitiveWordViolationCount)
	require.EqualValues(t, 1, updated.AuthVersion)
	var session UserSession
	require.NoError(t, DB.First(&session, "sid = ?", "enabled-reset-session").Error)
	require.Equal(t, UserSessionStatusActive, session.Status)
}

func TestSensitiveWordConcurrentEnableAndHitDoNotWriteBackStaleCount(t *testing.T) {
	setupSensitiveWordTest(t)
	saveSensitiveWordTestConfig(t, "block", true, SensitiveWordBanThreshold)
	addSensitiveWordTestRule(t, "并发边界规则", []string{"并发边界命中词"}, SensitiveWordScopeGlobal, nil)
	user := createSensitiveWordTestUser(t, 987654, false)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
		"status":                         common.UserStatusEnabled,
		"sensitive_word_violation_count": SensitiveWordBanThreshold - 1,
	}).Error)

	// SQLite has no FOR UPDATE clause and may return a transient table-lock
	// error when the two writers overlap. Retry only that driver-level error;
	// the final assertions still exercise the row-locked production path on
	// MySQL/PostgreSQL.
	retryLocked := func(operation func() error) error {
		var err error
		for attempt := 0; attempt < 10; attempt++ {
			err = operation()
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), "locked") {
				return err
			}
			time.Sleep(5 * time.Millisecond)
		}
		return err
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	var enableErr, hitErr error
	var hitResult *SensitiveCheckResult
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		enableErr = retryLocked(func() error {
			_, err := EnableUserAndResetSensitiveWordViolations(user.Id)
			return err
		})
	}()
	go func() {
		defer wg.Done()
		<-start
		hitErr = retryLocked(func() error {
			var err error
			hitResult, err = CheckSensitiveRequest(sensitiveWordTestInput(user, "concurrent-enable-hit", "并发边界命中词"))
			return err
		})
	}()
	close(start)
	wg.Wait()
	require.NoError(t, enableErr)
	require.NoError(t, hitErr)
	require.NotNil(t, hitResult)

	var final User
	require.NoError(t, DB.First(&final, user.Id).Error)
	require.Equal(t, common.UserStatusEnabled, final.Status)
	// Either the enable commits first (the hit becomes count 1) or the hit
	// commits first (the enable clears its fifth hit). A stale count of five
	// with a disabled account is not an admissible outcome.
	require.LessOrEqual(t, final.SensitiveWordViolationCount, 1)
	require.Equal(t, user.Quota, final.Quota)
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
