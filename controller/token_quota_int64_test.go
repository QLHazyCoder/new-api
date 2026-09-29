package controller

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTokenRequestAcceptsNumberAndDecimalStringQuota(t *testing.T) {
	for _, payload := range []string{
		`{"name":"large-number","remain_quota":50000000000}`,
		`{"name":"large-string","remain_quota":"50000000000"}`,
	} {
		var request tokenRequest
		require.NoError(t, common.Unmarshal([]byte(payload), &request))
		require.Equal(t, int64(50000000000), request.RemainQuota)
	}
}

func TestTokenUsagePreservesExactInt64AndAuthenticatedGroupSuffix(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}))
	require.NoError(t, db.Create(&model.User{Id: 991, Username: "usage-int64", Status: common.UserStatusEnabled,
		AuthVersion: 1}).Error)
	require.NoError(t, db.Create(&model.Token{Id: 991, UserId: 991, Key: "opaque", Name: "test",
		Status: common.TokenStatusEnabled, ExpiredTime: -1,
		RemainQuota: math.MaxInt64, UsedQuota: math.MaxInt64}).Error)
	server := gin.New()
	server.GET("/api/usage/token/", middleware.TokenAuthReadOnly(), GetTokenUsage)
	request := httptest.NewRequest(http.MethodGet, "/api/usage/token/", nil)
	request.Header.Set("Authorization", "Bearer sk-opaque-vip")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"total_granted":18446744073709551614`)
	require.Contains(t, response.Body.String(), `"total_granted_raw":"18446744073709551614"`)
	require.Contains(t, response.Body.String(), `"total_used_raw":"9223372036854775807"`)
	require.Contains(t, response.Body.String(), `"total_available_raw":"9223372036854775807"`)
}

func TestLegacyBillingMissingTokenReturnsErrorWithoutPanic(t *testing.T) {
	setupModelListControllerTestDB(t)
	previous := common.DisplayTokenStatEnabled
	common.DisplayTokenStatEnabled = true
	t.Cleanup(func() { common.DisplayTokenStatEnabled = previous })
	for _, handler := range []gin.HandlerFunc{GetSubscription, GetUsage} {
		response := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(response)
		ctx.Set("token_id", 123456)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/dashboard/billing/usage", strings.NewReader(""))
		handler(ctx)
		require.Equal(t, http.StatusOK, response.Code)
		require.Contains(t, response.Body.String(), `"error"`)
	}
}
