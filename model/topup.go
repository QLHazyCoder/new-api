package model

import (
	"errors"
	"fmt"
	"maps"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type TopUp struct {
	Id              int     `json:"id"`
	UserId          int     `json:"user_id" gorm:"index"`
	Amount          int64   `json:"amount"`
	Money           float64 `json:"money"`
	TradeNo         string  `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentMethod   string  `json:"payment_method" gorm:"type:varchar(50)"`
	PaymentProvider string  `json:"payment_provider" gorm:"type:varchar(50);default:''"`
	// PricingSnapshot is an immutable audit record created with a pending
	// order. It deliberately remains outside the public TopUp JSON payload;
	// history handlers parse and expose a safe view of it when appropriate.
	PricingSnapshot string `json:"-" gorm:"type:text"`
	CreateTime      int64  `json:"create_time"`
	CompleteTime    int64  `json:"complete_time"`
	Status          string `json:"status"`
}

const (
	PaymentMethodStripe       = "stripe"
	PaymentMethodCreem        = "creem"
	PaymentMethodWaffo        = "waffo"
	PaymentMethodWaffoPancake = "waffo_pancake"
	PaymentMethodBalance      = "balance"
)

const (
	PaymentProviderEpay         = "epay"
	PaymentProviderStripe       = "stripe"
	PaymentProviderCreem        = "creem"
	PaymentProviderWaffo        = "waffo"
	PaymentProviderWaffoPancake = "waffo_pancake"
	PaymentProviderBalance      = "balance"
)

var (
	ErrPaymentMethodMismatch    = errors.New("payment method mismatch")
	ErrTopUpNotFound            = errors.New("topup not found")
	ErrTopUpStatusInvalid       = errors.New("topup status invalid")
	ErrInvalidTopUpQuota        = errors.New("invalid top-up quota")
	ErrTopUpQuotaLimitExceeded  = errors.New("top-up quota limit exceeded")
	ErrWalletQuotaLimitExceeded = errors.New("wallet quota limit exceeded")
)

// TopUpCompletionResult is shared by every payment callback.  Keeping the
// settlement result independent from a provider prevents a provider-specific
// path from skipping quota protection, referral accounting, or cache repair.
type TopUpCompletionResult struct {
	TradeNo            string
	UserId             int
	Amount             int64
	QuotaToAdd         int64
	PayMoney           float64
	PaymentMethod      string
	PaymentProvider    string
	InviteRewardQuota  int64
	InviteRewardUserId int
	AlreadyCompleted   bool
}

type CompleteTopUpOptions struct {
	TradeNo                 string
	ExpectedPaymentProvider string
	CallerIp                string
	CallbackPaymentMethod   string
	StripeCustomer          string
	CustomerEmail           string
}

func (topUp *TopUp) Insert() error {
	var err error
	err = DB.Create(topUp).Error
	return err
}

func topUpQuotaMaxCurrent(creditedQuota int64) (int64, error) {
	if creditedQuota <= 0 || creditedQuota > common.MaxWalletQuota {
		return 0, ErrInvalidTopUpQuota
	}
	return common.MaxWalletQuota - creditedQuota, nil
}

// ValidateTopUpQuotaCapacity performs the user-facing pre-payment check. The
// settlement path repeats the same invariant with an atomic conditional
// update, because the wallet balance can change after checkout creation.
func ValidateTopUpQuotaCapacity(userId int, creditedQuota int64) error {
	maxCurrentQuota, err := topUpQuotaMaxCurrent(creditedQuota)
	if err != nil {
		return err
	}

	var user User
	if err := DB.Select("quota").Where("id = ?", userId).First(&user).Error; err != nil {
		return err
	}
	if user.Quota > maxCurrentQuota {
		return ErrTopUpQuotaLimitExceeded
	}
	return nil
}

// creditTopUpQuota atomically enforces the wallet ceiling while adding quota.
// Keeping the predicate and increment in one UPDATE prevents two
// concurrent callbacks from both passing a separate read/check.
func creditTopUpQuota(tx *gorm.DB, userId int, creditedQuota int64, updates map[string]any) error {
	_, err := topUpQuotaMaxCurrent(creditedQuota)
	if err != nil {
		return err
	}
	if err := updateUserQuotaWithDeltaTx(tx, userId, creditedQuota, maps.Clone(updates)); err == nil {
		return nil
	} else if errors.Is(err, common.ErrWalletQuotaOverflow) {
		return ErrTopUpQuotaLimitExceeded
	} else {
		return err
	}
}

func (topUp *TopUp) Update() error {
	var err error
	err = DB.Save(topUp).Error
	return err
}

func GetTopUpById(id int) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("id = ?", id).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func GetTopUpByTradeNo(tradeNo string) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("trade_no = ?", tradeNo).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func UpdatePendingTopUpStatus(tradeNo string, expectedPaymentProvider string, targetStatus string) error {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if expectedPaymentProvider != "" && topUp.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}

		topUp.Status = targetStatus
		return tx.Save(topUp).Error
	})
}

// getTopUpQuotaToAdd is the one place where a persisted order is converted to
// wallet quota.  A pending order carries its price (and, for supported
// providers, its pricing snapshot), so a later policy change cannot alter its
// settlement amount.  WalletQuotaFromDecimalStrict keeps this conversion in
// the signed int64 domain instead of passing through a float64/JS-safe integer.
func getTopUpQuotaToAdd(topUp *TopUp) (int64, error) {
	switch topUp.PaymentProvider {
	case PaymentProviderStripe:
		return common.WalletQuotaFromDecimalStrict(
			decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
		)
	case PaymentProviderCreem:
		return common.WalletQuotaFromDecimalStrict(decimal.NewFromInt(topUp.Amount))
	default:
		return common.WalletQuotaFromDecimalStrict(
			decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
		)
	}
}

func calculateTopUpInviteReward(quotaToAdd int64) (int64, string) {
	if quotaToAdd <= 0 || !operation_setting.IsPaymentComplianceConfirmed() {
		return 0, ""
	}
	if common.TopUpInviteRewardPercent <= 0 {
		return 0, ""
	}
	percent := decimal.NewFromFloat(common.TopUpInviteRewardPercent)
	reward, err := common.WalletQuotaFromDecimalStrict(
		decimal.NewFromInt(quotaToAdd).Mul(percent).Div(decimal.NewFromInt(100)),
	)
	if err != nil {
		return 0, percent.String()
	}
	return reward, percent.String()
}

// grantTopUpInviteRewardTx updates the aggregate fields and immutable ledger
// in the same transaction as the payment order.  The ledger idempotency key
// protects the accounting invariant even if a future caller reuses this path.
func grantTopUpInviteRewardTx(tx *gorm.DB, inviterId int, topUp *TopUp, quotaToAdd int64) (reward int64, rewardUserId int, err error) {
	if inviterId <= 0 || inviterId == topUp.UserId {
		return 0, 0, nil
	}
	reward, rewardPercent := calculateTopUpInviteReward(quotaToAdd)
	if reward <= 0 {
		return 0, 0, nil
	}
	if err := updateUserQuotaFieldDeltaTx(tx, inviterId, "aff_quota", reward); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	if err := updateUserQuotaFieldDeltaTx(tx, inviterId, "aff_history", reward); err != nil {
		return 0, 0, err
	}
	idempotencyKey := affiliateRewardIdempotencyKey("topup", topUp.TradeNo)
	if err := createAffiliateRewardEventTx(tx, &AffiliateRewardEvent{
		InviterId:      inviterId,
		InviteeId:      topUp.UserId,
		EventType:      AffiliateRewardEventTypeTopUp,
		SourceType:     AffiliateRewardSourceTypeTopUp,
		SourceId:       topUp.TradeNo,
		IdempotencyKey: &idempotencyKey,
		BaseQuota:      quotaToAdd,
		RewardPercent:  rewardPercent,
		RewardQuota:    reward,
		AffQuotaDelta:  reward,
	}); err != nil {
		return 0, 0, err
	}
	return reward, inviterId, nil
}

// CompleteTopUp is the only order-settlement transaction used by top-up
// providers.  It retains the upstream row locking and provider checks while
// preserving this fork's referral ledger and full-int64 wallet guarantees.
func CompleteTopUp(opts CompleteTopUpOptions) (*TopUpCompletionResult, error) {
	if opts.TradeNo == "" {
		return nil, errors.New("missing topup trade number")
	}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	completion := &TopUpCompletionResult{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		if err := lockForUpdate(tx).Where(refCol+" = ?", opts.TradeNo).First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}

		completion.TradeNo = topUp.TradeNo
		completion.UserId = topUp.UserId
		completion.Amount = topUp.Amount
		completion.PayMoney = topUp.Money
		completion.PaymentMethod = topUp.PaymentMethod
		completion.PaymentProvider = topUp.PaymentProvider

		if opts.ExpectedPaymentProvider != "" && topUp.PaymentProvider != opts.ExpectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status == common.TopUpStatusSuccess {
			completion.AlreadyCompleted = true
			return nil
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}

		quotaToAdd, quotaErr := getTopUpQuotaToAdd(topUp)
		if quotaErr != nil || quotaToAdd <= 0 {
			return ErrInvalidTopUpQuota
		}

		user := &User{}
		if err := tx.Select("id", "email", "inviter_id").Where("id = ?", topUp.UserId).First(user).Error; err != nil {
			return err
		}

		if opts.CallbackPaymentMethod != "" {
			topUp.PaymentMethod = opts.CallbackPaymentMethod
		}
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		updateFields := map[string]any{}
		if opts.StripeCustomer != "" {
			updateFields["stripe_customer"] = opts.StripeCustomer
		}
		if opts.CustomerEmail != "" && user.Email == "" {
			updateFields["email"] = opts.CustomerEmail
		}
		if err := creditTopUpQuota(tx, topUp.UserId, quotaToAdd, updateFields); err != nil {
			return err
		}

		reward, rewardUserId, err := grantTopUpInviteRewardTx(tx, user.InviterId, topUp, quotaToAdd)
		if err != nil {
			return err
		}

		completion.QuotaToAdd = quotaToAdd
		completion.PaymentMethod = topUp.PaymentMethod
		completion.InviteRewardQuota = reward
		completion.InviteRewardUserId = rewardUserId
		return nil
	})
	if err != nil {
		return nil, err
	}
	if completion.AlreadyCompleted {
		return completion, nil
	}

	// The user cache only carries spendable quota, so only the actual top-up is
	// applied as a delta.  Referral rewards are aff_quota, not spendable quota;
	// invalidate that user's cache rather than accidentally crediting Quota.
	syncCreditUserQuotaCache(completion.UserId, completion.QuotaToAdd, "topup")
	if opts.CustomerEmail != "" {
		_ = invalidateUserCache(completion.UserId)
	}
	if completion.InviteRewardUserId > 0 {
		_ = invalidateUserCache(completion.InviteRewardUserId)
		RecordLog(
			completion.InviteRewardUserId,
			LogTypeSystem,
			fmt.Sprintf("Referral topup reward %s (topup_user_id=%d, trade_no=%s)", logger.LogQuota64(completion.InviteRewardQuota), completion.UserId, completion.TradeNo),
		)
	}
	return completion, nil
}

// RechargeEpay retains the provider callback API but delegates settlement to
// the shared transaction.  alreadyDone is intentionally returned for Epay's
// retry protocol.
func RechargeEpay(tradeNo string, actualPaymentMethod string, callerIp string) (bool, error) {
	result, err := CompleteTopUp(CompleteTopUpOptions{
		TradeNo:                 tradeNo,
		ExpectedPaymentProvider: PaymentProviderEpay,
		CallerIp:                callerIp,
		CallbackPaymentMethod:   actualPaymentMethod,
	})
	if err != nil {
		if !errors.Is(err, ErrTopUpNotFound) && !errors.Is(err, ErrPaymentMethodMismatch) && !errors.Is(err, ErrTopUpStatusInvalid) {
			common.SysError("epay topup failed: " + err.Error())
		}
		return false, err
	}
	if result.AlreadyCompleted {
		return true, nil
	}

	common.SysLog(fmt.Sprintf("易支付充值成功 trade_no=%s user_id=%d quota_to_add=%d money=%.2f", result.TradeNo, result.UserId, result.QuotaToAdd, result.PayMoney))
	RecordTopupLog(result.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%f", logger.LogQuota64(result.QuotaToAdd), result.PayMoney), callerIp, result.PaymentMethod, PaymentProviderEpay)
	return false, nil
}

func Recharge(referenceId string, customerId string, callerIp string) error {
	result, err := CompleteTopUp(CompleteTopUpOptions{
		TradeNo:                 referenceId,
		ExpectedPaymentProvider: PaymentProviderStripe,
		CallerIp:                callerIp,
		StripeCustomer:          customerId,
	})
	if err != nil {
		common.SysError("topup failed: " + err.Error())
		// Preserve the upstream public error surface for the browser return path.
		return errors.New("充值失败，请稍后重试")
	}
	if !result.AlreadyCompleted {
		RecordTopupLog(result.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%d", logger.FormatQuota64(result.QuotaToAdd), result.Amount), callerIp, result.PaymentMethod, PaymentMethodStripe)
	}
	return nil
}

// topUpQueryWindowSeconds 限制充值记录查询的时间窗口（秒）。
const topUpQueryWindowSeconds int64 = 30 * 24 * 60 * 60

// topUpQueryCutoff 返回允许查询的最早 create_time（秒级 Unix 时间戳）。
func topUpQueryCutoff() int64 {
	return common.GetTimestamp() - topUpQueryWindowSeconds
}

func GetUserTopUps(userId int, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	// Start transaction
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	cutoff := topUpQueryCutoff()

	// Get total count within transaction
	err = tx.Model(&TopUp{}).Where("user_id = ? AND create_time >= ?", userId, cutoff).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated topups within same transaction
	err = tx.Where("user_id = ? AND create_time >= ?", userId, cutoff).Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// GetAllTopUps 获取全平台的充值记录（管理员使用，不限制时间窗口）
func GetAllTopUps(pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err = tx.Model(&TopUp{}).Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// searchTopUpCountHardLimit 搜索充值记录时 COUNT 的安全上限，
// 防止对超大表执行无界 COUNT 触发 DoS。
const searchTopUpCountHardLimit = 10000

// SearchUserTopUps 按订单号搜索某用户的充值记录
func SearchUserTopUps(userId int, keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{}).Where("user_id = ? AND create_time >= ?", userId, topUpQueryCutoff())
	if keyword != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to count search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// SearchAllTopUps 按订单号搜索全平台充值记录（管理员使用，不限制时间窗口）
func SearchAllTopUps(keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{})
	if keyword != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to count search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// ManualCompleteTopUp 管理员手动完成订单并给用户充值
func ManualCompleteTopUp(tradeNo string, callerIp string) error {
	result, err := CompleteTopUp(CompleteTopUpOptions{
		TradeNo:  tradeNo,
		CallerIp: callerIp,
	})
	if err != nil {
		return err
	}
	if !result.AlreadyCompleted {
		RecordTopupLog(result.UserId, fmt.Sprintf("管理员补单成功，充值金额: %v，支付金额：%f", logger.FormatQuota64(result.QuotaToAdd), result.PayMoney), callerIp, result.PaymentMethod, "admin")
	}
	return nil
}

func RechargeCreem(referenceId string, customerEmail string, customerName string, callerIp string) error {
	result, err := CompleteTopUp(CompleteTopUpOptions{
		TradeNo:                 referenceId,
		ExpectedPaymentProvider: PaymentProviderCreem,
		CallerIp:                callerIp,
		CustomerEmail:           customerEmail,
	})
	if err != nil {
		common.SysError("creem topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}
	if !result.AlreadyCompleted {
		RecordTopupLog(result.UserId, fmt.Sprintf("使用Creem充值成功，充值额度: %v，支付金额：%.2f", logger.FormatQuota64(result.QuotaToAdd), result.PayMoney), callerIp, result.PaymentMethod, PaymentMethodCreem)
	}
	return nil
}

func RechargeWaffo(tradeNo string, callerIp string) error {
	result, err := CompleteTopUp(CompleteTopUpOptions{
		TradeNo:                 tradeNo,
		ExpectedPaymentProvider: PaymentProviderWaffo,
		CallerIp:                callerIp,
	})
	if err != nil {
		common.SysError("waffo topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}
	if !result.AlreadyCompleted {
		RecordTopupLog(result.UserId, fmt.Sprintf("Waffo充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota64(result.QuotaToAdd), result.PayMoney), callerIp, result.PaymentMethod, PaymentMethodWaffo)
	}
	return nil
}

func RechargeWaffoPancake(tradeNo string) error {
	result, err := CompleteTopUp(CompleteTopUpOptions{
		TradeNo:                 tradeNo,
		ExpectedPaymentProvider: PaymentProviderWaffoPancake,
	})
	if err != nil {
		common.SysError("waffo pancake topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}
	if !result.AlreadyCompleted {
		RecordLog(result.UserId, LogTypeTopup, fmt.Sprintf("Waffo Pancake充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota64(result.QuotaToAdd), result.PayMoney))
	}
	return nil
}
