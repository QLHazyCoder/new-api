package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// updateTokenQuotaDeltaTx applies the paired token accounting update:
// remain_quota += delta and used_quota -= delta. Both columns are guarded so
// a failed int64 boundary check cannot leave the token half-updated.
func updateTokenQuotaDeltaTx(tx *gorm.DB, tokenID int, delta int64) error {
	if delta == common.MinWalletQuota {
		return common.ErrWalletQuotaOverflow
	}
	query := tx.Model(&Token{}).Where("id = ?", tokenID)
	if delta > 0 {
		query = query.Where("remain_quota <= ? AND used_quota >= ?", common.MaxWalletQuota-delta, common.MinWalletQuota+delta)
	} else if delta < 0 {
		query = query.Where("remain_quota >= ? AND used_quota <= ?", common.MinWalletQuota-delta, common.MaxWalletQuota+delta)
	}
	result := query.Updates(map[string]interface{}{
		"remain_quota":  gorm.Expr("remain_quota + ?", delta),
		"used_quota":    gorm.Expr("used_quota - ?", delta),
		"accessed_time": common.GetTimestamp(),
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var token Token
	if err := tx.Select("id", "remain_quota", "used_quota").Where("id = ?", tokenID).First(&token).Error; err != nil {
		return err
	}
	return common.ErrWalletQuotaOverflow
}
