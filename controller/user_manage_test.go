package controller

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupManageUserTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedisEnabled := common.RedisEnabled
	previousMainDatabaseType, previousLogDatabaseType := common.MainDatabaseType(), common.LogDatabaseType()
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.UserSession{}, &model.Log{}, &model.CasbinRule{}, &model.AuthzRole{},
	))

	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedisEnabled
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func performManageUserRequest(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/manage", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 9999)
	c.Set("role", common.RoleRootUser)
	c.Set("username", "root-operator")
	ManageUser(c)
	return recorder
}

func performSensitiveWordUnbanRequest(t *testing.T, userID int) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/sensitive-words/users/%d/unban", userID), nil)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", userID)}}
	c.Set("id", 9999)
	c.Set("role", common.RoleRootUser)
	c.Set("username", "root-operator")
	UnbanSensitiveWordUser(c)
	return recorder
}

func TestManageUserDisableAdvancesAuthVersionOnceAndRevokesSession(t *testing.T) {
	db := setupManageUserTestDB(t)
	now := time.Now().Unix()
	user := model.User{
		Username: "managed-disable-user", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
	}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&model.UserSession{
		SID: "managed-disable-session", UserID: user.Id, Version: 1, UserAuthVersion: 1,
		Status: model.UserSessionStatusActive, RefreshHash: "refresh-hash", LoginMethod: "password",
		LastActiveAt: now, ExpiresAt: now + 3600,
	}).Error)

	recorder := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"disable"}`, user.Id))
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)

	var updated model.User
	require.NoError(t, db.First(&updated, user.Id).Error)
	assert.Equal(t, common.UserStatusDisabled, updated.Status)
	assert.EqualValues(t, 2, updated.AuthVersion)
	var session model.UserSession
	require.NoError(t, db.First(&session, "sid = ?", "managed-disable-session").Error)
	assert.Equal(t, model.UserSessionStatusRevoked, session.Status)
}

func TestManageUserEnableResetsSensitiveWordViolationsWithoutChangingBalance(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SensitiveWordAuditEvent{}))
	now := time.Now().Unix()
	user := model.User{
		Username: "managed-enable-reset-user", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusDisabled, Group: "default", AuthVersion: 2,
		SensitiveWordViolationCount: 5, SensitiveWordWhitelist: true,
		Quota: 87654321, UsedQuota: 12345,
	}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&model.SensitiveWordAuditEvent{
		RequestID: "managed-enable-history", UserID: user.Id, FullPrompt: "历史证据必须保留",
		MatchedWords: `["历史词"]`, Blocked: true, ViolationCount: 5, CreatedAt: time.Now(),
	}).Error)
	require.NoError(t, db.Create(&model.UserSession{
		SID: "managed-enable-stale-session", UserID: user.Id, Version: 1, UserAuthVersion: 2,
		Status: model.UserSessionStatusActive, RefreshHash: "managed-enable-stale-refresh",
		LoginMethod: "password", LastActiveAt: now, ExpiresAt: now + 3600,
	}).Error)

	recorder := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"enable"}`, user.Id))
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"success":true`)

	var updated model.User
	require.NoError(t, db.First(&updated, user.Id).Error)
	require.Equal(t, common.UserStatusEnabled, updated.Status)
	require.Zero(t, updated.SensitiveWordViolationCount)
	require.EqualValues(t, 3, updated.AuthVersion)
	require.Equal(t, 87654321, updated.Quota)
	require.Equal(t, 12345, updated.UsedQuota)
	require.True(t, updated.SensitiveWordWhitelist)
	var session model.UserSession
	require.NoError(t, db.First(&session, "sid = ?", "managed-enable-stale-session").Error)
	require.Equal(t, model.UserSessionStatusRevoked, session.Status)
	require.Equal(t, "sensitive_word_enable", session.RevokedReason)
	var historyCount int64
	require.NoError(t, db.Model(&model.SensitiveWordAuditEvent{}).Where("user_id = ?", user.Id).Count(&historyCount).Error)
	require.EqualValues(t, 1, historyCount, "启用清零不得删除历史审计事件")

	var enableAuditFound bool
	var enableAuditParams map[string]interface{}
	var logs []model.Log
	require.NoError(t, db.Where("type = ?", model.LogTypeManage).Find(&logs).Error)
	for _, log := range logs {
		other, parseErr := common.StrToMap(log.Other)
		if parseErr != nil {
			continue
		}
		op, ok := other["op"].(map[string]interface{})
		if !ok || op["action"] != "sensitive_word.enable_reset" {
			continue
		}
		enableAuditFound = true
		enableAuditParams, _ = op["params"].(map[string]interface{})
		break
	}
	require.True(t, enableAuditFound)
	require.Equal(t, float64(5), enableAuditParams["before"])
	require.Equal(t, float64(0), enableAuditParams["after"])
	require.Equal(t, false, enableAuditParams["balance_changed"])
	require.Equal(t, "user_manage_enable", enableAuditParams["source"])

	// A repeated enable must not advance auth_version or revoke a session that
	// was created after the first successful reset.
	require.NoError(t, db.Create(&model.UserSession{
		SID: "managed-enable-new-session", UserID: user.Id, Version: 1, UserAuthVersion: 3,
		Status: model.UserSessionStatusActive, RefreshHash: "managed-enable-new-refresh",
		LoginMethod: "password", LastActiveAt: now, ExpiresAt: now + 3600,
	}).Error)
	recorder = performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"enable"}`, user.Id))
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.NoError(t, db.First(&updated, user.Id).Error)
	require.EqualValues(t, 3, updated.AuthVersion)
	session = model.UserSession{}
	require.NoError(t, db.First(&session, "sid = ?", "managed-enable-new-session").Error)
	require.Equal(t, model.UserSessionStatusActive, session.Status)
}

func TestSensitiveWordUnbanEndpointUsesSameEnableResetSemantics(t *testing.T) {
	db := setupManageUserTestDB(t)
	user := model.User{
		Username: "sensitive-unban-reset-user", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusDisabled, Group: "default", AuthVersion: 4,
		SensitiveWordViolationCount: 3, Quota: 314159, UsedQuota: 2718,
	}
	require.NoError(t, db.Create(&user).Error)

	recorder := performSensitiveWordUnbanRequest(t, user.Id)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"success":true`)
	var updated model.User
	require.NoError(t, db.First(&updated, user.Id).Error)
	require.Equal(t, common.UserStatusEnabled, updated.Status)
	require.Zero(t, updated.SensitiveWordViolationCount)
	require.EqualValues(t, 5, updated.AuthVersion)
	require.Equal(t, 314159, updated.Quota)
	require.Equal(t, 2718, updated.UsedQuota)

	var found bool
	var logs []model.Log
	require.NoError(t, db.Where("type = ?", model.LogTypeManage).Find(&logs).Error)
	for _, log := range logs {
		other, parseErr := common.StrToMap(log.Other)
		if parseErr != nil {
			continue
		}
		op, ok := other["op"].(map[string]interface{})
		if !ok || op["action"] != "sensitive_word.enable_reset" {
			continue
		}
		params, _ := op["params"].(map[string]interface{})
		if params["source"] == "sensitive_word_unban" && params["balance_changed"] == false {
			found = true
			break
		}
	}
	require.True(t, found, "专用解封入口必须写入统一启用重置审计")
}

func TestManageUserDemoteAdvancesAuthVersionAndRevokesSessionsOnce(t *testing.T) {
	db := setupManageUserTestDB(t)
	previousMaster := common.IsMasterNode
	common.IsMasterNode = false
	t.Cleanup(func() { common.IsMasterNode = previousMaster })
	require.NoError(t, authz.Init(db))

	now := time.Now().Unix()
	user := model.User{
		Username: "managed-demote-user", Password: "password", Role: common.RoleAdminUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
	}
	require.NoError(t, db.Create(&user).Error)
	for _, sid := range []string{"managed-demote-session-one", "managed-demote-session-two"} {
		require.NoError(t, db.Create(&model.UserSession{
			SID: sid, UserID: user.Id, Version: 1, UserAuthVersion: 1,
			Status: model.UserSessionStatusActive, RefreshHash: "refresh-" + sid, LoginMethod: "password",
			LastActiveAt: now, ExpiresAt: now + 3600,
		}).Error)
	}

	sessionUpdateCount := 0
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:count_demote_session_updates", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "user_sessions" {
			sessionUpdateCount++
		}
	}))

	recorder := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"demote"}`, user.Id))
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)

	var updated model.User
	require.NoError(t, db.First(&updated, user.Id).Error)
	assert.Equal(t, common.RoleCommonUser, updated.Role)
	assert.EqualValues(t, 2, updated.AuthVersion)
	var sessions []model.UserSession
	require.NoError(t, db.Where("user_id = ?", user.Id).Order("sid asc").Find(&sessions).Error)
	require.Len(t, sessions, 2)
	for _, session := range sessions {
		assert.Equal(t, model.UserSessionStatusRevoked, session.Status)
		assert.Equal(t, "admin_demote", session.RevokedReason)
	}
	assert.Equal(t, 1, sessionUpdateCount)
}

func TestManageUserDeleteReturnsImmediatelyAndUnknownActionFails(t *testing.T) {
	db := setupManageUserTestDB(t)
	deleted := model.User{
		Username: "managed-delete-user", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "delete-aff",
	}
	require.NoError(t, db.Create(&deleted).Error)

	recorder := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"delete"}`, deleted.Id))
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	var deletedCount int64
	require.NoError(t, db.Unscoped().Model(&model.User{}).Where("id = ? AND deleted_at IS NOT NULL", deleted.Id).Count(&deletedCount).Error)
	assert.EqualValues(t, 1, deletedCount)

	unchanged := model.User{
		Username: "managed-unknown-user", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "unknown-aff",
	}
	require.NoError(t, db.Create(&unchanged).Error)
	recorder = performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"unknown"}`, unchanged.Id))
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	require.NoError(t, db.First(&unchanged, unchanged.Id).Error)
	assert.EqualValues(t, 1, unchanged.AuthVersion)
	assert.Equal(t, common.UserStatusEnabled, unchanged.Status)
}

func TestManageUserQuotaRejectsDeletedUsersAndOutOfRangeValues(t *testing.T) {
	db := setupManageUserTestDB(t)
	deleted := model.User{
		Username: "managed-deleted-quota-user", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", Quota: 100, AuthVersion: 1, AffCode: "deleted-quota-aff",
	}
	require.NoError(t, db.Create(&deleted).Error)
	require.NoError(t, db.Delete(&deleted).Error)

	recorder := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"add_quota","mode":"add","value":25}`, deleted.Id))
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	var stored model.User
	require.NoError(t, db.Unscoped().First(&stored, deleted.Id).Error)
	assert.Equal(t, 100, stored.Quota)

	active := model.User{
		Username: "managed-overflow-quota-user", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", Quota: math.MaxInt32, AuthVersion: 1, AffCode: "overflow-quota-aff",
	}
	require.NoError(t, db.Create(&active).Error)
	recorder = performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"add_quota","mode":"add","value":1}`, active.Id))
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	stored = model.User{}
	require.NoError(t, db.First(&stored, active.Id).Error)
	assert.Equal(t, math.MaxInt32, stored.Quota)
}

func TestManageUserQuotaModesPersistExpectedValues(t *testing.T) {
	db := setupManageUserTestDB(t)
	user := model.User{
		Username: "managed-quota-modes-user", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", Quota: 100, AuthVersion: 1, AffCode: "quota-modes-aff",
	}
	require.NoError(t, db.Create(&user).Error)

	for _, test := range []struct {
		mode  string
		value int
		want  int
	}{
		{mode: "add", value: 25, want: 125},
		{mode: "subtract", value: 5, want: 120},
		{mode: "override", value: 42, want: 42},
	} {
		recorder := performManageUserRequest(t, fmt.Sprintf(`{"id":%d,"action":"add_quota","mode":%q,"value":%d}`, user.Id, test.mode, test.value))
		assert.Contains(t, recorder.Body.String(), `"success":true`)
		var stored model.User
		require.NoError(t, db.First(&stored, user.Id).Error)
		assert.Equal(t, test.want, stored.Quota)
	}
}

func TestUpdateUserPersistsSensitiveWordControlsWithoutChangingQuota(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, authz.Init(db))
	user := model.User{
		Username:    "safety-controls-user",
		Password:    "password",
		DisplayName: "before",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		Quota:       7654321,
		AffCode:     "sensitive-controls-aff",
		AuthVersion: 1,
	}
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/user/", strings.NewReader(fmt.Sprintf(`{
		"id": %d,
		"username": "safety-controls-user",
		"display_name": "after",
		"group": "default",
		"remark": "content-safety review",
		"sensitive_word_violation_count": 4,
		"sensitive_word_whitelist": true,
		"quota": 0
	}`, user.Id)))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", 9999)
	ctx.Set("role", common.RoleRootUser)
	ctx.Set("username", "root-operator")

	UpdateUser(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"success":true`, recorder.Body.String())
	var updated model.User
	require.NoError(t, db.First(&updated, user.Id).Error)
	require.Equal(t, 4, updated.SensitiveWordViolationCount)
	require.True(t, updated.SensitiveWordWhitelist)
	require.Equal(t, 7654321, updated.Quota, "用户编辑内容安全字段不得修改余额或内部额度")

	var auditLogs []model.Log
	require.NoError(t, db.Where("type = ?", model.LogTypeManage).Find(&auditLogs).Error)
	var auditContent string
	for _, log := range auditLogs {
		auditContent += log.Other
	}
	require.Contains(t, auditContent, "sensitive_word.violations_update")
	require.Contains(t, auditContent, "sensitive_word.whitelist_update")

	recorder = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/user/", strings.NewReader(fmt.Sprintf(`{
		"id": %d,
		"username": "safety-controls-user",
		"display_name": "after",
		"group": "default"
	}`, user.Id)))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", 9999)
	ctx.Set("role", common.RoleRootUser)
	ctx.Set("username", "root-operator")
	UpdateUser(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"success":true`, recorder.Body.String())
	require.NoError(t, db.First(&updated, user.Id).Error)
	require.Equal(t, 4, updated.SensitiveWordViolationCount, "兼容旧客户端时省略字段必须保留已有违规次数")
	require.True(t, updated.SensitiveWordWhitelist, "兼容旧客户端时省略字段必须保留已有白名单状态")
}
