package model

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type Midjourney struct {
	Id                      int    `json:"id"`
	Code                    int    `json:"code"`
	UserId                  int    `json:"user_id" gorm:"index"`
	Action                  string `json:"action" gorm:"type:varchar(40);index"`
	MjId                    string `json:"mj_id" gorm:"index"`
	Prompt                  string `json:"prompt"`
	PromptEn                string `json:"prompt_en"`
	Description             string `json:"description"`
	State                   string `json:"state"`
	SubmitTime              int64  `json:"submit_time" gorm:"index"`
	StartTime               int64  `json:"start_time" gorm:"index"`
	FinishTime              int64  `json:"finish_time" gorm:"index"`
	ImageUrl                string `json:"image_url"`
	VideoUrl                string `json:"video_url"`
	VideoUrls               string `json:"video_urls"`
	Status                  string `json:"status" gorm:"type:varchar(20);index"`
	Progress                string `json:"progress" gorm:"type:varchar(30);index"`
	FailReason              string `json:"fail_reason"`
	ChannelId               int    `json:"channel_id"`
	Quota                   int    `json:"quota"`
	Buttons                 string `json:"buttons"`
	Properties              string `json:"properties"`
	BillingReviewPending    bool   `json:"-" gorm:"not null;default:false"`
	BillingFailureConfirmed bool   `json:"-" gorm:"not null;default:false;index"`
	BillingRequestID        string `json:"-" gorm:"type:varchar(128);index"`

	TokenId          int  `json:"-" gorm:"default:0"`
	BillingChannelId int  `json:"-" gorm:"default:0"`
	PreparedQuota    int  `json:"-" gorm:"-:all"`
	BillingBillable  bool `json:"-" gorm:"-:all"`
}

// TaskQueryParams 用于包含所有搜索条件的结构体，可以根据需求添加更多字段
type TaskQueryParams struct {
	ChannelID      string
	MjID           string
	StartTimestamp string
	EndTimestamp   string
}

func GetAllUserTask(userId int, startIdx int, num int, queryParams TaskQueryParams) []*Midjourney {
	var tasks []*Midjourney
	var err error

	// 初始化查询构建器
	query := DB.Where("user_id = ?", userId)

	if queryParams.MjID != "" {
		query = query.Where("mj_id = ?", queryParams.MjID)
	}
	if queryParams.StartTimestamp != "" {
		// 假设您已将前端传来的时间戳转换为数据库所需的时间格式，并处理了时间戳的验证和解析
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != "" {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}

	// 获取数据
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&tasks).Error
	if err != nil {
		return nil
	}

	return tasks
}

func GetAllTasks(startIdx int, num int, queryParams TaskQueryParams) []*Midjourney {
	var tasks []*Midjourney
	var err error

	// 初始化查询构建器
	query := DB

	// 添加过滤条件
	if queryParams.ChannelID != "" {
		query = query.Where("channel_id = ?", queryParams.ChannelID)
	}
	if queryParams.MjID != "" {
		query = query.Where("mj_id = ?", queryParams.MjID)
	}
	if queryParams.StartTimestamp != "" {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != "" {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}

	// 获取数据
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&tasks).Error
	if err != nil {
		return nil
	}

	return tasks
}

func GetAllUnFinishTasks() []*Midjourney {
	var tasks []*Midjourney
	// Tasks without an upstream ID cannot be polled after their review marker is saved.
	err := DB.Where("progress != ?", "100%").
		Where("mj_id <> ? OR billing_review_pending = ?", "", false).
		Find(&tasks).Error
	if err != nil {
		return nil
	}
	return tasks
}

// HasUnfinishedMidjourneyTasks reports whether at least one Midjourney task is
// still in progress. It is a cheap existence check (LIMIT 1) used to decide
// whether the midjourney_poll system task needs to run; when no task is pending
// the scheduler skips creating a row entirely.
func HasUnfinishedMidjourneyTasks() bool {
	var id int
	err := DB.Model(&Midjourney{}).
		Where("progress != ?", "100%").
		Where("mj_id <> ? OR billing_review_pending = ?", "", false).
		Limit(1).
		Pluck("id", &id).Error
	return err == nil && id != 0
}

func GetByOnlyMJId(mjId string) *Midjourney {
	var mj *Midjourney
	var err error
	err = DB.Where("mj_id = ?", mjId).First(&mj).Error
	if err != nil {
		return nil
	}
	return mj
}

func GetByMJId(userId int, mjId string) *Midjourney {
	var mj *Midjourney
	var err error
	err = DB.Where("user_id = ? and mj_id = ?", userId, mjId).First(&mj).Error
	if err != nil {
		return nil
	}
	return mj
}

func GetByMJIds(userId int, mjIds []string) []*Midjourney {
	var mj []*Midjourney
	var err error
	err = DB.Where("user_id = ? and mj_id in (?)", userId, mjIds).Find(&mj).Error
	if err != nil {
		return nil
	}
	return mj
}

func GetMjByuId(id int) *Midjourney {
	var mj *Midjourney
	var err error
	err = DB.Where("id = ?", id).First(&mj).Error
	if err != nil {
		return nil
	}
	return mj
}

func UpdateProgress(id int, progress string) error {
	return DB.Model(&Midjourney{}).Where("id = ?", id).Update("progress", progress).Error
}

func (midjourney *Midjourney) Insert() error {
	if midjourney.BillingRequestID == "" {
		if midjourney.PreparedQuota > 0 {
			return errors.New("prepared Midjourney billing has no request ID")
		}
		return DB.Create(midjourney).Error
	}
	if midjourney.Id != 0 || midjourney.Quota != 0 || midjourney.PreparedQuota <= 0 || midjourney.PreparedQuota > common.MaxChargeQuota || midjourney.UserId <= 0 {
		return errors.New("invalid prepared Midjourney task billing")
	}
	desired := *midjourney
	var staged Midjourney
	var persistedID int
	err := withBillingTransaction(context.Background(), func(tx *gorm.DB) error {
		staged = desired
		staged.Id = 0
		staged.Quota = 0
		var operation BillingOperation
		if err := lockForUpdate(tx).Where("request_id = ? AND phase = ?", desired.BillingRequestID, "initial").First(&operation).Error; err != nil {
			return err
		}
		expectedState := BillingReserved
		if !desired.BillingBillable {
			expectedState = BillingRefundRequested
		}
		if operation.State != expectedState || operation.TaskId != 0 || operation.FundingSource != "midjourney_wallet" ||
			operation.UserId != desired.UserId || operation.ChannelId != desired.GetBillingChannelId() ||
			operation.TokenId != desired.TokenId || operation.WalletAmount != int64(desired.PreparedQuota) ||
			operation.TokenAmount != tokenAmountForMidjourney(desired.PreparedQuota, desired.TokenId) ||
			operation.PreConsumed != int64(desired.PreparedQuota) || operation.Actual != int64(desired.PreparedQuota) ||
			(operation.UpstreamResultRecorded && (operation.UpstreamTaskID != desired.MjId || operation.UpstreamCode != desired.Code)) {
			return errors.New("Midjourney submission reservation does not match task")
		}
		if !desired.BillingBillable {
			staged.TokenId = 0
			staged.BillingChannelId = 0
		}
		if err := tx.Create(&staged).Error; err != nil {
			return err
		}
		state := BillingSettlementRequested
		if !desired.BillingBillable {
			state = BillingRefundRequested
		}
		if err := tx.Model(&operation).Updates(map[string]any{
			"task_id": staged.Id, "state": state, "actual": int64(desired.PreparedQuota),
			"updated_at": common.GetTimestamp(),
		}).Error; err != nil {
			return err
		}
		persistedID = staged.Id
		return nil
	})
	if err != nil {
		return err
	}
	desired.Id = persistedID
	desired.Quota = 0
	*midjourney = desired
	return nil
}

func (midjourney *Midjourney) Update() error {
	var err error
	err = DB.Save(midjourney).Error
	return err
}

func (midjourney *Midjourney) UpdateBillingState() error {
	return DB.Model(midjourney).
		Select("quota", "token_id", "billing_channel_id").
		Updates(midjourney).Error
}

func (midjourney *Midjourney) GetBillingChannelId() int {
	if midjourney.BillingChannelId > 0 {
		return midjourney.BillingChannelId
	}
	return midjourney.ChannelId
}

// UpdateWithStatus performs a conditional UPDATE guarded by fromStatus (CAS).
// Returns (true, nil) if this caller won the update, (false, nil) if
// another process already moved the task out of fromStatus.
// UpdateWithStatus performs a conditional UPDATE guarded by fromStatus (CAS).
// Uses Model().Select("*").Updates() to avoid GORM Save()'s INSERT fallback.
func (midjourney *Midjourney) UpdateWithStatus(fromStatus string) (bool, error) {
	result := DB.Model(midjourney).Where("status = ?", fromStatus).Select("*").Updates(midjourney)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func MarkMidjourneyTaskBillingReviewRequired(taskID int) (bool, error) {
	if taskID <= 0 {
		return false, errors.New("invalid Midjourney task id")
	}
	result := DB.Model(&Midjourney{}).
		Where("id = ? AND billing_review_pending = ?", taskID, false).
		Update("billing_review_pending", true)
	return result.RowsAffected > 0, result.Error
}

func ListMidjourneyBillingReviewTasks(limit int) ([]Midjourney, error) {
	var tasks []Midjourney
	err := DB.Where("billing_review_pending = ?", true).
		Order("submit_time asc").Limit(limit).Find(&tasks).Error
	return tasks, err
}

func MjBulkUpdate(mjIds []string, params map[string]any) error {
	return DB.Model(&Midjourney{}).
		Where("mj_id in (?)", mjIds).
		Updates(params).Error
}

func MjBulkUpdateByTaskIds(taskIDs []int, params map[string]any) error {
	return DB.Model(&Midjourney{}).
		Where("id in (?)", taskIDs).
		Updates(params).Error
}

// CountAllTasks returns total midjourney tasks for admin query
func CountAllTasks(queryParams TaskQueryParams) int64 {
	var total int64
	query := DB.Model(&Midjourney{})
	if queryParams.ChannelID != "" {
		query = query.Where("channel_id = ?", queryParams.ChannelID)
	}
	if queryParams.MjID != "" {
		query = query.Where("mj_id = ?", queryParams.MjID)
	}
	if queryParams.StartTimestamp != "" {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != "" {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}
	_ = query.Count(&total).Error
	return total
}

// CountAllUserTask returns total midjourney tasks for user
func CountAllUserTask(userId int, queryParams TaskQueryParams) int64 {
	var total int64
	query := DB.Model(&Midjourney{}).Where("user_id = ?", userId)
	if queryParams.MjID != "" {
		query = query.Where("mj_id = ?", queryParams.MjID)
	}
	if queryParams.StartTimestamp != "" {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != "" {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}
	_ = query.Count(&total).Error
	return total
}
