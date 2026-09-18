package model

import (
	"errors"
	"fmt"

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
	PricingSnapshot string  `json:"-" gorm:"type:text"`
	CreateTime      int64   `json:"create_time"`
	CompleteTime    int64   `json:"complete_time"`
	Status          string  `json:"status"`
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
	ErrPaymentMethodMismatch   = errors.New("payment method mismatch")
	ErrTopUpNotFound           = errors.New("topup not found")
	ErrTopUpStatusInvalid      = errors.New("topup status invalid")
	ErrInvalidTopUpQuota       = errors.New("invalid top-up quota")
	ErrTopUpQuotaLimitExceeded = errors.New("top-up quota limit exceeded")
)

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

// creditTopUpQuota atomically enforces the int64 storage ceiling while adding
// quota. Keeping the predicate and increment in one UPDATE prevents two
// concurrent callbacks from both passing a separate read/check.
func creditTopUpQuota(tx *gorm.DB, userId int, creditedQuota int64, updates map[string]interface{}) error {
	_, err := topUpQuotaMaxCurrent(creditedQuota)
	if err != nil {
		return err
	}

	updateFields := make(map[string]interface{}, len(updates)+1)
	for key, value := range updates {
		updateFields[key] = value
	}
	if err := updateUserQuotaWithDeltaTx(tx, userId, creditedQuota, updateFields); err == nil {
		return nil
	} else if errors.Is(err, common.ErrWalletQuotaOverflow) {
		return ErrTopUpQuotaLimitExceeded
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		return gorm.ErrRecordNotFound
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

func getTopUpQuotaToAdd(topUp *TopUp) (int64, error) {
	switch topUp.PaymentProvider {
	case PaymentProviderStripe:
		return common.WalletQuotaFromDecimal(decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
	case PaymentProviderCreem:
		return common.WalletQuotaFromDecimal(decimal.NewFromInt(topUp.Amount))
	default:
		return common.WalletQuotaFromDecimal(decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
	}
}

// RechargeEpay keeps the upstream callback API while routing completion
// through the shared transaction so invite rewards, pricing snapshots and
// quota protection remain consistent across payment providers.
func RechargeEpay(tradeNo string, actualPaymentMethod string, callerIp string) (bool, error) {
	result, err := CompleteTopUp(CompleteTopUpOptions{
		TradeNo:                 tradeNo,
		ExpectedPaymentProvider: PaymentProviderEpay,
		CallerIp:                callerIp,
		CallbackPaymentMethod:   actualPaymentMethod,
	})
	if err != nil {
		return false, err
	}
	return result.AlreadyCompleted, nil
}

func calculateTopUpInviteReward(quotaToAdd int64) (int64, string) {
	if quotaToAdd <= 0 || !operation_setting.IsPaymentComplianceConfirmed() {
		return 0, ""
	}
	if common.TopUpInviteRewardPercent <= 0 {
		return 0, ""
	}
	percent := decimal.NewFromFloat(common.TopUpInviteRewardPercent)
	reward, err := common.WalletQuotaFromDecimal(decimal.NewFromInt(quotaToAdd).
		Mul(percent).
		Div(decimal.NewFromInt(100)))
	if err != nil {
		return 0, percent.String()
	}
	return reward, percent.String()
}

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

		updateFields := map[string]interface{}{}
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

		completion.Amount = topUp.Amount
		completion.QuotaToAdd = quotaToAdd
		completion.PaymentMethod = topUp.PaymentMethod
		completion.InviteRewardQuota = reward
		completion.InviteRewardUserId = rewardUserId
		return nil
	})

	if err != nil {
		return nil, err
	}
	if !completion.AlreadyCompleted {
		syncCreditUserQuotaCache(completion.UserId, completion.QuotaToAdd, "topup")
		if opts.CustomerEmail != "" {
			_ = invalidateUserCache(completion.UserId)
		}
		if completion.InviteRewardUserId > 0 {
			syncCreditUserQuotaCache(completion.InviteRewardUserId, completion.InviteRewardQuota, "invite reward")
			RecordLog(
				completion.InviteRewardUserId,
				LogTypeSystem,
				fmt.Sprintf("Referral topup reward %s (topup_user_id=%d, trade_no=%s)", logger.LogQuota64(completion.InviteRewardQuota), completion.UserId, completion.TradeNo),
			)
		}
	}
	return completion, nil
}

func Recharge(referenceId string, customerId string, callerIp string) (err error) {
	result, err := CompleteTopUp(CompleteTopUpOptions{
		TradeNo:                 referenceId,
		ExpectedPaymentProvider: PaymentProviderStripe,
		CallerIp:                callerIp,
		StripeCustomer:          customerId,
	})
	if err != nil {
		common.SysError("topup failed: " + err.Error())
		return err
	}
	if !result.AlreadyCompleted {
		RecordTopupLog(result.UserId, fmt.Sprintf("Stripe topup succeeded, quota: %v, amount: %d", logger.FormatQuota64(result.QuotaToAdd), result.Amount), callerIp, result.PaymentMethod, PaymentMethodStripe)
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
		RecordTopupLog(result.UserId, fmt.Sprintf("Admin completed topup, quota: %v, amount: %f", logger.FormatQuota64(result.QuotaToAdd), result.PayMoney), callerIp, result.PaymentMethod, "admin")
	}
	return nil
}

func RechargeCreem(referenceId string, customerEmail string, customerName string, callerIp string) (err error) {
	result, err := CompleteTopUp(CompleteTopUpOptions{
		TradeNo:                 referenceId,
		ExpectedPaymentProvider: PaymentProviderCreem,
		CallerIp:                callerIp,
		CustomerEmail:           customerEmail,
	})
	if err != nil {
		common.SysError("creem topup failed: " + err.Error())
		return err
	}
	if !result.AlreadyCompleted {
		RecordTopupLog(result.UserId, fmt.Sprintf("Creem topup succeeded, quota: %v, amount: %.2f", logger.FormatQuota64(result.QuotaToAdd), result.PayMoney), callerIp, result.PaymentMethod, PaymentMethodCreem)
	}
	return nil
}

func RechargeWaffo(tradeNo string, callerIp string) (err error) {
	result, err := CompleteTopUp(CompleteTopUpOptions{
		TradeNo:                 tradeNo,
		ExpectedPaymentProvider: PaymentProviderWaffo,
		CallerIp:                callerIp,
	})
	if err != nil {
		common.SysError("waffo topup failed: " + err.Error())
		return err
	}
	if !result.AlreadyCompleted {
		RecordTopupLog(result.UserId, fmt.Sprintf("Waffo topup succeeded, quota: %v, amount: %.2f", logger.FormatQuota64(result.QuotaToAdd), result.PayMoney), callerIp, result.PaymentMethod, PaymentMethodWaffo)
	}
	return nil
}

func RechargeWaffoPancake(tradeNo string) (err error) {
	result, err := CompleteTopUp(CompleteTopUpOptions{
		TradeNo:                 tradeNo,
		ExpectedPaymentProvider: PaymentProviderWaffoPancake,
	})
	if err != nil {
		common.SysError("waffo pancake topup failed: " + err.Error())
		return err
	}
	if !result.AlreadyCompleted {
		RecordLog(result.UserId, LogTypeTopup, fmt.Sprintf("Waffo Pancake topup succeeded, quota: %v, amount: %.2f", logger.FormatQuota64(result.QuotaToAdd), result.PayMoney))
	}
	return nil
}
