package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PlaygroundImageTaskStatus string

const (
	PlaygroundImageTaskQueued      PlaygroundImageTaskStatus = "queued"
	PlaygroundImageTaskRunning     PlaygroundImageTaskStatus = "running"
	PlaygroundImageTaskSaving      PlaygroundImageTaskStatus = "saving"
	PlaygroundImageTaskSucceeded   PlaygroundImageTaskStatus = "succeeded"
	PlaygroundImageTaskFailed      PlaygroundImageTaskStatus = "failed"
	PlaygroundImageTaskInterrupted PlaygroundImageTaskStatus = "interrupted"
	PlaygroundImageTaskCancelled   PlaygroundImageTaskStatus = "cancelled"

	PlaygroundImageModeGenerate = "generate"
	PlaygroundImageModeEdit     = "edit"

	playgroundImageQueueLockType  = "playground_image_queue"
	playgroundImageConcurrencyKey = "PlaygroundImageMaxConcurrency"

	// PlaygroundImageMaxStoredResultsPerUser limits retained successful images,
	// without restricting queued or running generation tasks.
	PlaygroundImageMaxStoredResultsPerUser = 50
)

var ErrPlaygroundImageTaskNotFound = errors.New("playground image task not found")

type PlaygroundImageBatch struct {
	ID             int64  `json:"-" gorm:"primaryKey"`
	BatchID        string `json:"batch_id" gorm:"type:varchar(64);uniqueIndex"`
	UserID         int    `json:"user_id" gorm:"uniqueIndex:idx_playground_image_user_client,priority:1;index"`
	ClientBatchID  string `json:"client_batch_id" gorm:"type:varchar(128);uniqueIndex:idx_playground_image_user_client,priority:2"`
	Mode           string `json:"mode" gorm:"type:varchar(16);index"`
	Prompt         string `json:"prompt" gorm:"type:text"`
	Model          string `json:"model" gorm:"type:varchar(255)"`
	RequestGroup   string `json:"group" gorm:"type:varchar(64)"`
	RequestPayload string `json:"-" gorm:"type:text"`
	ReferenceFiles string `json:"-" gorm:"type:text"`
	TaskCount      int    `json:"task_count"`
	CreatedAt      int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt      int64  `json:"updated_at" gorm:"bigint"`
	ExpiresAt      int64  `json:"expires_at" gorm:"bigint;index"`
}

type PlaygroundImageTask struct {
	ID                int64                     `json:"-" gorm:"primaryKey"`
	TaskID            string                    `json:"task_id" gorm:"type:varchar(64);uniqueIndex"`
	BatchRecordID     int64                     `json:"-" gorm:"uniqueIndex:idx_playground_image_batch_index,priority:1;index"`
	TaskIndex         int                       `json:"task_index" gorm:"uniqueIndex:idx_playground_image_batch_index,priority:2"`
	UserID            int                       `json:"user_id" gorm:"index:idx_playground_image_user_created,priority:1;index"`
	Status            PlaygroundImageTaskStatus `json:"status" gorm:"type:varchar(32);index:idx_playground_image_status_lease,priority:1;index"`
	ErrorMessage      string                    `json:"error,omitempty" gorm:"type:text"`
	ErrorCode         string                    `json:"error_code,omitempty" gorm:"type:varchar(128)"`
	ResultPath        string                    `json:"-" gorm:"type:varchar(512)"`
	ResultMimeType    string                    `json:"mime_type,omitempty" gorm:"type:varchar(64)"`
	ResultSize        int64                     `json:"result_size,omitempty"`
	LeaseOwner        string                    `json:"-" gorm:"type:varchar(160);index"`
	LeaseUntil        int64                     `json:"-" gorm:"bigint;index:idx_playground_image_status_lease,priority:2;index"`
	UpstreamStartedAt int64                     `json:"-" gorm:"bigint"`
	StartedAt         int64                     `json:"started_at,omitempty" gorm:"bigint"`
	FinishedAt        int64                     `json:"finished_at,omitempty" gorm:"bigint"`
	CreatedAt         int64                     `json:"created_at" gorm:"bigint;index:idx_playground_image_user_created,priority:2;index"`
	UpdatedAt         int64                     `json:"updated_at" gorm:"bigint"`
	ExpiresAt         int64                     `json:"expires_at" gorm:"bigint;index"`
	Hidden            bool                      `json:"-" gorm:"index"`
	DiscardResult     bool                      `json:"-"`
}

type CreatePlaygroundImageBatchParams struct {
	BatchID        string
	UserID         int
	ClientBatchID  string
	Mode           string
	Prompt         string
	Model          string
	RequestGroup   string
	RequestPayload string
	ReferenceFiles string
	Count          int
	ExpiresAt      int64
}

type PlaygroundImageBatchSummary struct {
	BatchID     string `json:"batch_id"`
	Total       int    `json:"total"`
	Queued      int    `json:"queued"`
	Running     int    `json:"running"`
	Saving      int    `json:"saving"`
	Succeeded   int    `json:"succeeded"`
	Failed      int    `json:"failed"`
	Interrupted int    `json:"interrupted"`
	Cancelled   int    `json:"cancelled"`
	CreatedAt   int64  `json:"created_at"`
	ExpiresAt   int64  `json:"expires_at"`
}

type PlaygroundImageDeleteResult struct {
	BatchRecordID int64
	ResultPath    string
	WasActive     bool
}

func (status PlaygroundImageTaskStatus) IsTerminal() bool {
	switch status {
	case PlaygroundImageTaskSucceeded,
		PlaygroundImageTaskFailed,
		PlaygroundImageTaskInterrupted,
		PlaygroundImageTaskCancelled:
		return true
	default:
		return false
	}
}

func newPlaygroundImageID(prefix string) string {
	return prefix + uuid.NewString()
}

func CreatePlaygroundImageBatch(params CreatePlaygroundImageBatchParams) (*PlaygroundImageBatch, bool, error) {
	if params.UserID <= 0 || params.ClientBatchID == "" || params.Count <= 0 {
		return nil, false, errors.New("invalid playground image batch")
	}

	var existing PlaygroundImageBatch
	err := DB.Where("user_id = ? AND client_batch_id = ?", params.UserID, params.ClientBatchID).First(&existing).Error
	if err == nil {
		return &existing, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}

	now := common.GetTimestamp()
	if params.BatchID == "" {
		params.BatchID = newPlaygroundImageID("imgbatch_")
	}
	if params.ExpiresAt <= now {
		params.ExpiresAt = now + int64((7 * 24 * time.Hour).Seconds())
	}
	batch := &PlaygroundImageBatch{
		BatchID:        params.BatchID,
		UserID:         params.UserID,
		ClientBatchID:  params.ClientBatchID,
		Mode:           params.Mode,
		Prompt:         params.Prompt,
		Model:          params.Model,
		RequestGroup:   params.RequestGroup,
		RequestPayload: params.RequestPayload,
		ReferenceFiles: params.ReferenceFiles,
		TaskCount:      params.Count,
		CreatedAt:      now,
		UpdatedAt:      now,
		ExpiresAt:      params.ExpiresAt,
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(batch).Error; err != nil {
			return err
		}
		const insertBatchSize = 500
		for start := 0; start < params.Count; start += insertBatchSize {
			end := min(start+insertBatchSize, params.Count)
			tasks := make([]PlaygroundImageTask, end-start)
			for offset := range tasks {
				tasks[offset] = PlaygroundImageTask{
					TaskID:        newPlaygroundImageID("imgtask_"),
					BatchRecordID: batch.ID,
					TaskIndex:     start + offset,
					UserID:        params.UserID,
					Status:        PlaygroundImageTaskQueued,
					CreatedAt:     now,
					UpdatedAt:     now,
					ExpiresAt:     params.ExpiresAt,
				}
			}
			if err := tx.Create(&tasks).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		return batch, true, nil
	}

	if lookupErr := DB.Where("user_id = ? AND client_batch_id = ?", params.UserID, params.ClientBatchID).First(&existing).Error; lookupErr == nil {
		return &existing, false, nil
	}
	return nil, false, err
}

func GetPlaygroundImageBatchForUser(batchID string, userID int) (*PlaygroundImageBatch, error) {
	var batch PlaygroundImageBatch
	if err := DB.Where("batch_id = ? AND user_id = ?", batchID, userID).First(&batch).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPlaygroundImageTaskNotFound
		}
		return nil, err
	}
	return &batch, nil
}

func GetPlaygroundImageBatchByRecordID(id int64) (*PlaygroundImageBatch, error) {
	var batch PlaygroundImageBatch
	if err := DB.Where("id = ?", id).First(&batch).Error; err != nil {
		return nil, err
	}
	return &batch, nil
}

func GetPlaygroundImageBatchesByRecordIDs(ids []int64) (map[int64]PlaygroundImageBatch, error) {
	result := make(map[int64]PlaygroundImageBatch, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	var batches []PlaygroundImageBatch
	if err := DB.Where("id IN ?", ids).Find(&batches).Error; err != nil {
		return nil, err
	}
	for _, batch := range batches {
		result[batch.ID] = batch
	}
	return result, nil
}

func GetPlaygroundImageTaskByTaskID(taskID string) (*PlaygroundImageTask, error) {
	var task PlaygroundImageTask
	if err := DB.Where("task_id = ?", taskID).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPlaygroundImageTaskNotFound
		}
		return nil, err
	}
	return &task, nil
}

func GetPlaygroundImageTaskForUser(taskID string, userID int) (*PlaygroundImageTask, *PlaygroundImageBatch, error) {
	var task PlaygroundImageTask
	if err := DB.Where("task_id = ? AND user_id = ?", taskID, userID).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrPlaygroundImageTaskNotFound
		}
		return nil, nil, err
	}
	batch, err := GetPlaygroundImageBatchByRecordID(task.BatchRecordID)
	if err != nil {
		return nil, nil, err
	}
	return &task, batch, nil
}

func ListPlaygroundImageTasks(userID, page, pageSize int, now int64) ([]PlaygroundImageTask, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 100
	}
	query := DB.Model(&PlaygroundImageTask{}).
		Where("user_id = ? AND hidden = ? AND expires_at > ?", userID, false, now)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var tasks []PlaygroundImageTask
	err := query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&tasks).Error
	return tasks, total, err
}

// HideExcessPlaygroundImageResults keeps only the newest successful image
// results visible for a user. Result files are removed by the worker after the
// database transaction commits, so file cleanup can be retried safely.
func HideExcessPlaygroundImageResults(userID int, now int64) ([]PlaygroundImageTask, error) {
	if userID <= 0 {
		return nil, errors.New("invalid playground image user")
	}

	var excess []PlaygroundImageTask
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).
			Where("user_id = ? AND status = ? AND hidden = ? AND result_path <> '' AND expires_at > ?", userID, PlaygroundImageTaskSucceeded, false, now).
			Order("id DESC").
			Offset(PlaygroundImageMaxStoredResultsPerUser).
			Find(&excess).Error; err != nil {
			return err
		}
		if len(excess) == 0 {
			return nil
		}

		ids := make([]int64, len(excess))
		for index, task := range excess {
			ids[index] = task.ID
		}
		result := tx.Model(&PlaygroundImageTask{}).
			Where("id IN ? AND user_id = ? AND status = ? AND hidden = ?", ids, userID, PlaygroundImageTaskSucceeded, false).
			Updates(map[string]any{
				"hidden":         true,
				"discard_result": true,
				"updated_at":     now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(ids)) {
			return fmt.Errorf("hid %d of %d excess playground image results", result.RowsAffected, len(ids))
		}
		return nil
	})
	return excess, err
}

func ListPlaygroundImageUsersOverStoredResultLimit(now int64, limit int) ([]int, error) {
	if limit <= 0 {
		limit = 100
	}
	type userRow struct {
		UserID int `gorm:"column:user_id"`
	}
	var rows []userRow
	err := DB.Model(&PlaygroundImageTask{}).
		Select("user_id").
		Where("status = ? AND hidden = ? AND result_path <> '' AND expires_at > ?", PlaygroundImageTaskSucceeded, false, now).
		Group("user_id").
		Having("COUNT(*) > ?", PlaygroundImageMaxStoredResultsPerUser).
		Order("user_id ASC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	userIDs := make([]int, len(rows))
	for index, row := range rows {
		userIDs[index] = row.UserID
	}
	return userIDs, nil
}

func ListHiddenPlaygroundImageTasksWithResults(limit int) ([]PlaygroundImageTask, error) {
	if limit <= 0 {
		limit = 500
	}
	var tasks []PlaygroundImageTask
	err := DB.Where("hidden = ? AND result_path <> '' AND status IN ?", true, []PlaygroundImageTaskStatus{
		PlaygroundImageTaskSucceeded,
		PlaygroundImageTaskCancelled,
	}).Order("updated_at ASC").Limit(limit).Find(&tasks).Error
	return tasks, err
}

func GetPlaygroundImageBatchSummary(batch *PlaygroundImageBatch) (*PlaygroundImageBatchSummary, error) {
	type statusCount struct {
		Status PlaygroundImageTaskStatus
		Count  int
	}
	var rows []statusCount
	if err := DB.Model(&PlaygroundImageTask{}).
		Select("status, COUNT(*) AS count").
		Where("batch_record_id = ?", batch.ID).
		Group("status").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	summary := &PlaygroundImageBatchSummary{
		BatchID:   batch.BatchID,
		Total:     batch.TaskCount,
		CreatedAt: batch.CreatedAt,
		ExpiresAt: batch.ExpiresAt,
	}
	for _, row := range rows {
		switch row.Status {
		case PlaygroundImageTaskQueued:
			summary.Queued = row.Count
		case PlaygroundImageTaskRunning:
			summary.Running = row.Count
		case PlaygroundImageTaskSaving:
			summary.Saving = row.Count
		case PlaygroundImageTaskSucceeded:
			summary.Succeeded = row.Count
		case PlaygroundImageTaskFailed:
			summary.Failed = row.Count
		case PlaygroundImageTaskInterrupted:
			summary.Interrupted = row.Count
		case PlaygroundImageTaskCancelled:
			summary.Cancelled = row.Count
		}
	}
	return summary, nil
}

func lockPlaygroundImageQueue(tx *gorm.DB, now int64) error {
	lock := SystemTaskLock{Type: playgroundImageQueueLockType, UpdatedAt: now}
	if err := tx.Where("type = ?", playgroundImageQueueLockType).FirstOrCreate(&lock).Error; err != nil {
		return err
	}
	return lockForUpdate(tx).Where("type = ?", playgroundImageQueueLockType).First(&lock).Error
}

func recoverExpiredPlaygroundImageLeases(tx *gorm.DB, now int64) error {
	active := []PlaygroundImageTaskStatus{PlaygroundImageTaskRunning, PlaygroundImageTaskSaving}
	interruptedUpdates := map[string]any{
		"status":        PlaygroundImageTaskInterrupted,
		"error_message": "Generation was interrupted because the worker stopped",
		"error_code":    "worker_interrupted",
		"lease_owner":   "",
		"lease_until":   0,
		"finished_at":   now,
		"updated_at":    now,
		"expires_at":    now + int64((7 * 24 * time.Hour).Seconds()),
	}
	if err := tx.Model(&PlaygroundImageTask{}).
		Where("status IN ? AND lease_until > 0 AND lease_until < ? AND upstream_started_at > 0", active, now).
		Updates(interruptedUpdates).Error; err != nil {
		return err
	}
	return tx.Model(&PlaygroundImageTask{}).
		Where("status IN ? AND lease_until > 0 AND lease_until < ? AND upstream_started_at = 0", active, now).
		Updates(map[string]any{
			"status":      PlaygroundImageTaskQueued,
			"lease_owner": "",
			"lease_until": 0,
			"started_at":  0,
			"updated_at":  now,
		}).Error
}

func resolvePlaygroundImageMaxConcurrency(tx *gorm.DB, fallback int) (int, error) {
	var option Option
	err := tx.Select("value").Where(&Option{Key: playgroundImageConcurrencyKey}).First(&option).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return max(0, fallback), nil
	}
	if err != nil {
		return 0, err
	}
	_, value, err := normalizePlaygroundImageMaxConcurrency(option.Value)
	return value, err
}

func ClaimPlaygroundImageTasks(owner string, maxConcurrency, claimLimit int, now, leaseUntil int64) ([]PlaygroundImageTask, error) {
	if owner == "" {
		return nil, errors.New("playground image worker owner is required")
	}
	if claimLimit <= 0 {
		claimLimit = 500
	}
	var claimed []PlaygroundImageTask
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockPlaygroundImageQueue(tx, now); err != nil {
			return err
		}
		if err := recoverExpiredPlaygroundImageLeases(tx, now); err != nil {
			return err
		}
		var err error
		maxConcurrency, err = resolvePlaygroundImageMaxConcurrency(tx, maxConcurrency)
		if err != nil {
			return err
		}
		limit := claimLimit
		if maxConcurrency > 0 {
			var activeCount int64
			if err := tx.Model(&PlaygroundImageTask{}).
				Where("status IN ? AND lease_until >= ?", []PlaygroundImageTaskStatus{PlaygroundImageTaskRunning, PlaygroundImageTaskSaving}, now).
				Count(&activeCount).Error; err != nil {
				return err
			}
			available := maxConcurrency - int(activeCount)
			if available <= 0 {
				return nil
			}
			if available < limit {
				limit = available
			}
		}
		if err := lockForUpdate(tx).
			Where("status = ? AND hidden = ? AND expires_at > ?", PlaygroundImageTaskQueued, false, now).
			Order("id ASC").
			Limit(limit).
			Find(&claimed).Error; err != nil {
			return err
		}
		if len(claimed) == 0 {
			return nil
		}
		ids := make([]int64, len(claimed))
		for index := range claimed {
			ids[index] = claimed[index].ID
			claimed[index].Status = PlaygroundImageTaskRunning
			claimed[index].LeaseOwner = owner
			claimed[index].LeaseUntil = leaseUntil
			claimed[index].StartedAt = now
			claimed[index].UpdatedAt = now
		}
		result := tx.Model(&PlaygroundImageTask{}).
			Where("id IN ? AND status = ?", ids, PlaygroundImageTaskQueued).
			Updates(map[string]any{
				"status":      PlaygroundImageTaskRunning,
				"lease_owner": owner,
				"lease_until": leaseUntil,
				"started_at":  now,
				"updated_at":  now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(ids)) {
			return fmt.Errorf("claimed %d of %d playground image tasks", result.RowsAffected, len(ids))
		}
		return nil
	})
	return claimed, err
}

func HeartbeatPlaygroundImageTask(taskID, owner string, leaseUntil, now int64) error {
	return DB.Model(&PlaygroundImageTask{}).
		Where("task_id = ? AND lease_owner = ? AND status IN ?", taskID, owner, []PlaygroundImageTaskStatus{PlaygroundImageTaskRunning, PlaygroundImageTaskSaving}).
		Updates(map[string]any{"lease_until": leaseUntil, "updated_at": now}).Error
}

func MarkPlaygroundImageTaskUpstreamStarted(taskID, owner string, now int64) error {
	result := DB.Model(&PlaygroundImageTask{}).
		Where("task_id = ? AND lease_owner = ? AND status = ?", taskID, owner, PlaygroundImageTaskRunning).
		Updates(map[string]any{"upstream_started_at": now, "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrPlaygroundImageTaskNotFound
	}
	return nil
}

func MarkPlaygroundImageTaskSaving(taskID, owner string, now int64) error {
	result := DB.Model(&PlaygroundImageTask{}).
		Where("task_id = ? AND lease_owner = ? AND status = ?", taskID, owner, PlaygroundImageTaskRunning).
		Updates(map[string]any{"status": PlaygroundImageTaskSaving, "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrPlaygroundImageTaskNotFound
	}
	return nil
}

func CompletePlaygroundImageTask(taskID, owner, resultPath, mimeType string, resultSize, now int64) (bool, int64, error) {
	var discard bool
	var batchRecordID int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		var task PlaygroundImageTask
		if err := lockForUpdate(tx).Where("task_id = ? AND lease_owner = ?", taskID, owner).First(&task).Error; err != nil {
			return err
		}
		batchRecordID = task.BatchRecordID
		discard = task.DiscardResult || task.Hidden
		updates := map[string]any{
			"lease_owner": "",
			"lease_until": 0,
			"finished_at": now,
			"updated_at":  now,
			"expires_at":  now + int64((7 * 24 * time.Hour).Seconds()),
		}
		if discard {
			updates["status"] = PlaygroundImageTaskCancelled
			updates["result_path"] = resultPath
			updates["result_mime_type"] = mimeType
			updates["result_size"] = resultSize
		} else {
			updates["status"] = PlaygroundImageTaskSucceeded
			updates["result_path"] = resultPath
			updates["result_mime_type"] = mimeType
			updates["result_size"] = resultSize
		}
		return tx.Model(&PlaygroundImageTask{}).Where("id = ?", task.ID).Updates(updates).Error
	})
	return discard, batchRecordID, err
}

func InterruptPlaygroundImageTask(taskID, owner, message, code string, now int64) (int64, error) {
	var batchRecordID int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		var task PlaygroundImageTask
		if err := lockForUpdate(tx).Where("task_id = ? AND lease_owner = ?", taskID, owner).First(&task).Error; err != nil {
			return err
		}
		batchRecordID = task.BatchRecordID
		return tx.Model(&PlaygroundImageTask{}).Where("id = ?", task.ID).Updates(map[string]any{
			"status":        PlaygroundImageTaskInterrupted,
			"error_message": message,
			"error_code":    code,
			"lease_owner":   "",
			"lease_until":   0,
			"finished_at":   now,
			"updated_at":    now,
			"expires_at":    now + int64((7 * 24 * time.Hour).Seconds()),
		}).Error
	})
	return batchRecordID, err
}

func FailPlaygroundImageTask(taskID, owner, message, code string, now int64) (int64, error) {
	var batchRecordID int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		var task PlaygroundImageTask
		if err := lockForUpdate(tx).Where("task_id = ? AND lease_owner = ?", taskID, owner).First(&task).Error; err != nil {
			return err
		}
		batchRecordID = task.BatchRecordID
		status := PlaygroundImageTaskFailed
		if task.DiscardResult || task.Hidden {
			status = PlaygroundImageTaskCancelled
		}
		return tx.Model(&PlaygroundImageTask{}).Where("id = ?", task.ID).Updates(map[string]any{
			"status":        status,
			"error_message": message,
			"error_code":    code,
			"lease_owner":   "",
			"lease_until":   0,
			"finished_at":   now,
			"updated_at":    now,
			"expires_at":    now + int64((7 * 24 * time.Hour).Seconds()),
		}).Error
	})
	return batchRecordID, err
}

func DeletePlaygroundImageTask(taskID string, userID int, now int64) (*PlaygroundImageDeleteResult, error) {
	result := &PlaygroundImageDeleteResult{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var task PlaygroundImageTask
		if err := lockForUpdate(tx).Where("task_id = ? AND user_id = ?", taskID, userID).First(&task).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPlaygroundImageTaskNotFound
			}
			return err
		}
		result.BatchRecordID = task.BatchRecordID
		result.ResultPath = task.ResultPath
		switch task.Status {
		case PlaygroundImageTaskQueued:
			return tx.Model(&PlaygroundImageTask{}).Where("id = ?", task.ID).Updates(map[string]any{
				"status":         PlaygroundImageTaskCancelled,
				"hidden":         true,
				"discard_result": true,
				"finished_at":    now,
				"updated_at":     now,
			}).Error
		case PlaygroundImageTaskRunning, PlaygroundImageTaskSaving:
			result.WasActive = true
			return tx.Model(&PlaygroundImageTask{}).Where("id = ?", task.ID).Updates(map[string]any{
				"hidden":         true,
				"discard_result": true,
				"updated_at":     now,
			}).Error
		default:
			return tx.Model(&PlaygroundImageTask{}).Where("id = ?", task.ID).Updates(map[string]any{
				"hidden":         true,
				"discard_result": true,
				"updated_at":     now,
			}).Error
		}
	})
	return result, err
}

func ClearPlaygroundImageTaskResult(taskID, resultPath string) error {
	if resultPath == "" {
		return nil
	}
	return DB.Model(&PlaygroundImageTask{}).
		Where("task_id = ? AND result_path = ?", taskID, resultPath).
		Updates(map[string]any{
			"result_path":      "",
			"result_mime_type": "",
			"result_size":      0,
			"updated_at":       common.GetTimestamp(),
		}).Error
}

func GetPlaygroundImageBatchReferencesIfTerminal(batchRecordID int64) (string, error) {
	var references string
	err := DB.Transaction(func(tx *gorm.DB) error {
		var batch PlaygroundImageBatch
		if err := lockForUpdate(tx).Where("id = ?", batchRecordID).First(&batch).Error; err != nil {
			return err
		}
		if batch.ReferenceFiles == "" {
			return nil
		}
		var active int64
		if err := tx.Model(&PlaygroundImageTask{}).
			Where("batch_record_id = ? AND status IN ?", batchRecordID, []PlaygroundImageTaskStatus{PlaygroundImageTaskQueued, PlaygroundImageTaskRunning, PlaygroundImageTaskSaving}).
			Count(&active).Error; err != nil {
			return err
		}
		if active > 0 {
			return nil
		}
		references = batch.ReferenceFiles
		return nil
	})
	return references, err
}

func ClearPlaygroundImageBatchReferences(batchRecordID int64, references string) error {
	if references == "" {
		return nil
	}
	return DB.Model(&PlaygroundImageBatch{}).
		Where("id = ? AND reference_files = ?", batchRecordID, references).
		Updates(map[string]any{
			"reference_files": "",
			"updated_at":      common.GetTimestamp(),
		}).Error
}

func ListPlaygroundImageBatchesWithReferences(limit int) ([]PlaygroundImageBatch, error) {
	if limit <= 0 {
		limit = 100
	}
	var batches []PlaygroundImageBatch
	err := DB.Where("reference_files <> ''").Order("id ASC").Limit(limit).Find(&batches).Error
	return batches, err
}

func ListExpiredPlaygroundImageTasks(now int64, limit int) ([]PlaygroundImageTask, error) {
	if limit <= 0 {
		limit = 500
	}
	var tasks []PlaygroundImageTask
	err := DB.Where("expires_at <= ? AND status NOT IN ?", now, []PlaygroundImageTaskStatus{PlaygroundImageTaskRunning, PlaygroundImageTaskSaving}).
		Order("id ASC").Limit(limit).Find(&tasks).Error
	return tasks, err
}

func DeleteExpiredPlaygroundImageTask(taskID string, now int64) error {
	return DB.Where(
		"task_id = ? AND expires_at <= ? AND status NOT IN ?",
		taskID,
		now,
		[]PlaygroundImageTaskStatus{PlaygroundImageTaskRunning, PlaygroundImageTaskSaving},
	).Delete(&PlaygroundImageTask{}).Error
}

func DeleteExpiredPlaygroundImageBatches(now int64) error {
	return DB.Where("expires_at <= ? AND reference_files = '' AND NOT EXISTS (SELECT 1 FROM playground_image_tasks WHERE playground_image_tasks.batch_record_id = playground_image_batches.id)", now).
		Delete(&PlaygroundImageBatch{}).Error
}
