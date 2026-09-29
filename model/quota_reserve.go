package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

func reserveUserQuotaDB(id int, quota int64) (bool, error) {
	return reserveUserQuotaTx(DB, id, quota)
}

func reserveUserQuotaTx(tx *gorm.DB, id int, quota int64) (bool, error) {
	result := tx.Model(&User{}).
		Where("id = ? AND quota >= ?", id, quota).
		Update("quota", gorm.Expr("quota - ?", quota))
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 1 {
		return true, nil
	}
	var user User
	if err := tx.Select("id", "quota").Where("id = ?", id).First(&user).Error; err != nil {
		return false, err
	}
	return false, nil
}

func reserveTokenQuotaDB(id int, quota int64) (bool, error) {
	return reserveTokenQuotaTx(DB, id, quota)
}

func reserveTokenQuotaTx(tx *gorm.DB, id int, quota int64) (bool, error) {
	result := tx.Model(&Token{}).
		Where("id = ? AND remain_quota >= ? AND remain_quota >= ? AND used_quota <= ?", id, quota, common.MinWalletQuota+quota, common.MaxWalletQuota-quota).
		Updates(map[string]any{
			"remain_quota":  gorm.Expr("remain_quota - ?", quota),
			"used_quota":    gorm.Expr("used_quota + ?", quota),
			"accessed_time": common.GetTimestamp(),
		})
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 1 {
		return true, nil
	}
	var token Token
	if err := tx.Select("id", "remain_quota").Where("id = ?", id).First(&token).Error; err != nil {
		return false, err
	}
	if token.RemainQuota < quota {
		return false, nil
	}
	return false, common.ErrWalletQuotaOverflow
}

// TryReserveUserQuotaTx is the transactional form used by billing sessions.
func TryReserveUserQuotaTx(tx *gorm.DB, id int, quota int64) (bool, error) {
	if tx == nil || quota < 0 {
		return false, errors.New("invalid quota reservation")
	}
	if quota == 0 {
		return true, nil
	}
	return reserveUserQuotaTx(tx, id, quota)
}

func TryReserveTokenQuotaTx(tx *gorm.DB, id int, quota int64, unlimited bool) (bool, error) {
	if tx == nil || quota < 0 {
		return false, errors.New("invalid token reservation")
	}
	if quota == 0 {
		return true, nil
	}
	if unlimited {
		return true, updateTokenQuotaDeltaTx(tx, id, -quota)
	}
	return reserveTokenQuotaTx(tx, id, quota)
}

// Reservations are authorized exclusively by the committed database balance.
// A cache outage, stale hash, or obsolete batch flag must not grant credit.
func TryReserveUserQuota(id int, quota int) (bool, error) {
	if quota < 0 {
		return false, errors.New("quota 不能为负数！")
	}
	if quota == 0 {
		return true, nil
	}
	return reserveUserQuotaDB(id, int64(quota))
}

// Unlimited tokens may go into debt, but their paired counters still must fit int64.
func TryReserveTokenQuota(id int, key string, quota int, unlimited bool) (bool, error) {
	if quota < 0 {
		return false, errors.New("quota 不能为负数！")
	}
	if quota == 0 {
		return true, nil
	}
	if unlimited {
		err := DecreaseTokenQuota(id, key, quota)
		return err == nil, err
	}
	return reserveTokenQuotaDB(id, int64(quota))
}
