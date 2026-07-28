package controller

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupPlaygroundImageControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalMainType := common.MainDatabaseType()
	originalLogType := common.LogDatabaseType()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	model.DB = db
	model.LOG_DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, db.AutoMigrate(
		&model.PlaygroundImageBatch{},
		&model.PlaygroundImageTask{},
	))
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.SetDatabaseTypes(originalMainType, originalLogType)
		_ = sqlDB.Close()
	})
	return db
}

func runPlaygroundImageGenerationBatchRequest(t *testing.T, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", 1)
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/playground/image-batches/generations",
		bytes.NewReader(body),
	)
	context.Request.Header.Set("Content-Type", "application/json")
	CreatePlaygroundImageGenerationBatch(context)
	return recorder
}

func TestCreatePlaygroundImageGenerationBatchSplitsCountAndIsIdempotent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPlaygroundImageControllerTestDB(t)
	const modelName = "gemini-3.1-flash-image-1K"
	body, err := common.Marshal(map[string]any{
		"client_batch_id": "browser-batch-1",
		"count":           10,
		"model":           modelName,
		"group":           "image",
		"prompt":          "draw a test image",
		"resolution":      "1K",
		"n":               99,
	})
	require.NoError(t, err)

	recorder := runPlaygroundImageGenerationBatchRequest(t, body)
	require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())

	var batch model.PlaygroundImageBatch
	require.NoError(t, db.Where("user_id = ? AND client_batch_id = ?", 1, "browser-batch-1").First(&batch).Error)
	assert.Equal(t, modelName, batch.Model)
	assert.Equal(t, 10, batch.TaskCount)

	var requestPayload map[string]any
	require.NoError(t, common.Unmarshal([]byte(batch.RequestPayload), &requestPayload))
	assert.Equal(t, modelName, requestPayload["model"])
	assert.EqualValues(t, 1, requestPayload["n"])
	_, hasCount := requestPayload["count"]
	assert.False(t, hasCount)
	_, hasClientBatchID := requestPayload["client_batch_id"]
	assert.False(t, hasClientBatchID)

	var taskCount int64
	require.NoError(t, db.Model(&model.PlaygroundImageTask{}).Where("batch_record_id = ?", batch.ID).Count(&taskCount).Error)
	assert.EqualValues(t, 10, taskCount)

	duplicateBody, err := common.Marshal(map[string]any{
		"client_batch_id": "browser-batch-1",
		"count":           25,
		"model":           "different-model",
		"prompt":          "this payload must not replace the original",
	})
	require.NoError(t, err)
	duplicate := runPlaygroundImageGenerationBatchRequest(t, duplicateBody)
	require.Equal(t, http.StatusAccepted, duplicate.Code, duplicate.Body.String())
	require.NoError(t, db.Model(&model.PlaygroundImageTask{}).Where("batch_record_id = ?", batch.ID).Count(&taskCount).Error)
	assert.EqualValues(t, 10, taskCount)
}

func TestCreatePlaygroundImageGenerationBatchRejectsCountAboveMaximum(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPlaygroundImageControllerTestDB(t)
	body, err := common.Marshal(map[string]any{
		"client_batch_id": "too-many-images",
		"count":           model.PlaygroundImageMaxBatchCount + 1,
		"model":           "gpt-image-2",
		"group":           "image",
		"prompt":          "draw too many images",
	})
	require.NoError(t, err)

	recorder := runPlaygroundImageGenerationBatchRequest(t, body)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "between 1 and 50")

	var taskCount int64
	require.NoError(t, db.Model(&model.PlaygroundImageTask{}).Count(&taskCount).Error)
	assert.Zero(t, taskCount)
}

func TestCreatePlaygroundImageEditBatchStoresReferenceOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPlaygroundImageControllerTestDB(t)
	t.Setenv("PLAYGROUND_IMAGE_STORAGE_PATH", t.TempDir())

	var imageBytes bytes.Buffer
	referenceImage := image.NewRGBA(image.Rect(0, 0, 2, 2))
	referenceImage.Set(0, 0, color.RGBA{R: 255, A: 255})
	require.NoError(t, png.Encode(&imageBytes, referenceImage))

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("client_batch_id", "edit-batch-1"))
	require.NoError(t, writer.WriteField("count", "3"))
	require.NoError(t, writer.WriteField("model", "gpt-image-2"))
	require.NoError(t, writer.WriteField("group", "image"))
	require.NoError(t, writer.WriteField("prompt", "edit this reference"))
	require.NoError(t, writer.WriteField("n", "77"))
	part, err := writer.CreateFormFile("image", "reference.png")
	require.NoError(t, err)
	_, err = part.Write(imageBytes.Bytes())
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", 1)
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/playground/image-batches/edits",
		&body,
	)
	context.Request.Header.Set("Content-Type", writer.FormDataContentType())
	CreatePlaygroundImageEditBatch(context)
	require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())

	var batch model.PlaygroundImageBatch
	require.NoError(t, db.Where("client_batch_id = ?", "edit-batch-1").First(&batch).Error)
	assert.Equal(t, 3, batch.TaskCount)
	var references []service.PlaygroundImageReferenceFile
	require.NoError(t, common.Unmarshal([]byte(batch.ReferenceFiles), &references))
	require.Len(t, references, 1)
	file, err := service.OpenPlaygroundImageStoredFile(references[0].Path)
	require.NoError(t, err)
	require.NoError(t, file.Close())

	var payload service.PlaygroundImageEditPayload
	require.NoError(t, common.Unmarshal([]byte(batch.RequestPayload), &payload))
	assert.Equal(t, []string{"1"}, payload.Fields["n"])
	relayBody, contentType, err := buildPlaygroundImageEditRelayRequest(&batch)
	require.NoError(t, err)
	relayRequest := httptest.NewRequest(http.MethodPost, "/pg/images/edits", relayBody)
	relayRequest.Header.Set("Content-Type", contentType)
	require.NoError(t, relayRequest.ParseMultipartForm(32<<20))
	t.Cleanup(func() {
		if relayRequest.MultipartForm != nil {
			_ = relayRequest.MultipartForm.RemoveAll()
		}
	})
	assert.Equal(t, "gpt-image-2", relayRequest.FormValue("model"))
	assert.Equal(t, "1", relayRequest.FormValue("n"))
	require.Len(t, relayRequest.MultipartForm.File["image"], 1)
	var taskCount int64
	require.NoError(t, db.Model(&model.PlaygroundImageTask{}).Where("batch_record_id = ?", batch.ID).Count(&taskCount).Error)
	assert.EqualValues(t, 3, taskCount)
}

func TestDeletePlaygroundImageTaskHardDeletesCompletedResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPlaygroundImageControllerTestDB(t)
	storageRoot := t.TempDir()
	t.Setenv("PLAYGROUND_IMAGE_STORAGE_PATH", storageRoot)

	batch, created, err := model.CreatePlaygroundImageBatch(model.CreatePlaygroundImageBatchParams{
		UserID:         1,
		ClientBatchID:  "delete-completed-image",
		Mode:           model.PlaygroundImageModeGenerate,
		Prompt:         "delete this image",
		Model:          "gpt-image-2",
		RequestPayload: `{"model":"gpt-image-2","n":1}`,
		Count:          1,
	})
	require.NoError(t, err)
	require.True(t, created)
	var task model.PlaygroundImageTask
	require.NoError(t, db.Where("batch_record_id = ?", batch.ID).First(&task).Error)

	resultPath := filepath.Join("results", "1", "delete-completed.png")
	fullPath := filepath.Join(storageRoot, resultPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o750))
	require.NoError(t, os.WriteFile(fullPath, []byte("image"), 0o640))
	require.NoError(t, db.Model(&model.PlaygroundImageTask{}).Where("id = ?", task.ID).Updates(map[string]any{
		"status":      model.PlaygroundImageTaskSucceeded,
		"result_path": resultPath,
	}).Error)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", 1)
	context.Params = gin.Params{{Key: "id", Value: task.TaskID}}
	context.Request = httptest.NewRequest(http.MethodDelete, "/api/playground/image-tasks/"+task.TaskID, nil)
	DeletePlaygroundImageTask(context)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var remaining model.PlaygroundImageTask
	err = db.Where("task_id = ?", task.TaskID).First(&remaining).Error
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	var remainingBatch model.PlaygroundImageBatch
	err = db.Where("id = ?", batch.ID).First(&remainingBatch).Error
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = os.Stat(fullPath)
	assert.ErrorIs(t, err, os.ErrNotExist)

	secondRecorder := httptest.NewRecorder()
	secondContext, _ := gin.CreateTestContext(secondRecorder)
	secondContext.Set("id", 1)
	secondContext.Params = gin.Params{{Key: "id", Value: task.TaskID}}
	secondContext.Request = httptest.NewRequest(http.MethodDelete, "/api/playground/image-tasks/"+task.TaskID, nil)
	DeletePlaygroundImageTask(secondContext)

	assert.Equal(t, http.StatusOK, secondRecorder.Code)
	var secondResponse struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(secondRecorder.Body.Bytes(), &secondResponse))
	assert.True(t, secondResponse.Success)
}

func TestPlaygroundImageSignedContentStaysOnMainSiteAndDownloads(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPlaygroundImageControllerTestDB(t)
	storageRoot := t.TempDir()
	t.Setenv("PLAYGROUND_IMAGE_STORAGE_PATH", storageRoot)

	var imageBytes bytes.Buffer
	value := image.NewRGBA(image.Rect(0, 0, 2, 2))
	value.Set(0, 0, color.RGBA{B: 255, A: 255})
	require.NoError(t, png.Encode(&imageBytes, value))
	relativePath := filepath.Join("results", "1", "2026", "07", "26", "imgtask_content.png")
	fullPath := filepath.Join(storageRoot, relativePath)
	require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o750))
	require.NoError(t, os.WriteFile(fullPath, imageBytes.Bytes(), 0o640))

	expiresAt := time.Now().Add(time.Hour).Unix()
	task := model.PlaygroundImageTask{
		TaskID:         "imgtask_content",
		UserID:         1,
		Status:         model.PlaygroundImageTaskSucceeded,
		ResultPath:     relativePath,
		ResultMimeType: "image/png",
		ResultSize:     int64(imageBytes.Len()),
		CreatedAt:      time.Now().Unix(),
		ExpiresAt:      expiresAt,
	}
	require.NoError(t, db.Create(&task).Error)

	contentURL := playgroundImageContentURL(task.TaskID, expiresAt, false)
	assert.True(t, strings.HasPrefix(contentURL, "/api/playground/image-tasks/"))
	assert.NotContains(t, contentURL, "image.qlhazycoder")

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "id", Value: task.TaskID}}
	context.Request = httptest.NewRequest(http.MethodGet, contentURL, nil)
	GetPlaygroundImageTaskContent(context)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Equal(t, "image/png", recorder.Header().Get("Content-Type"))
	assert.Equal(t, imageBytes.Bytes(), recorder.Body.Bytes())

	downloadURL := playgroundImageContentURL(task.TaskID, expiresAt, true)
	downloadRecorder := httptest.NewRecorder()
	downloadContext, _ := gin.CreateTestContext(downloadRecorder)
	downloadContext.Params = gin.Params{{Key: "id", Value: task.TaskID}}
	downloadContext.Request = httptest.NewRequest(http.MethodGet, downloadURL, nil)
	GetPlaygroundImageTaskContent(downloadContext)
	require.Equal(t, http.StatusOK, downloadRecorder.Code)
	assert.Contains(t, downloadRecorder.Header().Get("Content-Disposition"), "attachment")

	invalidRecorder := httptest.NewRecorder()
	invalidContext, _ := gin.CreateTestContext(invalidRecorder)
	invalidContext.Params = gin.Params{{Key: "id", Value: task.TaskID}}
	invalidContext.Request = httptest.NewRequest(
		http.MethodGet,
		strings.Replace(contentURL, "signature=", "signature=x", 1),
		nil,
	)
	GetPlaygroundImageTaskContent(invalidContext)
	assert.Equal(t, http.StatusForbidden, invalidRecorder.Code)
}
