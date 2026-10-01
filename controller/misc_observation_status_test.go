package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetStatusPublishesObservationCapabilities(t *testing.T) {
	previousOptions := common.OptionMap
	previousErrorLog := constant.ErrorLogEnabled
	common.OptionMap = map[string]string{}
	constant.ErrorLogEnabled = true
	t.Cleanup(func() {
		common.OptionMap = previousOptions
		constant.ErrorLogEnabled = previousErrorLog
	})

	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/status", nil)

	GetStatus(context)

	var payload struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Contains(t, payload.Data, "perf_metrics_enabled")
	require.Contains(t, payload.Data, "error_log_enabled")
	require.IsType(t, true, payload.Data["perf_metrics_enabled"])
	require.Equal(t, true, payload.Data["error_log_enabled"])
}
