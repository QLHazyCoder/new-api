package model

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type preBillingMidjourney struct {
	ID         int `gorm:"primaryKey"`
	UserId     int
	MjId       string
	Status     string
	Progress   string
	SubmitTime int64
	Quota      int
}

func TestBillingTransactionRetriesMySQLDeadlock(t *testing.T) {
	attempts := 0
	err := withBillingTransaction(context.Background(), func(*gorm.DB) error {
		attempts++
		if attempts < billingTransactionAttempts {
			return &mysql.MySQLError{Number: 1213, Message: "deadlock found when trying to get lock"}
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, billingTransactionAttempts, attempts)
}

func TestBillingSettlementConcurrentHotAccount(t *testing.T) {
	db := openSensitiveWordMatrixDB(t, "sqlite", "")
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(8)
	previous, previousLog := DB, LOG_DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	DB, LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	initCol()
	t.Cleanup(func() {
		DB, LOG_DB = previous, previousLog
		common.SetDatabaseTypes(oldMain, oldLog)
		initCol()
	})
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &Channel{}, &BillingOperation{}))
	user := User{Username: "billing-hot-account", Quota: 1_000_000, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	token := Token{UserId: user.Id, Key: "billing-hot-token", Status: common.TokenStatusEnabled, RemainQuota: 1_000_000, ExpiredTime: -1}
	require.NoError(t, db.Create(&token).Error)
	channel := Channel{Id: 901, Name: "billing-hot-channel", Key: "billing-hot"}
	require.NoError(t, db.Create(&channel).Error)

	const operationCount = 4
	requestIDs := make([]string, operationCount)
	for i := range requestIDs {
		requestIDs[i] = fmt.Sprintf("hot-account-%d", i)
		op := &BillingOperation{RequestId: requestIDs[i], UserId: user.Id, ChannelId: channel.Id,
			TokenId: token.Id, FundingSource: "wallet", PreConsumed: 100}
		require.NoError(t, CreateBillingReservation(op, func(tx *gorm.DB) error {
			reserved, err := TryReserveUserQuotaTx(tx, user.Id, 100)
			require.NoError(t, err)
			require.True(t, reserved)
			reserved, err = TryReserveTokenQuotaTx(tx, token.Id, 100, false)
			require.NoError(t, err)
			require.True(t, reserved)
			op.WalletAmount, op.TokenAmount = 100, 100
			return nil
		}))
		require.NoError(t, RequestBillingSettlement(requestIDs[i], 25, 0, 25, 125, true))
	}

	var waitGroup sync.WaitGroup
	errorsCh := make(chan error, operationCount)
	for _, requestID := range requestIDs {
		requestID := requestID
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			errorsCh <- RecoverBillingSettlement(requestID)
		}()
	}
	waitGroup.Wait()
	close(errorsCh)
	for err := range errorsCh {
		require.NoError(t, err)
	}

	var persistedUser User
	require.NoError(t, db.First(&persistedUser, user.Id).Error)
	require.EqualValues(t, 999_500, persistedUser.Quota)
	require.EqualValues(t, 500, persistedUser.UsedQuota)
	require.EqualValues(t, operationCount, persistedUser.RequestCount)
	var persistedToken Token
	require.NoError(t, db.First(&persistedToken, token.Id).Error)
	require.EqualValues(t, 999_500, persistedToken.RemainQuota)
	require.EqualValues(t, 500, persistedToken.UsedQuota)
	var persistedChannel Channel
	require.NoError(t, db.First(&persistedChannel, channel.Id).Error)
	require.EqualValues(t, 500, persistedChannel.UsedQuota)
	var settled int64
	require.NoError(t, db.Model(&BillingOperation{}).Where("phase = ? AND state = ?", "settle", BillingSettled).Count(&settled).Error)
	require.EqualValues(t, operationCount, settled)
}

func TestBillingLegacyRootConcurrentInitialization(t *testing.T) {
	db := openSensitiveWordMatrixDB(t, "sqlite", "")
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(8)
	previous, previousLog := DB, LOG_DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	DB, LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	initCol()
	t.Cleanup(func() {
		DB, LOG_DB = previous, previousLog
		common.SetDatabaseTypes(oldMain, oldLog)
		initCol()
	})
	require.NoError(t, db.AutoMigrate(&User{}, &BillingOperation{}))
	user := User{Username: "billing-legacy-root-race", Quota: 1_000, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)

	const operationCount = 4
	var waitGroup sync.WaitGroup
	errorsCh := make(chan error, operationCount)
	for i := 0; i < operationCount; i++ {
		phase := fmt.Sprintf("legacy-consume:%d", i)
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			errorsCh <- ApplyLegacyBillingDelta("legacy-root-race", phase, user.Id, 0, 0, 0, 10, 0, false, false)
		}()
	}
	waitGroup.Wait()
	close(errorsCh)
	for err := range errorsCh {
		require.NoError(t, err)
	}

	var rootCount int64
	require.NoError(t, db.Model(&BillingOperation{}).Where("request_id = ? AND phase = ?", "legacy-root-race", "initial").Count(&rootCount).Error)
	require.EqualValues(t, 1, rootCount)
	var phaseCount int64
	require.NoError(t, db.Model(&BillingOperation{}).Where("request_id = ? AND phase LIKE ?", "legacy-root-race", "legacy-consume:%").Count(&phaseCount).Error)
	require.EqualValues(t, operationCount, phaseCount)
	require.EqualValues(t, 960, getUserQuotaFromDB(t, user.Id))
}

func (preBillingMidjourney) TableName() string { return "midjourneys" }

func TestBillingOperationDatabaseMatrix(t *testing.T) {
	for _, driver := range []struct {
		name    string
		dialect common.DatabaseType
		dsn     string
	}{
		{"sqlite", common.DatabaseTypeSQLite, ""},
		{"mysql", common.DatabaseTypeMySQL, os.Getenv("TEST_MYSQL_DSN")},
		{"postgres", common.DatabaseTypePostgreSQL, os.Getenv("TEST_POSTGRES_DSN")},
	} {
		t.Run(driver.name, func(t *testing.T) {
			if driver.name != "sqlite" && driver.dsn == "" {
				t.Skip("isolated TEST_" + driver.name + "_DSN is not configured")
			}
			db := openSensitiveWordMatrixDB(t, driver.name, driver.dsn)
			previous, previousLog := DB, LOG_DB
			oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
			DB, LOG_DB = db, db
			common.SetDatabaseTypes(driver.dialect, driver.dialect)
			initCol()
			t.Cleanup(func() {
				DB, LOG_DB = previous, previousLog
				common.SetDatabaseTypes(oldMain, oldLog)
				initCol()
			})
			// Simulate upgrade from a database that predates the operation table.
			require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &Channel{}, &Task{}, &preBillingMidjourney{}))
			legacyMidjourney := preBillingMidjourney{UserId: 999, MjId: "before-p32", Status: "IN_PROGRESS", Progress: "10%", SubmitTime: 1, Quota: 25}
			require.NoError(t, db.Create(&legacyMidjourney).Error)
			user := User{Username: "billing-upgrade", Quota: 100, Status: common.UserStatusEnabled}
			require.NoError(t, db.Create(&user).Error)
			token := Token{UserId: user.Id, Key: "billing-upgrade-key", Status: common.TokenStatusEnabled,
				RemainQuota: 100, ExpiredTime: -1}
			require.NoError(t, db.Create(&token).Error)
			require.NoError(t, db.AutoMigrate(&BillingOperation{}, &Midjourney{}))
			require.NoError(t, db.AutoMigrate(&BillingOperation{}, &Midjourney{}), "migration must tolerate repeated startup")
			require.True(t, db.Migrator().HasColumn(&Midjourney{}, "BillingReviewPending"))
			require.True(t, db.Migrator().HasColumn(&Midjourney{}, "BillingFailureConfirmed"))
			require.True(t, db.Migrator().HasColumn(&Midjourney{}, "BillingRequestID"))
			require.True(t, db.Migrator().HasColumn(&BillingOperation{}, "UpstreamTaskID"))
			require.True(t, db.Migrator().HasColumn(&BillingOperation{}, "UpstreamResultRecorded"))
			var migratedMidjourney Midjourney
			require.NoError(t, db.First(&migratedMidjourney, legacyMidjourney.ID).Error)
			require.Equal(t, "before-p32", migratedMidjourney.MjId)
			require.False(t, migratedMidjourney.BillingReviewPending)
			require.False(t, migratedMidjourney.BillingFailureConfirmed)

			op := &BillingOperation{RequestId: "test-billing-upgrade", UserId: user.Id, ChannelId: 301, TokenId: token.Id,
				FundingSource: "wallet", PreConsumed: 30}
			require.NoError(t, CreateBillingReservation(op, func(tx *gorm.DB) error {
				wallet, err := TryReserveUserQuotaTx(tx, user.Id, 30)
				require.NoError(t, err)
				require.True(t, wallet)
				tokenReserved, err := TryReserveTokenQuotaTx(tx, token.Id, 30, false)
				require.NoError(t, err)
				require.True(t, tokenReserved)
				op.WalletAmount, op.TokenAmount = 30, 30
				return nil
			}))
			require.Error(t, CreateBillingReservation(&BillingOperation{RequestId: op.RequestId, UserId: user.Id}, func(*gorm.DB) error {
				t.Fatal("duplicate reservation must not reach the financial callback")
				return nil
			}))
			require.EqualValues(t, 70, getUserQuotaFromDB(t, user.Id))
			require.EqualValues(t, 70, getTokenFromDB(t, token.Id).RemainQuota)
			require.NoError(t, RequestBillingRefund(op.RequestId))
			require.NoError(t, RefundBillingOperation(op.RequestId))
			require.NoError(t, RefundBillingOperation(op.RequestId))
			require.EqualValues(t, 100, getUserQuotaFromDB(t, user.Id))
			require.EqualValues(t, 100, getTokenFromDB(t, token.Id).RemainQuota)

			// A missing channel must roll the completed charge, usage and state
			// back together; after it is created, a replay remains idempotent.
			op = &BillingOperation{RequestId: "test-billing-settlement", UserId: user.Id, ChannelId: 301,
				TokenId: token.Id, FundingSource: "wallet", PreConsumed: 30}
			require.NoError(t, CreateBillingReservation(op, func(tx *gorm.DB) error {
				wallet, err := TryReserveUserQuotaTx(tx, user.Id, 30)
				require.NoError(t, err)
				require.True(t, wallet)
				tokenReserved, err := TryReserveTokenQuotaTx(tx, token.Id, 30, false)
				require.NoError(t, err)
				require.True(t, tokenReserved)
				op.WalletAmount, op.TokenAmount = 30, 30
				return nil
			}))
			require.Error(t, ApplyBillingAdjustment(op.RequestId, "settle", 0, 0, 0, false, true, 30, true))
			var used User
			require.NoError(t, db.First(&used, user.Id).Error)
			require.Zero(t, used.UsedQuota)
			require.Zero(t, used.RequestCount)
			require.EqualValues(t, 70, used.Quota)
			channel := Channel{Id: 301, Name: "billing-test", Key: "test"}
			require.NoError(t, db.Create(&channel).Error)
			require.NoError(t, ApplyBillingAdjustment(op.RequestId, "settle", 0, 0, 0, false, true, 30, true))
			require.NoError(t, ApplyBillingAdjustment(op.RequestId, "settle", 0, 0, 0, false, true, 30, true))
			require.NoError(t, db.First(&used, user.Id).Error)
			require.EqualValues(t, 30, used.UsedQuota)
			require.EqualValues(t, 1, used.RequestCount)
			require.NoError(t, db.First(&channel, channel.Id).Error)
			require.EqualValues(t, 30, channel.UsedQuota)

			ordinaryIntent := &BillingOperation{RequestId: "test-billing-ordinary-intent", UserId: user.Id,
				ChannelId: channel.Id, TokenId: token.Id, FundingSource: "wallet", PreConsumed: 10}
			require.NoError(t, CreateBillingReservation(ordinaryIntent, func(tx *gorm.DB) error {
				reserved, err := TryReserveUserQuotaTx(tx, user.Id, 10)
				require.NoError(t, err)
				require.True(t, reserved)
				reserved, err = TryReserveTokenQuotaTx(tx, token.Id, 10, false)
				require.NoError(t, err)
				require.True(t, reserved)
				ordinaryIntent.WalletAmount, ordinaryIntent.TokenAmount = 10, 10
				return nil
			}))
			require.NoError(t, RequestBillingSettlement(ordinaryIntent.RequestId, 0, 0, 0, 10, true))
			var pendingOrdinary BillingOperation
			require.NoError(t, db.Where("request_id = ? AND phase = ?", ordinaryIntent.RequestId, "initial").First(&pendingOrdinary).Error)
			require.Equal(t, BillingSettlementRequested, pendingOrdinary.State)
			require.NoError(t, RecoverBillingSettlement(ordinaryIntent.RequestId))
			require.NoError(t, RecoverBillingSettlement(ordinaryIntent.RequestId))
			require.NoError(t, db.Where("request_id = ? AND phase = ?", ordinaryIntent.RequestId, "initial").First(&pendingOrdinary).Error)
			require.Equal(t, BillingSettled, pendingOrdinary.State)

			settleUser := User{Username: "billing-task", AffCode: "billing-task-" + driver.name, Quota: 100, Status: common.UserStatusEnabled}
			require.NoError(t, db.Create(&settleUser).Error)
			settleToken := Token{UserId: settleUser.Id, Key: "billing-task-key", Status: common.TokenStatusEnabled,
				RemainQuota: 100, ExpiredTime: -1}
			require.NoError(t, db.Create(&settleToken).Error)
			taskOp := &BillingOperation{RequestId: "test-billing-task-settlement", UserId: settleUser.Id,
				ChannelId: channel.Id, TokenId: settleToken.Id, FundingSource: "wallet", PreConsumed: 30}
			require.NoError(t, CreateBillingReservation(taskOp, func(tx *gorm.DB) error {
				wallet, err := TryReserveUserQuotaTx(tx, settleUser.Id, 30)
				require.NoError(t, err)
				require.True(t, wallet)
				tokenReserved, err := TryReserveTokenQuotaTx(tx, settleToken.Id, 30, false)
				require.NoError(t, err)
				require.True(t, tokenReserved)
				taskOp.WalletAmount, taskOp.TokenAmount = 30, 30
				return nil
			}))
			task := &Task{TaskID: "known-result", UserId: settleUser.Id, ChannelId: channel.Id,
				Quota: 24, Status: TaskStatusSuccess, Progress: "100%", FinishTime: common.GetTimestamp()}
			require.NoError(t, InsertTaskWithBillingIntent(context.Background(), taskOp.RequestId, task, 24))
			assertTaskPending := func() {
				var persisted Task
				require.NoError(t, db.First(&persisted, task.ID).Error)
				require.Equal(t, TaskStatusBillingPending, persisted.Status)
				require.Zero(t, persisted.Quota)
				var pending BillingOperation
				require.NoError(t, db.Where("request_id = ? AND phase = ?", taskOp.RequestId, "initial").First(&pending).Error)
				require.Equal(t, BillingSettlementRequested, pending.State)
				require.EqualValues(t, task.ID, pending.TaskId)
			}
			assertTaskPending()
			callback := "test:billing_task_settlement_failure_" + driver.name
			failWallet := true
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
				if failWallet && tx.Statement.Table == "users" {
					tx.AddError(errors.New("forced task settlement wallet failure"))
				}
			}))
			require.Error(t, RecoverBillingTaskSettlement(taskOp.RequestId))
			assertTaskPending()
			var pendingUser User
			require.NoError(t, db.First(&pendingUser, settleUser.Id).Error)
			require.EqualValues(t, 70, pendingUser.Quota)
			require.Zero(t, pendingUser.UsedQuota)
			failWallet = false
			require.NoError(t, RecoverBillingTaskSettlement(taskOp.RequestId))
			require.NoError(t, RecoverBillingTaskSettlement(taskOp.RequestId))
			require.NoError(t, db.Callback().Update().Remove(callback))
			var settledTask Task
			require.NoError(t, db.First(&settledTask, task.ID).Error)
			require.Equal(t, TaskStatus(TaskStatusSuccess), settledTask.Status)
			require.Equal(t, 24, settledTask.Quota)
			require.NoError(t, db.First(&pendingUser, settleUser.Id).Error)
			require.EqualValues(t, 76, pendingUser.Quota)
			require.EqualValues(t, 24, pendingUser.UsedQuota)
			require.EqualValues(t, 1, pendingUser.RequestCount)
			require.EqualValues(t, 76, getTokenFromDB(t, settleToken.Id).RemainQuota)
			var settledRoot BillingOperation
			require.NoError(t, db.Where("request_id = ? AND phase = ?", taskOp.RequestId, "initial").First(&settledRoot).Error)
			require.Equal(t, BillingSettled, settledRoot.State)
			var settlePhases int64
			require.NoError(t, db.Model(&BillingOperation{}).Where("request_id = ? AND phase = ?", taskOp.RequestId, "settle").Count(&settlePhases).Error)
			require.EqualValues(t, 1, settlePhases)

			mjUser := User{Username: "billing-midjourney-" + driver.name, AffCode: "billing-midjourney-" + driver.name,
				Quota: 100, Status: common.UserStatusEnabled}
			require.NoError(t, db.Create(&mjUser).Error)
			mjToken := Token{UserId: mjUser.Id, Key: "billing-midjourney-key-" + driver.name, Status: common.TokenStatusEnabled,
				RemainQuota: 100, ExpiredTime: -1}
			require.NoError(t, db.Create(&mjToken).Error)
			mjChannel := Channel{Id: 302, Name: "billing-midjourney-" + driver.name, Key: "test"}
			require.NoError(t, db.Create(&mjChannel).Error)
			mjRequestID := "midjourney:database-matrix-" + driver.name
			require.NoError(t, CreateMidjourneyBillingReservation(mjRequestID, mjUser.Id, mjChannel.Id, mjToken.Id, 10))
			mjTask := Midjourney{UserId: mjUser.Id, MjId: "known-result-" + driver.name, ChannelId: mjChannel.Id,
				BillingChannelId: mjChannel.Id, TokenId: mjToken.Id, BillingRequestID: mjRequestID,
				PreparedQuota: 10, BillingBillable: true, Progress: "0%"}
			require.NoError(t, mjTask.Insert())
			mjCallback := "test:midjourney_settlement_failure_" + driver.name
			failMidjourneyWallet := true
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register(mjCallback, func(tx *gorm.DB) {
				if failMidjourneyWallet && tx.Statement.Table == "users" {
					tx.AddError(errors.New("forced Midjourney wallet failure"))
				}
			}))
			_, err := ChargeMidjourneyTask(&mjTask, 10, mjToken.Id)
			require.Error(t, err)
			var pendingMidjourney BillingOperation
			require.NoError(t, db.Where("request_id = ? AND phase = ?", mjRequestID, "initial").First(&pendingMidjourney).Error)
			require.Equal(t, BillingSettlementRequested, pendingMidjourney.State)
			require.EqualValues(t, 90, getUserQuotaFromDB(t, mjUser.Id), "the reservation is durable before settlement")
			require.EqualValues(t, 90, getTokenFromDB(t, mjToken.Id).RemainQuota)
			failMidjourneyWallet = false
			require.NoError(t, RecoverBillingTaskSettlement(mjRequestID))
			require.NoError(t, RecoverBillingTaskSettlement(mjRequestID))
			require.NoError(t, db.Callback().Update().Remove(mjCallback))
			require.EqualValues(t, 90, getUserQuotaFromDB(t, mjUser.Id))
			require.EqualValues(t, 90, getTokenFromDB(t, mjToken.Id).RemainQuota)
			var settledMidjourney Midjourney
			require.NoError(t, db.First(&settledMidjourney, mjTask.Id).Error)
			require.Equal(t, 10, settledMidjourney.Quota)
			refunded, err := RefundMidjourneyTask(&mjTask)
			require.NoError(t, err)
			require.True(t, refunded)
			refunded, err = RefundMidjourneyTask(&mjTask)
			require.NoError(t, err)
			require.False(t, refunded)
			require.EqualValues(t, 100, getUserQuotaFromDB(t, mjUser.Id))
			require.EqualValues(t, 100, getTokenFromDB(t, mjToken.Id).RemainQuota)
		})
	}
}
