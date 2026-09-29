package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to open test db: " + err.Error())
	}
	sqlDB, err := db.DB()
	if err != nil {
		panic("failed to get sql.DB: " + err.Error())
	}
	sqlDB.SetMaxOpenConns(1)

	model.DB = db
	model.LOG_DB = db

	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = true

	if err := db.AutoMigrate(
		&model.Task{},
		&model.User{},
		&model.Token{},
		&model.Log{},
		&model.Channel{},
		&model.Midjourney{},
		&model.TopUp{},
		&model.SubscriptionPlan{},
		&model.UserSubscription{},
		&model.SubscriptionPreConsumeRecord{},
		&model.BillingOperation{},
		&model.SystemTask{},
		&model.SystemTaskLock{},
	); err != nil {
		panic("failed to migrate: " + err.Error())
	}

	os.Exit(m.Run())
}

// ---------------------------------------------------------------------------
// Seed helpers
// ---------------------------------------------------------------------------

func truncate(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM tasks")
		model.DB.Exec("DELETE FROM users")
		model.DB.Exec("DELETE FROM tokens")
		model.DB.Exec("DELETE FROM logs")
		model.DB.Exec("DELETE FROM channels")
		model.DB.Exec("DELETE FROM midjourneys")
		model.DB.Exec("DELETE FROM top_ups")
		model.DB.Exec("DELETE FROM subscription_pre_consume_records")
		model.DB.Exec("DELETE FROM billing_operations")
		model.DB.Exec("DELETE FROM user_subscriptions")
		model.DB.Exec("DELETE FROM subscription_plans")
		model.DB.Exec("DELETE FROM system_task_locks")
		model.DB.Exec("DELETE FROM system_tasks")
	})
}

func seedUser(t *testing.T, id int, quota int64) {
	t.Helper()
	user := &model.User{Id: id, Username: "test_user", Quota: quota, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(user).Error)
}

func seedToken(t *testing.T, id int, userId int, key string, remainQuota int64) {
	t.Helper()
	token := &model.Token{
		Id:          id,
		UserId:      userId,
		Key:         key,
		Name:        "test_token",
		Status:      common.TokenStatusEnabled,
		RemainQuota: remainQuota,
		UsedQuota:   0,
	}
	require.NoError(t, model.DB.Create(token).Error)
}

func seedSubscription(t *testing.T, id int, userId int, amountTotal int64, amountUsed int64) {
	t.Helper()
	sub := &model.UserSubscription{
		Id:          id,
		UserId:      userId,
		AmountTotal: amountTotal,
		AmountUsed:  amountUsed,
		Status:      "active",
		StartTime:   time.Now().Unix(),
		EndTime:     time.Now().Add(30 * 24 * time.Hour).Unix(),
	}
	require.NoError(t, model.DB.Create(sub).Error)
}

func seedSubscriptionPlan(t *testing.T, id int, allowWalletOverflow bool) {
	t.Helper()
	plan := &model.SubscriptionPlan{
		Id:                  id,
		Title:               "test subscription",
		DurationUnit:        model.SubscriptionDurationMonth,
		DurationValue:       1,
		AllowWalletOverflow: common.GetPointer(allowWalletOverflow),
		QuotaResetPeriod:    model.SubscriptionResetNever,
	}
	require.NoError(t, model.DB.Create(plan).Error)
	model.InvalidateSubscriptionPlanCache(id)
}

func seedSubscriptionWithPlan(t *testing.T, id, userID, planID int, amountTotal, amountUsed int64, allowWalletOverflow bool) {
	t.Helper()
	sub := &model.UserSubscription{
		Id:                  id,
		UserId:              userID,
		PlanId:              planID,
		AmountTotal:         amountTotal,
		AmountUsed:          amountUsed,
		Status:              "active",
		StartTime:           time.Now().Unix(),
		EndTime:             time.Now().Add(30 * 24 * time.Hour).Unix(),
		AllowWalletOverflow: allowWalletOverflow,
	}
	require.NoError(t, model.DB.Create(sub).Error)
}

func seedChannel(t *testing.T, id int) {
	t.Helper()
	ch := &model.Channel{Id: id, Name: "test_channel", Key: "sk-test", Status: common.ChannelStatusEnabled}
	require.NoError(t, model.DB.Create(ch).Error)
}

func seedChargedAccounting(t *testing.T, userID, channelID, tokenID, quota, requestCount int) {
	t.Helper()
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", userID).Updates(map[string]any{
		"used_quota":    quota,
		"request_count": requestCount,
	}).Error)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channelID).
		Update("used_quota", quota).Error)
	if tokenID > 0 {
		require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).
			Update("used_quota", quota).Error)
	}
}

func makeTask(userId, channelId, quota, tokenId int, billingSource string, subscriptionId int) *model.Task {
	return &model.Task{
		TaskID:    "task_" + time.Now().Format("150405.000"),
		UserId:    userId,
		ChannelId: channelId,
		Quota:     quota,
		Status:    model.TaskStatus(model.TaskStatusInProgress),
		Group:     "default",
		Data:      json.RawMessage(`{}`),
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
		Properties: model.Properties{
			OriginModelName: "test-model",
		},
		PrivateData: model.TaskPrivateData{
			BillingSource:  billingSource,
			SubscriptionId: subscriptionId,
			TokenId:        tokenId,
			BillingContext: &model.TaskBillingContext{
				ModelPrice:      0.02,
				GroupRatio:      1.0,
				OriginModelName: "test-model",
			},
		},
	}
}

func TestTaskBillingOtherStoresMappedModelDiagnosticsInAdminInfo(t *testing.T) {
	other := taskBillingOther(&model.Task{
		Properties: model.Properties{
			OriginModelName:   "requested-model",
			UpstreamModelName: "upstream-model",
		},
	})

	snapshot := other.Snapshot()
	for _, key := range []string{"is_model_mapped", "upstream_model_name", "response_model"} {
		assert.NotContains(t, snapshot, key)
	}
	adminInfo, ok := snapshot["admin_info"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, adminInfo["is_model_mapped"])
	assert.Equal(t, "upstream-model", adminInfo["upstream_model_name"])
}

func newBillingTestContext(tokenQuota int) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Set("token_quota", tokenQuota)
	return c
}

func TestBillingOperationRollsBackAndRetriesRefundWithoutDoubleCredit(t *testing.T) {
	truncate(t)
	const userID, tokenID = 980, 980
	seedUser(t, userID, 100)
	seedToken(t, tokenID, userID, "p32-operation", 100)
	info := &relaycommon.RelayInfo{UserId: userID, TokenId: tokenID, TokenKey: "p32-operation",
		RequestId: "p32-operation-retry", ForcePreConsume: true,
		UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}}
	const callback = "test:p32_billing_wallet_failure"
	failWallet := true
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if failWallet && tx.Statement.Table == "users" {
			tx.AddError(errors.New("forced wallet failure"))
		}
	}))
	t.Cleanup(func() { require.NoError(t, model.DB.Callback().Update().Remove(callback)) })
	require.NotNil(t, PreConsumeBilling(newBillingTestContext(100), 60, info))
	assert.EqualValues(t, 100, getUserQuota(t, userID))
	assert.EqualValues(t, 100, getTokenRemainQuota(t, tokenID))
	var count int64
	require.NoError(t, model.DB.Model(&model.BillingOperation{}).Where("request_id = ?", info.RequestId).Count(&count).Error)
	assert.Zero(t, count)

	failWallet = false
	require.Nil(t, PreConsumeBilling(newBillingTestContext(100), 60, info))
	assert.EqualValues(t, 40, getUserQuota(t, userID))
	assert.EqualValues(t, 40, getTokenRemainQuota(t, tokenID))
	require.NotNil(t, PreConsumeBilling(newBillingTestContext(100), 60, info), "same request cannot pre-consume twice")
	assert.EqualValues(t, 40, getUserQuota(t, userID))
	require.NoError(t, model.RequestBillingRefund(info.RequestId))

	const refundCallback = "test:p32_billing_refund_token_failure"
	failToken := true
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(refundCallback, func(tx *gorm.DB) {
		if failToken && tx.Statement.Table == "tokens" {
			tx.AddError(errors.New("forced token refund failure"))
		}
	}))
	t.Cleanup(func() { require.NoError(t, model.DB.Callback().Update().Remove(refundCallback)) })
	require.Error(t, model.RefundBillingOperation(info.RequestId))
	assert.EqualValues(t, 40, getUserQuota(t, userID), "wallet credit must roll back with token failure")
	assert.EqualValues(t, 40, getTokenRemainQuota(t, tokenID))
	pending, err := model.ListPendingBillingRefunds(10)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	failToken = false
	RecoverBillingOnce(context.Background())
	require.NoError(t, model.RefundBillingOperation(info.RequestId))
	assert.EqualValues(t, 100, getUserQuota(t, userID))
	assert.EqualValues(t, 100, getTokenRemainQuota(t, tokenID))
	require.NoError(t, model.DB.Model(&model.BillingOperation{}).Where("request_id = ? AND phase = ?", info.RequestId, "refund").Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestRealtimePreChargesOnlySettleRemainingDelta(t *testing.T) {
	truncate(t)
	previous := ratio_setting.ModelRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"p32-realtime":1}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previous)) })
	const userID, tokenID = 986, 986
	seedUser(t, userID, 10000)
	seedToken(t, tokenID, userID, "realtime-group-key", 10000)
	zero := float64(0)
	info := &relaycommon.RelayInfo{RequestId: "p32-realtime-986", UserId: userID, TokenId: tokenID,
		TokenKey: "realtime-group-key", UserQuota: 10000, OriginModelName: "p32-realtime",
		UsingGroup: "default", UserSetting: dto.UserSetting{QuotaWarningThreshold: &zero}}
	usage := &dto.RealtimeUsage{TotalTokens: 10, InputTokens: 10,
		InputTokenDetails: dto.InputTokenDetails{TextTokens: 10}}
	ctx := newBillingTestContext(0)
	require.NoError(t, PreWssConsumeQuota(ctx, info, usage))
	require.NoError(t, PreWssConsumeQuota(ctx, info, usage))
	require.Positive(t, info.RealtimePreChargedQuota)
	require.Equal(t, 2, info.LegacyBillingSequence)
	var pending model.BillingOperation
	require.NoError(t, model.DB.Where("request_id = ? AND phase = ?", info.RequestId, "initial").First(&pending).Error)
	assert.Equal(t, model.BillingReserved, pending.State)
	assert.EqualValues(t, info.RealtimePreChargedQuota, pending.PreConsumed)
	assert.EqualValues(t, info.RealtimePreChargedQuota, pending.WalletAmount)
	assert.EqualValues(t, info.RealtimePreChargedQuota, pending.TokenAmount)
	stale, err := model.ListStaleReservedBillingOperations(common.GetTimestamp()+1, 10)
	require.NoError(t, err)
	require.Len(t, stale, 1, "an interrupted realtime pre-consume remains visible for review")
	assert.Equal(t, info.RequestId, stale[0].RequestId)
	final := info.RealtimePreChargedQuota + 5
	require.NoError(t, SettleBilling(ctx, info, final))
	require.NoError(t, SettleBilling(ctx, info, final))
	assert.EqualValues(t, 10000-final, getUserQuota(t, userID))
	assert.EqualValues(t, 10000-final, getTokenRemainQuota(t, tokenID))
	used, count := getUserUsageAccounting(t, userID)
	assert.Equal(t, final, used)
	assert.Equal(t, 1, count)
	var steps int64
	require.NoError(t, model.DB.Model(&model.BillingOperation{}).Where("request_id = ?", info.RequestId).Count(&steps).Error)
	assert.EqualValues(t, 4, steps)
	require.NoError(t, model.DB.Where("request_id = ? AND phase = ?", info.RequestId, "initial").First(&pending).Error)
	assert.Equal(t, model.BillingSettled, pending.State)
	assert.EqualValues(t, final, pending.Actual)
	assert.EqualValues(t, final, pending.WalletAmount)
	assert.EqualValues(t, final, pending.TokenAmount)
}

func TestNewBillingSessionSubscriptionFirstPersistsMixedAllocations(t *testing.T) {
	truncate(t)

	const userID, tokenID, subID, planID = 310, 310, 310, 310
	const preConsumed = 100
	seedUser(t, userID, 1_000)
	seedToken(t, tokenID, userID, "sk-mixed-allocation", 1_000)
	seedSubscriptionPlan(t, planID, true)
	seedSubscriptionWithPlan(t, subID, userID, planID, 1_000, 950, true)

	info := &relaycommon.RelayInfo{
		UserId:          userID,
		TokenId:         tokenID,
		TokenKey:        "sk-mixed-allocation",
		RequestId:       "req-mixed-allocation",
		OriginModelName: "test-model",
		UsingGroup:      "default",
		UserSetting:     dto.UserSetting{BillingPreference: "subscription_first"},
	}

	session, apiErr := NewBillingSession(newBillingTestContext(1_000), info, preConsumed)
	require.Nil(t, apiErr)
	require.NotNil(t, session)
	assert.Equal(t, BillingSourceMixed, info.BillingSource)
	require.Len(t, info.BillingAllocations, 2)
	assert.Equal(t, BillingSourceSubscription, info.BillingAllocations[0].Source)
	assert.Equal(t, 50, info.BillingAllocations[0].Quota)
	assert.Equal(t, BillingSourceWallet, info.BillingAllocations[1].Source)
	assert.Equal(t, 50, info.BillingAllocations[1].Quota)
	assert.EqualValues(t, 1_000, getSubscriptionUsed(t, subID))
	assert.EqualValues(t, 950, getUserQuota(t, userID))

	require.NoError(t, session.Settle(70))
	require.Len(t, info.BillingAllocations, 2)
	assert.Equal(t, 50, info.BillingAllocations[0].Quota)
	assert.Equal(t, 20, info.BillingAllocations[1].Quota)
	assert.EqualValues(t, 980, getUserQuota(t, userID))
	assert.EqualValues(t, 930, getTokenRemainQuota(t, tokenID))
}

func TestMixedTaskBillingPersistsAllocationsAcrossRecalculateAndRefund(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID, subID, planID = 312, 312, 312, 312, 312
	const preConsumed, actualQuota = 100, 70

	// This is the state immediately after subscription_first consumed the last
	// 50 subscription units and charged the remaining 50 to the wallet.
	seedUser(t, userID, 950)
	seedToken(t, tokenID, userID, "mixed-task-recalc", 950)
	seedChannel(t, channelID)
	seedSubscriptionPlan(t, planID, true)
	seedSubscriptionWithPlan(t, subID, userID, planID, 1_000, 1_000, true)
	seedChargedAccounting(t, userID, channelID, tokenID, preConsumed, 1)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceMixed, 0)
	task.PrivateData.BillingAllocations = []model.BillingAllocation{
		{
			Source:                             BillingSourceSubscription,
			Quota:                              50,
			SubscriptionId:                     subID,
			SubscriptionPlanId:                 planID,
			SubscriptionPlanTitle:              "test subscription",
			SubscriptionAmountTotal:            1_000,
			SubscriptionAmountUsedAfterConsume: 1_000,
		},
		{Source: BillingSourceWallet, Quota: 50},
	}
	require.NoError(t, model.DB.Create(task).Error)

	// The lower actual charge must refund the wallet portion first, and persist
	// that revised allocation atomically with the task quota.
	RecalculateTaskQuota(ctx, task, actualQuota, "mixed task actual usage")
	assert.EqualValues(t, 980, getUserQuota(t, userID))
	assert.EqualValues(t, 980, getTokenRemainQuota(t, tokenID))
	assert.EqualValues(t, 70, getTokenUsedQuota(t, tokenID))
	assert.EqualValues(t, 1_000, getSubscriptionUsed(t, subID))
	assert.Equal(t, actualQuota, getTaskQuota(t, task.ID))
	var billingRoot model.BillingOperation
	require.NoError(t, model.DB.Where("request_id = ? AND phase = ?", fmt.Sprintf("task:%d", task.ID), "initial").First(&billingRoot).Error)
	assert.Equal(t, "mixed", billingRoot.FundingSource)
	assert.Equal(t, subID, billingRoot.SubscriptionId)
	assert.EqualValues(t, 20, billingRoot.WalletAmount)
	assert.EqualValues(t, 50, billingRoot.SubscriptionAmount)
	assert.EqualValues(t, actualQuota, billingRoot.TokenAmount)

	var settled model.Task
	require.NoError(t, model.DB.First(&settled, task.ID).Error)
	require.Len(t, settled.PrivateData.BillingAllocations, 2)
	assert.Equal(t, BillingSourceSubscription, settled.PrivateData.BillingAllocations[0].Source)
	assert.Equal(t, 50, settled.PrivateData.BillingAllocations[0].Quota)
	assert.Equal(t, BillingSourceWallet, settled.PrivateData.BillingAllocations[1].Source)
	assert.Equal(t, 20, settled.PrivateData.BillingAllocations[1].Quota)

	// A later failure refunds exactly the persisted split: 20 wallet units and
	// 50 subscription units, never the entire task from the wallet.
	assert.True(t, RefundTaskQuota(ctx, &settled, "mixed task failed"))
	assert.EqualValues(t, 1_000, getUserQuota(t, userID))
	assert.EqualValues(t, 1_050, getTokenRemainQuota(t, tokenID))
	assert.Zero(t, getTokenUsedQuota(t, tokenID))
	assert.EqualValues(t, 950, getSubscriptionUsed(t, subID))
	assert.Zero(t, getTaskQuota(t, task.ID))
	require.NoError(t, model.DB.Where("request_id = ? AND phase = ?", fmt.Sprintf("task:%d", task.ID), "initial").First(&billingRoot).Error)
	assert.Equal(t, model.BillingRefunded, billingRoot.State)
	assert.Zero(t, billingRoot.WalletAmount)
	assert.Zero(t, billingRoot.SubscriptionAmount)
	assert.Zero(t, billingRoot.TokenAmount)
	changed, err := model.FinalizeTaskBillingWithStatus(&settled, 30, false, settled.Status)
	require.NoError(t, err)
	assert.False(t, changed, "a refunded task cannot be charged again by a later recalculation")
	assert.EqualValues(t, 1_000, getUserQuota(t, userID))
	assert.EqualValues(t, 1_050, getTokenRemainQuota(t, tokenID))
	assert.EqualValues(t, 950, getSubscriptionUsed(t, subID))

	var refunded model.Task
	require.NoError(t, model.DB.First(&refunded, task.ID).Error)
	assert.Empty(t, refunded.PrivateData.BillingAllocations)

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
	var other map[string]any
	require.NoError(t, json.Unmarshal([]byte(log.Other), &other))
	assert.Contains(t, other, "billing_refund_allocations")
}

func TestMixedTaskSubmissionSettlementPersistsFinalAllocationBeforeRefund(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID, subID, planID = 313, 313, 313, 313, 313
	seedUser(t, userID, 950)
	seedToken(t, tokenID, userID, "mixed-task-submit", 1_000)
	seedChannel(t, channelID)
	seedSubscriptionPlan(t, planID, false)
	seedSubscriptionWithPlan(t, subID, userID, planID, 1_000, 950, false)
	info := &relaycommon.RelayInfo{
		UserId: userID, TokenId: tokenID, TokenKey: "mixed-task-submit", RequestId: "mixed-task-submit-request",
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channelID}, OriginModelName: "test-model", UsingGroup: "default",
		UserSetting: dto.UserSetting{BillingPreference: "subscription_first"},
	}
	require.Nil(t, PreConsumeBilling(newBillingTestContext(1_000), 100, info))
	require.Equal(t, BillingSourceMixed, info.BillingSource)

	task := makeTask(userID, channelID, 70, tokenID, BillingSourceMixed, info.SubscriptionId)
	task.Status = model.TaskStatus(model.TaskStatusSuccess)
	task.Progress = "100%"
	task.PrivateData.BillingAllocations = model.NewTaskBillingAllocationsFromRelay(info.BillingAllocations)
	require.NoError(t, model.InsertTaskWithBillingIntent(ctx, info.RequestId, task, task.Quota))
	info.BillableUsageObserved = true
	require.NoError(t, SettleTaskBilling(newBillingTestContext(1_000), info, task, task.Quota, model.TaskStatusBillingPending))

	assert.Equal(t, 70, getTaskQuota(t, task.ID))
	assert.EqualValues(t, 930, getUserQuota(t, userID))
	assert.EqualValues(t, 1_000, getSubscriptionUsed(t, subID))
	assert.EqualValues(t, 930, getTokenRemainQuota(t, tokenID))
	require.Len(t, task.PrivateData.BillingAllocations, 2)
	assert.Equal(t, BillingSourceSubscription, task.PrivateData.BillingAllocations[0].Source)
	assert.Equal(t, 50, task.PrivateData.BillingAllocations[0].Quota)
	assert.Equal(t, BillingSourceWallet, task.PrivateData.BillingAllocations[1].Source)
	assert.Equal(t, 20, task.PrivateData.BillingAllocations[1].Quota)

	fromStatus := task.Status
	task.Status = model.TaskStatus(model.TaskStatusFailure)
	changed, err := model.FinalizeTaskBillingWithStatus(task, 0, true, fromStatus)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.EqualValues(t, 950, getUserQuota(t, userID))
	assert.EqualValues(t, 950, getSubscriptionUsed(t, subID))
	assert.EqualValues(t, 1_000, getTokenRemainQuota(t, tokenID))
	assert.Zero(t, getTaskQuota(t, task.ID))
	assert.Empty(t, task.PrivateData.BillingAllocations)
}

func TestNewBillingSessionStrictSubscriptionUsesMixedBilling(t *testing.T) {
	truncate(t)

	const userID, tokenID, subID, planID = 311, 311, 311, 311
	seedUser(t, userID, 1_000)
	seedToken(t, tokenID, userID, "sk-strict-subscription", 1_000)
	seedSubscriptionPlan(t, planID, false)
	seedSubscriptionWithPlan(t, subID, userID, planID, 1_000, 950, false)

	info := &relaycommon.RelayInfo{
		UserId:          userID,
		TokenId:         tokenID,
		TokenKey:        "sk-strict-subscription",
		RequestId:       "req-strict-subscription",
		OriginModelName: "test-model",
		UsingGroup:      "default",
		UserSetting:     dto.UserSetting{BillingPreference: "subscription_first"},
	}

	session, apiErr := NewBillingSession(newBillingTestContext(1_000), info, 100)
	require.Nil(t, apiErr)
	require.NotNil(t, session)
	assert.Equal(t, BillingSourceMixed, info.BillingSource)
	assert.Equal(t, 100, info.FinalPreConsumedQuota)
	assert.EqualValues(t, 950, getUserQuota(t, userID))
	assert.EqualValues(t, 1_000, getSubscriptionUsed(t, subID))
	assert.EqualValues(t, 900, getTokenRemainQuota(t, tokenID))
	require.Len(t, info.BillingAllocations, 2)
	assert.Equal(t, BillingSourceSubscription, info.BillingAllocations[0].Source)
	assert.Equal(t, 50, info.BillingAllocations[0].Quota)
	assert.Equal(t, BillingSourceWallet, info.BillingAllocations[1].Source)
	assert.Equal(t, 50, info.BillingAllocations[1].Quota)
}

func TestPriceDataOtherRatiosFilterAndSnapshot(t *testing.T) {
	priceData := types.PriceData{}

	priceData.AddOtherRatio("zero", 0)
	priceData.AddOtherRatio("negative", -0.5)
	priceData.AddOtherRatio("nan", math.NaN())
	priceData.AddOtherRatio("inf", math.Inf(1))
	priceData.AddOtherRatio("one", 1)
	priceData.AddOtherRatio("positive", 2.5)

	ratios := priceData.OtherRatios()
	require.Len(t, ratios, 2)
	assert.Equal(t, 1.0, ratios["one"])
	assert.Equal(t, 2.5, ratios["positive"])
	assert.True(t, priceData.HasOtherRatio("one"))
	assert.False(t, priceData.HasOtherRatio("zero"))

	ratios["positive"] = 99
	ratios["new"] = 3
	nextSnapshot := priceData.OtherRatios()
	assert.Equal(t, 2.5, nextSnapshot["positive"])
	assert.NotContains(t, nextSnapshot, "new")
}

func TestPriceDataReplaceAndApplyOtherRatios(t *testing.T) {
	priceData := types.PriceData{}

	replaced := priceData.ReplaceOtherRatios(map[string]float64{
		"zero":     0,
		"negative": -3,
		"nan":      math.NaN(),
		"inf":      math.Inf(1),
		"one":      1,
		"duration": 2,
		"size":     1.5,
	})

	require.True(t, replaced)
	assert.Equal(t, 3.0, priceData.OtherRatioMultiplier())
	assert.Equal(t, 30.0, priceData.ApplyOtherRatiosToFloat(10))
	assert.Equal(t, 10.0, priceData.RemoveOtherRatiosFromFloat(30))
	assert.True(t, decimal.NewFromInt(30).Equal(priceData.ApplyOtherRatiosToDecimal(decimal.NewFromInt(10))))

	replaced = priceData.ReplaceOtherRatios(map[string]float64{
		"zero": 0,
		"nan":  math.NaN(),
	})

	require.False(t, replaced)
	assert.Nil(t, priceData.OtherRatios())
	assert.Equal(t, 1.0, priceData.OtherRatioMultiplier())
}

func TestTaskBillingOtherFiltersHistoricalOtherRatios(t *testing.T) {
	task := makeTask(1, 1, 100, 0, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.OtherRatios = map[string]float64{
		"seconds":  2,
		"identity": 1,
		"zero":     0,
		"negative": -1,
		"nan":      math.NaN(),
		"inf":      math.Inf(1),
	}

	other := taskBillingOther(task).Snapshot()

	assert.Equal(t, 2.0, other["seconds"])
	assert.Equal(t, 1.0, other["identity"])
	assert.NotContains(t, other, "zero")
	assert.NotContains(t, other, "negative")
	assert.NotContains(t, other, "nan")
	assert.NotContains(t, other, "inf")
	assert.NotContains(t, other, "billing_mode")
	assert.NotContains(t, other, "expr_b64")
	assert.NotContains(t, other, "matched_tier")
	assert.NotContains(t, other, "usage_facts")
}

func TestTaskBillingOtherIncludesTieredSnapshotAndKeepsUsageFactsNested(t *testing.T) {
	task := makeTask(1, 1, 100, 0, BillingSourceWallet, 0)
	expression := `tier("720P", u("seconds") * 5)`
	task.PrivateData.BillingContext.TieredSnapshot = &billingexpr.BillingSnapshot{
		ExprString:    expression,
		EstimatedTier: "720P",
		UsageFacts: map[string]any{
			"resolution": "720P",
			"seconds":    5,
		},
	}

	other := taskBillingOther(task).Snapshot()

	assert.Equal(t, "tiered_expr", other["billing_mode"])
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte(expression)), other["expr_b64"])
	assert.Equal(t, "720P", other["matched_tier"])
	facts, ok := other["usage_facts"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, map[string]any{
		"resolution": "720P",
		"seconds":    5,
	}, facts)
	assert.NotContains(t, other, "resolution")
	assert.NotContains(t, other, "seconds")
}

func TestTaskBillingOtherOmitsEmptyUsageFacts(t *testing.T) {
	task := makeTask(1, 1, 100, 0, BillingSourceWallet, 0)
	expression := `tier("base", 1)`
	task.PrivateData.BillingContext.TieredSnapshot = &billingexpr.BillingSnapshot{
		ExprString:    expression,
		EstimatedTier: "base",
		UsageFacts:    map[string]any{},
	}

	other := taskBillingOther(task).Snapshot()

	assert.Equal(t, "tiered_expr", other["billing_mode"])
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte(expression)), other["expr_b64"])
	assert.Equal(t, "base", other["matched_tier"])
	assert.NotContains(t, other, "usage_facts")
}

func callLogTaskConsumption(t *testing.T, info *relaycommon.RelayInfo, task *model.Task) *model.Log {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	ctx.Set("token_name", "test_token")
	LogTaskConsumption(ctx, info, task)
	log := getLastLog(t)
	require.NotNil(t, log)
	return log
}

func TestLogTaskConsumptionIncludesTieredSnapshotUsageFacts(t *testing.T) {
	truncate(t)
	const userID, channelID = 40, 40
	seedUser(t, userID, 10_000)
	seedChannel(t, channelID)

	expression := `tier("720P", u("seconds") * 5)`
	task := makeTask(userID, channelID, 100, 0, BillingSourceWallet, 0)
	info := &relaycommon.RelayInfo{
		UserId:          userID,
		TokenId:         0,
		OriginModelName: "wan2.5-i2v-preview",
		UsingGroup:      "default",
		ChannelMeta:     &relaycommon.ChannelMeta{ChannelId: channelID},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{Action: "GENERATE"},
		PriceData: types.PriceData{
			ModelPrice:     0.02,
			Quota:          100,
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
		},
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			ExprString:    expression,
			EstimatedTier: "720P",
			UsageFacts: map[string]any{
				"resolution": "720P",
				"seconds":    5,
			},
		},
	}

	log := callLogTaskConsumption(t, info, task)

	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	assert.Equal(t, "tiered_expr", other["billing_mode"])
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte(expression)), other["expr_b64"])
	assert.Equal(t, "720P", other["matched_tier"])
	facts, ok := other["usage_facts"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "720P", facts["resolution"])
	assert.Equal(t, float64(5), facts["seconds"])
	assert.NotContains(t, other, "resolution")
	assert.NotContains(t, other, "seconds")
	assert.Contains(t, log.Content, "计算参数：")
	assert.Contains(t, log.Content, "resolution: 720P")
	assert.Contains(t, log.Content, "seconds: 5")
}

func TestLogTaskConsumptionWithoutSnapshotKeepsRatioMode(t *testing.T) {
	truncate(t)
	const userID, channelID = 41, 41
	seedUser(t, userID, 10_000)
	seedChannel(t, channelID)

	priceData := types.PriceData{
		ModelPrice:     0.02,
		Quota:          100,
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
	}
	priceData.AddOtherRatio("size", 2)
	task := makeTask(userID, channelID, 100, 0, BillingSourceWallet, 0)
	info := &relaycommon.RelayInfo{
		UserId:          userID,
		TokenId:         0,
		OriginModelName: "test-model",
		UsingGroup:      "default",
		ChannelMeta:     &relaycommon.ChannelMeta{ChannelId: channelID},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{Action: "GENERATE"},
		PriceData:       priceData,
	}

	log := callLogTaskConsumption(t, info, task)

	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	assert.Equal(t, true, other["is_task"])
	assert.Equal(t, "/v1/videos", other["request_path"])
	assert.NotContains(t, other, "billing_mode")
	assert.NotContains(t, other, "expr_b64")
	assert.NotContains(t, other, "matched_tier")
	assert.NotContains(t, other, "usage_facts")
	assert.Contains(t, log.Content, "计算参数：")
	assert.Contains(t, log.Content, "size: 2.00")
}

// Task logs distinguish jobs the client polls from requests whose HTTP call
// returned the deliverable itself, and flag results the gateway did not keep.
func TestLogTaskConsumptionMarksInlineResultsAndDiscardedArtifacts(t *testing.T) {
	for _, tc := range []struct {
		name               string
		status             model.TaskStatus
		discarded          bool
		pinnedProtocol     string
		wantSync, wantKept bool
	}{
		{"asynchronous job", model.TaskStatusNotStart, false, "", false, true},
		{"immediate result on a discarding route", model.TaskStatusSuccess, true, "", true, false},
		{"openai image request waits for an asynchronous upstream task", model.TaskStatusNotStart, false, jsplugin.ProtocolOpenAIImage, true, true},
		{"immediate failure", model.TaskStatusFailure, false, "", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			const userID, channelID = 43, 43
			seedUser(t, userID, 10_000)
			seedChannel(t, channelID)
			task := makeTask(userID, channelID, 100, 0, BillingSourceWallet, 0)
			task.Status = tc.status
			task.PrivateData.ResultDiscarded = tc.discarded
			info := &relaycommon.RelayInfo{
				UserId: userID, OriginModelName: "qwen-image-plus", UsingGroup: "default",
				ChannelMeta:   &relaycommon.ChannelMeta{ChannelId: channelID},
				TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: "text_to_image"},
				PriceData:     types.PriceData{ModelPrice: 0.03, Quota: 100, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}},
			}
			gin.SetMode(gin.TestMode)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
			ctx.Set("token_name", "test_token")
			if tc.pinnedProtocol != "" {
				ctx.Set(jsplugin.ContextKeyPinnedEndpoint, jsplugin.PinnedEndpoint{Protocol: tc.pinnedProtocol})
			}
			LogTaskConsumption(ctx, info, task)
			log := getLastLog(t)
			require.NotNil(t, log)
			var other map[string]any
			require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
			assert.Equal(t, true, other["is_task"])
			if tc.wantSync {
				assert.Equal(t, true, other["task_sync"])
			} else {
				assert.NotContains(t, other, "task_sync")
			}
			if tc.wantKept {
				assert.NotContains(t, other, "result_discarded")
			} else {
				assert.Equal(t, true, other["result_discarded"])
			}
		})
	}
}

func TestTaskBillingOtherSeparatesPluginAndRootDiagnostics(t *testing.T) {
	task := makeTask(1, 1, 100, 0, BillingSourceWallet, 0)
	task.TaskID = "task_public"
	task.PrivateData.UpstreamTaskID = "upstream-private"
	task.PrivateData.NodeName = "node-a"
	task.PrivateData.Execution = &model.TaskExecutionSnapshot{
		TaskPlugin: &model.TaskPluginSnapshot{
			Key:     "document-parser",
			Name:    "Document Parser",
			Version: "1.2.3",
			Author: &model.TaskPluginAuthorSnapshot{
				Name: "Community Author",
				URL:  "https://plugins.example/author",
			},
			APIVersion: 1,
			Generation: 42,
		},
	}

	other := taskBillingOther(task).Snapshot()

	assert.Equal(t, "task_public", other["task_id"])
	adminInfo, ok := other["admin_info"].(map[string]any)
	require.True(t, ok)
	pluginInfo, ok := adminInfo["task_plugin"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "document-parser", pluginInfo["key"])
	assert.Equal(t, "1.2.3", pluginInfo["version"])
	assert.Equal(t, map[string]any{
		"name": "Community Author",
		"url":  "https://plugins.example/author",
	}, pluginInfo["author"])

	rootInfo, ok := other["root_info"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "upstream-private", rootInfo["upstream_task_id"])
	assert.Equal(t, "node-a", rootInfo["node_name"])
	runtimeInfo, ok := rootInfo["task_plugin"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, uint64(42), runtimeInfo["generation"])
	assert.NotContains(t, runtimeInfo, "author")
}

func TestTaskBillingContextPriceDataFiltersMultiplier(t *testing.T) {
	priceData := taskBillingContextPriceData(&model.TaskBillingContext{
		OtherRatios: map[string]float64{
			"seconds":  2,
			"size":     3,
			"identity": 1,
			"zero":     0,
			"negative": -1,
			"nan":      math.NaN(),
			"inf":      math.Inf(1),
		},
	})

	require.NotNil(t, priceData)
	assert.Equal(t, 6.0, priceData.OtherRatioMultiplier())
	assert.Equal(t, map[string]float64{
		"seconds":  2,
		"size":     3,
		"identity": 1,
	}, priceData.OtherRatios())
}

// ---------------------------------------------------------------------------
// Read-back helpers
// ---------------------------------------------------------------------------

func getUserQuota(t *testing.T, id int) int {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Select("quota").Where("id = ?", id).First(&user).Error)
	return int(user.Quota)
}

func getUserUsageAccounting(t *testing.T, id int) (int, int) {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Select("used_quota", "request_count").Where("id = ?", id).First(&user).Error)
	return int(user.UsedQuota), user.RequestCount
}

func getChannelUsedQuota(t *testing.T, id int) int64 {
	t.Helper()
	var channel model.Channel
	require.NoError(t, model.DB.Select("used_quota").Where("id = ?", id).First(&channel).Error)
	return channel.UsedQuota
}

func getTokenRemainQuota(t *testing.T, id int) int {
	t.Helper()
	var token model.Token
	require.NoError(t, model.DB.Select("remain_quota").Where("id = ?", id).First(&token).Error)
	return int(token.RemainQuota)
}

func getTokenUsedQuota(t *testing.T, id int) int {
	t.Helper()
	var token model.Token
	require.NoError(t, model.DB.Select("used_quota").Where("id = ?", id).First(&token).Error)
	return int(token.UsedQuota)
}

func getSubscriptionUsed(t *testing.T, id int) int64 {
	t.Helper()
	var sub model.UserSubscription
	require.NoError(t, model.DB.Select("amount_used").Where("id = ?", id).First(&sub).Error)
	return sub.AmountUsed
}

func getTaskQuota(t *testing.T, id int64) int {
	t.Helper()
	var task model.Task
	require.NoError(t, model.DB.Select("quota").Where("id = ?", id).First(&task).Error)
	return task.Quota
}

func getMidjourneyTask(t *testing.T, id int) model.Midjourney {
	t.Helper()
	var task model.Midjourney
	require.NoError(t, model.DB.First(&task, id).Error)
	return task
}

func getLastLog(t *testing.T) *model.Log {
	t.Helper()
	var log model.Log
	err := model.LOG_DB.Order("id desc").First(&log).Error
	if err != nil {
		return nil
	}
	return &log
}

func countLogs(t *testing.T) int64 {
	t.Helper()
	var count int64
	model.LOG_DB.Model(&model.Log{}).Count(&count)
	return count
}

// ===========================================================================
// Legacy Midjourney billing tests
// ===========================================================================

func TestPrepareMidjourneyTaskBillingKeepsUnbilledMarkerClear(t *testing.T) {
	task := &model.Midjourney{Quota: 900, TokenId: 7, BillingChannelId: 8}

	prepared, err := PrepareMidjourneyTaskBilling(&relaycommon.RelayInfo{}, task, 900, false)

	require.NoError(t, err)
	assert.False(t, prepared)
	assert.Zero(t, task.Quota)
	assert.Zero(t, task.TokenId)
	assert.Zero(t, task.BillingChannelId)
}

func TestMidjourneyReservationIsAuditedWithoutInferringUnknownSubmissionFailure(t *testing.T) {
	truncate(t)
	const userID, tokenID, channelID, quota = 48, 48, 48, 300
	seedUser(t, userID, 1000)
	seedToken(t, tokenID, userID, "midjourney-unknown-submit", 1000)
	seedChannel(t, channelID)
	task := &model.Midjourney{UserId: userID, ChannelId: channelID}
	prepared, err := PrepareMidjourneyTaskBilling(&relaycommon.RelayInfo{UserId: userID, TokenId: tokenID}, task, quota, true)
	require.NoError(t, err)
	require.True(t, prepared)
	assert.EqualValues(t, 700, getUserQuota(t, userID))
	assert.EqualValues(t, 700, getTokenRemainQuota(t, tokenID))
	assert.EqualValues(t, quota, getTokenUsedQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Zero(t, usedQuota)
	assert.Zero(t, requestCount)

	stale, err := model.ListStaleReservedBillingOperations(common.GetTimestamp()+1, 10)
	require.NoError(t, err)
	require.Len(t, stale, 1)
	assert.Equal(t, task.BillingRequestID, stale[0].RequestId)
	assert.Zero(t, stale[0].TaskId)
	assert.Equal(t, model.BillingReserved, stale[0].State)
	assert.EqualValues(t, 700, getUserQuota(t, userID), "unknown upstream result must remain held for review")
}

func TestMidjourneyRequestReachabilityClassification(t *testing.T) {
	assert.False(t, MidjourneyRequestMayHaveReachedUpstream(nil))
	assert.False(t, MidjourneyRequestMayHaveReachedUpstream(errors.Join(ErrMidjourneyRequestNotSent, errors.New("invalid URL"))))
	assert.True(t, MidjourneyRequestMayHaveReachedUpstream(errors.New("connection reset after request write")))
}

func TestUnsubmittedMidjourneyReservationCanBeReleasedIdempotently(t *testing.T) {
	truncate(t)
	const userID, tokenID, channelID, quota = 45, 45, 45, 300
	seedUser(t, userID, 1000)
	seedToken(t, tokenID, userID, "midjourney-local-request-error", 1000)
	seedChannel(t, channelID)
	task := &model.Midjourney{UserId: userID, ChannelId: channelID}
	prepared, err := PrepareMidjourneyTaskBilling(&relaycommon.RelayInfo{UserId: userID, TokenId: tokenID}, task, quota, true)
	require.NoError(t, err)
	require.True(t, prepared)
	require.NoError(t, ReleaseUnsubmittedMidjourneyBilling(task.BillingRequestID))
	require.NoError(t, ReleaseUnsubmittedMidjourneyBilling(task.BillingRequestID))
	assert.EqualValues(t, 1000, getUserQuota(t, userID))
	assert.EqualValues(t, 1000, getTokenRemainQuota(t, tokenID))
	assert.Zero(t, getTokenUsedQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Zero(t, usedQuota)
	assert.Zero(t, requestCount)
	var operation model.BillingOperation
	require.NoError(t, model.DB.Where("request_id = ? AND phase = ?", task.BillingRequestID, "initial").First(&operation).Error)
	assert.Equal(t, model.BillingRefunded, operation.State)
}

func TestMidjourneyAcceptedResultSurvivesTaskInsertFailure(t *testing.T) {
	truncate(t)
	const userID, tokenID, channelID, quota = 46, 46, 46, 300
	seedUser(t, userID, 1000)
	seedToken(t, tokenID, userID, "midjourney-task-insert-failure", 1000)
	seedChannel(t, channelID)
	task := &model.Midjourney{UserId: userID, ChannelId: channelID, MjId: "upstream-accepted-46", Code: 1}
	prepared, err := PrepareMidjourneyTaskBilling(&relaycommon.RelayInfo{UserId: userID, TokenId: tokenID}, task, quota, true)
	require.NoError(t, err)
	require.True(t, prepared)
	require.NoError(t, RecordMidjourneyTaskBillingResult(task, true))
	require.NoError(t, model.DB.Exec(`CREATE TRIGGER fail_midjourney_task_insert
		BEFORE INSERT ON midjourneys BEGIN SELECT RAISE(ABORT, 'forced task insert failure'); END;`).Error)
	t.Cleanup(func() { model.DB.Exec("DROP TRIGGER IF EXISTS fail_midjourney_task_insert") })
	require.Error(t, task.Insert())
	var operation model.BillingOperation
	require.NoError(t, model.DB.Where("request_id = ? AND phase = ?", task.BillingRequestID, "initial").First(&operation).Error)
	assert.Equal(t, model.BillingReserved, operation.State)
	assert.Zero(t, operation.TaskId)
	assert.True(t, operation.UpstreamResultRecorded)
	assert.Equal(t, "upstream-accepted-46", operation.UpstreamTaskID)
	assert.Equal(t, 1, operation.UpstreamCode)
	assert.EqualValues(t, 700, getUserQuota(t, userID))
	assert.EqualValues(t, 700, getTokenRemainQuota(t, tokenID))
}

func TestMidjourneyKnownRejectionReleasesReservationExactlyOnce(t *testing.T) {
	truncate(t)
	const userID, tokenID, channelID, quota = 47, 47, 47, 300
	seedUser(t, userID, 1000)
	seedToken(t, tokenID, userID, "midjourney-known-rejection", 1000)
	seedChannel(t, channelID)
	task := &model.Midjourney{UserId: userID, ChannelId: channelID, Status: "FAILURE", FailReason: "rejected"}
	prepared, err := PrepareMidjourneyTaskBilling(&relaycommon.RelayInfo{UserId: userID, TokenId: tokenID}, task, quota, true)
	require.NoError(t, err)
	require.True(t, prepared)
	task.BillingBillable = false
	require.NoError(t, RecordMidjourneyTaskBillingResult(task, false))
	require.NoError(t, task.Insert())
	require.NoError(t, CancelMidjourneyTaskBilling(task))
	require.NoError(t, CancelMidjourneyTaskBilling(task))
	assert.EqualValues(t, 1000, getUserQuota(t, userID))
	assert.EqualValues(t, 1000, getTokenRemainQuota(t, tokenID))
	assert.Zero(t, getTokenUsedQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Zero(t, usedQuota)
	assert.Zero(t, requestCount)
	assert.Zero(t, getChannelUsedQuota(t, channelID))
	assert.Zero(t, getMidjourneyTask(t, task.Id).Quota)
	var root model.BillingOperation
	require.NoError(t, model.DB.Where("request_id = ? AND phase = ?", task.BillingRequestID, "initial").First(&root).Error)
	assert.Equal(t, model.BillingRefunded, root.State)
	var refundPhases int64
	require.NoError(t, model.DB.Model(&model.BillingOperation{}).Where("request_id = ? AND phase = ?", task.BillingRequestID, "refund").Count(&refundPhases).Error)
	assert.EqualValues(t, 1, refundPhases)
}

func TestSettleMidjourneyTaskBillingRequiresPersistedTask(t *testing.T) {
	truncate(t)

	const userID, tokenID, channelID = 49, 49, 49
	const initialUserQuota, initialTokenQuota, chargedQuota = 10000, 5000, 3000
	seedUser(t, userID, initialUserQuota)
	seedToken(t, tokenID, userID, "sk-midjourney-unpersisted", initialTokenQuota)
	seedChannel(t, channelID)

	relayInfo := &relaycommon.RelayInfo{
		UserId:    userID,
		TokenId:   tokenID,
		TokenKey:  "sk-midjourney-unpersisted",
		UserQuota: initialUserQuota,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: channelID,
		},
	}
	task := &model.Midjourney{UserId: userID, ChannelId: channelID}
	prepared, err := PrepareMidjourneyTaskBilling(relayInfo, task, chargedQuota, true)
	require.NoError(t, err)
	require.True(t, prepared)
	assert.Equal(t, initialUserQuota-chargedQuota, getUserQuota(t, userID))
	assert.Equal(t, initialTokenQuota-chargedQuota, getTokenRemainQuota(t, tokenID))

	billed, err := SettleMidjourneyTaskBilling(relayInfo, task, prepared)

	require.Error(t, err)
	assert.False(t, billed)
	assert.Equal(t, initialUserQuota-chargedQuota, getUserQuota(t, userID))
	assert.Equal(t, initialTokenQuota-chargedQuota, getTokenRemainQuota(t, tokenID))
}

func TestMidjourneyRefundRestoresEveryAccountingElementOnBillingChannel(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, billingChannelID, executionChannelID = 50, 50, 50, 51
	const initialUserQuota, initialTokenQuota, chargedQuota = 10000, 5000, 3000
	seedUser(t, userID, initialUserQuota)
	seedToken(t, tokenID, userID, "sk-midjourney", initialTokenQuota)
	seedChannel(t, billingChannelID)
	seedChannel(t, executionChannelID)

	relayInfo := &relaycommon.RelayInfo{
		UserId:     userID,
		TokenId:    tokenID,
		TokenKey:   "sk-midjourney",
		UserQuota:  initialUserQuota,
		UsingGroup: "default",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: billingChannelID,
		},
	}
	task := &model.Midjourney{
		UserId:    userID,
		Action:    "IMAGINE",
		MjId:      "mj-accounting-refund",
		ChannelId: executionChannelID,
		Progress:  "0%",
	}

	prepared, err := PrepareMidjourneyTaskBilling(relayInfo, task, chargedQuota, true)
	require.NoError(t, err)
	require.True(t, prepared)
	assert.Zero(t, task.Quota)
	assert.Equal(t, chargedQuota, task.PreparedQuota)
	assert.Equal(t, tokenID, task.TokenId)
	assert.Equal(t, billingChannelID, task.BillingChannelId)
	require.NoError(t, task.Insert())

	billed, err := SettleMidjourneyTaskBilling(relayInfo, task, prepared)
	require.NoError(t, err)
	require.True(t, billed)
	assert.Equal(t, initialUserQuota-chargedQuota, getUserQuota(t, userID))
	assert.Equal(t, initialTokenQuota-chargedQuota, getTokenRemainQuota(t, tokenID))
	persisted := getMidjourneyTask(t, task.Id)
	assert.Equal(t, chargedQuota, persisted.Quota)
	assert.Equal(t, tokenID, persisted.TokenId)
	assert.Equal(t, billingChannelID, persisted.BillingChannelId)

	assert.True(t, RefundMidjourneyQuota(ctx, task, "构图失败"))
	assert.Equal(t, initialUserQuota, getUserQuota(t, userID))
	assert.Equal(t, initialTokenQuota, getTokenRemainQuota(t, tokenID))
	assert.Zero(t, getTokenUsedQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Zero(t, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Zero(t, getChannelUsedQuota(t, billingChannelID))
	assert.Zero(t, getChannelUsedQuota(t, executionChannelID))

	persisted = getMidjourneyTask(t, task.Id)
	assert.Zero(t, persisted.Quota)
	assert.Equal(t, tokenID, persisted.TokenId)
	assert.Equal(t, billingChannelID, persisted.BillingChannelId)
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
	assert.Equal(t, chargedQuota, log.Quota)
	assert.Equal(t, tokenID, log.TokenId)
	assert.Equal(t, billingChannelID, log.ChannelId)

	assert.True(t, RefundMidjourneyQuota(ctx, task, "duplicate poll"))
	assert.Equal(t, int64(1), countLogs(t))
}

func TestSettleMidjourneyTaskBillingFundingFailureKeepsIntentAndRetries(t *testing.T) {
	truncate(t)

	const userID, tokenID, channelID = 52, 52, 52
	const initialUserQuota, initialTokenQuota, chargedQuota = 10000, 5000, 3000
	seedUser(t, userID, initialUserQuota)
	seedToken(t, tokenID, userID, "sk-midjourney-funding-failure", initialTokenQuota)
	seedChannel(t, channelID)

	relayInfo := &relaycommon.RelayInfo{
		UserId:    userID,
		TokenId:   tokenID,
		TokenKey:  "sk-midjourney-funding-failure",
		UserQuota: initialUserQuota,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: channelID,
		},
	}
	task := &model.Midjourney{UserId: userID, MjId: "mj-funding-failure", ChannelId: channelID}
	prepared, err := PrepareMidjourneyTaskBilling(relayInfo, task, chargedQuota, true)
	require.NoError(t, err)
	require.True(t, prepared)
	require.NoError(t, task.Insert())

	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_midjourney_user_update
		BEFORE UPDATE ON users
		WHEN OLD.id = 52
		BEGIN
			SELECT RAISE(ABORT, 'forced user quota failure');
		END;
	`).Error)
	t.Cleanup(func() {
		model.DB.Exec("DROP TRIGGER IF EXISTS fail_midjourney_user_update")
	})

	billed, err := SettleMidjourneyTaskBilling(relayInfo, task, prepared)

	require.Error(t, err)
	assert.False(t, billed)
	assert.Equal(t, initialUserQuota-chargedQuota, getUserQuota(t, userID))
	assert.Equal(t, initialTokenQuota-chargedQuota, getTokenRemainQuota(t, tokenID))
	persisted := getMidjourneyTask(t, task.Id)
	assert.Zero(t, persisted.Quota)
	assert.Equal(t, tokenID, persisted.TokenId)
	assert.Equal(t, channelID, persisted.BillingChannelId)
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Zero(t, usedQuota)
	assert.Zero(t, requestCount)
	assert.Zero(t, getChannelUsedQuota(t, channelID))
	assert.Zero(t, countLogs(t))
	var pending model.BillingOperation
	require.NoError(t, model.DB.Where("request_id = ? AND phase = ?", task.BillingRequestID, "initial").First(&pending).Error)
	assert.Equal(t, model.BillingSettlementRequested, pending.State)
	require.NoError(t, model.DB.Exec("DROP TRIGGER IF EXISTS fail_midjourney_user_update").Error)
	require.NoError(t, model.RecoverBillingTaskSettlement(pending.RequestId))
	require.NoError(t, model.RecoverBillingTaskSettlement(pending.RequestId))
	assert.Equal(t, initialUserQuota-chargedQuota, getUserQuota(t, userID))
	assert.Equal(t, initialTokenQuota-chargedQuota, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, chargedQuota, getMidjourneyTask(t, task.Id).Quota)
}

func TestMidjourneyReservationTokenFailureRollsBackWalletAndCanRetry(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 53, 53, 53
	const initialUserQuota, initialTokenQuota, chargedQuota = 10000, 5000, 3000
	seedUser(t, userID, initialUserQuota)
	seedToken(t, tokenID, userID, "sk-midjourney-token-failure", initialTokenQuota)
	seedChannel(t, channelID)

	relayInfo := &relaycommon.RelayInfo{
		UserId:    userID,
		TokenId:   tokenID,
		TokenKey:  "sk-midjourney-token-failure",
		UserQuota: initialUserQuota,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: channelID,
		},
	}
	task := &model.Midjourney{UserId: userID, MjId: "mj-token-failure", ChannelId: channelID}
	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_midjourney_token_update
		BEFORE UPDATE ON tokens
		WHEN OLD.id = 53
		BEGIN
			SELECT RAISE(ABORT, 'forced token quota failure');
		END;
	`).Error)
	t.Cleanup(func() {
		model.DB.Exec("DROP TRIGGER IF EXISTS fail_midjourney_token_update")
	})
	prepared, err := PrepareMidjourneyTaskBilling(relayInfo, task, chargedQuota, true)
	require.Error(t, err)
	assert.False(t, prepared)
	assert.Equal(t, initialUserQuota, getUserQuota(t, userID))
	assert.Equal(t, initialTokenQuota, getTokenRemainQuota(t, tokenID))
	assert.Zero(t, getTokenUsedQuota(t, tokenID))
	require.NoError(t, model.DB.Exec("DROP TRIGGER IF EXISTS fail_midjourney_token_update").Error)

	prepared, err = PrepareMidjourneyTaskBilling(relayInfo, task, chargedQuota, true)
	require.NoError(t, err)
	require.True(t, prepared)
	require.NoError(t, task.Insert())
	billed, err := SettleMidjourneyTaskBilling(relayInfo, task, prepared)
	require.NoError(t, err)
	assert.True(t, billed)
	assert.Equal(t, initialUserQuota-chargedQuota, getUserQuota(t, userID))
	assert.Equal(t, initialTokenQuota-chargedQuota, getTokenRemainQuota(t, tokenID))
	assert.True(t, RefundMidjourneyQuota(ctx, task, "confirmed upstream failure"))
	assert.Equal(t, initialUserQuota, getUserQuota(t, userID))
	assert.Equal(t, initialTokenQuota, getTokenRemainQuota(t, tokenID))
}

func TestMidjourneyConfirmedFailureCancelsUnappliedSettlement(t *testing.T) {
	truncate(t)
	const userID, tokenID, channelID, quota = 54, 54, 54, 300
	seedUser(t, userID, 1000)
	seedToken(t, tokenID, userID, "midjourney-pending-failure", 1000)
	seedChannel(t, channelID)

	task := &model.Midjourney{UserId: userID, MjId: "midjourney-pending-failure", ChannelId: channelID}
	prepared, err := PrepareMidjourneyTaskBilling(&relaycommon.RelayInfo{UserId: userID, TokenId: tokenID}, task, quota, true)
	require.NoError(t, err)
	require.True(t, prepared)
	require.NoError(t, task.Insert())
	requestID := task.BillingRequestID
	require.NoError(t, model.DB.Model(&model.Midjourney{}).Where("id = ?", task.Id).Updates(map[string]any{
		"status": "FAILURE", "progress": "100%", "billing_failure_confirmed": true,
		"finish_time": time.Now().UnixMilli(),
	}).Error)

	require.NoError(t, model.RecoverBillingTaskSettlement(requestID))
	require.NoError(t, model.RecoverBillingTaskSettlement(requestID))
	assert.EqualValues(t, 1000, getUserQuota(t, userID))
	assert.EqualValues(t, 1000, getTokenRemainQuota(t, tokenID))
	assert.Zero(t, getMidjourneyTask(t, task.Id).Quota)
	var root model.BillingOperation
	require.NoError(t, model.DB.Where("request_id = ? AND phase = ?", requestID, "initial").First(&root).Error)
	assert.Equal(t, model.BillingRefunded, root.State)
	var refunds int64
	require.NoError(t, model.DB.Model(&model.BillingOperation{}).Where("request_id = ? AND phase = ?", requestID, "refund").Count(&refunds).Error)
	assert.EqualValues(t, 1, refunds)
}

func TestMidjourneyRefundRecoveryRequiresConfirmedUpstreamFailure(t *testing.T) {
	truncate(t)
	now := time.Now().UnixMilli()
	unconfirmed := model.Midjourney{MjId: "mj-uncertain-failure", Status: "FAILURE", Progress: "100%", Quota: 50, FinishTime: now}
	require.NoError(t, model.DB.Create(&unconfirmed).Error)
	confirmed := model.Midjourney{MjId: "mj-confirmed-failure", Status: "FAILURE", Progress: "100%", Quota: 50, FinishTime: now, BillingFailureConfirmed: true}
	require.NoError(t, model.DB.Create(&confirmed).Error)

	tasks, err := model.ListConfirmedFailedMidjourneyTasksWithPendingRefund(10)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "mj-confirmed-failure", tasks[0].MjId)
}

func TestMidjourneyRefundTokenFailureRollsBackAndRetryIsIdempotent(t *testing.T) {
	truncate(t)
	const userID, tokenID, channelID, quota = 985, 985, 985, 30
	seedUser(t, userID, 100)
	seedToken(t, tokenID, userID, "midjourney-atomic-refund", 100)
	seedChannel(t, channelID)
	task := &model.Midjourney{UserId: userID, MjId: "midjourney-atomic-refund", ChannelId: channelID}
	info := &relaycommon.RelayInfo{UserId: userID, TokenId: tokenID, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channelID}}
	prepared, err := PrepareMidjourneyTaskBilling(info, task, quota, true)
	require.NoError(t, err)
	require.True(t, prepared)
	require.NoError(t, task.Insert())
	billed, err := SettleMidjourneyTaskBilling(info, task, prepared)
	require.NoError(t, err)
	require.True(t, billed)
	assert.EqualValues(t, 70, getUserQuota(t, userID))
	assert.EqualValues(t, 30, getChannelUsedQuota(t, channelID))

	require.NoError(t, model.DB.Exec(`CREATE TRIGGER fail_atomic_mj_refund BEFORE UPDATE ON tokens
		WHEN OLD.id = 985 BEGIN SELECT RAISE(ABORT, 'forced refund failure'); END`).Error)
	t.Cleanup(func() { model.DB.Exec("DROP TRIGGER IF EXISTS fail_atomic_mj_refund") })
	assert.False(t, RefundMidjourneyQuota(context.Background(), task, "confirmed failure"))
	assert.EqualValues(t, 70, getUserQuota(t, userID))
	assert.EqualValues(t, 30, getChannelUsedQuota(t, channelID))
	assert.Equal(t, quota, getMidjourneyTask(t, task.Id).Quota)
	require.NoError(t, model.DB.Exec("DROP TRIGGER IF EXISTS fail_atomic_mj_refund").Error)
	assert.True(t, RefundMidjourneyQuota(context.Background(), task, "confirmed failure"))
	assert.True(t, RefundMidjourneyQuota(context.Background(), task, "duplicate"))
	assert.EqualValues(t, 100, getUserQuota(t, userID))
	assert.Zero(t, getChannelUsedQuota(t, channelID))
	assert.Zero(t, getMidjourneyTask(t, task.Id).Quota)
	assert.EqualValues(t, 1, countLogs(t))
}

func TestPrepareMidjourneyTaskBillingRejectsSubscriptionBeforeCharge(t *testing.T) {
	task := &model.Midjourney{Quota: 900, TokenId: 7, BillingChannelId: 8}
	relayInfo := &relaycommon.RelayInfo{BillingSource: BillingSourceSubscription, SubscriptionId: 1}

	prepared, err := PrepareMidjourneyTaskBilling(relayInfo, task, 900, true)

	require.Error(t, err)
	assert.False(t, prepared)
	assert.Zero(t, task.Quota)
	assert.Zero(t, task.TokenId)
	assert.Zero(t, task.BillingChannelId)
}

func TestRefundMidjourneyQuotaUsesLegacyChannelFallbackWithoutTokenAdjustment(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 54, 54, 54
	const walletAfterCharge, tokenQuota, chargedQuota = 7000, 5000, 3000
	seedUser(t, userID, walletAfterCharge)
	seedToken(t, tokenID, userID, "sk-midjourney-legacy", tokenQuota)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, 0, chargedQuota, 1)
	task := &model.Midjourney{
		UserId:    userID,
		MjId:      "mj-legacy-fallback",
		Action:    "IMAGINE",
		ChannelId: channelID,
		Quota:     chargedQuota,
		TokenId:   0,
		Progress:  "0%",
	}
	require.NoError(t, task.Insert())

	assert.True(t, RefundMidjourneyQuota(ctx, task, "legacy failure"))

	assert.Equal(t, walletAfterCharge+chargedQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenQuota, getTokenRemainQuota(t, tokenID))
	assert.Zero(t, getTokenUsedQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Zero(t, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Zero(t, getChannelUsedQuota(t, channelID))
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, channelID, log.ChannelId)
	assert.Zero(t, log.TokenId)
}

// ===========================================================================
// RefundTaskQuota tests
// ===========================================================================

func TestRefundTaskQuota_Wallet(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 1, 1, 1
	const initQuota, preConsumed = 10000, 3000
	const tokenRemain = 5000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-test-key", tokenRemain)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, tokenID, preConsumed, 1)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	require.NoError(t, model.DB.Create(task).Error)

	assert.True(t, RefundTaskQuota(ctx, task, "task failed: upstream error"))

	// User quota should increase by preConsumed
	assert.Equal(t, initQuota+preConsumed, getUserQuota(t, userID))

	// Token remain_quota should increase, used_quota should decrease
	assert.Equal(t, tokenRemain+preConsumed, getTokenRemainQuota(t, tokenID))
	assert.Zero(t, getTokenUsedQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Zero(t, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Zero(t, getChannelUsedQuota(t, channelID))

	// A refund log should be created
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
	assert.Equal(t, preConsumed, log.Quota)
	assert.Equal(t, "test-model", log.ModelName)
	assert.Zero(t, task.Quota)
	assert.Zero(t, getTaskQuota(t, task.ID))
}

func TestRefundTaskQuota_Subscription(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID, subID = 2, 2, 2, 1
	const preConsumed = 2000
	const subTotal, subUsed int64 = 100000, 50000
	const tokenRemain = 8000

	seedUser(t, userID, 0)
	seedToken(t, tokenID, userID, "sk-sub-key", tokenRemain)
	seedChannel(t, channelID)
	seedSubscription(t, subID, userID, subTotal, subUsed)
	seedChargedAccounting(t, userID, channelID, tokenID, preConsumed, 1)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceSubscription, subID)
	require.NoError(t, model.DB.Create(task).Error)

	assert.True(t, RefundTaskQuota(ctx, task, "subscription task failed"))

	// Subscription used should decrease by preConsumed
	assert.Equal(t, subUsed-int64(preConsumed), getSubscriptionUsed(t, subID))

	// Token should also be refunded
	assert.Equal(t, tokenRemain+preConsumed, getTokenRemainQuota(t, tokenID))
	assert.Zero(t, getTokenUsedQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Zero(t, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Zero(t, getChannelUsedQuota(t, channelID))

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
	assert.Zero(t, getTaskQuota(t, task.ID))
}

func TestRefundTaskQuota_ZeroQuota(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID = 3
	seedUser(t, userID, 5000)

	task := makeTask(userID, 0, 0, 0, BillingSourceWallet, 0)

	assert.True(t, RefundTaskQuota(ctx, task, "zero quota task"))

	// No change to user quota
	assert.Equal(t, 5000, getUserQuota(t, userID))

	// No log created
	assert.Equal(t, int64(0), countLogs(t))
}

func TestRefundTaskQuota_NoToken(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 4, 4
	const initQuota, preConsumed = 10000, 1500

	seedUser(t, userID, initQuota)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, 0, preConsumed, 1)

	task := makeTask(userID, channelID, preConsumed, 0, BillingSourceWallet, 0) // TokenId=0
	require.NoError(t, model.DB.Create(task).Error)

	assert.True(t, RefundTaskQuota(ctx, task, "no token task failed"))

	// User quota refunded
	assert.Equal(t, initQuota+preConsumed, getUserQuota(t, userID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Zero(t, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Zero(t, getChannelUsedQuota(t, channelID))

	// Log created
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
	assert.Zero(t, getTaskQuota(t, task.ID))
}

func TestRefundTaskQuota_FundingFailureKeepsAccountingAndPendingMarker(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID, preConsumed = 5, 5, 1200
	seedUser(t, userID, 5000)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, 0, preConsumed, 1)
	task := makeTask(userID, channelID, preConsumed, 0, BillingSourceSubscription, 9999)
	task.Status = model.TaskStatusFailure
	require.NoError(t, model.DB.Create(task).Error)

	assert.False(t, RefundTaskQuota(ctx, task, "subscription missing"))
	assert.Equal(t, 5000, getUserQuota(t, userID))
	assert.Equal(t, preConsumed, task.Quota)
	assert.Equal(t, preConsumed, getTaskQuota(t, task.ID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Equal(t, preConsumed, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, int64(preConsumed), getChannelUsedQuota(t, channelID))
	assert.Equal(t, int64(0), countLogs(t))
}

// ===========================================================================
// RecalculateTaskQuota tests
// ===========================================================================

func TestRecalculate_PositiveDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 10, 10, 10
	const initQuota, preConsumed = 10000, 2000
	const actualQuota = 3000 // under-charged by 1000
	const tokenRemain = 5000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-recalc-pos", tokenRemain)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, tokenID, preConsumed, 1)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	require.NoError(t, model.DB.Create(task).Error)

	RecalculateTaskQuota(ctx, task, actualQuota, "adaptor adjustment")

	// User quota should decrease by the delta (1000 additional charge)
	assert.Equal(t, initQuota-(actualQuota-preConsumed), getUserQuota(t, userID))

	// Token should also be charged the delta
	assert.Equal(t, tokenRemain-(actualQuota-preConsumed), getTokenRemainQuota(t, tokenID))
	assert.Equal(t, actualQuota, getTokenUsedQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Equal(t, actualQuota, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, int64(actualQuota), getChannelUsedQuota(t, channelID))

	// task.Quota should be updated to actualQuota
	assert.Equal(t, actualQuota, task.Quota)

	// Log type should be Consume (additional charge)
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeConsume, log.Type)
	assert.Equal(t, actualQuota-preConsumed, log.Quota)
}

func TestRecalculate_NegativeDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 11, 11, 11
	const initQuota, preConsumed = 10000, 5000
	const actualQuota = 3000 // over-charged by 2000
	const tokenRemain = 5000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-recalc-neg", tokenRemain)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, tokenID, preConsumed, 1)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	require.NoError(t, model.DB.Create(task).Error)

	RecalculateTaskQuota(ctx, task, actualQuota, "adaptor adjustment")

	// User quota should increase by abs(delta) = 2000 (refund overpayment)
	assert.Equal(t, initQuota+(preConsumed-actualQuota), getUserQuota(t, userID))

	// Token should be refunded the difference
	assert.Equal(t, tokenRemain+(preConsumed-actualQuota), getTokenRemainQuota(t, tokenID))
	assert.Equal(t, actualQuota, getTokenUsedQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Equal(t, actualQuota, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, int64(actualQuota), getChannelUsedQuota(t, channelID))

	// task.Quota updated
	assert.Equal(t, actualQuota, task.Quota)

	// Log type should be Refund
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
	assert.Equal(t, preConsumed-actualQuota, log.Quota)
}

func TestRecalculate_ZeroDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID = 12
	const initQuota, preConsumed = 10000, 3000

	seedUser(t, userID, initQuota)

	task := makeTask(userID, 0, preConsumed, 0, BillingSourceWallet, 0)

	RecalculateTaskQuota(ctx, task, preConsumed, "exact match")

	// No change to user quota
	assert.Equal(t, initQuota, getUserQuota(t, userID))

	// No log created (delta is zero)
	assert.Equal(t, int64(0), countLogs(t))
}

func TestRecalculate_ActualQuotaZero(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, preConsumed = 13, 5000
	const initQuota = 10000

	seedUser(t, userID, initQuota)

	task := makeTask(userID, 0, preConsumed, 0, BillingSourceWallet, 0)
	require.NoError(t, model.DB.Create(task).Error)

	RecalculateTaskQuota(ctx, task, 0, "zero actual")

	assert.Equal(t, initQuota+preConsumed, getUserQuota(t, userID))
	assert.Zero(t, task.Quota)
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
	assert.Equal(t, preConsumed, log.Quota)
}

func TestRecalculate_RejectsNegativeActualQuota(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, preConsumed = 34, 5000
	const initQuota = 10000
	seedUser(t, userID, initQuota)
	task := makeTask(userID, 0, preConsumed, 0, BillingSourceWallet, 0)

	RecalculateTaskQuota(ctx, task, -1, "invalid negative actual")

	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, preConsumed, task.Quota)
	assert.Equal(t, int64(0), countLogs(t))
}

func TestRecalculate_Subscription_NegativeDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID, subID = 14, 14, 14, 2
	const preConsumed = 5000
	const actualQuota = 2000 // over-charged by 3000
	const subTotal, subUsed int64 = 100000, 50000
	const tokenRemain = 8000

	seedUser(t, userID, 0)
	seedToken(t, tokenID, userID, "sk-sub-recalc", tokenRemain)
	seedChannel(t, channelID)
	seedSubscription(t, subID, userID, subTotal, subUsed)
	seedChargedAccounting(t, userID, channelID, tokenID, preConsumed, 1)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceSubscription, subID)
	require.NoError(t, model.DB.Create(task).Error)

	RecalculateTaskQuota(ctx, task, actualQuota, "subscription over-charge")

	// Subscription used should decrease by delta (refund 3000)
	assert.Equal(t, subUsed-int64(preConsumed-actualQuota), getSubscriptionUsed(t, subID))

	// Token refunded
	assert.Equal(t, tokenRemain+(preConsumed-actualQuota), getTokenRemainQuota(t, tokenID))
	assert.Equal(t, actualQuota, getTokenUsedQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Equal(t, actualQuota, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, int64(actualQuota), getChannelUsedQuota(t, channelID))

	assert.Equal(t, actualQuota, task.Quota)

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

// ===========================================================================
// CAS + Billing integration tests
// Simulates the flow in updateVideoSingleTask (service/task_polling.go)
// ===========================================================================

// simulatePollBilling reproduces the CAS + billing logic from updateVideoSingleTask.
// It takes a persisted task (already in DB), applies the new status, and performs
// the conditional update + billing exactly as the polling loop does.
func simulatePollBilling(ctx context.Context, task *model.Task, newStatus model.TaskStatus, actualQuota int) {
	snap := task.Snapshot()

	shouldRefund := false
	shouldSettle := false
	quota := task.Quota

	task.Status = newStatus
	switch string(newStatus) {
	case model.TaskStatusSuccess:
		task.Progress = "100%"
		task.FinishTime = 9999
		shouldSettle = true
	case model.TaskStatusFailure:
		task.Progress = "100%"
		task.FinishTime = 9999
		task.FailReason = "upstream error"
		if quota != 0 {
			shouldRefund = true
		}
	default:
		task.Progress = "50%"
	}

	isDone := task.Status == model.TaskStatus(model.TaskStatusSuccess) || task.Status == model.TaskStatus(model.TaskStatusFailure)
	if isDone && snap.Status != task.Status {
		won, err := task.UpdateWithStatus(snap.Status)
		if err != nil {
			shouldRefund = false
			shouldSettle = false
		} else if !won {
			shouldRefund = false
			shouldSettle = false
		}
	} else if !snap.Equal(task.Snapshot()) {
		_, _ = task.UpdateWithStatus(snap.Status)
	}

	if shouldSettle && actualQuota > 0 {
		RecalculateTaskQuota(ctx, task, actualQuota, "test settle")
	}
	if shouldRefund {
		RefundTaskQuota(ctx, task, task.FailReason)
	}
}

func TestCASGuardedRefund_Win(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 20, 20, 20
	const initQuota, preConsumed = 10000, 4000
	const tokenRemain = 6000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-cas-refund-win", tokenRemain)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, tokenID, preConsumed, 1)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	require.NoError(t, model.DB.Create(task).Error)

	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusFailure), 0)

	// CAS wins: task in DB should now be FAILURE
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusFailure, reloaded.Status)
	assert.Zero(t, reloaded.Quota)

	// Refund should have happened
	assert.Equal(t, initQuota+preConsumed, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain+preConsumed, getTokenRemainQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Zero(t, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Zero(t, getChannelUsedQuota(t, channelID))

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

func TestCASGuardedRefund_Lose(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 21, 21, 21
	const initQuota, preConsumed = 10000, 4000
	const tokenRemain = 6000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-cas-refund-lose", tokenRemain)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, tokenID, preConsumed, 1)

	// Create task with IN_PROGRESS in DB
	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	require.NoError(t, model.DB.Create(task).Error)

	// Simulate another process already transitioning to FAILURE
	model.DB.Model(&model.Task{}).Where("id = ?", task.ID).Update("status", model.TaskStatusFailure)

	// Our process still has the old in-memory state (IN_PROGRESS) and tries to transition
	// task.Status is still IN_PROGRESS in the snapshot
	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusFailure), 0)

	// CAS lost: user quota should NOT change (no double refund)
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Equal(t, preConsumed, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, int64(preConsumed), getChannelUsedQuota(t, channelID))

	// No billing log should be created
	assert.Equal(t, int64(0), countLogs(t))
}

func TestCASGuardedSettle_Win(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 22, 22, 22
	const initQuota, preConsumed = 10000, 5000
	const actualQuota = 3000 // over-charged, should get partial refund
	const tokenRemain = 8000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-cas-settle-win", tokenRemain)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, tokenID, preConsumed, 1)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	require.NoError(t, model.DB.Create(task).Error)

	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusSuccess), actualQuota)

	// CAS wins: task should be SUCCESS
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusSuccess, reloaded.Status)

	// Settlement should refund the over-charge (5000 - 3000 = 2000 back to user)
	assert.Equal(t, initQuota+(preConsumed-actualQuota), getUserQuota(t, userID))
	assert.Equal(t, tokenRemain+(preConsumed-actualQuota), getTokenRemainQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Equal(t, actualQuota, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, int64(actualQuota), getChannelUsedQuota(t, channelID))

	// task.Quota should be updated to actualQuota
	assert.Equal(t, actualQuota, task.Quota)
}

func TestNonTerminalUpdate_NoBilling(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 23, 23
	const initQuota, preConsumed = 10000, 3000

	seedUser(t, userID, initQuota)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, 0, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	task.Progress = "20%"
	require.NoError(t, model.DB.Create(task).Error)

	// Simulate a non-terminal poll update (still IN_PROGRESS, progress changed)
	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusInProgress), 0)

	// User quota should NOT change
	assert.Equal(t, initQuota, getUserQuota(t, userID))

	// No billing log
	assert.Equal(t, int64(0), countLogs(t))

	// Task progress should be updated in DB
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.Equal(t, "50%", reloaded.Progress)
}

// ===========================================================================
// Mock adaptor for settleTaskBillingOnComplete tests
// ===========================================================================

type mockAdaptor struct {
	adjustReturn int
}

func (m *mockAdaptor) Init(_ *relaycommon.RelayInfo) {}
func (m *mockAdaptor) FetchTask(string, string, *model.Task, string) (*http.Response, error) {
	return nil, nil
}
func (m *mockAdaptor) ParseTaskResult(*model.Task, *http.Response, []byte) (*relaycommon.TaskInfo, error) {
	return nil, nil
}
func (m *mockAdaptor) AdjustBillingOnComplete(_ *model.Task, _ *relaycommon.TaskInfo) int {
	return m.adjustReturn
}

// ===========================================================================
// PerCallBilling tests — settleTaskBillingOnComplete
// ===========================================================================

func TestSettle_PerCallBilling_SkipsAdaptorAdjust(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 30, 30, 30
	const initQuota, preConsumed = 10000, 5000
	const tokenRemain = 8000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-percall-adaptor", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.PerCallBilling = true

	adaptor := &mockAdaptor{adjustReturn: 2000}
	taskResult := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}

	settled := settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)

	// Per-call: no adjustment despite adaptor returning 2000
	assert.False(t, settled)
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, preConsumed, task.Quota)
	assert.Equal(t, int64(0), countLogs(t))
}

func TestSettle_PerCallBilling_SkipsTotalTokens(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 31, 31, 31
	const initQuota, preConsumed = 10000, 4000
	const tokenRemain = 7000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-percall-tokens", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.PerCallBilling = true

	adaptor := &mockAdaptor{adjustReturn: 0}
	taskResult := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess, TotalTokens: 9999}

	settled := settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)

	// Per-call: no recalculation by tokens
	assert.False(t, settled)
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, preConsumed, task.Quota)
	assert.Equal(t, int64(0), countLogs(t))
}

func TestSettle_NonPerCallBilling_AppliesAdaptorAdjustment(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 32, 32, 32
	const initQuota, preConsumed = 10000, 5000
	const adaptorQuota = 3000
	const tokenRemain = 8000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-nonpercall-adj", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	// PerCallBilling defaults to false
	require.NoError(t, model.DB.Create(task).Error)

	adaptor := &mockAdaptor{adjustReturn: adaptorQuota}
	taskResult := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}

	settled := settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)

	// Non-per-call: adaptor adjustment applies (refund 2000)
	assert.True(t, settled)
	assert.Equal(t, initQuota+(preConsumed-adaptorQuota), getUserQuota(t, userID))
	assert.Equal(t, tokenRemain+(preConsumed-adaptorQuota), getTokenRemainQuota(t, tokenID))
	assert.Equal(t, adaptorQuota, task.Quota)

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

func TestSettle_TieredEvaluationFailureKeepsPreConsumedCharge(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, preConsumed = 33, 5_000
	const initialQuota = 10_000
	seedUser(t, userID, initialQuota)

	task := makeTask(userID, 0, preConsumed, 0, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.TieredSnapshot = &billingexpr.BillingSnapshot{
		ExprString:       `tier("broken",`,
		ExprHash:         billingexpr.ExprHashString(`tier("broken",`),
		GroupRatio:       1,
		QuotaPerUnit:     1_000,
		ExprVersion:      1,
		TaskUsageBilling: true,
	}

	settled := settleTaskBillingOnComplete(ctx, &mockAdaptor{}, task, &relaycommon.TaskInfo{Status: model.TaskStatusFailure})

	assert.True(t, settled)
	assert.Equal(t, preConsumed, task.Quota)
	assert.Equal(t, initialQuota, getUserQuota(t, userID))
	assert.Equal(t, int64(0), countLogs(t))
}

func TestSettle_TieredFailureReturnsFalseForCallerRefund(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID = 37
	const initialQuota, preConsumed = 10_000, 25
	seedUser(t, userID, initialQuota)

	expression := `tier("base", u("seconds") + u("clips") * 10)`
	task := makeTask(userID, 0, preConsumed, 0, BillingSourceWallet, 0)
	task.Status = model.TaskStatusFailure
	task.PrivateData.BillingContext.TieredSnapshot = &billingexpr.BillingSnapshot{
		ExprString:       expression,
		ExprHash:         billingexpr.ExprHashString(expression),
		GroupRatio:       1,
		QuotaPerUnit:     1,
		ExprVersion:      1,
		TaskUsageBilling: true,
		UsageFacts:       map[string]any{"seconds": float64(5), "clips": float64(2)},
		EstimatedTier:    "base",
	}
	require.NoError(t, model.DB.Create(task).Error)

	settled := settleTaskBillingOnComplete(
		ctx,
		&mockAdaptor{adjustReturn: 1},
		task,
		&relaycommon.TaskInfo{Status: model.TaskStatusFailure, UsageFacts: map[string]any{"seconds": float64(8)}},
	)

	assert.False(t, settled)
	assert.Equal(t, preConsumed, task.Quota)
	assert.Equal(t, map[string]any{"seconds": float64(5), "clips": float64(2)}, task.PrivateData.BillingContext.TieredSnapshot.UsageFacts)
	assert.Equal(t, "base", task.PrivateData.BillingContext.TieredSnapshot.EstimatedTier)
	assert.Equal(t, initialQuota, getUserQuota(t, userID))
	assert.Equal(t, int64(0), countLogs(t))
}

func TestSettle_TieredSuccessStillRecomputes(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID = 38
	const initialQuota, preConsumed = 10_000, 50
	seedUser(t, userID, initialQuota)

	expression := `tier("base", u("seconds") + u("clips") * 10)`
	task := makeTask(userID, 0, preConsumed, 0, BillingSourceWallet, 0)
	task.Status = model.TaskStatusSuccess
	task.PrivateData.BillingContext.TieredSnapshot = &billingexpr.BillingSnapshot{
		ExprString:       expression,
		ExprHash:         billingexpr.ExprHashString(expression),
		GroupRatio:       1,
		QuotaPerUnit:     1,
		ExprVersion:      1,
		TaskUsageBilling: true,
		UsageFacts:       map[string]any{"seconds": float64(5), "clips": float64(2)},
		EstimatedTier:    "base",
	}
	require.NoError(t, model.DB.Create(task).Error)

	settled := settleTaskBillingOnComplete(
		ctx,
		&mockAdaptor{adjustReturn: 1},
		task,
		&relaycommon.TaskInfo{Status: model.TaskStatusSuccess, UsageFacts: map[string]any{"seconds": float64(8)}},
	)

	assert.True(t, settled)
	assert.Equal(t, 28, task.Quota)
	assert.Equal(t, map[string]any{"seconds": float64(8), "clips": float64(2)}, task.PrivateData.BillingContext.TieredSnapshot.UsageFacts)
	assert.Equal(t, "base", task.PrivateData.BillingContext.TieredSnapshot.EstimatedTier)
	assert.Equal(t, initialQuota+(preConsumed-28), getUserQuota(t, userID))

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	assert.Equal(t, "tiered_expr", other["billing_mode"])
	assert.Equal(t, "base", other["matched_tier"])
	facts, ok := other["usage_facts"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, map[string]any{"seconds": float64(8), "clips": float64(2)}, facts)
}

func TestSettle_TieredUsageFactsMergeCompletionOverSubmission(t *testing.T) {
	tests := []struct {
		name            string
		completionFacts map[string]any
		expectedQuota   int
		expectedFacts   map[string]any
	}{
		{
			name:          "submission facts survive missing completion facts",
			expectedQuota: 25,
			expectedFacts: map[string]any{"seconds": float64(5), "clips": float64(2)},
		},
		{
			name:            "completion facts partially override submission facts",
			completionFacts: map[string]any{"seconds": float64(8)},
			expectedQuota:   28,
			expectedFacts:   map[string]any{"seconds": float64(8), "clips": float64(2)},
		},
		{
			name:            "completion facts fully override submission facts",
			completionFacts: map[string]any{"seconds": float64(8), "clips": float64(3)},
			expectedQuota:   38,
			expectedFacts:   map[string]any{"seconds": float64(8), "clips": float64(3)},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			truncate(t)
			const userID = 34
			const initialQuota = 10_000
			const preConsumed = 50
			seedUser(t, userID, initialQuota)

			expression := `tier("base", u("seconds") + u("clips") * 10)`
			submissionFacts := map[string]any{"seconds": float64(5), "clips": float64(2)}
			task := makeTask(userID, 0, preConsumed, 0, BillingSourceWallet, 0)
			task.PrivateData.BillingContext.TieredSnapshot = &billingexpr.BillingSnapshot{
				ExprString:       expression,
				ExprHash:         billingexpr.ExprHashString(expression),
				GroupRatio:       1,
				QuotaPerUnit:     1,
				ExprVersion:      1,
				TaskUsageBilling: true,
				UsageFacts:       submissionFacts,
				EstimatedTier:    "base",
			}
			require.NoError(t, model.DB.Create(task).Error)

			settled := settleTaskBillingOnComplete(
				context.Background(),
				&mockAdaptor{},
				task,
				&relaycommon.TaskInfo{Status: model.TaskStatusSuccess, UsageFacts: testCase.completionFacts},
			)

			assert.True(t, settled)
			assert.Equal(t, testCase.expectedQuota, task.Quota)
			assert.Equal(t, map[string]any{"seconds": float64(5), "clips": float64(2)}, submissionFacts)
			require.NotNil(t, task.PrivateData.BillingContext.TieredSnapshot)
			assert.Equal(t, testCase.expectedFacts, task.PrivateData.BillingContext.TieredSnapshot.UsageFacts)
			assert.Equal(t, "base", task.PrivateData.BillingContext.TieredSnapshot.EstimatedTier)

			log := getLastLog(t)
			require.NotNil(t, log)
			var other map[string]any
			require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
			assert.Equal(t, "tiered_expr", other["billing_mode"])
			assert.Equal(t, "base", other["matched_tier"])
			facts, ok := other["usage_facts"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, testCase.expectedFacts, facts)
			assert.NotContains(t, other, "seconds")
			assert.NotContains(t, other, "clips")
		})
	}
}

func TestSettle_TieredSnapshotWriteBackUsesSettledFactsAndMatchedTier(t *testing.T) {
	truncate(t)
	const userID = 36
	const initialQuota = 10_000
	const preConsumed = 25
	seedUser(t, userID, initialQuota)

	expression := `u("resolution") == "1080P" ? tier("1080P", u("seconds") * 10) : tier("720P", u("seconds") * 5)`
	task := makeTask(userID, 0, preConsumed, 0, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.TieredSnapshot = &billingexpr.BillingSnapshot{
		ExprString:       expression,
		ExprHash:         billingexpr.ExprHashString(expression),
		GroupRatio:       1,
		QuotaPerUnit:     1,
		ExprVersion:      1,
		TaskUsageBilling: true,
		UsageFacts:       map[string]any{"resolution": "720P", "seconds": float64(5)},
		EstimatedTier:    "720P",
	}
	require.NoError(t, model.DB.Create(task).Error)

	settled := settleTaskBillingOnComplete(
		context.Background(),
		&mockAdaptor{},
		task,
		&relaycommon.TaskInfo{
			Status:     model.TaskStatusSuccess,
			UsageFacts: map[string]any{"resolution": "1080P"},
		},
	)

	require.True(t, settled)
	snap := task.PrivateData.BillingContext.TieredSnapshot
	require.NotNil(t, snap)
	assert.Equal(t, map[string]any{"resolution": "1080P", "seconds": float64(5)}, snap.UsageFacts)
	assert.Equal(t, "1080P", snap.EstimatedTier)
	assert.Equal(t, 50, task.Quota)

	log := getLastLog(t)
	require.NotNil(t, log)
	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	assert.Equal(t, "tiered_expr", other["billing_mode"])
	assert.Equal(t, "1080P", other["matched_tier"])
	facts, ok := other["usage_facts"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "1080P", facts["resolution"])
	assert.Equal(t, float64(5), facts["seconds"])
	assert.NotContains(t, other, "resolution")
	assert.NotContains(t, other, "seconds")
}

func TestSettle_TokenRecalcFallsBackToCompletionTokens(t *testing.T) {
	previousRatios := ratio_setting.ModelRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"test-model":1}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousRatios))
	})

	tests := []struct {
		name             string
		totalTokens      int
		completionTokens int
		wantSettled      bool
		wantQuota        int
	}{
		{
			name:             "total tokens still win when both are present",
			totalTokens:      80,
			completionTokens: 20,
			wantSettled:      true,
			wantQuota:        80,
		},
		{
			name:             "completion tokens trigger recalc when total is zero",
			totalTokens:      0,
			completionTokens: 80,
			wantSettled:      true,
			wantQuota:        80,
		},
		{
			name:        "neither token count skips recalc",
			wantSettled: false,
			wantQuota:   50,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			truncate(t)
			const userID, tokenID, channelID = 35, 35, 35
			const initialQuota, preConsumed, tokenRemain = 10_000, 50, 8_000
			seedUser(t, userID, initialQuota)
			seedToken(t, tokenID, userID, "sk-completion-fallback", tokenRemain)
			seedChannel(t, channelID)

			task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
			require.NoError(t, model.DB.Create(task).Error)
			settled := settleTaskBillingOnComplete(
				context.Background(),
				&mockAdaptor{},
				task,
				&relaycommon.TaskInfo{
					Status:           model.TaskStatusSuccess,
					TotalTokens:      testCase.totalTokens,
					CompletionTokens: testCase.completionTokens,
				},
			)

			assert.Equal(t, testCase.wantSettled, settled)
			assert.Equal(t, testCase.wantQuota, task.Quota)
		})
	}
}
