package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMidjourneyTimeoutNeedsConfirmedUpstreamFailureBeforeRefund(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedis, previousMemory := common.RedisEnabled, common.MemoryCacheEnabled
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	model.InitColumnNamesForTest()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB, model.LOG_DB = database, database
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.Channel{}, &model.Midjourney{}, &model.BillingOperation{}, &model.Log{}))
	service.InitHttpClient()
	t.Cleanup(func() {
		_ = sqlDB.Close()
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled, common.MemoryCacheEnabled = previousRedis, previousMemory
		common.SetDatabaseTypes(previousMainType, previousLogType)
		model.InitColumnNamesForTest()
	})

	var upstreamFailure atomic.Bool
	submittedAt := time.Now().Add(-2 * time.Hour).UnixMilli()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mj/task/list-by-condition" {
			http.NotFound(w, r)
			return
		}
		status, progress, reason := "IN_PROGRESS", "20%", ""
		if upstreamFailure.Load() {
			status, progress, reason = "FAILURE", "100%", "confirmed upstream failure"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]dto.MidjourneyDto{{MjId: "mj-billing-timeout", Status: status, Progress: progress, FailReason: reason, SubmitTime: submittedAt}})
	}))
	defer upstream.Close()

	baseURL := upstream.URL
	channel := model.Channel{Id: 881, Name: "midjourney billing test", Key: "test-key", Status: common.ChannelStatusEnabled, BaseURL: &baseURL, UsedQuota: 30}
	require.NoError(t, database.Create(&channel).Error)
	user := model.User{Id: 882, Username: "midjourney-billing-test", Password: "test-password", Quota: 70, UsedQuota: 30, RequestCount: 1}
	require.NoError(t, database.Create(&user).Error)
	task := model.Midjourney{Id: 883, UserId: user.Id, MjId: "mj-billing-timeout", ChannelId: channel.Id,
		Status: "IN_PROGRESS", Progress: "10%", SubmitTime: submittedAt, Quota: 30}
	require.NoError(t, database.Create(&task).Error)
	operation := model.BillingOperation{RequestId: "midjourney:883", Phase: "initial", State: model.BillingSettled,
		UserId: user.Id, ChannelId: channel.Id, FundingSource: "midjourney_wallet", WalletAmount: 30,
		PreConsumed: 30, Actual: 30, UsageCounted: true, TaskId: int64(task.Id)}
	require.NoError(t, database.Create(&operation).Error)

	runMidjourneyTaskUpdateOnce(context.Background(), nil)
	var pending model.Midjourney
	require.NoError(t, database.First(&pending, task.Id).Error)
	require.Equal(t, "IN_PROGRESS", pending.Status)
	require.Equal(t, 30, pending.Quota)
	require.True(t, pending.BillingReviewPending)
	require.False(t, pending.BillingFailureConfirmed)
	var currentUser model.User
	require.NoError(t, database.First(&currentUser, user.Id).Error)
	require.EqualValues(t, 70, currentUser.Quota)

	upstreamFailure.Store(true)
	runMidjourneyTaskUpdateOnce(context.Background(), nil)
	require.NoError(t, database.First(&pending, task.Id).Error)
	require.Equal(t, "FAILURE", pending.Status)
	require.Zero(t, pending.Quota)
	require.False(t, pending.BillingReviewPending)
	require.True(t, pending.BillingFailureConfirmed)
	require.NoError(t, database.First(&currentUser, user.Id).Error)
	require.EqualValues(t, 100, currentUser.Quota)
	require.EqualValues(t, 0, currentUser.UsedQuota)
	var currentChannel model.Channel
	require.NoError(t, database.First(&currentChannel, channel.Id).Error)
	require.Zero(t, currentChannel.UsedQuota)
}
