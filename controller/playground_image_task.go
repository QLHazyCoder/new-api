package controller

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	playgroundImageTaskPageSizeMax = 200
	playgroundImageReferenceMax    = 4
)

type playgroundImageTaskResponse struct {
	ID         string                          `json:"id"`
	BatchID    string                          `json:"batch_id"`
	TaskIndex  int                             `json:"task_index"`
	Mode       string                          `json:"mode"`
	Prompt     string                          `json:"prompt"`
	Model      string                          `json:"model"`
	Group      string                          `json:"group"`
	Config     map[string]any                  `json:"config"`
	Status     model.PlaygroundImageTaskStatus `json:"status"`
	Error      string                          `json:"error,omitempty"`
	ErrorCode  string                          `json:"error_code,omitempty"`
	Image      *playgroundImageTaskImage       `json:"image,omitempty"`
	CreatedAt  int64                           `json:"created_at"`
	StartedAt  int64                           `json:"started_at,omitempty"`
	FinishedAt int64                           `json:"finished_at,omitempty"`
	ExpiresAt  int64                           `json:"expires_at"`
}

type playgroundImageTaskImage struct {
	URL         string `json:"url"`
	DownloadURL string `json:"download_url"`
	MimeType    string `json:"mime_type"`
	Size        int64  `json:"size"`
}

func playgroundImageAPIError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"success": false,
		"message": message,
	})
}

func playgroundImageAccepted(c *gin.Context, data any) {
	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"message": "",
		"data":    data,
	})
}

func playgroundImageJSONString(payload map[string]json.RawMessage, key string) (string, error) {
	raw, exists := payload[key]
	if !exists {
		return "", nil
	}
	var value string
	if err := common.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("%s must be a string", key)
	}
	return value, nil
}

func playgroundImageCount(payload map[string]json.RawMessage) (int, error) {
	raw, exists := payload["count"]
	if !exists {
		return 1, nil
	}
	var count int
	if err := common.Unmarshal(raw, &count); err != nil || count < 1 || count > model.PlaygroundImageMaxBatchCount {
		return 0, fmt.Errorf("count must be an integer between 1 and %d", model.PlaygroundImageMaxBatchCount)
	}
	return count, nil
}

func validatePlaygroundImageBatchFields(clientBatchID, modelName, prompt string, count int) error {
	if strings.TrimSpace(clientBatchID) == "" || len(clientBatchID) > 128 {
		return errors.New("client_batch_id must contain 1 to 128 characters")
	}
	if strings.TrimSpace(modelName) == "" {
		return errors.New("model is required")
	}
	if strings.TrimSpace(prompt) == "" {
		return errors.New("prompt is required")
	}
	if count < 1 || count > model.PlaygroundImageMaxBatchCount {
		return fmt.Errorf("count must be an integer between 1 and %d", model.PlaygroundImageMaxBatchCount)
	}
	return nil
}

func createPlaygroundImageBatchResponse(batch *model.PlaygroundImageBatch) (any, error) {
	summary, err := model.GetPlaygroundImageBatchSummary(batch)
	if err != nil {
		return nil, err
	}
	return summary, nil
}

func CreatePlaygroundImageGenerationBatch(c *gin.Context) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		playgroundImageAPIError(c, http.StatusBadRequest, err.Error())
		return
	}
	body, err := storage.Bytes()
	if err != nil {
		playgroundImageAPIError(c, http.StatusBadRequest, err.Error())
		return
	}
	var payload map[string]json.RawMessage
	if err := common.Unmarshal(body, &payload); err != nil {
		playgroundImageAPIError(c, http.StatusBadRequest, "invalid image generation request")
		return
	}
	clientBatchID, err := playgroundImageJSONString(payload, "client_batch_id")
	if err != nil {
		playgroundImageAPIError(c, http.StatusBadRequest, err.Error())
		return
	}
	modelName, err := playgroundImageJSONString(payload, "model")
	if err != nil {
		playgroundImageAPIError(c, http.StatusBadRequest, err.Error())
		return
	}
	prompt, err := playgroundImageJSONString(payload, "prompt")
	if err != nil {
		playgroundImageAPIError(c, http.StatusBadRequest, err.Error())
		return
	}
	requestGroup, err := playgroundImageJSONString(payload, "group")
	if err != nil {
		playgroundImageAPIError(c, http.StatusBadRequest, err.Error())
		return
	}
	count, err := playgroundImageCount(payload)
	if err != nil {
		playgroundImageAPIError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := validatePlaygroundImageBatchFields(clientBatchID, modelName, prompt, count); err != nil {
		playgroundImageAPIError(c, http.StatusBadRequest, err.Error())
		return
	}

	delete(payload, "client_batch_id")
	delete(payload, "count")
	payload["n"] = json.RawMessage("1")
	requestPayload, err := common.Marshal(payload)
	if err != nil {
		playgroundImageAPIError(c, http.StatusInternalServerError, "failed to store image request")
		return
	}
	batch, _, err := model.CreatePlaygroundImageBatch(model.CreatePlaygroundImageBatchParams{
		UserID:         c.GetInt("id"),
		ClientBatchID:  clientBatchID,
		Mode:           model.PlaygroundImageModeGenerate,
		Prompt:         prompt,
		Model:          modelName,
		RequestGroup:   requestGroup,
		RequestPayload: string(requestPayload),
		Count:          count,
	})
	if err != nil {
		playgroundImageAPIError(c, http.StatusInternalServerError, "failed to create image tasks")
		return
	}
	response, err := createPlaygroundImageBatchResponse(batch)
	if err != nil {
		playgroundImageAPIError(c, http.StatusInternalServerError, "failed to read image batch")
		return
	}
	playgroundImageAccepted(c, response)
}

func firstPlaygroundImageFormValue(values map[string][]string, key string) string {
	if len(values[key]) == 0 {
		return ""
	}
	return values[key][0]
}

func clonePlaygroundImageFormValues(values map[string][]string) map[string][]string {
	cloned := make(map[string][]string, len(values))
	for key, items := range values {
		cloned[key] = append([]string(nil), items...)
	}
	return cloned
}

func CreatePlaygroundImageEditBatch(c *gin.Context) {
	form, err := c.MultipartForm()
	if err != nil {
		playgroundImageAPIError(c, http.StatusBadRequest, "invalid image edit request")
		return
	}
	defer form.RemoveAll()
	clientBatchID := firstPlaygroundImageFormValue(form.Value, "client_batch_id")
	modelName := firstPlaygroundImageFormValue(form.Value, "model")
	prompt := firstPlaygroundImageFormValue(form.Value, "prompt")
	requestGroup := firstPlaygroundImageFormValue(form.Value, "group")
	count := 1
	if countValue := firstPlaygroundImageFormValue(form.Value, "count"); countValue != "" {
		count, err = strconv.Atoi(countValue)
		if err != nil {
			playgroundImageAPIError(c, http.StatusBadRequest, fmt.Sprintf("count must be an integer between 1 and %d", model.PlaygroundImageMaxBatchCount))
			return
		}
	}
	if err := validatePlaygroundImageBatchFields(clientBatchID, modelName, prompt, count); err != nil {
		playgroundImageAPIError(c, http.StatusBadRequest, err.Error())
		return
	}
	files := append([]*multipart.FileHeader(nil), form.File["image"]...)
	files = append(files, form.File["image[]"]...)
	if len(files) == 0 || len(files) > playgroundImageReferenceMax {
		playgroundImageAPIError(c, http.StatusBadRequest, "image edits require 1 to 4 reference images")
		return
	}

	batchID := "imgbatch_" + uuid.NewString()
	references := make([]service.PlaygroundImageReferenceFile, 0, len(files))
	removeSavedReferences := func() {
		encoded, marshalErr := common.Marshal(references)
		if marshalErr == nil {
			_ = service.RemovePlaygroundImageReferencesJSON(string(encoded))
		}
	}
	for index, header := range files {
		reference, saveErr := service.SavePlaygroundImageReference(batchID, index, header)
		if saveErr != nil {
			removeSavedReferences()
			playgroundImageAPIError(c, http.StatusBadRequest, saveErr.Error())
			return
		}
		references = append(references, reference)
	}
	referenceJSON, err := common.Marshal(references)
	if err != nil {
		removeSavedReferences()
		playgroundImageAPIError(c, http.StatusInternalServerError, "failed to store reference images")
		return
	}
	fields := clonePlaygroundImageFormValues(form.Value)
	delete(fields, "client_batch_id")
	delete(fields, "count")
	fields["n"] = []string{"1"}
	editPayload, err := common.Marshal(service.PlaygroundImageEditPayload{Fields: fields})
	if err != nil {
		removeSavedReferences()
		playgroundImageAPIError(c, http.StatusInternalServerError, "failed to store image edit request")
		return
	}
	batch, created, err := model.CreatePlaygroundImageBatch(model.CreatePlaygroundImageBatchParams{
		BatchID:        batchID,
		UserID:         c.GetInt("id"),
		ClientBatchID:  clientBatchID,
		Mode:           model.PlaygroundImageModeEdit,
		Prompt:         prompt,
		Model:          modelName,
		RequestGroup:   requestGroup,
		RequestPayload: string(editPayload),
		ReferenceFiles: string(referenceJSON),
		Count:          count,
	})
	if err != nil {
		removeSavedReferences()
		playgroundImageAPIError(c, http.StatusInternalServerError, "failed to create image edit tasks")
		return
	}
	if !created {
		removeSavedReferences()
	}
	response, err := createPlaygroundImageBatchResponse(batch)
	if err != nil {
		playgroundImageAPIError(c, http.StatusInternalServerError, "failed to read image batch")
		return
	}
	playgroundImageAccepted(c, response)
}

func playgroundImageBatchConfig(batch model.PlaygroundImageBatch) map[string]any {
	config := make(map[string]any)
	if batch.Mode == model.PlaygroundImageModeGenerate {
		_ = common.Unmarshal([]byte(batch.RequestPayload), &config)
		return config
	}
	var payload service.PlaygroundImageEditPayload
	if err := common.Unmarshal([]byte(batch.RequestPayload), &payload); err != nil {
		return config
	}
	for key, values := range payload.Fields {
		if len(values) > 0 {
			config[key] = values[0]
		}
	}
	return config
}

func playgroundImageContentSignature(taskID string, expiresAt int64) string {
	return common.GenerateHMAC(fmt.Sprintf("playground-image:%s:%d", taskID, expiresAt))
}

func playgroundImageContentURL(taskID string, expiresAt int64, download bool) string {
	query := url.Values{}
	query.Set("expires", strconv.FormatInt(expiresAt, 10))
	query.Set("signature", playgroundImageContentSignature(taskID, expiresAt))
	if download {
		query.Set("download", "1")
	}
	return "/api/playground/image-tasks/" + url.PathEscape(taskID) + "/content?" + query.Encode()
}

func buildPlaygroundImageTaskResponse(task model.PlaygroundImageTask, batch model.PlaygroundImageBatch) playgroundImageTaskResponse {
	response := playgroundImageTaskResponse{
		ID:         task.TaskID,
		BatchID:    batch.BatchID,
		TaskIndex:  task.TaskIndex,
		Mode:       batch.Mode,
		Prompt:     batch.Prompt,
		Model:      batch.Model,
		Group:      batch.RequestGroup,
		Config:     playgroundImageBatchConfig(batch),
		Status:     task.Status,
		Error:      task.ErrorMessage,
		ErrorCode:  task.ErrorCode,
		CreatedAt:  task.CreatedAt,
		StartedAt:  task.StartedAt,
		FinishedAt: task.FinishedAt,
		ExpiresAt:  task.ExpiresAt,
	}
	if task.Status == model.PlaygroundImageTaskSucceeded && task.ResultPath != "" {
		response.Image = &playgroundImageTaskImage{
			URL:         playgroundImageContentURL(task.TaskID, task.ExpiresAt, false),
			DownloadURL: playgroundImageContentURL(task.TaskID, task.ExpiresAt, true),
			MimeType:    task.ResultMimeType,
			Size:        task.ResultSize,
		}
	}
	return response
}

func ListPlaygroundImageTasks(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "100"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 100
	}
	if pageSize > playgroundImageTaskPageSizeMax {
		pageSize = playgroundImageTaskPageSizeMax
	}
	tasks, total, err := model.ListPlaygroundImageTasks(c.GetInt("id"), page, pageSize, common.GetTimestamp())
	if err != nil {
		playgroundImageAPIError(c, http.StatusInternalServerError, "failed to list image tasks")
		return
	}
	batchIDs := make([]int64, 0, len(tasks))
	seenBatchIDs := make(map[int64]struct{}, len(tasks))
	for _, task := range tasks {
		if _, exists := seenBatchIDs[task.BatchRecordID]; exists {
			continue
		}
		seenBatchIDs[task.BatchRecordID] = struct{}{}
		batchIDs = append(batchIDs, task.BatchRecordID)
	}
	batches, err := model.GetPlaygroundImageBatchesByRecordIDs(batchIDs)
	if err != nil {
		playgroundImageAPIError(c, http.StatusInternalServerError, "failed to load image task batches")
		return
	}
	items := make([]playgroundImageTaskResponse, 0, len(tasks))
	for _, task := range tasks {
		batch, exists := batches[task.BatchRecordID]
		if !exists {
			continue
		}
		items = append(items, buildPlaygroundImageTaskResponse(task, batch))
	}
	common.ApiSuccess(c, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func GetPlaygroundImageBatch(c *gin.Context) {
	batch, err := model.GetPlaygroundImageBatchForUser(c.Param("id"), c.GetInt("id"))
	if err != nil {
		if errors.Is(err, model.ErrPlaygroundImageTaskNotFound) {
			playgroundImageAPIError(c, http.StatusNotFound, "image batch not found")
			return
		}
		playgroundImageAPIError(c, http.StatusInternalServerError, "failed to read image batch")
		return
	}
	summary, err := model.GetPlaygroundImageBatchSummary(batch)
	if err != nil {
		playgroundImageAPIError(c, http.StatusInternalServerError, "failed to summarize image batch")
		return
	}
	common.ApiSuccess(c, summary)
}

func RetryPlaygroundImageTask(c *gin.Context) {
	task, batch, err := model.GetPlaygroundImageTaskForUser(c.Param("id"), c.GetInt("id"))
	if err != nil || task.Hidden {
		playgroundImageAPIError(c, http.StatusNotFound, "image task not found")
		return
	}
	if batch.Mode == model.PlaygroundImageModeEdit {
		playgroundImageAPIError(c, http.StatusBadRequest, "upload the reference images again to retry this edit")
		return
	}
	if !task.Status.IsTerminal() {
		playgroundImageAPIError(c, http.StatusConflict, "active image tasks cannot be retried")
		return
	}
	retryBatch, _, err := model.CreatePlaygroundImageBatch(model.CreatePlaygroundImageBatchParams{
		UserID:         c.GetInt("id"),
		ClientBatchID:  "retry-" + uuid.NewString(),
		Mode:           model.PlaygroundImageModeGenerate,
		Prompt:         batch.Prompt,
		Model:          batch.Model,
		RequestGroup:   batch.RequestGroup,
		RequestPayload: batch.RequestPayload,
		Count:          1,
	})
	if err != nil {
		playgroundImageAPIError(c, http.StatusInternalServerError, "failed to retry image task")
		return
	}
	response, err := createPlaygroundImageBatchResponse(retryBatch)
	if err != nil {
		playgroundImageAPIError(c, http.StatusInternalServerError, "failed to read retry batch")
		return
	}
	playgroundImageAccepted(c, response)
}

func DeletePlaygroundImageTask(c *gin.Context) {
	result, err := model.DeletePlaygroundImageTask(c.Param("id"), c.GetInt("id"), common.GetTimestamp())
	if err != nil {
		if errors.Is(err, model.ErrPlaygroundImageTaskNotFound) {
			playgroundImageAPIError(c, http.StatusNotFound, "image task not found")
			return
		}
		playgroundImageAPIError(c, http.StatusInternalServerError, "failed to delete image task")
		return
	}
	if !result.WasActive {
		if result.ResultPath != "" {
			if err := service.RemovePlaygroundImageResult(result.ResultPath); err != nil {
				common.SysError(fmt.Sprintf("remove deleted playground image result: %v", err))
				service.CleanupPlaygroundImageBatchReferences(result.BatchRecordID)
				common.ApiSuccess(c, nil)
				return
			}
		}
		if _, err := model.DeleteHiddenPlaygroundImageTask(c.Param("id"), result.ResultPath); err != nil && !errors.Is(err, model.ErrPlaygroundImageTaskNotFound) {
			common.SysError(fmt.Sprintf("hard-delete playground image task: %v", err))
		}
	}
	service.CleanupPlaygroundImageBatchReferences(result.BatchRecordID)
	common.ApiSuccess(c, nil)
}

func GetPlaygroundImageTaskContent(c *gin.Context) {
	expiresAt, err := strconv.ParseInt(c.Query("expires"), 10, 64)
	if err != nil || expiresAt <= common.GetTimestamp() {
		http.Error(c.Writer, "image link expired", http.StatusGone)
		return
	}
	task, err := model.GetPlaygroundImageTaskByTaskID(c.Param("id"))
	if err != nil || task.Hidden || task.Status != model.PlaygroundImageTaskSucceeded || task.ResultPath == "" {
		http.NotFound(c.Writer, c.Request)
		return
	}
	if expiresAt != task.ExpiresAt {
		http.Error(c.Writer, "invalid image link", http.StatusForbidden)
		return
	}
	expectedSignature := playgroundImageContentSignature(task.TaskID, expiresAt)
	if !hmac.Equal([]byte(c.Query("signature")), []byte(expectedSignature)) {
		http.Error(c.Writer, "invalid image signature", http.StatusForbidden)
		return
	}
	file, err := service.OpenPlaygroundImageResult(task.ResultPath)
	if err != nil {
		http.NotFound(c.Writer, c.Request)
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		http.NotFound(c.Writer, c.Request)
		return
	}
	c.Header("Content-Type", task.ResultMimeType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", fmt.Sprintf("private, max-age=%d", max(0, expiresAt-common.GetTimestamp())))
	if c.Query("download") == "1" {
		filename := task.TaskID + filepath.Ext(task.ResultPath)
		c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	}
	http.ServeContent(c.Writer, c.Request, filepath.Base(task.ResultPath), stat.ModTime(), file)
}

func buildPlaygroundImageEditRelayRequest(batch *model.PlaygroundImageBatch) (io.Reader, string, error) {
	var payload service.PlaygroundImageEditPayload
	if err := common.Unmarshal([]byte(batch.RequestPayload), &payload); err != nil {
		return nil, "", err
	}
	var references []service.PlaygroundImageReferenceFile
	if err := common.Unmarshal([]byte(batch.ReferenceFiles), &references); err != nil {
		return nil, "", err
	}
	reader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)
	contentType := writer.FormDataContentType()
	go func() {
		for key, values := range payload.Fields {
			for _, value := range values {
				if err := writer.WriteField(key, value); err != nil {
					_ = pipeWriter.CloseWithError(err)
					return
				}
			}
		}
		for _, reference := range references {
			file, err := service.OpenPlaygroundImageStoredFile(reference.Path)
			if err != nil {
				_ = pipeWriter.CloseWithError(err)
				return
			}
			part, createErr := writer.CreateFormFile("image", reference.Name)
			if createErr == nil {
				_, createErr = io.Copy(part, file)
			}
			closeErr := file.Close()
			if createErr != nil {
				_ = pipeWriter.CloseWithError(createErr)
				return
			}
			if closeErr != nil {
				_ = pipeWriter.CloseWithError(closeErr)
				return
			}
		}
		if err := writer.Close(); err != nil {
			_ = pipeWriter.CloseWithError(err)
			return
		}
		_ = pipeWriter.Close()
	}()
	return reader, contentType, nil
}

func ExecutePlaygroundImageRelay(ctx context.Context, task *model.PlaygroundImageTask, batch *model.PlaygroundImageBatch) (*service.PlaygroundImageExecutionResult, error) {
	if task == nil || batch == nil {
		return nil, errors.New("image task and batch are required")
	}
	path := "/pg/images/generations"
	contentType := "application/json"
	var body io.Reader = strings.NewReader(batch.RequestPayload)
	if batch.Mode == model.PlaygroundImageModeEdit {
		path = "/pg/images/edits"
		var err error
		body, contentType, err = buildPlaygroundImageEditRelayRequest(batch)
		if err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, err
	}
	defer req.Body.Close()
	req.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()
	engine := gin.New()
	engine.Use(middleware.RequestId())
	engine.Use(middleware.BodyStorageCleanup())
	engine.Use(middleware.StatsMiddleware())
	engine.POST(path,
		middleware.SystemPerformanceCheck(),
		func(c *gin.Context) {
			userCache, cacheErr := model.GetUserCache(task.UserID)
			if cacheErr != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": cacheErr.Error()}})
				return
			}
			if userCache.Status != common.UserStatusEnabled {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": gin.H{"message": "user is disabled"}})
				return
			}
			c.Set("id", task.UserID)
			c.Set("username", userCache.Username)
			c.Set("role", common.RoleCommonUser)
			c.Set("group", userCache.Group)
			c.Set("user_group", userCache.Group)
			c.Set("use_access_token", false)
			userCache.WriteContext(c)
			c.Next()
		},
		middleware.Distribute(),
		func(c *gin.Context) {
			if batch.Mode == model.PlaygroundImageModeEdit {
				PlaygroundImageEdit(c)
				return
			}
			PlaygroundImageGeneration(c)
		},
	)
	engine.ServeHTTP(recorder, req)
	return &service.PlaygroundImageExecutionResult{
		StatusCode: recorder.Code,
		Header:     recorder.Header().Clone(),
		Body:       append([]byte(nil), recorder.Body.Bytes()...),
	}, nil
}
