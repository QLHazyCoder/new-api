package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestValidateLogRetentionDaysOption(t *testing.T) {
	for _, value := range []string{"0", "30", " 30 ", "3650"} {
		require.NoError(t, validateLogRetentionDaysOption(value))
	}

	for _, value := range []string{"-1", "3651", "1.5", "abc"} {
		require.Error(t, validateLogRetentionDaysOption(value))
	}
}

func TestUsageLogModelDiagnosticsAreProjectedByRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		require.NoError(t, sqlDB.Close())
	})

	require.NoError(t, db.Create(&model.Log{
		UserId:    7,
		TokenId:   71,
		CreatedAt: 1,
		Type:      model.LogTypeConsume,
		Other:     `{"model_ratio":1,"is_model_mapped":true,"upstream_model_name":"legacy-upstream","response_model":{"requested_model":"requested","upstream_model":"legacy-upstream","returned_model":"legacy-returned"},"admin_info":{"is_model_mapped":true,"upstream_model_name":"scoped-upstream","response_model":{"requested_model":"requested","upstream_model":"scoped-upstream","returned_model":"scoped-returned"}}}`,
	}).Error)

	selfBody := getSelfLogResponse(t, common.RoleCommonUser)
	for _, field := range []string{"is_model_mapped", "upstream_model_name", "response_model", "legacy-upstream", "scoped-upstream"} {
		require.NotContains(t, selfBody, field)
	}

	tokenRecorder := httptest.NewRecorder()
	tokenContext, _ := gin.CreateTestContext(tokenRecorder)
	tokenContext.Request = httptest.NewRequest(http.MethodGet, "/api/log/token", nil)
	tokenContext.Set("token_id", 71)
	GetLogByKey(tokenContext)
	require.Equal(t, http.StatusOK, tokenRecorder.Code)
	for _, field := range []string{"is_model_mapped", "upstream_model_name", "response_model", "legacy-upstream", "scoped-upstream"} {
		require.NotContains(t, tokenRecorder.Body.String(), field)
	}

	adminSelfBody := getSelfLogResponse(t, common.RoleAdminUser)
	require.Contains(t, adminSelfBody, "legacy-upstream")
	require.Contains(t, adminSelfBody, "scoped-upstream")

	rootSelfBody := getSelfLogResponse(t, common.RoleRootUser)
	require.Contains(t, rootSelfBody, "legacy-upstream")
	require.Contains(t, rootSelfBody, "scoped-upstream")

	adminRecorder := httptest.NewRecorder()
	adminContext, _ := gin.CreateTestContext(adminRecorder)
	adminContext.Request = httptest.NewRequest(http.MethodGet, "/api/log/?p=1&page_size=10", nil)
	adminContext.Set("role", common.RoleAdminUser)
	GetAllLogs(adminContext)
	require.Equal(t, http.StatusOK, adminRecorder.Code)
	require.Contains(t, adminRecorder.Body.String(), "legacy-upstream")
	require.Contains(t, adminRecorder.Body.String(), "scoped-upstream")
}

func getSelfLogResponse(t *testing.T, role int) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/log/self?p=1&page_size=10", nil)
	context.Set("id", 7)
	context.Set("role", role)
	GetUserLogs(context)
	require.Equal(t, http.StatusOK, recorder.Code)
	return recorder.Body.String()
}
