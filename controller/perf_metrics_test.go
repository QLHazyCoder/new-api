package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type perfMetricsResponse struct {
	Success bool                    `json:"success"`
	Data    perfmetrics.QueryResult `json:"data"`
}

type perfMetricsSummaryResponse struct {
	Success bool                         `json:"success"`
	Data    perfmetrics.SummaryAllResult `json:"data"`
}

func setupPerfMetricsControllerTest(t *testing.T) string {
	t.Helper()

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))

	originalGroupRatio := ratio_setting.GroupRatio2JSONString()
	originalUserUsableGroups := setting.UserUsableGroups2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"auto":1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default"}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatio))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUserUsableGroups))
	})

	require.NoError(t, db.Create(&model.User{
		Id:       41001,
		Username: "perf-default",
		Password: "password",
		Group:    "default",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		AffCode:  "perf-default",
	}).Error)
	require.NoError(t, db.Create(&model.User{
		Id:       41002,
		Username: "perf-admin",
		Password: "password",
		Group:    "default",
		Role:     common.RoleAdminUser,
		Status:   common.UserStatusEnabled,
		AffCode:  "perf-admin",
	}).Error)

	modelName := "gpt-perf-visibility-" + strings.ReplaceAll(t.Name(), "/", "-")
	bucketTs := time.Now().Unix() - 60
	require.NoError(t, db.Create(&model.PerfMetric{
		ModelName:      modelName,
		Group:          "default",
		BucketTs:       bucketTs,
		RequestCount:   7,
		SuccessCount:   7,
		TotalLatencyMs: 7000,
		OutputTokens:   700,
		GenerationMs:   7000,
	}).Error)
	require.NoError(t, db.Create(&model.PerfMetric{
		ModelName:      modelName,
		Group:          "vip",
		BucketTs:       bucketTs,
		RequestCount:   11,
		SuccessCount:   11,
		TotalLatencyMs: 11000,
		OutputTokens:   1100,
		GenerationMs:   11000,
	}).Error)

	return modelName
}

func performPerfMetricsRequest(t *testing.T, modelName string, userId int, group string) perfMetricsResponse {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/perf-metrics?model=%s&hours=1", modelName), nil)
	if group != "" {
		ctx.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/perf-metrics?model=%s&hours=1&group=%s", modelName, group), nil)
	}
	if userId > 0 {
		ctx.Set("id", userId)
	}

	GetPerfMetrics(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var body perfMetricsResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success)
	return body
}

func performPerfMetricsSummaryRequest(t *testing.T, userId int) perfMetricsSummaryResponse {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/perf-metrics/summary?hours=1", nil)
	if userId > 0 {
		ctx.Set("id", userId)
	}

	GetPerfMetricsSummary(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var body perfMetricsSummaryResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success)
	return body
}

func perfMetricGroupNames(groups []perfmetrics.GroupResult) []string {
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		names = append(names, group.Group)
	}
	return names
}

func TestGetPerfMetricsFiltersGroupsByVisibility(t *testing.T) {
	modelName := setupPerfMetricsControllerTest(t)

	anonymous := performPerfMetricsRequest(t, modelName, 0, "")
	require.Equal(t, []string{"default"}, perfMetricGroupNames(anonymous.Data.Groups))

	commonUser := performPerfMetricsRequest(t, modelName, 41001, "")
	require.Equal(t, []string{"default"}, perfMetricGroupNames(commonUser.Data.Groups))

	admin := performPerfMetricsRequest(t, modelName, 41002, "")
	require.ElementsMatch(t, []string{"default", "vip"}, perfMetricGroupNames(admin.Data.Groups))
}

func TestGetPerfMetricsRejectsInvisibleRequestedGroup(t *testing.T) {
	modelName := setupPerfMetricsControllerTest(t)

	commonUser := performPerfMetricsRequest(t, modelName, 41001, "vip")
	require.Empty(t, commonUser.Data.Groups)

	admin := performPerfMetricsRequest(t, modelName, 41002, "vip")
	require.Equal(t, []string{"vip"}, perfMetricGroupNames(admin.Data.Groups))
}

func TestGetPerfMetricsSummaryFiltersGroupsByVisibility(t *testing.T) {
	modelName := setupPerfMetricsControllerTest(t)

	commonUser := performPerfMetricsSummaryRequest(t, 41001)
	require.Len(t, commonUser.Data.Models, 1)
	require.Equal(t, modelName, commonUser.Data.Models[0].ModelName)
	require.Equal(t, int64(7), commonUser.Data.Models[0].RequestCount)

	admin := performPerfMetricsSummaryRequest(t, 41002)
	require.Len(t, admin.Data.Models, 1)
	require.Equal(t, modelName, admin.Data.Models[0].ModelName)
	require.Equal(t, int64(18), admin.Data.Models[0].RequestCount)
}
