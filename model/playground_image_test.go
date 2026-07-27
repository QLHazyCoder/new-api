package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreatePlaygroundImageBatchIsIdempotentAndPreservesModelName(t *testing.T) {
	truncateTables(t)
	const modelName = "gemini-3.1-flash-image-1K"
	batch, created, err := CreatePlaygroundImageBatch(CreatePlaygroundImageBatchParams{
		UserID:         1,
		ClientBatchID:  "client-batch-1",
		Mode:           PlaygroundImageModeGenerate,
		Prompt:         "draw a test image",
		Model:          modelName,
		RequestGroup:   "image",
		RequestPayload: `{"model":"gemini-3.1-flash-image-1K","n":1}`,
		Count:          3,
	})
	require.NoError(t, err)
	require.True(t, created)
	assert.Equal(t, modelName, batch.Model)

	duplicate, created, err := CreatePlaygroundImageBatch(CreatePlaygroundImageBatchParams{
		UserID:         1,
		ClientBatchID:  "client-batch-1",
		Mode:           PlaygroundImageModeGenerate,
		Prompt:         "different request must not replace the original",
		Model:          "different-model",
		RequestPayload: `{"model":"different-model","n":1}`,
		Count:          9,
	})
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, batch.ID, duplicate.ID)
	assert.Equal(t, modelName, duplicate.Model)
	assert.Equal(t, 3, duplicate.TaskCount)

	var taskCount int64
	require.NoError(t, DB.Model(&PlaygroundImageTask{}).Where("batch_record_id = ?", batch.ID).Count(&taskCount).Error)
	assert.EqualValues(t, 3, taskCount)
}

func TestCreatePlaygroundImageBatchInChunksWithoutProductCountCap(t *testing.T) {
	truncateTables(t)
	batch, created, err := CreatePlaygroundImageBatch(CreatePlaygroundImageBatchParams{
		UserID:         1,
		ClientBatchID:  "large-client-batch",
		Mode:           PlaygroundImageModeGenerate,
		Prompt:         "draw many images",
		Model:          "gpt-image-2",
		RequestPayload: `{"model":"gpt-image-2","n":1}`,
		Count:          503,
	})
	require.NoError(t, err)
	require.True(t, created)
	assert.Equal(t, 503, batch.TaskCount)

	var tasks []PlaygroundImageTask
	require.NoError(t, DB.Where("batch_record_id = ?", batch.ID).Order("task_index ASC").Find(&tasks).Error)
	require.Len(t, tasks, 503)
	assert.Equal(t, 0, tasks[0].TaskIndex)
	assert.Equal(t, 502, tasks[len(tasks)-1].TaskIndex)
}

func TestHideExcessPlaygroundImageResultsRetainsNewestFifty(t *testing.T) {
	truncateTables(t)
	batch, created, err := CreatePlaygroundImageBatch(CreatePlaygroundImageBatchParams{
		UserID:         1,
		ClientBatchID:  "result-retention",
		Mode:           PlaygroundImageModeGenerate,
		Prompt:         "draw many images",
		Model:          "gpt-image-2",
		RequestPayload: `{"model":"gpt-image-2","n":1}`,
		Count:          PlaygroundImageMaxStoredResultsPerUser + 2,
	})
	require.NoError(t, err)
	require.True(t, created)

	var tasks []PlaygroundImageTask
	require.NoError(t, DB.Where("batch_record_id = ?", batch.ID).Order("id ASC").Find(&tasks).Error)
	for index, task := range tasks {
		require.NoError(t, DB.Model(&PlaygroundImageTask{}).Where("id = ?", task.ID).Updates(map[string]any{
			"status":      PlaygroundImageTaskSucceeded,
			"result_path": fmt.Sprintf("results/1/%d.png", index),
		}).Error)
	}
	activeBatch, created, err := CreatePlaygroundImageBatch(CreatePlaygroundImageBatchParams{
		UserID:         1,
		ClientBatchID:  "result-retention-active",
		Mode:           PlaygroundImageModeGenerate,
		Prompt:         "keep generating",
		Model:          "gpt-image-2",
		RequestPayload: `{"model":"gpt-image-2","n":1}`,
		Count:          1,
	})
	require.NoError(t, err)
	require.True(t, created)
	var activeTask PlaygroundImageTask
	require.NoError(t, DB.Where("batch_record_id = ?", activeBatch.ID).First(&activeTask).Error)

	now := common.GetTimestamp()
	userIDs, err := ListPlaygroundImageUsersOverStoredResultLimit(now, 10)
	require.NoError(t, err)
	assert.Equal(t, []int{1}, userIDs)

	excess, err := HideExcessPlaygroundImageResults(1, now)
	require.NoError(t, err)
	require.Len(t, excess, 2)
	assert.Equal(t, []int64{tasks[1].ID, tasks[0].ID}, []int64{excess[0].ID, excess[1].ID})

	var visibleCount int64
	require.NoError(t, DB.Model(&PlaygroundImageTask{}).
		Where("user_id = ? AND status = ? AND hidden = ? AND result_path <> '' AND expires_at > ?", 1, PlaygroundImageTaskSucceeded, false, now).
		Count(&visibleCount).Error)
	assert.EqualValues(t, PlaygroundImageMaxStoredResultsPerUser, visibleCount)

	var hiddenTask PlaygroundImageTask
	require.NoError(t, DB.First(&hiddenTask, tasks[0].ID).Error)
	assert.True(t, hiddenTask.Hidden)
	assert.Equal(t, "results/1/0.png", hiddenTask.ResultPath)

	var reloadedActiveTask PlaygroundImageTask
	require.NoError(t, DB.First(&reloadedActiveTask, activeTask.ID).Error)
	assert.Equal(t, PlaygroundImageTaskQueued, reloadedActiveTask.Status)
	assert.False(t, reloadedActiveTask.Hidden)

	hiddenResults, err := ListHiddenPlaygroundImageTasksWithResults(10)
	require.NoError(t, err)
	require.Len(t, hiddenResults, 2)
}

func TestClaimPlaygroundImageTasksHonorsGlobalConcurrency(t *testing.T) {
	truncateTables(t)
	_, _, err := CreatePlaygroundImageBatch(CreatePlaygroundImageBatchParams{
		UserID:         1,
		ClientBatchID:  "limited-concurrency",
		Mode:           PlaygroundImageModeGenerate,
		Prompt:         "draw ten images",
		Model:          "gpt-image-2",
		RequestPayload: `{"model":"gpt-image-2","n":1}`,
		Count:          10,
	})
	require.NoError(t, err)
	now := common.GetTimestamp()
	claimed, err := ClaimPlaygroundImageTasks("worker-a", 3, 500, now, now+45)
	require.NoError(t, err)
	assert.Len(t, claimed, 3)

	secondClaim, err := ClaimPlaygroundImageTasks("worker-b", 3, 500, now, now+45)
	require.NoError(t, err)
	assert.Empty(t, secondClaim)

	for _, task := range claimed {
		_, err := FailPlaygroundImageTask(task.TaskID, "worker-a", "test completion", "test", now)
		require.NoError(t, err)
	}
	thirdClaim, err := ClaimPlaygroundImageTasks("worker-b", 3, 500, now, now+45)
	require.NoError(t, err)
	assert.Len(t, thirdClaim, 3)
}

func TestClaimPlaygroundImageTasksUnlimitedClaimsAllPending(t *testing.T) {
	truncateTables(t)
	_, _, err := CreatePlaygroundImageBatch(CreatePlaygroundImageBatchParams{
		UserID:         1,
		ClientBatchID:  "unlimited-concurrency",
		Mode:           PlaygroundImageModeGenerate,
		Prompt:         "draw ten images",
		Model:          "gpt-image-2",
		RequestPayload: `{"model":"gpt-image-2","n":1}`,
		Count:          10,
	})
	require.NoError(t, err)
	now := common.GetTimestamp()
	claimed, err := ClaimPlaygroundImageTasks("worker-unlimited", 0, 500, now, now+45)
	require.NoError(t, err)
	assert.Len(t, claimed, 10)
}

func TestClaimPlaygroundImageTasksUsesDatabaseConcurrencyAcrossInstances(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&Option{Key: playgroundImageConcurrencyKey, Value: "2"}).Error)
	_, _, err := CreatePlaygroundImageBatch(CreatePlaygroundImageBatchParams{
		UserID:         1,
		ClientBatchID:  "database-concurrency",
		Mode:           PlaygroundImageModeGenerate,
		Prompt:         "draw five images",
		Model:          "gpt-image-2",
		RequestPayload: `{"model":"gpt-image-2","n":1}`,
		Count:          5,
	})
	require.NoError(t, err)
	now := common.GetTimestamp()

	claimed, err := ClaimPlaygroundImageTasks("worker-stale-unlimited", 0, 500, now, now+45)
	require.NoError(t, err)
	require.Len(t, claimed, 2)
	for _, task := range claimed {
		_, err := FailPlaygroundImageTask(task.TaskID, "worker-stale-unlimited", "test completion", "test", now)
		require.NoError(t, err)
	}

	require.NoError(t, DB.Model(&Option{}).Where(&Option{Key: playgroundImageConcurrencyKey}).Update("value", "0").Error)
	claimed, err = ClaimPlaygroundImageTasks("worker-stale-limited", 2, 500, now, now+45)
	require.NoError(t, err)
	assert.Len(t, claimed, 3)
}

func TestPlaygroundImageConcurrencyOptionQueryEscapesMySQLKeyColumn(t *testing.T) {
	db, err := gorm.Open(
		mysql.New(mysql.Config{
			DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8&parseTime=True&loc=Local",
			SkipInitializeWithVersion: true,
		}),
		&gorm.Config{DryRun: true, DisableAutomaticPing: true},
	)
	require.NoError(t, err)

	statement := db.Select("value").Where(&Option{Key: playgroundImageConcurrencyKey}).First(&Option{}).Statement
	assert.Contains(t, statement.SQL.String(), "`key`")
	assert.NotContains(t, statement.SQL.String(), " WHERE key = ")
}

func TestExpiredStartedLeaseBecomesInterruptedWithoutResubmission(t *testing.T) {
	truncateTables(t)
	batch, _, err := CreatePlaygroundImageBatch(CreatePlaygroundImageBatchParams{
		UserID:         1,
		ClientBatchID:  "interrupted-batch",
		Mode:           PlaygroundImageModeGenerate,
		Prompt:         "draw one image",
		Model:          "gpt-image-2",
		RequestPayload: `{"model":"gpt-image-2","n":1}`,
		Count:          1,
	})
	require.NoError(t, err)
	now := common.GetTimestamp()
	claimed, err := ClaimPlaygroundImageTasks("worker-old", 0, 500, now, now+45)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.NoError(t, MarkPlaygroundImageTaskUpstreamStarted(claimed[0].TaskID, "worker-old", now))
	require.NoError(t, DB.Model(&PlaygroundImageTask{}).Where("task_id = ?", claimed[0].TaskID).Update("lease_until", now-1).Error)

	reclaimed, err := ClaimPlaygroundImageTasks("worker-new", 0, 500, now, now+45)
	require.NoError(t, err)
	assert.Empty(t, reclaimed)
	var task PlaygroundImageTask
	require.NoError(t, DB.Where("batch_record_id = ?", batch.ID).First(&task).Error)
	assert.Equal(t, PlaygroundImageTaskInterrupted, task.Status)
	assert.Equal(t, int64(0), task.LeaseUntil)
}

func TestDiscardedRunningTaskRetainsResultMetadataUntilFileRemoval(t *testing.T) {
	truncateTables(t)
	_, _, err := CreatePlaygroundImageBatch(CreatePlaygroundImageBatchParams{
		UserID:         1,
		ClientBatchID:  "discard-result",
		Mode:           PlaygroundImageModeGenerate,
		Prompt:         "draw one image",
		Model:          "gpt-image-2",
		RequestPayload: `{"model":"gpt-image-2","n":1}`,
		Count:          1,
	})
	require.NoError(t, err)
	now := common.GetTimestamp()
	claimed, err := ClaimPlaygroundImageTasks("worker-discard", 0, 500, now, now+45)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	deleteResult, err := DeletePlaygroundImageTask(claimed[0].TaskID, 1, now)
	require.NoError(t, err)
	require.True(t, deleteResult.WasActive)

	discard, _, err := CompletePlaygroundImageTask(
		claimed[0].TaskID,
		"worker-discard",
		"results/1/test.png",
		"image/png",
		128,
		now,
	)
	require.NoError(t, err)
	require.True(t, discard)
	var task PlaygroundImageTask
	require.NoError(t, DB.Where("task_id = ?", claimed[0].TaskID).First(&task).Error)
	assert.Equal(t, PlaygroundImageTaskCancelled, task.Status)
	assert.Equal(t, "results/1/test.png", task.ResultPath)

	require.NoError(t, ClearPlaygroundImageTaskResult(task.TaskID, task.ResultPath))
	require.NoError(t, DB.Where("task_id = ?", task.TaskID).First(&task).Error)
	assert.Empty(t, task.ResultPath)
}

func TestReferenceMetadataClearsOnlyAfterExplicitFileRemoval(t *testing.T) {
	truncateTables(t)
	const referenceJSON = `[{"path":"references/batch/0.png"}]`
	batch, _, err := CreatePlaygroundImageBatch(CreatePlaygroundImageBatchParams{
		UserID:         1,
		ClientBatchID:  "reference-cleanup",
		Mode:           PlaygroundImageModeEdit,
		Prompt:         "edit one image",
		Model:          "gpt-image-2",
		RequestPayload: `{"fields":{"model":["gpt-image-2"],"n":["1"]}}`,
		ReferenceFiles: referenceJSON,
		Count:          1,
	})
	require.NoError(t, err)
	var task PlaygroundImageTask
	require.NoError(t, DB.Where("batch_record_id = ?", batch.ID).First(&task).Error)
	_, err = DeletePlaygroundImageTask(task.TaskID, 1, common.GetTimestamp())
	require.NoError(t, err)

	references, err := GetPlaygroundImageBatchReferencesIfTerminal(batch.ID)
	require.NoError(t, err)
	assert.Equal(t, referenceJSON, references)
	require.NoError(t, DB.First(batch, batch.ID).Error)
	assert.Equal(t, referenceJSON, batch.ReferenceFiles)

	require.NoError(t, ClearPlaygroundImageBatchReferences(batch.ID, references))
	require.NoError(t, DB.First(batch, batch.ID).Error)
	assert.Empty(t, batch.ReferenceFiles)
}
