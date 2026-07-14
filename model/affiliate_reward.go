package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const (
	AffiliateRewardEventTypeRegistration  = "registration_inviter_reward"
	AffiliateRewardEventTypeTopUp         = "topup_reward"
	AffiliateRewardEventTypeQuotaTransfer = "quota_transfer"

	AffiliateRewardSourceTypeUser             = "user_registration"
	AffiliateRewardSourceTypeTopUp            = "topup"
	AffiliateRewardSourceTypeAffiliateBalance = "affiliate_balance"
)

// AffiliateRewardEvent is the append-only accounting ledger for affiliate
// rewards created after this table is deployed. Existing aggregate balances
// are intentionally not expanded into synthetic historical events.
type AffiliateRewardEvent struct {
	Id             int64   `json:"id" gorm:"primaryKey;autoIncrement"`
	InviterId      int     `json:"inviter_id" gorm:"not null;index:idx_affiliate_reward_inviter_created,priority:1"`
	InviteeId      int     `json:"invitee_id" gorm:"not null;index:idx_affiliate_reward_invitee_created,priority:1"`
	EventType      string  `json:"event_type" gorm:"type:varchar(32);not null"`
	SourceType     string  `json:"source_type" gorm:"type:varchar(32);not null"`
	SourceId       string  `json:"source_id" gorm:"type:varchar(255);not null"`
	IdempotencyKey *string `json:"idempotency_key,omitempty" gorm:"type:varchar(255);uniqueIndex:idx_affiliate_reward_idempotency"`
	BaseQuota      int64   `json:"base_quota" gorm:"type:bigint;not null"`
	RewardPercent  string  `json:"reward_percent" gorm:"type:varchar(32);not null"`
	RewardQuota    int64   `json:"reward_quota" gorm:"type:bigint;not null"`
	AffQuotaDelta  int64   `json:"aff_quota_delta" gorm:"type:bigint;not null"`
	UserQuotaDelta int64   `json:"user_quota_delta" gorm:"type:bigint;not null"`
	CreatedAt      int64   `json:"created_at" gorm:"autoCreateTime;index:idx_affiliate_reward_inviter_created,priority:2;index:idx_affiliate_reward_invitee_created,priority:2"`
}

func createAffiliateRewardEventTx(tx *gorm.DB, event *AffiliateRewardEvent) error {
	if tx == nil {
		return errors.New("affiliate reward event transaction is nil")
	}
	if event == nil {
		return errors.New("affiliate reward event is nil")
	}
	if event.InviterId <= 0 {
		return errors.New("affiliate reward event inviter id is invalid")
	}
	if event.EventType == "" || event.SourceType == "" {
		return errors.New("affiliate reward event type or source type is empty")
	}
	if event.CreatedAt == 0 {
		event.CreatedAt = common.GetTimestamp()
	}
	return tx.Create(event).Error
}

func affiliateRewardIdempotencyKey(namespace, sourceId string) string {
	digest := sha256.Sum256([]byte(namespace + "\x00" + sourceId))
	return namespace + ":" + hex.EncodeToString(digest[:])
}

// ReconcileAffiliateCounts replaces the denormalized counter with an absolute
// count from users.inviter_id. It is intentionally safe to run repeatedly:
// every inviter row is locked before its invitation rows are read, matching
// the lock order used by registration and hard deletion.
//
// Unscoped rows are included because aff_count has historically represented
// registrations, not only currently active accounts.
func ReconcileAffiliateCounts() error {
	var relationshipInviterIds []int
	if err := DB.Unscoped().Model(&User{}).
		Distinct("inviter_id").
		Where("inviter_id > 0").
		Pluck("inviter_id", &relationshipInviterIds).Error; err != nil {
		return err
	}
	var nonzeroCounterIds []int
	if err := DB.Unscoped().Model(&User{}).
		Where("aff_count <> ?", 0).
		Pluck("id", &nonzeroCounterIds).Error; err != nil {
		return err
	}

	idSet := make(map[int]struct{}, len(relationshipInviterIds)+len(nonzeroCounterIds))
	for _, id := range relationshipInviterIds {
		idSet[id] = struct{}{}
	}
	for _, id := range nonzeroCounterIds {
		idSet[id] = struct{}{}
	}
	inviterIds := make([]int, 0, len(idSet))
	for id := range idSet {
		inviterIds = append(inviterIds, id)
	}
	sort.Ints(inviterIds)

	for _, inviterId := range inviterIds {
		if err := DB.Transaction(func(tx *gorm.DB) error {
			var inviter User
			result := lockForUpdate(tx.Unscoped()).Select("id").Where("id = ?", inviterId).Limit(1).Find(&inviter)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return nil
			}

			var inviteeIds []int
			if err := lockForUpdate(tx.Unscoped()).Model(&User{}).
				Where("inviter_id = ?", inviterId).
				Order("id ASC").
				Pluck("id", &inviteeIds).Error; err != nil {
				return err
			}
			if err := tx.Unscoped().Model(&User{}).
				Where("id = ?", inviterId).
				Update("aff_count", len(inviteeIds)).Error; err != nil {
				return err
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

// SyncAffiliateCounts periodically repairs counters during rolling deployments.
// Once old instances stop writing legacy counters, the next pass converges all
// rows to users.inviter_id without creating reward history.
func SyncAffiliateCounts(interval time.Duration) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		if err := ReconcileAffiliateCounts(); err != nil {
			common.SysError("failed to reconcile affiliate counts: " + err.Error())
		}
	}
}
