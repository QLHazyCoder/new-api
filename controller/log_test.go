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

func TestSensitiveWordAuditRowsRequireAdministratorLogView(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousLogDB, previousLogType := model.LOG_DB, common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	model.LOG_DB = db
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.LOG_DB = previousLogDB
		common.SetLogDatabaseType(previousLogType)
		require.NoError(t, sqlDB.Close())
	})

	for _, log := range []model.Log{
		{UserId: 7, TokenId: 71, Type: model.LogTypeConsume, RequestId: "visible-use"},
		{UserId: 7, TokenId: 71, Type: model.LogTypeSensitiveWordBlock, RequestId: "private-audit", Other: `{"admin_info":{"keyword_filter":{"action":"observe","audit_id":42}}}`},
	} {
		require.NoError(t, db.Create(&log).Error)
	}

	type logPage struct {
		Data struct {
			Total int         `json:"total"`
			Items []model.Log `json:"items"`
		} `json:"data"`
	}
	var page logPage
	decodePage := func(body string) {
		t.Helper()
		page = logPage{}
		require.NoError(t, common.Unmarshal([]byte(body), &page))
	}

	decodePage(getSelfLogResponse(t, common.RoleCommonUser))
	require.Equal(t, 1, page.Data.Total)
	require.Len(t, page.Data.Items, 1)
	require.Equal(t, "visible-use", page.Data.Items[0].RequestId)
	for _, role := range []int{common.RoleAdminUser, common.RoleRootUser} {
		decodePage(getSelfLogResponse(t, role))
		require.Equal(t, 2, page.Data.Total)
		require.Equal(t, model.LogTypeSensitiveWordBlock, page.Data.Items[0].Type)
		require.Contains(t, page.Data.Items[0].Other, `"audit_id":42`)
	}

	selfRecorder := httptest.NewRecorder()
	selfContext, _ := gin.CreateTestContext(selfRecorder)
	selfContext.Request = httptest.NewRequest(http.MethodGet, "/api/log/self?type=8", nil)
	selfContext.Set("id", 7)
	selfContext.Set("role", common.RoleCommonUser)
	GetUserLogs(selfContext)
	decodePage(selfRecorder.Body.String())
	require.Zero(t, page.Data.Total)
	require.Empty(t, page.Data.Items)

	tokenRecorder := httptest.NewRecorder()
	tokenContext, _ := gin.CreateTestContext(tokenRecorder)
	tokenContext.Request = httptest.NewRequest(http.MethodGet, "/api/log/token", nil)
	tokenContext.Set("token_id", 71)
	GetLogByKey(tokenContext)
	var tokenResult struct {
		Data []model.Log `json:"data"`
	}
	require.NoError(t, common.Unmarshal(tokenRecorder.Body.Bytes(), &tokenResult))
	require.Len(t, tokenResult.Data, 1)
	require.Equal(t, "visible-use", tokenResult.Data[0].RequestId)

	adminRecorder := httptest.NewRecorder()
	adminContext, _ := gin.CreateTestContext(adminRecorder)
	adminContext.Request = httptest.NewRequest(http.MethodGet, "/api/log/?type=8", nil)
	adminContext.Set("role", common.RoleAdminUser)
	GetAllLogs(adminContext)
	decodePage(adminRecorder.Body.String())
	require.Equal(t, 1, page.Data.Total)
	require.Equal(t, model.LogTypeSensitiveWordBlock, page.Data.Items[0].Type)
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
