package service

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// BillingSession — 统一计费会话
// ---------------------------------------------------------------------------

// BillingSession 封装单次请求的预扣费/结算/退款生命周期。
// 实现 relaycommon.BillingSettler 接口。
type BillingSession struct {
	relayInfo        *relaycommon.RelayInfo
	funding          FundingSource
	preConsumedQuota int  // 实际预扣额度（信任用户可能为 0）
	tokenConsumed    int  // 令牌额度实际扣减量
	extraReserved    int  // 发送前补充预扣的额度（订阅退款时需要单独回滚）
	trusted          bool // 是否命中信任额度旁路
	settled          bool // Settle 全部完成（资金 + 令牌）
	refunded         bool // Refund 已调用
	reserveCount     int
	mu               sync.Mutex
}

// Settle applies the funding and token difference in a single durable phase.
func (s *BillingSession) Settle(actualQuota int) error {
	return s.settle(actualQuota, nil)
}

func (s *BillingSession) SettleTask(actualQuota int, task *model.Task, fromStatus model.TaskStatus) error {
	if task == nil {
		return errors.New("task is required for task billing settlement")
	}
	return s.settle(actualQuota, &model.BillingTaskCommit{Task: task, FromStatus: fromStatus})
}

func (s *BillingSession) settle(actualQuota int, taskCommit *model.BillingTaskCommit) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if actualQuota < 0 {
		return errors.New("actual billing quota cannot be negative")
	}
	if s.settled {
		return nil
	}
	delta := actualQuota - s.preConsumedQuota
	walletDelta, subDelta := s.fundingDeltas(delta)
	tokenDelta := int64(delta)
	if s.relayInfo.IsPlayground {
		tokenDelta = 0
	}
	var commit []*model.BillingTaskCommit
	if taskCommit != nil {
		commit = append(commit, taskCommit)
	} else if err := model.RequestBillingSettlement(s.relayInfo.RequestId, walletDelta, subDelta, tokenDelta, int64(actualQuota), actualQuota > 0 || s.relayInfo.BillableUsageObserved); err != nil {
		return err
	}
	if err := model.ApplyBillingAdjustment(s.relayInfo.RequestId, "settle", walletDelta, subDelta, tokenDelta, false, true, int64(actualQuota), actualQuota > 0 || s.relayInfo.BillableUsageObserved, commit...); err != nil {
		return err
	}
	s.applyFundingDeltas(walletDelta, subDelta)
	if s.funding.Source() == BillingSourceSubscription {
		s.relayInfo.SubscriptionPostDelta += int64(delta)
	} else if s.funding.Source() == BillingSourceMixed {
		// Mixed funding mutates its allocation slices during settlement; refresh
		// the request snapshot before task persistence and consume logging.
		s.syncRelayInfo()
	}
	s.settled = true
	return nil
}

// Refund 退还所有预扣费，幂等安全，异步执行。
func (s *BillingSession) Refund(c *gin.Context) {
	s.mu.Lock()
	if s.settled || s.refunded || !s.needsRefundLocked() {
		s.mu.Unlock()
		return
	}
	if err := model.RequestBillingRefund(s.relayInfo.RequestId); err != nil {
		s.mu.Unlock()
		common.SysError(fmt.Sprintf("failed to persist billing refund request %s: %v", s.relayInfo.RequestId, err))
		return
	}
	s.refunded = true
	s.mu.Unlock()

	logger.LogInfo(c, fmt.Sprintf("用户 %d 请求失败, 返还预扣费（token_quota=%s, funding=%s）",
		s.relayInfo.UserId,
		logger.FormatQuota(s.tokenConsumed),
		s.funding.Source(),
	))

	requestId := s.relayInfo.RequestId
	gopool.Go(func() {
		if err := model.RefundBillingOperation(requestId); err != nil {
			common.SysError(fmt.Sprintf("billing refund pending for request %s: %v", requestId, err))
		}
	})
}

// NeedsRefund 返回是否存在需要退还的预扣状态。
func (s *BillingSession) NeedsRefund() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.needsRefundLocked()
}

func (s *BillingSession) needsRefundLocked() bool {
	if s.settled || s.refunded {
		return false
	}
	if s.tokenConsumed > 0 {
		return true
	}
	if wallet, ok := s.funding.(*WalletFunding); ok && wallet.consumed > 0 {
		return true
	}
	if s.extraReserved > 0 {
		return true
	}
	// 订阅可能在 tokenConsumed=0 时仍预扣了额度
	if sub, ok := s.funding.(*SubscriptionFunding); ok && sub.preConsumed > 0 {
		return true
	}
	if mixed, ok := s.funding.(*MixedFunding); ok {
		return mixed.needsRefund()
	}
	return false
}

// GetPreConsumedQuota 返回实际预扣的额度。
func (s *BillingSession) GetPreConsumedQuota() int {
	return s.preConsumedQuota
}

func (s *BillingSession) Reserve(targetQuota int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	imageRequest := false
	if s.relayInfo != nil {
		_, imageRequest = s.relayInfo.Request.(*dto.ImageRequest)
		imageRequest = imageRequest || s.relayInfo.ImageRequestCount > 0
	}
	if s.settled || s.refunded || s.trusted && !imageRequest || targetQuota <= s.preConsumedQuota {
		return nil
	}

	delta := targetQuota - s.preConsumedQuota
	if delta <= 0 {
		return nil
	}

	walletDelta, subDelta := s.fundingDeltas(delta)
	tokenDelta := int64(delta)
	if s.relayInfo.IsPlayground {
		tokenDelta = 0
	}
	phase := fmt.Sprintf("reserve:%d", s.reserveCount+1)
	if err := model.ApplyBillingAdjustment(s.relayInfo.RequestId, phase, walletDelta, subDelta, tokenDelta, imageRequest, false, 0, false); err != nil {
		if errors.Is(err, model.ErrBillingWalletInsufficient) {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		if errors.Is(err, model.ErrBillingSubscriptionInsufficient) {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
	s.reserveCount++
	s.applyFundingDeltas(walletDelta, subDelta)
	s.preConsumedQuota += delta
	s.tokenConsumed += int(tokenDelta)
	s.extraReserved += delta
	if imageRequest {
		s.trusted = false
	}
	s.syncRelayInfo()
	return nil
}

// ---------------------------------------------------------------------------
// PreConsume — 统一预扣费入口（含信任额度旁路）
// ---------------------------------------------------------------------------

// preConsume 执行预扣费：信任检查 -> 资金来源预扣 -> 令牌预扣。
// 任一步骤失败时原子回滚已完成的步骤。
func (s *BillingSession) preConsume(c *gin.Context, quota int) *types.NewAPIError {
	effectiveQuota := quota

	// ---- 信任额度旁路 ----
	if s.shouldTrust(c) {
		s.trusted = true
		effectiveQuota = 0
		logger.LogInfo(c, fmt.Sprintf("用户 %d 额度充足, 信任且不需要预扣费 (funding=%s)", s.relayInfo.UserId, s.funding.Source()))
	} else if effectiveQuota > 0 {
		logger.LogInfo(c, fmt.Sprintf("用户 %d 需要预扣费 %s (funding=%s)", s.relayInfo.UserId, logger.FormatQuota(effectiveQuota), s.funding.Source()))
	}

	channelID := 0
	if s.relayInfo.ChannelMeta != nil {
		channelID = s.relayInfo.ChannelId
	}
	op := &model.BillingOperation{RequestId: s.relayInfo.RequestId, UserId: s.relayInfo.UserId,
		ChannelId: channelID, FundingSource: s.funding.Source(), PreConsumed: int64(effectiveQuota)}
	if !s.relayInfo.IsPlayground {
		op.TokenId = s.relayInfo.TokenId
	}
	err := model.CreateBillingReservation(op, func(tx *gorm.DB) error {
		// Funding rows are locked before the token row. This is the canonical
		// subscription -> user -> token order shared by every billing phase.
		if err := s.preConsumeFundingTx(tx, op, effectiveQuota); err != nil {
			return err
		}
		if effectiveQuota > 0 && !s.relayInfo.IsPlayground {
			reserved, err := model.TryReserveTokenQuotaTx(tx, op.TokenId, int64(effectiveQuota), s.relayInfo.TokenUnlimited)
			if err != nil {
				return err
			}
			if !reserved {
				return model.ErrBillingTokenInsufficient
			}
			op.TokenAmount = int64(effectiveQuota)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, model.ErrBillingTokenInsufficient) {
			return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		// TODO: model 层应定义哨兵错误（如 ErrNoActiveSubscription），用 errors.Is 替代字符串匹配
		if errors.Is(err, ErrInsufficientWalletQuota) {
			userQuota, quotaErr := model.GetUserQuota(s.relayInfo.UserId, false)
			if quotaErr != nil {
				userQuota = 0
			}
			return types.NewErrorWithStatusCode(
				fmt.Errorf("用户额度不足, 剩余额度: %s", logger.FormatQuota64(userQuota)),
				types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
				types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		errMsg := err.Error()
		if strings.Contains(errMsg, "no active subscription") || strings.Contains(errMsg, "subscription quota insufficient") {
			return types.NewErrorWithStatusCode(fmt.Errorf("订阅额度不足或未配置订阅: %s", errMsg), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
	s.tokenConsumed = int(op.TokenAmount)
	s.preConsumedQuota = effectiveQuota
	s.hydrateSubscriptionPlan()

	// ---- 同步 RelayInfo 兼容字段 ----
	s.syncRelayInfo()

	return nil
}

// fundingDeltas preserves the established subscription-first allocation: an
// upward difference goes to the wallet, a downward difference refunds wallet
// before subscription. A later image reservation always charges atomically.
func (s *BillingSession) fundingDeltas(delta int) (wallet, subscription int64) {
	switch funding := s.funding.(type) {
	case *WalletFunding:
		return int64(delta), 0
	case *SubscriptionFunding:
		return 0, int64(delta)
	case *MixedFunding:
		if delta >= 0 {
			return int64(delta), 0
		}
		walletRefund := min(-delta, funding.walletAmount)
		return -int64(walletRefund), int64(delta + walletRefund)
	}
	return 0, 0
}

func (s *BillingSession) applyFundingDeltas(wallet, subscription int64) {
	switch funding := s.funding.(type) {
	case *WalletFunding:
		funding.consumed += int(wallet)
	case *MixedFunding:
		funding.walletAmount += int(wallet)
		funding.wallet.consumed += int(wallet)
		funding.subscriptionAmount += int(subscription)
	}
}

func (s *BillingSession) preConsumeFundingTx(tx *gorm.DB, op *model.BillingOperation, amount int) error {
	switch funding := s.funding.(type) {
	case *WalletFunding:
		reserved, err := model.TryReserveUserQuotaTx(tx, funding.userId, int64(amount))
		if err != nil {
			return err
		}
		if !reserved {
			return ErrInsufficientWalletQuota
		}
		funding.consumed = amount
		op.WalletAmount = int64(amount)
	case *SubscriptionFunding:
		res, err := model.PreConsumeUserSubscriptionTx(tx, funding.requestId, funding.userId,
			funding.modelName, 0, funding.amount, funding.usingGroup)
		if err != nil {
			return err
		}
		funding.subscriptionId, funding.preConsumed = res.UserSubscriptionId, res.PreConsumed
		funding.AmountTotal, funding.AmountUsedAfter = res.AmountTotal, res.AmountUsedAfter
		op.SubscriptionId, op.SubscriptionAmount = res.UserSubscriptionId, res.PreConsumed
	case *MixedFunding:
		res, err := model.PreConsumeUserSubscriptionPartialTx(tx, funding.subscription.requestId,
			funding.subscription.userId, funding.subscription.modelName, 0, int64(amount), funding.subscription.usingGroup)
		if err != nil {
			return err
		}
		if res == nil || res.SubscriptionPreConsumeResult == nil || res.PreConsumed <= 0 {
			return fmt.Errorf("subscription quota insufficient, need=%d", amount)
		}
		funding.subscription.subscriptionId, funding.subscription.preConsumed = res.UserSubscriptionId, res.PreConsumed
		funding.subscription.AmountTotal, funding.subscription.AmountUsedAfter = res.AmountTotal, res.AmountUsedAfter
		funding.subscriptionAmount = int(res.PreConsumed)
		funding.walletAmount = amount - funding.subscriptionAmount
		if funding.walletAmount > 0 {
			reserved, err := model.TryReserveUserQuotaTx(tx, funding.wallet.userId, int64(funding.walletAmount))
			if err != nil {
				return err
			}
			if !reserved {
				return ErrInsufficientWalletQuota
			}
		}
		funding.wallet.consumed = funding.walletAmount
		op.SubscriptionId, op.SubscriptionAmount = res.UserSubscriptionId, res.PreConsumed
		op.WalletAmount = int64(funding.walletAmount)
	default:
		return fmt.Errorf("unsupported funding source %T", s.funding)
	}
	return nil
}

func (s *BillingSession) hydrateSubscriptionPlan() {
	var funding *SubscriptionFunding
	switch f := s.funding.(type) {
	case *SubscriptionFunding:
		funding = f
	case *MixedFunding:
		funding = f.subscription
	}
	if funding == nil || funding.subscriptionId <= 0 {
		return
	}
	if info, err := model.GetSubscriptionPlanInfoByUserSubscriptionId(funding.subscriptionId); err == nil && info != nil {
		funding.PlanId, funding.PlanTitle = info.PlanId, info.PlanTitle
	}
}

// shouldTrust 统一信任额度检查，适用于钱包和订阅。
func (s *BillingSession) shouldTrust(c *gin.Context) bool {
	// 异步任务（ForcePreConsume=true）必须预扣全额，不允许信任旁路
	if s.relayInfo.ForcePreConsume {
		return false
	}

	trustQuota := operation_setting.GetQuotaSetting().TrustQuotaUSD * common.QuotaPerUnit
	if trustQuota <= 0 || math.IsNaN(trustQuota) || math.IsInf(trustQuota, 0) {
		return false
	}

	// 检查令牌是否充足
	tokenTrusted := s.relayInfo.TokenUnlimited
	if !tokenTrusted {
		tokenQuota := common.GetContextKeyInt64(c, constant.ContextKey("token_quota"))
		tokenTrusted = float64(tokenQuota) > trustQuota
	}
	if !tokenTrusted {
		return false
	}

	switch s.funding.Source() {
	case BillingSourceWallet:
		return float64(s.relayInfo.UserQuota) > trustQuota
	case BillingSourceSubscription:
		// 订阅不能启用信任旁路。原因：
		// 1. PreConsumeUserSubscription 要求 amount>0 来创建预扣记录并锁定订阅
		// 2. SubscriptionFunding.PreConsume 忽略参数，始终用 s.amount 预扣
		// 3. 若信任旁路将 effectiveQuota 设为 0，会导致 preConsumedQuota 与实际订阅预扣不一致
		return false
	default:
		return false
	}
}

// syncRelayInfo 将 BillingSession 的状态同步到 RelayInfo 的兼容字段上。
func (s *BillingSession) syncRelayInfo() {
	info := s.relayInfo
	info.FinalPreConsumedQuota = s.preConsumedQuota
	info.BillingSource = s.funding.Source()

	if sub, ok := s.funding.(*SubscriptionFunding); ok {
		info.SubscriptionId = sub.subscriptionId
		info.SubscriptionPreConsumed = sub.preConsumed + int64(s.extraReserved)
		info.SubscriptionPostDelta = 0
		info.SubscriptionAmountTotal = sub.AmountTotal
		info.SubscriptionAmountUsedAfterPreConsume = sub.AmountUsedAfter + int64(s.extraReserved)
		info.SubscriptionPlanId = sub.PlanId
		info.SubscriptionPlanTitle = sub.PlanTitle
		info.BillingAllocations = nil
	} else if mixed, ok := s.funding.(*MixedFunding); ok {
		info.SubscriptionId = 0
		info.SubscriptionPreConsumed = 0
		info.SubscriptionPostDelta = 0
		info.SubscriptionAmountTotal = 0
		info.SubscriptionAmountUsedAfterPreConsume = 0
		info.SubscriptionPlanId = 0
		info.SubscriptionPlanTitle = ""
		info.BillingAllocations = mixed.Allocations()
	} else {
		info.SubscriptionId = 0
		info.SubscriptionPreConsumed = 0
		info.SubscriptionPostDelta = 0
		info.SubscriptionAmountTotal = 0
		info.SubscriptionAmountUsedAfterPreConsume = 0
		info.SubscriptionPlanId = 0
		info.SubscriptionPlanTitle = ""
		info.BillingAllocations = nil
	}
}

// ---------------------------------------------------------------------------
// NewBillingSession 工厂 — 根据计费偏好创建会话并处理回退
// ---------------------------------------------------------------------------

// NewBillingSession 根据用户计费偏好创建 BillingSession，处理 subscription_first / wallet_first 的回退。
func NewBillingSession(c *gin.Context, relayInfo *relaycommon.RelayInfo, preConsumedQuota int) (*BillingSession, *types.NewAPIError) {
	if relayInfo == nil {
		return nil, types.NewError(fmt.Errorf("relayInfo is nil"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	if relayInfo.RequestId == "" {
		relayInfo.RequestId = common.NewRequestId()
	}

	pref := common.NormalizeBillingPreference(relayInfo.UserSetting.BillingPreference)

	// 钱包路径需要先检查用户额度
	tryWallet := func() (*BillingSession, *types.NewAPIError) {
		userQuota, err := model.GetUserQuota(relayInfo.UserId, false)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
		}
		if userQuota <= 0 {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("用户额度不足, 剩余额度: %s", logger.FormatQuota64(userQuota)),
				types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
				types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		if userQuota-int64(preConsumedQuota) < 0 {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("预扣费额度失败, 用户剩余额度: %s, 需要预扣费额度: %s", logger.FormatQuota64(userQuota), logger.FormatQuota(preConsumedQuota)),
				types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
				types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		relayInfo.UserQuota = userQuota

		session := &BillingSession{
			relayInfo: relayInfo,
			funding:   &WalletFunding{userId: relayInfo.UserId},
		}
		if apiErr := session.preConsume(c, preConsumedQuota); apiErr != nil {
			return nil, apiErr
		}
		return session, nil
	}

	trySubscription := func() (*BillingSession, *types.NewAPIError) {
		subConsume := int64(preConsumedQuota)
		if subConsume <= 0 {
			subConsume = 1
		}
		session := &BillingSession{
			relayInfo: relayInfo,
			funding: &SubscriptionFunding{
				requestId:  relayInfo.RequestId,
				userId:     relayInfo.UserId,
				modelName:  relayInfo.GetBillingModelName(),
				usingGroup: relayInfo.UsingGroup,
				amount:     subConsume,
			},
		}
		// 必须传 subConsume 而非 preConsumedQuota，保证 SubscriptionFunding.amount、
		// preConsume 参数和 FinalPreConsumedQuota 三者一致，避免订阅多扣费。
		if apiErr := session.preConsume(c, int(subConsume)); apiErr != nil {
			return nil, apiErr
		}
		return session, nil
	}

	// Keep the local mixed-funding business contract as a narrow funding
	// source extension. The surrounding session lifecycle, image reservation,
	// plugin task flow, and retry behavior remain upstream rc.39.
	tryMixedSubscriptionWallet := func() (*BillingSession, *types.NewAPIError) {
		if preConsumedQuota <= 0 {
			return trySubscription()
		}
		userQuota, err := model.GetUserQuota(relayInfo.UserId, false)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
		}
		if userQuota <= 0 {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("订阅额度不足且用户余额不足, 剩余额度: %s", logger.FormatQuota64(userQuota)),
				types.ErrorCodeInsufficientUserQuota,
				http.StatusForbidden,
				types.ErrOptionWithSkipRetry(),
				types.ErrOptionWithNoRecordErrorLog(),
			)
		}
		relayInfo.UserQuota = userQuota

		session := &BillingSession{
			relayInfo: relayInfo,
			funding: &MixedFunding{
				subscription: &SubscriptionFunding{
					requestId:  relayInfo.RequestId,
					userId:     relayInfo.UserId,
					modelName:  relayInfo.GetBillingModelName(),
					usingGroup: relayInfo.UsingGroup,
					amount:     int64(preConsumedQuota),
				},
				wallet: &WalletFunding{userId: relayInfo.UserId},
			},
		}
		if apiErr := session.preConsume(c, preConsumedQuota); apiErr != nil {
			if apiErr.GetErrorCode() == types.ErrorCodeInsufficientUserQuota {
				if strings.Contains(apiErr.Error(), "subscription quota insufficient") || strings.Contains(apiErr.Error(), "no active subscription") {
					return tryWallet()
				}
			}
			return nil, apiErr
		}
		return session, nil
	}

	switch pref {
	case "subscription_only":
		return trySubscription()
	case "wallet_only":
		return tryWallet()
	case "wallet_first":
		session, err := tryWallet()
		if err != nil {
			if err.GetErrorCode() == types.ErrorCodeInsufficientUserQuota {
				return trySubscription()
			}
			return nil, err
		}
		return session, nil
	case "subscription_first":
		fallthrough
	default:
		hasSub, subCheckErr := model.HasActiveUserSubscriptionForGroup(relayInfo.UserId, relayInfo.UsingGroup)
		if subCheckErr != nil {
			return nil, types.NewError(subCheckErr, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
		}
		if !hasSub {
			return tryWallet()
		}
		session, apiErr := trySubscription()
		if apiErr != nil {
			if apiErr.GetErrorCode() == types.ErrorCodeInsufficientUserQuota {
				// subscription_first always consumes the remaining subscription quota
				// first, then covers the remainder from the wallet. The plan flag is
				// not a gate for this billing preference.
				return tryMixedSubscriptionWallet()
			}
			return nil, apiErr
		}
		return session, nil
	}
}
