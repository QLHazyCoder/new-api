package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

func midjourneyBillingRequestID(id int) string {
	return fmt.Sprintf("midjourney:%d", id)
}

func CreateMidjourneyBillingReservation(requestID string, userID, channelID, tokenID, quota int) error {
	if !billingOperationKeyValid(requestID, "initial") || userID <= 0 || channelID < 0 || tokenID < 0 || quota <= 0 || quota > common.MaxChargeQuota {
		return errors.New("invalid Midjourney billing reservation")
	}
	tokenAmount := int64(0)
	if tokenID > 0 {
		tokenAmount = int64(quota)
	}
	operation := &BillingOperation{
		RequestId: requestID, UserId: userID, ChannelId: channelID, TokenId: tokenID,
		FundingSource: "midjourney_wallet", WalletAmount: int64(quota), TokenAmount: tokenAmount,
		PreConsumed: int64(quota), Actual: int64(quota),
	}
	return CreateBillingReservation(operation, func(tx *gorm.DB) error {
		reserved, err := TryReserveUserQuotaTx(tx, userID, int64(quota))
		if err != nil {
			return err
		}
		if !reserved {
			return ErrBillingWalletInsufficient
		}
		if tokenID > 0 {
			var token Token
			if err := lockForUpdate(tx).Select("id", "user_id", "unlimited_quota").Where("id = ?", tokenID).First(&token).Error; err != nil {
				return err
			}
			if token.UserId != userID {
				return errors.New("Midjourney billing token owner mismatch")
			}
			reserved, err = TryReserveTokenQuotaTx(tx, tokenID, int64(quota), token.UnlimitedQuota)
			if err != nil {
				return err
			}
			if !reserved {
				return ErrBillingTokenInsufficient
			}
		}
		return nil
	})
}

func RecordMidjourneySubmissionResult(requestID, upstreamTaskID string, upstreamCode int, billable bool) error {
	if !billingOperationKeyValid(requestID, "initial") || len(upstreamTaskID) > 191 {
		return errors.New("invalid Midjourney submission result")
	}
	state := BillingReserved
	if !billable {
		state = BillingRefundRequested
	}
	result := DB.Model(&BillingOperation{}).
		Where("request_id = ? AND phase = ? AND state = ? AND task_id = ? AND funding_source = ?", requestID, "initial", BillingReserved, 0, "midjourney_wallet").
		Updates(map[string]any{"upstream_task_id": upstreamTaskID, "upstream_code": upstreamCode,
			"upstream_result_recorded": true, "state": state, "updated_at": common.GetTimestamp()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var existing BillingOperation
	if err := DB.Where("request_id = ? AND phase = ?", requestID, "initial").First(&existing).Error; err != nil {
		return err
	}
	if existing.UpstreamResultRecorded && existing.UpstreamTaskID == upstreamTaskID && existing.UpstreamCode == upstreamCode && existing.State == state {
		return nil
	}
	return errors.New("Midjourney submission result conflicts with its billing intent")
}

// ChargeMidjourneyTask applies a previously persisted settlement intent.
func ChargeMidjourneyTask(task *Midjourney, quota, tokenID int) (bool, error) {
	if task == nil || task.Id <= 0 || task.UserId <= 0 || quota <= 0 {
		return false, errors.New("invalid Midjourney charge")
	}
	requestID := task.BillingRequestID
	if requestID == "" {
		requestID = midjourneyBillingRequestID(task.Id)
	}
	var billed bool
	err := withBillingTransaction(context.Background(), func(tx *gorm.DB) error {
		var existing BillingOperation
		found := lockForUpdate(tx).Where("request_id = ? AND phase = ?", requestID, "initial").Limit(1).Find(&existing)
		if found.Error != nil {
			return found.Error
		}
		if found.RowsAffected == 0 {
			return errors.New("Midjourney task has no durable settlement intent")
		}
		var current Midjourney
		if err := lockForUpdate(tx).Where("id = ?", task.Id).First(&current).Error; err != nil {
			return err
		}
		if current.UserId != task.UserId {
			return errors.New("Midjourney billing owner changed")
		}
		if existing.FundingSource != "midjourney_wallet" || existing.UserId != current.UserId || existing.TokenId != tokenID ||
			existing.ChannelId != current.GetBillingChannelId() || existing.TaskId != int64(current.Id) ||
			existing.WalletAmount != int64(quota) || existing.TokenAmount != tokenAmountForMidjourney(quota, tokenID) ||
			existing.PreConsumed != int64(quota) {
			return errors.New("Midjourney settlement intent does not match task")
		}
		if existing.State == BillingSettled {
			if current.Quota == quota && existing.Actual == int64(quota) {
				return nil
			}
			return errors.New("Midjourney task has a different billing state")
		}
		if existing.State != BillingSettlementRequested || existing.Actual != int64(quota) {
			return errors.New("Midjourney task is not ready for settlement")
		}
		if current.Quota != 0 {
			return errors.New("Midjourney task already has a legacy billing marker")
		}
		channelID := current.GetBillingChannelId()
		if err := lockBillingUser(tx, current.UserId); err != nil {
			return err
		}
		if tokenID > 0 {
			var token Token
			if err := lockForUpdate(tx).Select("id", "user_id").Where("id = ?", tokenID).First(&token).Error; err != nil {
				return err
			}
			if token.UserId != current.UserId {
				return errors.New("Midjourney settlement token owner changed")
			}
		}
		if err := recordBillingUsageTx(tx, current.UserId, channelID, int64(quota), true); err != nil {
			return err
		}
		current.Quota, current.TokenId = quota, tokenID
		if err := tx.Model(&current).Updates(map[string]any{
			"quota": quota, "token_id": tokenID, "billing_channel_id": channelID,
		}).Error; err != nil {
			return err
		}
		now := common.GetTimestamp()
		tokenAmount := int64(0)
		if tokenID > 0 {
			tokenAmount = int64(quota)
		}
		if err := tx.Create(&BillingOperation{RequestId: requestID, Phase: "settle",
			State: BillingSettled, UserId: current.UserId, ChannelId: channelID, TokenId: tokenID,
			FundingSource: "midjourney_wallet", WalletAmount: int64(quota), TokenAmount: tokenAmount,
			Actual: int64(quota), UsageCounted: true, TaskId: int64(task.Id), CreatedAt: now, UpdatedAt: now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&existing).Updates(map[string]any{
			"state": BillingSettled, "pre_consumed": int64(quota), "usage_counted": true, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		billed = true
		return nil
	})
	if err == nil && billed {
		task.Quota, task.TokenId = quota, tokenID
	}
	return billed, err
}

func RecoverMidjourneyTaskSettlement(requestID string) error {
	var operation BillingOperation
	if err := DB.Where("request_id = ? AND phase = ?", requestID, "initial").First(&operation).Error; err != nil {
		return err
	}
	if operation.State == BillingSettled || operation.State == BillingRefunded {
		return nil
	}
	if operation.State != BillingSettlementRequested || operation.FundingSource != "midjourney_wallet" ||
		operation.TaskId <= 0 || operation.Actual <= 0 || operation.Actual > int64(common.MaxChargeQuota) {
		return errors.New("Midjourney settlement intent is incomplete")
	}
	var task Midjourney
	if err := DB.First(&task, operation.TaskId).Error; err != nil {
		return err
	}
	if task.BillingRequestID != requestID && !(task.BillingRequestID == "" && midjourneyBillingRequestID(task.Id) == requestID) {
		return errors.New("Midjourney task billing request ID does not match operation")
	}
	if task.Quota == 0 && task.Status == "FAILURE" && task.BillingFailureConfirmed {
		if err := RequestBillingRefund(requestID); err != nil {
			return err
		}
		return RefundBillingOperation(requestID)
	}
	_, err := ChargeMidjourneyTask(&task, int(operation.Actual), operation.TokenId)
	return err
}

func tokenAmountForMidjourney(quota, tokenID int) int64 {
	if tokenID <= 0 {
		return 0
	}
	return int64(quota)
}

func ensureMidjourneyLegacyBillingRoot(task *Midjourney, requestID string) error {
	if task == nil || task.Id <= 0 || requestID == "" {
		return errors.New("invalid Midjourney billing root request")
	}
	return withBillingTransaction(context.Background(), func(tx *gorm.DB) error {
		var root BillingOperation
		found := tx.Where("request_id = ? AND phase = ?", requestID, "initial").Limit(1).Find(&root)
		if found.Error != nil {
			return found.Error
		}
		if found.RowsAffected == 1 {
			return nil
		}
		var current Midjourney
		if err := lockForUpdate(tx).Where("id = ?", task.Id).First(&current).Error; err != nil {
			return err
		}
		found = tx.Where("request_id = ? AND phase = ?", requestID, "initial").Limit(1).Find(&root)
		if found.Error != nil {
			return found.Error
		}
		if found.RowsAffected == 1 {
			return nil
		}
		tokenAmount := int64(0)
		if current.TokenId > 0 {
			tokenAmount = int64(current.Quota)
		}
		now := common.GetTimestamp()
		root = BillingOperation{RequestId: requestID, Phase: "initial", State: BillingSettled,
			UserId: current.UserId, ChannelId: current.GetBillingChannelId(), TokenId: current.TokenId,
			FundingSource: "wallet", WalletAmount: int64(current.Quota), TokenAmount: tokenAmount,
			PreConsumed: int64(current.Quota), Actual: int64(current.Quota), TaskId: int64(current.Id),
			CreatedAt: now, UpdatedAt: now}
		return tx.Create(&root).Error
	})
}

// RefundMidjourneyTask also accepts pre-migration tasks: a positive persisted
// quota was their durable charge marker. New tasks are inserted with zero quota.
func RefundMidjourneyTask(task *Midjourney) (bool, error) {
	if task == nil || task.Id <= 0 {
		return false, errors.New("invalid Midjourney refund")
	}
	var current Midjourney
	if err := DB.First(&current, task.Id).Error; err != nil {
		return false, err
	}
	if current.UserId != task.UserId {
		return false, errors.New("Midjourney refund owner changed")
	}
	requestID := current.BillingRequestID
	if requestID == "" {
		requestID = midjourneyBillingRequestID(task.Id)
	}
	if current.Quota <= 0 {
		var root BillingOperation
		found := DB.Where("request_id = ? AND phase = ?", requestID, "initial").Limit(1).Find(&root)
		if found.Error != nil {
			return false, found.Error
		}
		if found.RowsAffected == 0 || root.State == BillingRefunded {
			return false, nil
		}
		if root.FundingSource != "midjourney_wallet" {
			return false, errors.New("Midjourney refund has an unsupported funding source")
		}
		if root.State == BillingSettlementRequested {
			if !current.BillingFailureConfirmed || current.Status != "FAILURE" {
				return false, errors.New("cannot release Midjourney reservation without confirmed failure")
			}
			if err := RequestBillingRefund(requestID); err != nil {
				return false, err
			}
		} else if root.State != BillingRefundRequested {
			return false, nil
		}
		if err := RefundBillingOperation(requestID); err != nil {
			return false, err
		}
		return true, nil
	}
	if err := ensureMidjourneyLegacyBillingRoot(&current, requestID); err != nil {
		return false, err
	}
	var refunded bool
	err := withBillingTransaction(context.Background(), func(tx *gorm.DB) error {
		root, err := lockBillingOperation(tx, requestID)
		if err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where("id = ?", task.Id).First(&current).Error; err != nil {
			return err
		}
		if current.UserId != task.UserId {
			return errors.New("Midjourney refund owner changed")
		}
		if root.State != BillingSettled || root.Actual != int64(current.Quota) {
			return errors.New("Midjourney refund amount does not match charge")
		}
		amount := int64(current.Quota)
		if err := lockBillingUser(tx, current.UserId); err != nil {
			return err
		}
		if current.TokenId > 0 {
			if err := lockBillingToken(tx, current.TokenId); err != nil {
				return err
			}
		}
		if err := updateUserQuotaWithDeltaTx(tx, current.UserId, amount, nil); err != nil {
			return err
		}
		if err := updateUserQuotaFieldDeltaTx(tx, current.UserId, "used_quota", -amount); err != nil {
			return err
		}
		if current.TokenId > 0 {
			if err := updateTokenQuotaDeltaTx(tx, current.TokenId, amount); err != nil {
				return err
			}
		}
		if channelID := current.GetBillingChannelId(); channelID > 0 {
			if err := updateChannelUsedQuotaTx(tx, channelID, -amount); err != nil {
				return err
			}
		}
		if err := tx.Model(&current).Update("quota", 0).Error; err != nil {
			return err
		}
		now := common.GetTimestamp()
		if err := tx.Model(&root).Updates(map[string]any{"state": BillingRefunded, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Create(&BillingOperation{RequestId: requestID, Phase: "refund", State: BillingRefunded,
			UserId: current.UserId, ChannelId: current.GetBillingChannelId(), TokenId: current.TokenId,
			FundingSource: "wallet", WalletAmount: -amount, TokenAmount: -root.TokenAmount,
			CreatedAt: now, UpdatedAt: now}).Error; err != nil {
			return err
		}
		refunded = true
		return nil
	})
	if err == nil && refunded {
		task.Quota = 0
	}
	return refunded, err
}

func ListConfirmedFailedMidjourneyTasksWithPendingRefund(limit int) ([]Midjourney, error) {
	var tasks []Midjourney
	err := DB.Where("status = ? AND quota > 0 AND billing_failure_confirmed = ? AND finish_time >= ?", "FAILURE", true, TaskRefundLegacyCutoff*1000).
		Order("id asc").Limit(limit).Find(&tasks).Error
	return tasks, err
}
