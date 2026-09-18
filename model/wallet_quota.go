package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var errUnsupportedWalletColumn = errors.New("unsupported wallet quota column")

func walletQuotaColumn(column string) bool {
	switch column {
	case "quota", "used_quota", "aff_quota", "aff_history":
		return true
	default:
		return false
	}
}

// updateUserQuotaWithDeltaTx applies a signed wallet delta and optional user
// updates in one guarded UPDATE. The predicate prevents SQL integer wraparound
// even when the caller is already inside a larger business transaction.
func updateUserQuotaWithDeltaTx(tx *gorm.DB, userID int, delta int64, updates map[string]interface{}) error {
	if delta == common.MinWalletQuota {
		return common.ErrWalletQuotaOverflow
	}
	if updates == nil {
		updates = make(map[string]interface{})
	}
	if delta == 0 {
		if len(updates) == 0 {
			return nil
		}
		result := tx.Model(&User{}).Where("id = ?", userID).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	}
	if delta > 0 {
		updates["quota"] = gorm.Expr("quota + ?", delta)
		result := tx.Model(&User{}).
			Where("id = ? AND quota <= ?", userID, common.MaxWalletQuota-delta).
			Updates(updates)
		return walletDeltaUpdateResult(tx, userID, result)
	}
	updates["quota"] = gorm.Expr("quota + ?", delta)
	result := tx.Model(&User{}).
		Where("id = ? AND quota >= ?", userID, common.MinWalletQuota-delta).
		Updates(updates)
	return walletDeltaUpdateResult(tx, userID, result)
}

func transferAffiliateQuotaTx(tx *gorm.DB, userID int, amount int64) error {
	if amount <= 0 {
		return errors.New("affiliate quota transfer must be positive")
	}
	result := tx.Model(&User{}).
		Where("id = ? AND aff_quota >= ? AND quota <= ?", userID, amount, common.MaxWalletQuota-amount).
		Updates(map[string]interface{}{
			"aff_quota": gorm.Expr("aff_quota - ?", amount),
			"quota":     gorm.Expr("quota + ?", amount),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var user User
	if err := tx.Select("id", "aff_quota", "quota").Where("id = ?", userID).First(&user).Error; err != nil {
		return err
	}
	if user.AffQuota < amount {
		return errors.New("邀请额度不足！")
	}
	return common.ErrWalletQuotaOverflow
}

func walletDeltaUpdateResult(tx *gorm.DB, userID int, result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var count int64
	if err := tx.Model(&User{}).Where("id = ?", userID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return common.ErrWalletQuotaOverflow
}

// updateUserQuotaFieldDeltaTx is the shared checked updater for wallet,
// affiliate-wallet and accumulated-usage columns.
func updateUserQuotaFieldDeltaTx(tx *gorm.DB, userID int, column string, delta int64) error {
	if !walletQuotaColumn(column) {
		return errUnsupportedWalletColumn
	}
	if delta == common.MinWalletQuota {
		return common.ErrWalletQuotaOverflow
	}
	if delta == 0 {
		return nil
	}
	field := gorm.Expr(column+" + ?", delta)
	query := tx.Model(&User{}).Where("id = ?", userID)
	if column == "quota" {
		if delta > 0 {
			query = query.Where("quota <= ?", common.MaxWalletQuota-delta)
		} else {
			query = query.Where("quota >= ?", common.MinWalletQuota-delta)
		}
	} else if delta > 0 {
		query = query.Where(column+" <= ?", common.MaxWalletQuota-delta)
	} else {
		query = query.Where(column+" >= ?", common.MinWalletQuota-delta)
	}
	result := query.Update(column, field)
	return walletDeltaUpdateResult(tx, userID, result)
}
