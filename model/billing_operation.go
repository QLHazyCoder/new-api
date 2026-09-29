package model

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	BillingReserved            = "reserved"
	BillingRefundRequested     = "refund_requested"
	BillingRefunded            = "refunded"
	BillingSettled             = "settled"
	BillingSettlementRequested = "settlement_requested"
	BillingReviewRequired      = "review_required"
)

var ErrBillingWalletInsufficient = errors.New("wallet quota insufficient")
var ErrBillingTokenInsufficient = errors.New("token quota is not enough")
var ErrBillingSubscriptionInsufficient = errors.New("subscription quota insufficient")
var ErrBillingTaskStateChanged = errors.New("task status changed before billing commit")

// BillingOperation is an append-only migration. The initial row holds the
// current allocation, while reserve/settle rows preserve each applied phase.
// Its unique key is server request ID plus phase, never a client-supplied key.
type BillingOperation struct {
	Id                     int64  `gorm:"primaryKey;autoIncrement"`
	RequestId              string `gorm:"type:varchar(128);not null;uniqueIndex:idx_billing_operation_request_phase"`
	Phase                  string `gorm:"type:varchar(40);not null;uniqueIndex:idx_billing_operation_request_phase"`
	State                  string `gorm:"type:varchar(24);not null;index"`
	UserId                 int    `gorm:"index"`
	ChannelId              int
	TokenId                int
	SubscriptionId         int
	FundingSource          string `gorm:"type:varchar(24)"`
	WalletAmount           int64
	SubscriptionAmount     int64
	TokenAmount            int64
	ExtraReserved          int64
	PreConsumed            int64
	Actual                 int64
	PendingTaskStatus      TaskStatus `gorm:"type:varchar(20)"`
	UsageCounted           bool
	TaskId                 int64  `gorm:"index"`
	UpstreamTaskID         string `gorm:"type:varchar(191)"`
	UpstreamCode           int
	UpstreamResultRecorded bool
	LeaseUntil             int64
	CreatedAt              int64
	UpdatedAt              int64
}

func billingOperationKeyValid(requestId, phase string) bool {
	return strings.TrimSpace(requestId) != "" && len(requestId) <= 128 && phase != "" && len(phase) <= 40
}

// CreateBillingReservation commits the snapshot, token and chosen funding
// together. A duplicate ID is rejected before executing any financial action.
func CreateBillingReservation(op *BillingOperation, apply func(*gorm.DB) error) error {
	if op == nil || !billingOperationKeyValid(op.RequestId, "initial") || op.UserId <= 0 || apply == nil {
		return errors.New("invalid billing reservation")
	}
	op.Phase = "initial"
	op.State = BillingReserved
	now := common.GetTimestamp()
	op.CreatedAt, op.UpdatedAt = now, now
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(op).Error; err != nil {
			return err
		}
		if err := apply(tx); err != nil {
			return err
		}
		return tx.Model(op).Updates(map[string]any{
			"channel_id": op.ChannelId, "token_id": op.TokenId, "subscription_id": op.SubscriptionId,
			"funding_source": op.FundingSource, "wallet_amount": op.WalletAmount,
			"subscription_amount": op.SubscriptionAmount, "token_amount": op.TokenAmount,
			"pre_consumed": op.PreConsumed,
		}).Error
	})
}

func lockBillingOperation(tx *gorm.DB, requestId string) (*BillingOperation, error) {
	var op BillingOperation
	if err := lockForUpdate(tx).Where("request_id = ? AND phase = ?", requestId, "initial").First(&op).Error; err != nil {
		return nil, err
	}
	return &op, nil
}

// ApplyBillingAdjustment uses positive deltas for consumption. An extra
// reservation may require a solvent wallet; ordinary settlement may incur debt.
type BillingTaskCommit struct {
	Task       *Task
	FromStatus TaskStatus
}

func ApplyBillingAdjustment(requestId, phase string, walletDelta, subscriptionDelta, tokenDelta int64, requireWalletBalance, settle bool, actual int64, countRequest bool, taskCommit ...*BillingTaskCommit) error {
	if !billingOperationKeyValid(requestId, phase) || phase == "initial" || phase == "refund" {
		return errors.New("invalid billing adjustment phase")
	}
	if len(taskCommit) > 1 || len(taskCommit) == 1 && (taskCommit[0] == nil || taskCommit[0].Task == nil) {
		return errors.New("invalid billing task commit")
	}
	var commit *BillingTaskCommit
	if len(taskCommit) == 1 {
		commit = taskCommit[0]
		if !settle || phase != "settle" || actual < 0 || actual > int64(common.MaxChargeQuota) {
			return errors.New("invalid task billing settlement")
		}
	}
	var updatedTask *Task
	err := DB.Transaction(func(tx *gorm.DB) error {
		var currentTask *Task
		var taskToCommit *Task
		if commit != nil {
			var locked Task
			if err := lockForUpdate(tx).Where("id = ?", commit.Task.ID).First(&locked).Error; err != nil {
				return err
			}
			currentTask = &locked
		}
		root, err := lockBillingOperation(tx, requestId)
		if err != nil {
			return err
		}
		var previous BillingOperation
		found := tx.Where("request_id = ? AND phase = ?", requestId, phase).Limit(1).Find(&previous)
		if found.Error != nil {
			return found.Error
		}
		if found.RowsAffected == 1 {
			if previous.WalletAmount != walletDelta || previous.SubscriptionAmount != subscriptionDelta || previous.TokenAmount != tokenDelta || previous.Actual != actual || previous.UsageCounted != countRequest {
				return errors.New("billing operation phase replay has different amounts")
			}
			if commit != nil && (currentTask.Status != commit.Task.Status || currentTask.Quota != int(actual)) {
				return errors.New("billing phase committed without matching task state")
			}
			if commit != nil {
				updatedTask = currentTask
			}
			return nil
		}
		if commit != nil && currentTask.Status != commit.FromStatus {
			return ErrBillingTaskStateChanged
		}
		if root.State != BillingReserved && !(commit != nil && root.State == BillingSettlementRequested && root.Actual == actual && root.PendingTaskStatus == commit.Task.Status) {
			return fmt.Errorf("billing operation state %s cannot be adjusted", root.State)
		}
		walletTotal, err := common.AddWalletQuota(root.WalletAmount, walletDelta)
		if err != nil || walletTotal < 0 {
			return common.ErrWalletQuotaOverflow
		}
		subTotal, err := common.AddWalletQuota(root.SubscriptionAmount, subscriptionDelta)
		if err != nil || subTotal < 0 {
			return common.ErrWalletQuotaOverflow
		}
		tokenTotal, err := common.AddWalletQuota(root.TokenAmount, tokenDelta)
		if err != nil || tokenTotal < 0 {
			return common.ErrWalletQuotaOverflow
		}
		if walletDelta != 0 {
			if requireWalletBalance && walletDelta > 0 {
				reserved, err := reserveUserQuotaTx(tx, root.UserId, walletDelta)
				if err != nil {
					return err
				}
				if !reserved {
					return ErrBillingWalletInsufficient
				}
			} else if err := updateUserQuotaWithDeltaTx(tx, root.UserId, -walletDelta, nil); err != nil {
				return err
			}
		}
		if subscriptionDelta != 0 {
			if root.SubscriptionId <= 0 {
				return errors.New("subscription id missing")
			}
			if requireWalletBalance && subscriptionDelta > 0 {
				err = reserveSubscriptionAdditionalTx(tx, root.SubscriptionId, subscriptionDelta)
			} else {
				err = postConsumeUserSubscriptionDeltaTx(tx, root.SubscriptionId, subscriptionDelta)
			}
			if err != nil {
				return err
			}
		}
		if tokenDelta != 0 && root.TokenId > 0 {
			if err := updateTokenQuotaDeltaTx(tx, root.TokenId, -tokenDelta); err != nil {
				return err
			}
		}
		if settle && countRequest {
			if err := recordBillingUsageTx(tx, root.UserId, root.ChannelId, actual, true); err != nil {
				return err
			}
		}
		state := root.State
		if settle {
			state = BillingSettled
		}
		now := common.GetTimestamp()
		step := BillingOperation{RequestId: requestId, Phase: phase, State: state, UserId: root.UserId, ChannelId: root.ChannelId,
			TokenId: root.TokenId, FundingSource: root.FundingSource, SubscriptionId: root.SubscriptionId,
			WalletAmount: walletDelta, SubscriptionAmount: subscriptionDelta, TokenAmount: tokenDelta,
			Actual: actual, UsageCounted: countRequest, CreatedAt: now, UpdatedAt: now}
		if commit != nil {
			step.TaskId = commit.Task.ID
		}
		if err := tx.Create(&step).Error; err != nil {
			return err
		}
		updates := map[string]any{"wallet_amount": walletTotal, "subscription_amount": subTotal,
			"token_amount": tokenTotal, "state": state, "updated_at": now}
		if commit != nil {
			updates["task_id"] = commit.Task.ID
			taskCopy := *commit.Task
			taskCopy.PrivateData.BillingAllocations = append([]BillingAllocation(nil), commit.Task.PrivateData.BillingAllocations...)
			if err := applyTaskBillingAllocationDeltas(&taskCopy, walletDelta, subscriptionDelta, root.SubscriptionId); err != nil {
				return err
			}
			taskToCommit = &taskCopy
			currentTask.Quota = int(actual)
			currentTask.Status = taskToCommit.Status
			currentTask.Progress = taskToCommit.Progress
			currentTask.StartTime = taskToCommit.StartTime
			currentTask.FinishTime = taskToCommit.FinishTime
			currentTask.FailReason = taskToCommit.FailReason
			currentTask.PrivateData = taskToCommit.PrivateData
			currentTask.Data = taskToCommit.Data
			currentTask.UpdatedAt = now
			if err := tx.Model(currentTask).Updates(map[string]any{
				"quota": currentTask.Quota, "status": currentTask.Status, "progress": currentTask.Progress,
				"start_time": currentTask.StartTime, "finish_time": currentTask.FinishTime,
				"fail_reason": currentTask.FailReason, "private_data": currentTask.PrivateData,
				"data": currentTask.Data, "updated_at": now,
			}).Error; err != nil {
				return err
			}
		}
		if settle {
			updates["actual"] = actual
		}
		if strings.HasPrefix(phase, "reserve:") {
			updates["extra_reserved"] = gorm.Expr("extra_reserved + ?", subscriptionDelta)
			updates["pre_consumed"] = gorm.Expr("pre_consumed + ?", walletDelta+subscriptionDelta)
		}
		if err := tx.Model(root).Updates(updates).Error; err != nil {
			return err
		}
		if commit != nil {
			updatedTask = currentTask
		}
		return nil
	})
	if err == nil && commit != nil && updatedTask != nil {
		*commit.Task = *updatedTask
	}
	return err
}

// InsertTaskWithBillingIntent makes the accepted upstream result recoverable
// before any final charge is applied. The task is hidden from pollers as a
// billing-pending row; its confirmed result is committed with the intent.
func InsertTaskWithBillingIntent(ctx context.Context, requestId string, task *Task, targetQuota int, omitColumns ...string) error {
	if task == nil || task.ID != 0 || targetQuota < 0 || targetQuota > common.MaxChargeQuota || task.Quota != targetQuota || !billingOperationKeyValid(requestId, "initial") {
		return errors.New("invalid task billing settlement request")
	}
	desired := *task
	staged := desired
	staged.Quota = 0
	staged.Status = TaskStatusBillingPending
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		root, err := lockBillingOperation(tx, requestId)
		if err != nil {
			return err
		}
		if root.State != BillingReserved || root.TaskId != 0 || root.UserId != desired.UserId {
			return fmt.Errorf("billing operation cannot accept task result in state %s", root.State)
		}
		create := tx
		if len(omitColumns) > 0 {
			create = create.Omit(omitColumns...)
		}
		if err := create.Create(&staged).Error; err != nil {
			return err
		}
		return tx.Model(root).Updates(map[string]any{
			"task_id": staged.ID, "state": BillingSettlementRequested,
			"actual": int64(targetQuota), "pending_task_status": desired.Status,
			"updated_at": common.GetTimestamp(),
		}).Error
	})
	if err == nil {
		desired.ID = staged.ID
		*task = desired
	}
	return err
}

func ListPendingBillingSettlements(limit int) ([]BillingOperation, error) {
	var pending []BillingOperation
	err := DB.Where("phase = ? AND state = ?", "initial", BillingSettlementRequested).
		Order("updated_at asc").Limit(limit).Find(&pending).Error
	return pending, err
}

func ClaimBillingSettlement(requestId string, leaseSeconds int64) (bool, error) {
	now := common.GetTimestamp()
	result := DB.Model(&BillingOperation{}).
		Where("request_id = ? AND phase = ? AND state = ? AND lease_until <= ?", requestId, "initial", BillingSettlementRequested, now).
		Update("lease_until", now+leaseSeconds)
	return result.RowsAffected == 1, result.Error
}

// RecoverBillingTaskSettlement settles only an intent created after a known
// upstream result was persisted alongside its task row.
func RecoverBillingTaskSettlement(requestId string) error {
	var root BillingOperation
	if err := DB.Where("request_id = ? AND phase = ?", requestId, "initial").First(&root).Error; err != nil {
		return err
	}
	if root.FundingSource == "midjourney_wallet" {
		return RecoverMidjourneyTaskSettlement(requestId)
	}
	if root.State == BillingSettled {
		return nil
	}
	if root.State != BillingSettlementRequested || root.TaskId <= 0 || root.Actual < 0 || root.Actual > int64(common.MaxChargeQuota) || root.PreConsumed < 0 || root.WalletAmount < 0 || root.SubscriptionAmount < 0 || root.TokenAmount < 0 {
		return errors.New("billing settlement intent is incomplete")
	}
	var task Task
	if err := DB.First(&task, root.TaskId).Error; err != nil {
		return err
	}
	if task.Status != TaskStatusBillingPending || task.Quota != 0 {
		return errors.New("billing-pending task state changed before recovery")
	}
	delta := root.Actual - root.PreConsumed
	var walletDelta, subscriptionDelta int64
	switch root.FundingSource {
	case "wallet":
		walletDelta = delta
	case "subscription":
		subscriptionDelta = delta
	case "mixed":
		if delta >= 0 {
			walletDelta = delta
		} else {
			walletRefund := min(-delta, root.WalletAmount)
			walletDelta = -walletRefund
			subscriptionDelta = delta + walletRefund
		}
	default:
		return fmt.Errorf("unsupported billing funding source %q", root.FundingSource)
	}
	desired := task
	desired.Status = root.PendingTaskStatus
	desired.Quota = int(root.Actual)
	commit := &BillingTaskCommit{Task: &desired, FromStatus: TaskStatusBillingPending}
	var tokenDelta int64
	if root.TokenId > 0 {
		tokenDelta = delta
	}
	return ApplyBillingAdjustment(requestId, "settle", walletDelta, subscriptionDelta, tokenDelta, false, true, root.Actual, true, commit)
}

// RequestBillingRefund durably records a known failure before attempting any
// credit. An unknown upstream outcome must never call this function.
func RequestBillingRefund(requestId string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		op, err := lockBillingOperation(tx, requestId)
		if err != nil {
			return err
		}
		switch op.State {
		case BillingRefunded, BillingRefundRequested:
			return nil
		case BillingReserved:
			return tx.Model(op).Updates(map[string]any{"state": BillingRefundRequested, "updated_at": common.GetTimestamp()}).Error
		case BillingSettlementRequested:
			if op.FundingSource != "midjourney_wallet" || op.TaskId <= 0 {
				return fmt.Errorf("cannot refund operation in state %s", op.State)
			}
			return tx.Model(op).Updates(map[string]any{"state": BillingRefundRequested, "updated_at": common.GetTimestamp()}).Error
		default:
			return fmt.Errorf("cannot refund operation in state %s", op.State)
		}
	})
}

func RefundBillingOperation(requestId string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		op, err := lockBillingOperation(tx, requestId)
		if err != nil {
			return err
		}
		if op.State == BillingRefunded {
			return nil
		}
		if op.State != BillingRefundRequested {
			return fmt.Errorf("billing refund not requested: %s", op.State)
		}
		if op.WalletAmount > 0 {
			if err := updateUserQuotaWithDeltaTx(tx, op.UserId, op.WalletAmount, nil); err != nil {
				return err
			}
		}
		if op.SubscriptionAmount > 0 {
			if op.SubscriptionId <= 0 {
				return errors.New("subscription id missing on refund")
			}
			if op.ExtraReserved > 0 {
				if err := postConsumeUserSubscriptionDeltaTx(tx, op.SubscriptionId, -op.ExtraReserved); err != nil {
					return err
				}
			}
			if err := refundSubscriptionPreConsumeTx(tx, requestId); err != nil {
				return err
			}
		}
		if op.TokenAmount > 0 && op.TokenId > 0 {
			if err := updateTokenQuotaDeltaTx(tx, op.TokenId, op.TokenAmount); err != nil {
				return err
			}
		}
		if err := tx.Model(op).Updates(map[string]any{"state": BillingRefunded, "updated_at": common.GetTimestamp()}).Error; err != nil {
			return err
		}
		return tx.Create(&BillingOperation{RequestId: requestId, Phase: "refund", State: BillingRefunded,
			UserId: op.UserId, TokenId: op.TokenId, SubscriptionId: op.SubscriptionId,
			WalletAmount: -op.WalletAmount, SubscriptionAmount: -op.SubscriptionAmount,
			TokenAmount: -op.TokenAmount, CreatedAt: common.GetTimestamp(), UpdatedAt: common.GetTimestamp()}).Error
	})
}

func ListPendingBillingRefunds(limit int) ([]BillingOperation, error) {
	var pending []BillingOperation
	err := DB.Where("phase = ? AND state = ?", "initial", BillingRefundRequested).
		Order("id asc").Limit(limit).Find(&pending).Error
	return pending, err
}

func ClaimBillingRefund(requestId string, leaseSeconds int64) (bool, error) {
	now := common.GetTimestamp()
	result := DB.Model(&BillingOperation{}).
		Where("request_id = ? AND phase = ? AND state = ? AND lease_until <= ?", requestId, "initial", BillingRefundRequested, now).
		Update("lease_until", now+leaseSeconds)
	return result.RowsAffected == 1, result.Error
}

func ListStaleReservedBillingOperations(cutoff int64, limit int) ([]BillingOperation, error) {
	var pending []BillingOperation
	err := DB.Where("phase = ? AND state = ? AND task_id = ? AND created_at < ?", "initial", BillingReserved, 0, cutoff).
		Order("id asc").Limit(limit).Find(&pending).Error
	return pending, err
}

func recordBillingUsageTx(tx *gorm.DB, userId, channelId int, amount int64, countRequest bool) error {
	if amount < 0 {
		return errors.New("negative billing usage")
	}
	if amount > 0 {
		if err := updateUserQuotaFieldDeltaTx(tx, userId, "used_quota", amount); err != nil {
			return err
		}
	}
	if countRequest {
		result := tx.Model(&User{}).Where("id = ? AND request_count < ?", userId, int64(2147483647)).
			Update("request_count", gorm.Expr("request_count + 1"))
		if err := walletDeltaUpdateResult(tx, userId, result); err != nil {
			return err
		}
	}
	if amount > 0 && channelId > 0 {
		return updateChannelUsedQuotaTx(tx, channelId, amount)
	}
	return nil
}

// ApplyLegacyBillingDelta protects older no-session call sites. It records the
// phase and moves the wallet/subscription and token in the same transaction.
func ApplyLegacyBillingDelta(requestId, phase string, userId, channelId, tokenId, subscriptionId int, delta, usage int64, requireBalance, countRequest bool) error {
	if !billingOperationKeyValid(requestId, phase) || userId <= 0 || delta == common.MinWalletQuota {
		return errors.New("invalid legacy billing operation")
	}
	settles := phase == "legacy-charge" || phase == "legacy-settle"
	if !settles && !strings.HasPrefix(phase, "legacy-consume:") {
		return errors.New("invalid legacy billing phase")
	}
	if !settles && countRequest {
		return errors.New("legacy reservation cannot count a request")
	}
	if settles && (usage < 0 || usage > int64(common.MaxChargeQuota)) {
		return errors.New("legacy billing usage exceeds the single-request charge range")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		fundingSource := "wallet"
		if subscriptionId > 0 {
			fundingSource = "subscription"
		}
		var root BillingOperation
		rootFound := lockForUpdate(tx).Where("request_id = ? AND phase = ?", requestId, "initial").Limit(1).Find(&root)
		if rootFound.Error != nil {
			return rootFound.Error
		}
		if rootFound.RowsAffected == 0 {
			root = BillingOperation{RequestId: requestId, Phase: "initial", State: BillingReserved,
				UserId: userId, ChannelId: channelId, TokenId: tokenId, SubscriptionId: subscriptionId,
				FundingSource: fundingSource, CreatedAt: common.GetTimestamp(), UpdatedAt: common.GetTimestamp()}
			if err := tx.Create(&root).Error; err != nil {
				return err
			}
		} else if root.UserId != userId || root.ChannelId != channelId || root.TokenId != tokenId ||
			root.SubscriptionId != subscriptionId || root.FundingSource != fundingSource {
			return errors.New("legacy billing request identity changed")
		}

		var previous BillingOperation
		found := tx.Where("request_id = ? AND phase = ?", requestId, phase).Limit(1).Find(&previous)
		if found.Error != nil {
			return found.Error
		}
		if found.RowsAffected == 1 {
			if previous.UserId != userId || previous.ChannelId != channelId || previous.TokenId != tokenId ||
				previous.SubscriptionId != subscriptionId || previous.Actual != delta || previous.UsageCounted != countRequest {
				return errors.New("legacy billing replay has different amounts")
			}
			return nil
		}
		if root.State != BillingReserved {
			return fmt.Errorf("legacy billing operation is already %s", root.State)
		}
		walletDelta, subscriptionDelta, tokenDelta := int64(0), int64(0), int64(0)
		if subscriptionId > 0 {
			var subscription UserSubscription
			if err := lockForUpdate(tx).Select("id", "user_id").Where("id = ?", subscriptionId).First(&subscription).Error; err != nil {
				return err
			}
			if subscription.UserId != userId {
				return errors.New("legacy billing subscription owner mismatch")
			}
			subscriptionDelta = delta
			if requireBalance && delta > 0 {
				if err := reserveSubscriptionAdditionalTx(tx, subscriptionId, delta); err != nil {
					return err
				}
			} else if err := postConsumeUserSubscriptionDeltaTx(tx, subscriptionId, delta); err != nil {
				return err
			}
		} else if requireBalance && delta > 0 {
			reserved, err := reserveUserQuotaTx(tx, userId, delta)
			if err != nil {
				return err
			}
			if !reserved {
				return ErrBillingWalletInsufficient
			}
			walletDelta = delta
		} else if delta != 0 {
			if err := updateUserQuotaWithDeltaTx(tx, userId, -delta, nil); err != nil {
				return err
			}
			walletDelta = delta
		}
		if tokenId > 0 && delta != 0 {
			var token Token
			if err := lockForUpdate(tx).Select("id", "user_id", "unlimited_quota").Where("id = ?", tokenId).First(&token).Error; err != nil {
				return err
			}
			if token.UserId != userId {
				return errors.New("legacy billing token owner mismatch")
			}
			tokenDelta = delta
			if requireBalance && delta > 0 {
				reserved, err := TryReserveTokenQuotaTx(tx, tokenId, delta, token.UnlimitedQuota)
				if err != nil {
					return err
				}
				if !reserved {
					return ErrBillingTokenInsufficient
				}
			} else if err := updateTokenQuotaDeltaTx(tx, tokenId, -delta); err != nil {
				return err
			}
		}
		if settles {
			if err := recordBillingUsageTx(tx, userId, channelId, usage, countRequest); err != nil {
				return err
			}
		}
		walletTotal, err := common.AddWalletQuota(root.WalletAmount, walletDelta)
		if err != nil || walletTotal < 0 {
			return common.ErrWalletQuotaOverflow
		}
		subscriptionTotal, err := common.AddWalletQuota(root.SubscriptionAmount, subscriptionDelta)
		if err != nil || subscriptionTotal < 0 {
			return common.ErrWalletQuotaOverflow
		}
		tokenTotal, err := common.AddWalletQuota(root.TokenAmount, tokenDelta)
		if err != nil || tokenTotal < 0 {
			return common.ErrWalletQuotaOverflow
		}
		preConsumed := root.PreConsumed
		if !settles {
			preConsumed, err = common.AddWalletQuota(preConsumed, delta)
			if err != nil || preConsumed < 0 {
				return common.ErrWalletQuotaOverflow
			}
		}
		state := BillingReserved
		if settles {
			state = BillingSettled
		}
		op := BillingOperation{RequestId: requestId, Phase: phase, State: state,
			UserId: userId, ChannelId: channelId, TokenId: tokenId, SubscriptionId: subscriptionId,
			WalletAmount: walletDelta, SubscriptionAmount: subscriptionDelta, TokenAmount: tokenDelta,
			Actual: delta, UsageCounted: countRequest, CreatedAt: common.GetTimestamp(), UpdatedAt: common.GetTimestamp()}
		if err := tx.Create(&op).Error; err != nil {
			return err
		}
		updates := map[string]any{
			"wallet_amount": walletTotal, "subscription_amount": subscriptionTotal,
			"token_amount": tokenTotal, "pre_consumed": preConsumed, "state": state,
			"updated_at": common.GetTimestamp(),
		}
		if settles {
			updates["actual"] = usage
			updates["usage_counted"] = countRequest
		}
		return tx.Model(&root).Updates(updates).Error
	})
}
