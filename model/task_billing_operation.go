package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// FinalizeTaskBilling atomically adjusts every balance, accumulated usage,
// allocation snapshot and task quota. Existing tasks need no backfill: their
// first terminal billing change creates a durable operation in the same tx.
func FinalizeTaskBilling(task *Task, actual int, refund bool) (bool, error) {
	if task == nil {
		return false, errors.New("invalid task billing operation")
	}
	return FinalizeTaskBillingWithStatus(task, actual, refund, task.Status)
}

// FinalizeTaskBillingWithStatus CASes the terminal status and its financial
// consequences in the same transaction. fromStatus is the persisted status
// observed before the upstream terminal result was processed.
func FinalizeTaskBillingWithStatus(task *Task, actual int, refund bool, fromStatus TaskStatus) (bool, error) {
	if task == nil || task.ID <= 0 || actual < 0 || actual > common.MaxChargeQuota {
		return false, errors.New("invalid task billing operation")
	}
	phase := fmt.Sprintf("recalc:%d", actual)
	if refund {
		phase = "refund"
		actual = 0
	}
	if err := ensureTaskBillingRoot(task); err != nil {
		return false, err
	}
	var updated *Task
	err := withBillingTransaction(context.Background(), func(tx *gorm.DB) error {
		requestRoot, err := lockTaskBillingOperation(tx, task.ID)
		if err != nil {
			return err
		}
		var current Task
		if err := lockForUpdate(tx).Where("id = ?", task.ID).First(&current).Error; err != nil {
			return err
		}
		if current.UserId != task.UserId || current.Quota != task.Quota || current.Quota < 0 || current.Quota > common.MaxChargeQuota {
			return errors.New("task billing snapshot changed; reload before retry")
		}
		if current.Status != fromStatus {
			return nil
		}
		if current.BillingReviewPending {
			return nil
		}
		if incoming := task.PrivateData.BillingContext; incoming != nil && incoming.TieredSnapshot != nil {
			if stored := current.PrivateData.BillingContext; stored != nil && stored.TieredSnapshot != nil &&
				stored.TieredSnapshot.ExprHash == incoming.TieredSnapshot.ExprHash &&
				stored.TieredSnapshot.ExprString == incoming.TieredSnapshot.ExprString {
				stored.TieredSnapshot = incoming.TieredSnapshot
			}
		}
		root := *requestRoot
		if root.UserId != current.UserId || root.TaskId != int64(current.ID) {
			return errors.New("task billing root does not match task")
		}
		switch root.State {
		case BillingRefunded:
			return nil
		case BillingReserved, BillingSettled:
		default:
			return fmt.Errorf("task billing operation cannot finalize from state %s", root.State)
		}
		current.PrivateData = task.PrivateData
		delta := actual - current.Quota
		var previous BillingOperation
		stepFound := tx.Where("request_id = ? AND phase = ?", root.RequestId, phase).Limit(1).Find(&previous)
		if stepFound.Error != nil {
			return stepFound.Error
		}
		if stepFound.RowsAffected == 1 && (delta != 0 || previous.Actual != int64(actual)) {
			return errors.New("task billing phase replay does not match current quota")
		}
		var walletDelta, subDelta int64
		tokenDelta := int64(0)
		if current.PrivateData.TokenId > 0 {
			tokenDelta = int64(delta)
		}
		subID := root.SubscriptionId
		if delta != 0 {
			var fundingErr error
			var adjustedSubscriptionID int
			walletDelta, subDelta, adjustedSubscriptionID, fundingErr = applyTaskFundingTx(tx, &current, delta)
			if fundingErr != nil {
				return fundingErr
			}
			if adjustedSubscriptionID > 0 {
				subID = adjustedSubscriptionID
			}
			if err := updateUserQuotaFieldDeltaTx(tx, current.UserId, "used_quota", int64(delta)); err != nil {
				return err
			}
			if tokenDelta != 0 {
				if err := updateTokenQuotaDeltaTx(tx, current.PrivateData.TokenId, -tokenDelta); err != nil {
					return err
				}
			}
			if current.ChannelId > 0 {
				if err := updateChannelUsedQuotaTx(tx, current.ChannelId, int64(delta)); err != nil {
					return err
				}
			}
		}
		walletTotal, err := common.AddWalletQuota(root.WalletAmount, walletDelta)
		if err != nil || walletTotal < 0 {
			return common.ErrWalletQuotaOverflow
		}
		subscriptionTotal, err := common.AddWalletQuota(root.SubscriptionAmount, subDelta)
		if err != nil || subscriptionTotal < 0 {
			return common.ErrWalletQuotaOverflow
		}
		tokenTotal, err := common.AddWalletQuota(root.TokenAmount, tokenDelta)
		if err != nil || tokenTotal < 0 {
			return common.ErrWalletQuotaOverflow
		}
		state := BillingSettled
		if refund {
			state = BillingRefunded
		}
		step := BillingOperation{RequestId: root.RequestId, Phase: phase, State: state,
			TaskId: current.ID, UserId: current.UserId, TokenId: current.PrivateData.TokenId,
			FundingSource: root.FundingSource, SubscriptionId: subID,
			WalletAmount: walletDelta, SubscriptionAmount: subDelta, TokenAmount: tokenDelta,
			Actual: int64(actual), CreatedAt: common.GetTimestamp(), UpdatedAt: common.GetTimestamp()}
		if stepFound.RowsAffected == 0 {
			if err := tx.Create(&step).Error; err != nil {
				return err
			}
		}
		current.Quota = actual
		current.Status = task.Status
		current.Progress = task.Progress
		current.StartTime = task.StartTime
		current.FinishTime = task.FinishTime
		current.FailReason = task.FailReason
		current.Data = task.Data
		current.UpdatedAt = common.GetTimestamp()
		if err := tx.Model(&current).Updates(map[string]any{
			"quota": current.Quota, "private_data": current.PrivateData, "status": current.Status,
			"progress": current.Progress, "start_time": current.StartTime, "finish_time": current.FinishTime,
			"fail_reason": current.FailReason, "data": current.Data, "updated_at": current.UpdatedAt,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&root).Updates(map[string]any{
			"state": state, "actual": actual, "wallet_amount": walletTotal,
			"subscription_amount": subscriptionTotal, "token_amount": tokenTotal,
			"subscription_id": subID, "channel_id": current.ChannelId,
			"updated_at": common.GetTimestamp(),
		}).Error; err != nil {
			return err
		}
		updated = &current
		return nil
	})
	if err == nil && updated != nil {
		*task = *updated
	}
	return updated != nil, err
}

// ensureTaskBillingRoot bootstraps legacy task rows before the main billing
// transaction. It never changes a balance, so the task-only lock cannot form
// a cycle with the root-first settlement path.
func ensureTaskBillingRoot(task *Task) error {
	if task == nil || task.ID <= 0 {
		return errors.New("invalid task billing root request")
	}
	requestID := fmt.Sprintf("task:%d", task.ID)
	return withBillingTransaction(context.Background(), func(tx *gorm.DB) error {
		var root BillingOperation
		found := tx.Where("task_id = ? AND phase = ?", task.ID, "initial").Limit(1).Find(&root)
		if found.Error != nil {
			return found.Error
		}
		if found.RowsAffected == 0 {
			found = tx.Where("request_id = ? AND phase = ?", requestID, "initial").Limit(1).Find(&root)
		}
		if found.Error != nil {
			return found.Error
		}
		if found.RowsAffected == 1 {
			if root.TaskId != 0 {
				if root.TaskId != int64(task.ID) {
					return errors.New("task billing root does not match task")
				}
				return nil
			}
		}
		var current Task
		if err := lockForUpdate(tx).Where("id = ?", task.ID).First(&current).Error; err != nil {
			return err
		}
		found = tx.Where("task_id = ? AND phase = ?", task.ID, "initial").Limit(1).Find(&root)
		if found.RowsAffected == 0 {
			found = tx.Where("request_id = ? AND phase = ?", requestID, "initial").Limit(1).Find(&root)
		}
		if found.Error != nil {
			return found.Error
		}
		if found.RowsAffected == 1 {
			if root.TaskId == 0 {
				if root.UserId != current.UserId {
					return errors.New("task billing root owner does not match task")
				}
				if err := tx.Model(&root).Updates(map[string]any{"task_id": current.ID, "updated_at": common.GetTimestamp()}).Error; err != nil {
					return err
				}
			}
			return nil
		}
		fundingSource, walletAmount, subscriptionAmount, tokenAmount, subscriptionID, err := taskBillingRootSnapshot(&current)
		if err != nil {
			return err
		}
		root = BillingOperation{RequestId: requestID, Phase: "initial", State: BillingReserved,
			TaskId: current.ID, UserId: current.UserId, ChannelId: current.ChannelId, TokenId: current.PrivateData.TokenId,
			SubscriptionId: subscriptionID, FundingSource: fundingSource,
			WalletAmount: walletAmount, SubscriptionAmount: subscriptionAmount, TokenAmount: tokenAmount,
			PreConsumed: int64(current.Quota), Actual: int64(current.Quota), CreatedAt: common.GetTimestamp(), UpdatedAt: common.GetTimestamp()}
		return tx.Create(&root).Error
	})
}

func lockTaskBillingOperation(tx *gorm.DB, taskID int64) (*BillingOperation, error) {
	if taskID <= 0 {
		return nil, errors.New("invalid task billing operation id")
	}
	var root BillingOperation
	requestID := fmt.Sprintf("task:%d", taskID)
	if err := lockForUpdate(tx).Where("task_id = ? AND phase = ?", taskID, "initial").First(&root).Error; err == nil {
		return &root, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err := lockForUpdate(tx).Where("request_id = ? AND phase = ?", requestID, "initial").First(&root).Error; err != nil {
		return nil, err
	}
	return &root, nil
}

func taskBillingRootSnapshot(task *Task) (fundingSource string, walletAmount, subscriptionAmount, tokenAmount int64, subscriptionID int, err error) {
	if task == nil || task.Quota < 0 || task.Quota > common.MaxChargeQuota {
		err = errors.New("invalid task quota snapshot")
		return
	}
	quota := int64(task.Quota)
	fundingSource = task.PrivateData.BillingSource
	if task.PrivateData.TokenId > 0 {
		tokenAmount = quota
	}
	switch fundingSource {
	case "", "wallet":
		fundingSource = "wallet"
		walletAmount = quota
	case "subscription":
		subscriptionID = task.PrivateData.SubscriptionId
		if quota > 0 && subscriptionID <= 0 {
			err = errors.New("task subscription billing snapshot has no subscription ID")
			return
		}
		subscriptionAmount = quota
	case "mixed":
		if len(task.PrivateData.BillingAllocations) == 0 {
			walletAmount = quota
			return
		}
		var allocated int64
		for _, allocation := range task.PrivateData.BillingAllocations {
			if allocation.Quota < 0 {
				err = errors.New("negative task billing allocation")
				return
			}
			amount := int64(allocation.Quota)
			switch allocation.Source {
			case "wallet":
				walletAmount, err = common.AddWalletQuota(walletAmount, amount)
			case "subscription":
				if amount > 0 && allocation.SubscriptionId <= 0 ||
					subscriptionID > 0 && allocation.SubscriptionId > 0 && subscriptionID != allocation.SubscriptionId {
					err = errors.New("invalid mixed subscription allocation")
					return
				}
				if allocation.SubscriptionId > 0 {
					subscriptionID = allocation.SubscriptionId
				}
				subscriptionAmount, err = common.AddWalletQuota(subscriptionAmount, amount)
			default:
				err = fmt.Errorf("unsupported task billing allocation source %q", allocation.Source)
				return
			}
			if err != nil {
				return
			}
			allocated, err = common.AddWalletQuota(allocated, amount)
			if err != nil {
				return
			}
		}
		if allocated != quota {
			err = errors.New("mixed task billing allocations do not match task quota")
		}
	default:
		err = fmt.Errorf("unsupported task billing source %q", fundingSource)
	}
	return
}

// MarkTaskBillingReviewRequired records an unresolved outcome without
// interpreting a poller failure or timeout as an upstream failure. Balances
// and usage counters remain untouched.
func MarkTaskBillingReviewRequired(task *Task, fromStatus TaskStatus) (bool, error) {
	if task == nil || task.ID <= 0 || task.Quota < 0 {
		return false, errors.New("invalid task billing review request")
	}
	if err := ensureTaskBillingRoot(task); err != nil {
		return false, err
	}
	var updated *Task
	err := withBillingTransaction(context.Background(), func(tx *gorm.DB) error {
		root, err := lockTaskBillingOperation(tx, int64(task.ID))
		if err != nil {
			return err
		}
		var current Task
		if err := lockForUpdate(tx).Where("id = ?", task.ID).First(&current).Error; err != nil {
			return err
		}
		if current.UserId != task.UserId || current.Quota != task.Quota {
			return errors.New("task billing snapshot changed; reload before review")
		}
		if current.Status != fromStatus || current.BillingReviewPending {
			return nil
		}
		current.Status = task.Status
		current.Progress = task.Progress
		current.FinishTime = task.FinishTime
		current.FailReason = task.FailReason
		current.PrivateData = task.PrivateData
		current.Data = task.Data
		current.BillingReviewPending = true
		current.UpdatedAt = common.GetTimestamp()
		if err := tx.Model(&current).Updates(map[string]any{
			"status": current.Status, "progress": current.Progress, "finish_time": current.FinishTime,
			"fail_reason": current.FailReason, "private_data": current.PrivateData, "data": current.Data,
			"billing_review_pending": true, "updated_at": current.UpdatedAt,
		}).Error; err != nil {
			return err
		}
		if root.State == BillingSettled || root.State == BillingReserved {
			if err := tx.Model(root).Updates(map[string]any{
				"state": BillingReviewRequired, "updated_at": common.GetTimestamp(),
			}).Error; err != nil {
				return err
			}
		}
		updated = &current
		return nil
	})
	if err == nil && updated != nil {
		*task = *updated
	}
	return updated != nil, err
}

// CommitTaskTerminalState preserves the explicit legacy no-refund timeout
// policy while making its status/quota marker update transactional.
func CommitTaskTerminalState(task *Task, fromStatus TaskStatus, clearQuota bool) (bool, error) {
	if task == nil || task.ID <= 0 {
		return false, errors.New("invalid terminal task state")
	}
	var updated *Task
	err := withBillingTransaction(context.Background(), func(tx *gorm.DB) error {
		var current Task
		if err := lockForUpdate(tx).Where("id = ?", task.ID).First(&current).Error; err != nil {
			return err
		}
		if current.Status != fromStatus {
			return nil
		}
		current.Status = task.Status
		current.Progress = task.Progress
		current.FinishTime = task.FinishTime
		current.FailReason = task.FailReason
		current.PrivateData = task.PrivateData
		current.Data = task.Data
		if clearQuota {
			current.Quota = 0
		}
		current.UpdatedAt = common.GetTimestamp()
		if err := tx.Model(&current).Updates(map[string]any{
			"status": current.Status, "progress": current.Progress, "finish_time": current.FinishTime,
			"fail_reason": current.FailReason, "private_data": current.PrivateData,
			"data": current.Data, "quota": current.Quota, "updated_at": current.UpdatedAt,
		}).Error; err != nil {
			return err
		}
		updated = &current
		return nil
	})
	if err == nil && updated != nil {
		*task = *updated
	}
	return updated != nil, err
}

// applyTaskFundingTx mirrors mixed-funding settlement: wallet pays increases;
// refunds return to wallet first and then to the frozen subscription split.
func applyTaskFundingTx(tx *gorm.DB, task *Task, delta int) (walletDelta, subscriptionDelta int64, subscriptionID int, err error) {
	if task.PrivateData.BillingSource == "mixed" && len(task.PrivateData.BillingAllocations) > 0 {
		if delta > 0 {
			walletDelta = int64(delta)
			for i := range task.PrivateData.BillingAllocations {
				if task.PrivateData.BillingAllocations[i].Source == "wallet" {
					task.PrivateData.BillingAllocations[i].Quota += delta
					return walletDelta, 0, 0, updateUserQuotaWithDeltaTx(tx, task.UserId, -walletDelta, nil)
				}
			}
			task.PrivateData.BillingAllocations = append(task.PrivateData.BillingAllocations, BillingAllocation{Source: "wallet", Quota: delta})
			return walletDelta, 0, 0, updateUserQuotaWithDeltaTx(tx, task.UserId, -walletDelta, nil)
		}
		remaining := -delta
		for _, source := range []string{"wallet", "subscription"} {
			for i := range task.PrivateData.BillingAllocations {
				a := &task.PrivateData.BillingAllocations[i]
				if a.Source != source || a.Quota <= 0 || remaining <= 0 {
					continue
				}
				part := min(remaining, a.Quota)
				if source == "wallet" {
					walletDelta -= int64(part)
				} else {
					if a.SubscriptionId <= 0 || subscriptionID != 0 && subscriptionID != a.SubscriptionId {
						return 0, 0, 0, errors.New("invalid mixed subscription allocation")
					}
					subscriptionID = a.SubscriptionId
					subscriptionDelta -= int64(part)
					a.SubscriptionAmountUsedAfterConsume = max(a.SubscriptionAmountUsedAfterConsume-int64(part), 0)
				}
				a.Quota -= part
				remaining -= part
			}
		}
		if remaining > 0 {
			return 0, 0, 0, fmt.Errorf("mixed billing allocations are insufficient by %d", remaining)
		}
		if subscriptionDelta != 0 {
			if err := postConsumeUserSubscriptionDeltaTx(tx, subscriptionID, subscriptionDelta); err != nil {
				return 0, 0, 0, err
			}
		}
		if walletDelta != 0 {
			if err := updateUserQuotaWithDeltaTx(tx, task.UserId, -walletDelta, nil); err != nil {
				return 0, 0, 0, err
			}
		}
		allocations := task.PrivateData.BillingAllocations[:0]
		for _, a := range task.PrivateData.BillingAllocations {
			if a.Quota > 0 {
				allocations = append(allocations, a)
			}
		}
		task.PrivateData.BillingAllocations = allocations
		return walletDelta, subscriptionDelta, subscriptionID, nil
	}
	if task.PrivateData.BillingSource == "subscription" && task.PrivateData.SubscriptionId > 0 {
		subscriptionID = task.PrivateData.SubscriptionId
		return 0, int64(delta), subscriptionID, postConsumeUserSubscriptionDeltaTx(tx, subscriptionID, int64(delta))
	}
	return int64(delta), 0, 0, updateUserQuotaWithDeltaTx(tx, task.UserId, -int64(delta), nil)
}

func applyTaskBillingAllocationDeltas(task *Task, walletDelta, subscriptionDelta int64, subscriptionID int) error {
	if task.PrivateData.BillingSource != "mixed" {
		return nil
	}
	allocations := task.PrivateData.BillingAllocations
	if walletDelta != 0 {
		found := false
		for i := range allocations {
			if allocations[i].Source != "wallet" {
				continue
			}
			quota, err := common.AddWalletQuota(int64(allocations[i].Quota), walletDelta)
			if err != nil || quota < 0 || int64(int(quota)) != quota {
				return errors.New("mixed wallet allocation overflow")
			}
			allocations[i].Quota = int(quota)
			found = true
			break
		}
		if !found {
			if walletDelta < 0 || int64(int(walletDelta)) != walletDelta {
				return errors.New("mixed wallet allocation is missing")
			}
			allocations = append(allocations, BillingAllocation{Source: "wallet", Quota: int(walletDelta)})
		}
	}
	if subscriptionDelta != 0 {
		if subscriptionID <= 0 {
			return errors.New("mixed subscription allocation is missing")
		}
		found := false
		for i := range allocations {
			allocation := &allocations[i]
			if allocation.Source != "subscription" || allocation.SubscriptionId != subscriptionID {
				continue
			}
			quota, err := common.AddWalletQuota(int64(allocation.Quota), subscriptionDelta)
			if err != nil || quota < 0 || int64(int(quota)) != quota {
				return errors.New("mixed subscription allocation overflow")
			}
			used, err := common.AddWalletQuota(allocation.SubscriptionAmountUsedAfterConsume, subscriptionDelta)
			if err != nil {
				return err
			}
			allocation.Quota = int(quota)
			allocation.SubscriptionAmountUsedAfterConsume = max(used, 0)
			found = true
			break
		}
		if !found {
			return errors.New("mixed subscription allocation does not match the billed subscription")
		}
	}
	compacted := allocations[:0]
	for _, allocation := range allocations {
		if allocation.Quota > 0 {
			compacted = append(compacted, allocation)
		}
	}
	task.PrivateData.BillingAllocations = compacted
	return nil
}

// Only confirmed failed, non-legacy tasks are recoverable without querying
// their upstream provider. Successful or ambiguous tasks require review.
func ListConfirmedFailedTasksWithPendingRefund(limit int) ([]Task, error) {
	var tasks []Task
	err := DB.Where("status = ? AND quota > 0 AND billing_review_pending = ? AND submit_time >= ?", TaskStatusFailure, false, TaskRefundLegacyCutoff).
		Order("id asc").Limit(limit).Find(&tasks).Error
	return tasks, err
}

func ListBillingReviewTasks(limit int) ([]Task, error) {
	var tasks []Task
	err := DB.Where("billing_review_pending = ?", true).Order("updated_at asc").Limit(limit).Find(&tasks).Error
	return tasks, err
}
