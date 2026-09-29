package service

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/bytedance/gopkg/util/gopool"
)

// RecoverBillingOnce only retries explicitly requested refunds or terminal
// failed tasks. An in-flight request with an unknown upstream result is never
// inferred to have failed solely because its reservation is old.
func RecoverBillingOnce(ctx context.Context) {
	settlements, err := model.ListPendingBillingSettlements(100)
	if err != nil {
		common.SysError("billing settlement recovery query failed: " + err.Error())
	} else {
		for _, op := range settlements {
			if ctx.Err() != nil {
				return
			}
			claimed, err := model.ClaimBillingSettlement(op.RequestId, 60)
			if err != nil {
				common.SysError(fmt.Sprintf("billing settlement lease failed for %s: %v", op.RequestId, err))
				continue
			}
			if claimed {
				if err := model.RecoverBillingTaskSettlement(op.RequestId); err != nil {
					common.SysError(fmt.Sprintf("billing settlement still pending for %s: %v", op.RequestId, err))
				}
			}
		}
	}
	pending, err := model.ListPendingBillingRefunds(100)
	if err != nil {
		common.SysError("billing refund recovery query failed: " + err.Error())
		return
	}
	for _, op := range pending {
		if ctx.Err() != nil {
			return
		}
		claimed, err := model.ClaimBillingRefund(op.RequestId, 60)
		if err != nil {
			common.SysError(fmt.Sprintf("billing refund lease failed for %s: %v", op.RequestId, err))
		} else if claimed {
			if err := model.RefundBillingOperation(op.RequestId); err != nil {
				common.SysError(fmt.Sprintf("billing refund still pending for %s: %v", op.RequestId, err))
			}
		}
	}
	tasks, err := model.ListConfirmedFailedTasksWithPendingRefund(100)
	if err != nil {
		common.SysError("failed task billing recovery query failed: " + err.Error())
		return
	}
	for i := range tasks {
		if ctx.Err() != nil {
			return
		}
		RefundTaskQuota(ctx, &tasks[i], tasks[i].FailReason)
	}
	reviewTasks, err := model.ListBillingReviewTasks(100)
	if err != nil {
		common.SysError("billing review task query failed: " + err.Error())
	} else if len(reviewTasks) > 0 {
		common.SysError(fmt.Sprintf("%d task billing outcomes remain unknown and require manual review (first task %s); no automatic refund or additional charge", len(reviewTasks), reviewTasks[0].TaskID))
	}
	midjourneyTasks, err := model.ListConfirmedFailedMidjourneyTasksWithPendingRefund(100)
	if err != nil {
		common.SysError("Midjourney billing recovery query failed: " + err.Error())
		return
	}
	for i := range midjourneyTasks {
		if ctx.Err() != nil {
			return
		}
		RefundMidjourneyQuota(ctx, &midjourneyTasks[i], midjourneyTasks[i].FailReason)
	}
	midjourneyReviewTasks, err := model.ListMidjourneyBillingReviewTasks(100)
	if err != nil {
		common.SysError("Midjourney billing review query failed: " + err.Error())
	} else if len(midjourneyReviewTasks) > 0 {
		common.SysError(fmt.Sprintf("%d Midjourney task outcomes require billing review (first task %s); no automatic refund", len(midjourneyReviewTasks), midjourneyReviewTasks[0].MjId))
	}
	// Audit only: failure cannot be inferred from elapsed time alone.
	stale, err := model.ListStaleReservedBillingOperations(common.GetTimestamp()-3600, 100)
	if err != nil {
		common.SysError("billing pending operation audit failed: " + err.Error())
	} else if len(stale) > 0 {
		common.SysError(fmt.Sprintf("%d billing reservations older than one hour need outcome review (first request %s, upstream task %s, code %d); no automatic refund", len(stale), stale[0].RequestId, stale[0].UpstreamTaskID, stale[0].UpstreamCode))
	}
}

func StartBillingRecoveryWorker() {
	gopool.Go(func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			RecoverBillingOnce(context.Background())
			<-ticker.C
		}
	})
}
