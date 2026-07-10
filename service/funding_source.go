package service

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// ---------------------------------------------------------------------------
// FundingSource — 资金来源接口（钱包 or 订阅）
// ---------------------------------------------------------------------------

// FundingSource 抽象了预扣费的资金来源。
type FundingSource interface {
	// Source 返回资金来源标识："wallet" 或 "subscription"
	Source() string
	// PreConsume 从该资金来源预扣 amount 额度
	PreConsume(amount int) error
	// Settle 根据差额调整资金来源（正数补扣，负数退还）
	Settle(delta int) error
	// Refund 退还所有预扣费
	Refund() error
}

// ---------------------------------------------------------------------------
// WalletFunding — 钱包资金来源实现
// ---------------------------------------------------------------------------

type WalletFunding struct {
	userId   int
	consumed int // 实际预扣的用户额度
}

func (w *WalletFunding) Source() string { return BillingSourceWallet }

func (w *WalletFunding) PreConsume(amount int) error {
	if amount <= 0 {
		return nil
	}
	if err := model.DecreaseUserQuota(w.userId, amount, false); err != nil {
		return err
	}
	w.consumed = amount
	return nil
}

func (w *WalletFunding) Settle(delta int) error {
	if delta == 0 {
		return nil
	}
	if delta > 0 {
		return model.DecreaseUserQuota(w.userId, delta, false)
	}
	return model.IncreaseUserQuota(w.userId, -delta, false)
}

func (w *WalletFunding) Refund() error {
	if w.consumed <= 0 {
		return nil
	}
	// IncreaseUserQuota 是 quota += N 的非幂等操作，不能重试，否则会多退额度。
	// 订阅的 RefundSubscriptionPreConsume 有 requestId 幂等保护所以可以重试。
	return model.IncreaseUserQuota(w.userId, w.consumed, false)
}

// ---------------------------------------------------------------------------
// SubscriptionFunding — 订阅资金来源实现
// ---------------------------------------------------------------------------

type SubscriptionFunding struct {
	requestId      string
	userId         int
	modelName      string
	usingGroup     string
	amount         int64 // 预扣的订阅额度（subConsume）
	subscriptionId int
	preConsumed    int64
	// 以下字段在 PreConsume 成功后填充，供 RelayInfo 同步使用
	AmountTotal     int64
	AmountUsedAfter int64
	PlanId          int
	PlanTitle       string
}

func (s *SubscriptionFunding) Source() string { return BillingSourceSubscription }

func (s *SubscriptionFunding) PreConsume(_ int) error {
	// amount 参数被忽略，使用内部 s.amount（已在构造时根据 preConsumedQuota 计算）
	res, err := model.PreConsumeUserSubscription(s.requestId, s.userId, s.modelName, 0, s.amount, s.usingGroup)
	if err != nil {
		return err
	}
	s.subscriptionId = res.UserSubscriptionId
	s.preConsumed = res.PreConsumed
	s.AmountTotal = res.AmountTotal
	s.AmountUsedAfter = res.AmountUsedAfter
	// 获取订阅计划信息
	if planInfo, err := model.GetSubscriptionPlanInfoByUserSubscriptionId(res.UserSubscriptionId); err == nil && planInfo != nil {
		s.PlanId = planInfo.PlanId
		s.PlanTitle = planInfo.PlanTitle
	}
	return nil
}

func (s *SubscriptionFunding) Settle(delta int) error {
	if delta == 0 {
		return nil
	}
	return model.PostConsumeUserSubscriptionDelta(s.subscriptionId, int64(delta))
}

func (s *SubscriptionFunding) Refund() error {
	if s.preConsumed <= 0 {
		return nil
	}
	return refundWithRetry(func() error {
		return model.RefundSubscriptionPreConsume(s.requestId)
	})
}

// ---------------------------------------------------------------------------
// MixedFunding — subscription_first uses subscription quota first, then wallet
// ---------------------------------------------------------------------------

type MixedFunding struct {
	subscription *SubscriptionFunding
	wallet       *WalletFunding

	subscriptionAmount int
	walletAmount       int
}

func (m *MixedFunding) Source() string { return BillingSourceMixed }

func (m *MixedFunding) PreConsume(amount int) error {
	if amount <= 0 {
		return nil
	}
	if m.subscription == nil || m.wallet == nil {
		return fmt.Errorf("mixed funding source is incomplete")
	}

	res, err := model.PreConsumeUserSubscriptionPartial(
		m.subscription.requestId,
		m.subscription.userId,
		m.subscription.modelName,
		0,
		int64(amount),
		m.subscription.usingGroup,
	)
	if err != nil {
		return err
	}
	if res == nil || res.SubscriptionPreConsumeResult == nil || res.PreConsumed <= 0 {
		return fmt.Errorf("subscription quota insufficient, need=%d", amount)
	}
	m.subscription.subscriptionId = res.UserSubscriptionId
	m.subscription.preConsumed = res.PreConsumed
	m.subscription.AmountTotal = res.AmountTotal
	m.subscription.AmountUsedAfter = res.AmountUsedAfter
	if planInfo, err := model.GetSubscriptionPlanInfoByUserSubscriptionId(res.UserSubscriptionId); err == nil && planInfo != nil {
		m.subscription.PlanId = planInfo.PlanId
		m.subscription.PlanTitle = planInfo.PlanTitle
	}

	m.subscriptionAmount = int(res.PreConsumed)
	m.walletAmount = amount - m.subscriptionAmount
	if m.walletAmount < 0 {
		m.walletAmount = 0
	}
	if m.walletAmount > 0 {
		userQuota, err := model.GetUserQuota(m.wallet.userId, false)
		if err != nil {
			_ = m.subscription.Refund()
			return err
		}
		if userQuota < m.walletAmount {
			_ = m.subscription.Refund()
			return fmt.Errorf("user quota is not enough, user quota: %s, need quota: %s", logger.FormatQuota(userQuota), logger.FormatQuota(m.walletAmount))
		}
		if err := m.wallet.PreConsume(m.walletAmount); err != nil {
			if m.subscription != nil && m.subscription.preConsumed > 0 {
				_ = m.subscription.Refund()
			}
			return err
		}
	}
	return nil
}

func (m *MixedFunding) Settle(delta int) error {
	if delta == 0 {
		return nil
	}
	if delta > 0 {
		return m.settlePositive(delta)
	}
	return m.settleNegative(-delta)
}

func (m *MixedFunding) settlePositive(delta int) error {
	if delta <= 0 {
		return nil
	}
	if err := m.wallet.Settle(delta); err != nil {
		return err
	}
	m.walletAmount += delta
	m.wallet.consumed += delta
	return nil
}

func (m *MixedFunding) settleNegative(refund int) error {
	if refund <= 0 {
		return nil
	}
	walletRefund := min(refund, m.walletAmount)
	if walletRefund > 0 {
		if err := m.wallet.Settle(-walletRefund); err != nil {
			return err
		}
		m.walletAmount -= walletRefund
		m.wallet.consumed -= walletRefund
		if m.wallet.consumed < 0 {
			m.wallet.consumed = 0
		}
		refund -= walletRefund
	}
	if refund > 0 {
		if err := m.subscription.Settle(-refund); err != nil {
			return err
		}
		m.subscriptionAmount -= refund
		if m.subscriptionAmount < 0 {
			m.subscriptionAmount = 0
		}
	}
	return nil
}

func (m *MixedFunding) Refund() error {
	if m.wallet != nil && m.wallet.consumed > 0 {
		if err := m.wallet.Refund(); err != nil {
			return err
		}
	}
	if m.subscription != nil && m.subscription.preConsumed > 0 {
		return m.subscription.Refund()
	}
	return nil
}

func (m *MixedFunding) Allocations() []relaycommon.BillingAllocation {
	if m == nil {
		return nil
	}
	allocations := make([]relaycommon.BillingAllocation, 0, 2)
	if m.subscription != nil && m.subscriptionAmount > 0 {
		usedAfter := m.subscription.AmountUsedAfter - m.subscription.preConsumed + int64(m.subscriptionAmount)
		allocations = append(allocations, relaycommon.BillingAllocation{
			Source:                             BillingSourceSubscription,
			Quota:                              m.subscriptionAmount,
			SubscriptionId:                     m.subscription.subscriptionId,
			SubscriptionPlanId:                 m.subscription.PlanId,
			SubscriptionPlanTitle:              m.subscription.PlanTitle,
			SubscriptionAmountTotal:            m.subscription.AmountTotal,
			SubscriptionAmountUsedAfterConsume: usedAfter,
		})
	}
	if m.walletAmount > 0 {
		allocations = append(allocations, relaycommon.BillingAllocation{
			Source: BillingSourceWallet,
			Quota:  m.walletAmount,
		})
	}
	return allocations
}

// refundWithRetry 尝试多次执行退款操作以提高成功率，只能用于基于事务的退款函数！！！！！！
// try to refund with retries, only for refund functions based on transactions!!!
func refundWithRetry(fn func() error) error {
	if fn == nil {
		return nil
	}
	const maxAttempts = 3
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if i < maxAttempts-1 {
			time.Sleep(time.Duration(200*(i+1)) * time.Millisecond)
		}
	}
	return lastErr
}
