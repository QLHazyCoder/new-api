package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setRegistrationAffiliateSettingsForTest(t *testing.T, newUserQuota, inviteeQuota, inviterQuota int, complianceConfirmed bool) {
	t.Helper()
	paymentSetting := operation_setting.GetPaymentSetting()
	oldNewUserQuota := common.QuotaForNewUser
	oldInviteeQuota := common.QuotaForInvitee
	oldInviterQuota := common.QuotaForInviter
	oldConfirmed := paymentSetting.ComplianceConfirmed
	oldTermsVersion := paymentSetting.ComplianceTermsVersion

	common.QuotaForNewUser = newUserQuota
	common.QuotaForInvitee = inviteeQuota
	common.QuotaForInviter = inviterQuota
	paymentSetting.ComplianceConfirmed = complianceConfirmed
	if complianceConfirmed {
		paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	} else {
		paymentSetting.ComplianceTermsVersion = ""
	}

	t.Cleanup(func() {
		common.QuotaForNewUser = oldNewUserQuota
		common.QuotaForInvitee = oldInviteeQuota
		common.QuotaForInviter = oldInviterQuota
		paymentSetting.ComplianceConfirmed = oldConfirmed
		paymentSetting.ComplianceTermsVersion = oldTermsVersion
	})
}

func findAffiliateRewardEventsForTest(t *testing.T) []AffiliateRewardEvent {
	t.Helper()
	var events []AffiliateRewardEvent
	require.NoError(t, DB.Order("id ASC").Find(&events).Error)
	return events
}

func affiliateRewardIdempotencyKeyForTest(namespace, sourceId string) string {
	digest := sha256.Sum256([]byte(namespace + "\x00" + sourceId))
	return namespace + ":" + hex.EncodeToString(digest[:])
}

func TestInsertWithTxCountsInvitationWhenRegistrationRewardDisabled(t *testing.T) {
	truncateTables(t)
	setRegistrationAffiliateSettingsForTest(t, 0, 100, 250, false)

	inviter := User{Id: 501, Username: "count_only_inviter", Status: common.UserStatusEnabled, AffCode: "count_only"}
	require.NoError(t, DB.Create(&inviter).Error)
	invitee := User{Username: "count_only_invitee", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return invitee.InsertWithTx(tx, inviter.Id)
	}))

	var storedInviter User
	require.NoError(t, DB.First(&storedInviter, inviter.Id).Error)
	assert.Equal(t, 1, storedInviter.AffCount)
	assert.Zero(t, storedInviter.AffQuota)
	assert.Zero(t, storedInviter.AffHistoryQuota)

	var storedInvitee User
	require.NoError(t, DB.First(&storedInvitee, invitee.Id).Error)
	assert.Equal(t, inviter.Id, storedInvitee.InviterId)
	assert.Zero(t, storedInvitee.Quota)
	assert.Empty(t, findAffiliateRewardEventsForTest(t))
}

func TestInsertWithTxPersistsRegistrationRewardAndLedgerAtomically(t *testing.T) {
	truncateTables(t)
	setRegistrationAffiliateSettingsForTest(t, 500, 100, 250, true)

	inviter := User{Id: 511, Username: "reward_inviter", Status: common.UserStatusEnabled, AffCode: "reward_inviter"}
	require.NoError(t, DB.Create(&inviter).Error)
	invitee := User{Username: "reward_invitee", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return invitee.InsertWithTx(tx, inviter.Id)
	}))

	var storedInviter User
	require.NoError(t, DB.First(&storedInviter, inviter.Id).Error)
	assert.Equal(t, 1, storedInviter.AffCount)
	assert.Equal(t, 250, storedInviter.AffQuota)
	assert.Equal(t, 250, storedInviter.AffHistoryQuota)

	var storedInvitee User
	require.NoError(t, DB.First(&storedInvitee, invitee.Id).Error)
	assert.Equal(t, inviter.Id, storedInvitee.InviterId)
	assert.Equal(t, 600, storedInvitee.Quota)

	events := findAffiliateRewardEventsForTest(t)
	require.Len(t, events, 1)
	event := events[0]
	assert.Equal(t, inviter.Id, event.InviterId)
	assert.Equal(t, invitee.Id, event.InviteeId)
	assert.Equal(t, AffiliateRewardEventTypeRegistration, event.EventType)
	assert.Equal(t, AffiliateRewardSourceTypeUser, event.SourceType)
	assert.Equal(t, strconv.Itoa(invitee.Id), event.SourceId)
	assert.Equal(t, 250, int(event.RewardQuota))
	assert.Equal(t, 250, int(event.AffQuotaDelta))
	assert.Zero(t, event.UserQuotaDelta)
	require.NotNil(t, event.IdempotencyKey)
	assert.Equal(t, affiliateRewardIdempotencyKeyForTest("registration_inviter", strconv.Itoa(invitee.Id)), *event.IdempotencyKey)
}

func TestInsertWithTxRollsBackInvitationAccountingWithOuterTransaction(t *testing.T) {
	truncateTables(t)
	setRegistrationAffiliateSettingsForTest(t, 0, 100, 250, true)

	inviter := User{Id: 521, Username: "rollback_inviter", Status: common.UserStatusEnabled, AffCode: "rollback_inviter"}
	require.NoError(t, DB.Create(&inviter).Error)
	invitee := User{Username: "rollback_invitee", Status: common.UserStatusEnabled}
	errRollback := errors.New("rollback registration")
	err := DB.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, invitee.InsertWithTx(tx, inviter.Id))
		return errRollback
	})
	require.ErrorIs(t, err, errRollback)

	var inviteeCount int64
	require.NoError(t, DB.Model(&User{}).Where("username = ?", invitee.Username).Count(&inviteeCount).Error)
	assert.Zero(t, inviteeCount)
	var storedInviter User
	require.NoError(t, DB.First(&storedInviter, inviter.Id).Error)
	assert.Zero(t, storedInviter.AffCount)
	assert.Zero(t, storedInviter.AffQuota)
	assert.Zero(t, storedInviter.AffHistoryQuota)
	assert.Empty(t, findAffiliateRewardEventsForTest(t))
}

func TestCompleteTopUpRollsBackBalancesWhenLedgerIdempotencyConflicts(t *testing.T) {
	truncateTables(t)
	setTopUpInviteRewardForPaymentGuardTest(t, 10, true)

	insertUserForPaymentGuardTest(t, 531, 0)
	insertUserForPaymentGuardTest(t, 532, 0, 531)
	insertTopUpForPaymentGuardTest(t, "ledger-conflict", 532, PaymentProviderEpay)
	idempotencyKey := affiliateRewardIdempotencyKeyForTest("topup", "ledger-conflict")
	require.NoError(t, DB.Create(&AffiliateRewardEvent{
		InviterId:      531,
		InviteeId:      532,
		EventType:      AffiliateRewardEventTypeTopUp,
		SourceType:     AffiliateRewardSourceTypeTopUp,
		SourceId:       "ledger-conflict",
		IdempotencyKey: &idempotencyKey,
	}).Error)

	result, err := CompleteTopUp(CompleteTopUpOptions{
		TradeNo:                 "ledger-conflict",
		ExpectedPaymentProvider: PaymentProviderEpay,
	})
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, "ledger-conflict"))
	assert.Zero(t, getUserQuotaForPaymentGuardTest(t, 532))
	affQuota, affHistory := getUserAffiliateQuotaForPaymentGuardTest(t, 531)
	assert.Zero(t, affQuota)
	assert.Zero(t, affHistory)
	assert.Len(t, findAffiliateRewardEventsForTest(t), 1)
}

func TestCompleteTopUpUsesFixedLengthIdempotencyKeyForMaximumTradeNumber(t *testing.T) {
	truncateTables(t)
	setTopUpInviteRewardForPaymentGuardTest(t, 10, true)

	tradeNo := strings.Repeat("t", 255)
	insertUserForPaymentGuardTest(t, 533, 0)
	insertUserForPaymentGuardTest(t, 534, 0, 533)
	insertTopUpForPaymentGuardTest(t, tradeNo, 534, PaymentProviderEpay)

	result, err := CompleteTopUp(CompleteTopUpOptions{
		TradeNo:                 tradeNo,
		ExpectedPaymentProvider: PaymentProviderEpay,
	})
	require.NoError(t, err)
	require.False(t, result.AlreadyCompleted)
	events := findAffiliateRewardEventsForTest(t)
	require.Len(t, events, 1)
	assert.Equal(t, tradeNo, events[0].SourceId)
	require.NotNil(t, events[0].IdempotencyKey)
	assert.Equal(t, affiliateRewardIdempotencyKeyForTest("topup", tradeNo), *events[0].IdempotencyKey)
	assert.Len(t, *events[0].IdempotencyKey, len("topup:")+sha256.Size*2)

	result, err = CompleteTopUp(CompleteTopUpOptions{
		TradeNo:                 tradeNo,
		ExpectedPaymentProvider: PaymentProviderEpay,
	})
	require.NoError(t, err)
	require.True(t, result.AlreadyCompleted)
	assert.Len(t, findAffiliateRewardEventsForTest(t), 1)
}

func TestTransferAffQuotaToQuotaPersistsBalancedLedgerEvent(t *testing.T) {
	truncateTables(t)
	transferQuota := int(common.QuotaPerUnit)
	user := User{
		Id:       541,
		Username: "affiliate_transfer",
		Status:   common.UserStatusEnabled,
		Quota:    100,
		AffQuota: transferQuota + 50,
		AffCode:  "affiliate_transfer",
	}
	require.NoError(t, DB.Create(&user).Error)

	require.NoError(t, user.TransferAffQuotaToQuota(transferQuota))
	assert.Equal(t, 100+transferQuota, user.Quota)
	assert.Equal(t, 50, user.AffQuota)

	events := findAffiliateRewardEventsForTest(t)
	require.Len(t, events, 1)
	event := events[0]
	assert.Equal(t, user.Id, event.InviterId)
	assert.Zero(t, event.InviteeId)
	assert.Equal(t, AffiliateRewardEventTypeQuotaTransfer, event.EventType)
	assert.Equal(t, int64(transferQuota), event.BaseQuota)
	assert.Equal(t, -int64(transferQuota), event.AffQuotaDelta)
	assert.Equal(t, int64(transferQuota), event.UserQuotaDelta)
	assert.Nil(t, event.IdempotencyKey)
}

func TestReconcileAffiliateCountsRepeatedlyConvergesWithoutCreatingRewardHistory(t *testing.T) {
	truncateTables(t)

	inviter := User{Id: 551, Username: "backfill_inviter", Status: common.UserStatusEnabled, AffCode: "backfill_inviter", AffCount: 99}
	activeInvitee := User{Id: 552, Username: "backfill_active", Status: common.UserStatusEnabled, AffCode: "backfill_active", InviterId: inviter.Id}
	deletedInvitee := User{Id: 553, Username: "backfill_deleted", Status: common.UserStatusEnabled, AffCode: "backfill_deleted", InviterId: inviter.Id}
	unrelated := User{Id: 554, Username: "backfill_unrelated", Status: common.UserStatusEnabled, AffCode: "backfill_unrelated", AffCount: 42}
	require.NoError(t, DB.Create(&inviter).Error)
	require.NoError(t, DB.Create(&activeInvitee).Error)
	require.NoError(t, DB.Create(&deletedInvitee).Error)
	require.NoError(t, DB.Delete(&deletedInvitee).Error)
	require.NoError(t, DB.Create(&unrelated).Error)

	require.NoError(t, ReconcileAffiliateCounts())
	var storedInviter User
	require.NoError(t, DB.First(&storedInviter, inviter.Id).Error)
	assert.Equal(t, 2, storedInviter.AffCount)
	var storedUnrelated User
	require.NoError(t, DB.First(&storedUnrelated, unrelated.Id).Error)
	assert.Zero(t, storedUnrelated.AffCount)
	assert.Empty(t, findAffiliateRewardEventsForTest(t))

	require.NoError(t, DB.Model(&User{}).Where("id = ?", inviter.Id).Update("aff_count", 7).Error)
	legacyInvitee := User{Id: 555, Username: "backfill_legacy", Status: common.UserStatusEnabled, AffCode: "backfill_legacy", InviterId: inviter.Id}
	require.NoError(t, DB.Create(&legacyInvitee).Error)
	require.NoError(t, ReconcileAffiliateCounts())
	require.NoError(t, DB.First(&storedInviter, inviter.Id).Error)
	assert.Equal(t, 3, storedInviter.AffCount)
	require.NoError(t, ReconcileAffiliateCounts())
	require.NoError(t, DB.First(&storedInviter, inviter.Id).Error)
	assert.Equal(t, 3, storedInviter.AffCount)
	assert.Empty(t, findAffiliateRewardEventsForTest(t))
}

func TestReconcileAffiliateCountsAndRegistrationIncrementConverge(t *testing.T) {
	truncateTables(t)
	setRegistrationAffiliateSettingsForTest(t, 0, 0, 0, true)

	inviter := User{Id: 571, Username: "concurrent_inviter", Status: common.UserStatusEnabled, AffCode: "concurrent_inviter", AffCount: 9}
	existingInvitee := User{Id: 572, Username: "concurrent_existing", Status: common.UserStatusEnabled, AffCode: "concurrent_existing", InviterId: inviter.Id}
	require.NoError(t, DB.Create(&inviter).Error)
	require.NoError(t, DB.Create(&existingInvitee).Error)

	newInvitee := User{Username: "concurrent_new", Status: common.UserStatusEnabled}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		errs <- ReconcileAffiliateCounts()
	}()
	go func() {
		defer wg.Done()
		<-start
		errs <- DB.Transaction(func(tx *gorm.DB) error {
			return newInvitee.InsertWithTx(tx, inviter.Id)
		})
	}()
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	var storedInviter User
	require.NoError(t, DB.First(&storedInviter, inviter.Id).Error)
	assert.Equal(t, 2, storedInviter.AffCount)
	assert.Empty(t, findAffiliateRewardEventsForTest(t))
}

func TestHardDeleteReconcilesInviterCount(t *testing.T) {
	truncateTables(t)
	inviter := User{Id: 561, Username: "delete_inviter", Status: common.UserStatusEnabled, AffCode: "delete_inviter", AffCount: 1}
	invitee := User{Id: 562, Username: "delete_invitee", Status: common.UserStatusEnabled, AffCode: "delete_invitee", InviterId: inviter.Id}
	require.NoError(t, DB.Create(&inviter).Error)
	require.NoError(t, DB.Create(&invitee).Error)

	require.NoError(t, HardDeleteUserById(invitee.Id))
	var storedInviter User
	require.NoError(t, DB.First(&storedInviter, inviter.Id).Error)
	assert.Zero(t, storedInviter.AffCount)
}
