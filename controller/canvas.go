package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func canvasError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	message := "Failed to save canvas content"
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		status = http.StatusNotFound
		message = "Canvas content not found"
	case errors.Is(err, model.ErrCanvasConflict):
		status = http.StatusConflict
		message = model.ErrCanvasConflict.Error()
	case errors.Is(err, model.ErrCanvasInvalid):
		status = http.StatusUnprocessableEntity
		message = "Invalid canvas content"
	case errors.Is(err, service.ErrCanvasAssetTooLarge):
		status = http.StatusRequestEntityTooLarge
		message = err.Error()
	}
	if status == http.StatusInternalServerError {
		common.SysError("canvas operation failed: " + err.Error())
	}
	common.ApiErrorStatus(c, status, common.NewMessage(message))
}

func CanvasBootstrap(c *gin.Context) {
	user, err := model.GetUserCache(c.GetInt("id"))
	if err != nil {
		canvasError(c, err)
		return
	}
	options, err := service.CanvasModelCatalog(user.Group)
	if err != nil {
		canvasError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"protocol": 1, "user_id": user.Id, "models": options, "groups": service.GetUserUsableGroups(user.Group), "embedded": true, "extensions": false, "max_document_bytes": model.CanvasMaxDocumentBytes})
}

func ListCanvasProjects(c *gin.Context) {
	offset, _ := strconv.Atoi(c.Query("offset"))
	var records []model.CanvasProject
	query := model.DB.Where("user_id = ?", c.GetInt("id"))
	if c.Query("trash") == "true" {
		query = query.Where("deleted_at > 0")
	} else {
		query = query.Where("deleted_at = 0")
	}
	if err := query.Order("updated_at DESC").Order("id DESC").Offset(max(0, offset)).Limit(100).Find(&records).Error; err != nil {
		canvasError(c, err)
		return
	}
	next := -1
	if len(records) == 100 {
		next = max(0, offset) + 100
	}
	common.ApiSuccess(c, gin.H{"items": records, "next_offset": next})
}

func GetCanvasProject(c *gin.Context) {
	var record model.CanvasProject
	if err := model.DB.Where("user_id = ? AND project_id = ? AND deleted_at = 0", c.GetInt("id"), c.Param("project_id")).First(&record).Error; err != nil {
		canvasError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"project": record, "document": json.RawMessage(record.Document)})
}

func PutCanvasProject(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, model.CanvasMaxDocumentBytes+4096)
	var request struct {
		ProjectID string         `json:"project_id"`
		Revision  int64          `json:"revision"`
		Document  map[string]any `json:"document"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.Revision < 0 {
		canvasError(c, model.ErrCanvasInvalid)
		return
	}
	projectID := c.Param("project_id")
	if projectID == "" {
		projectID = request.ProjectID
	}
	if c.Request.Method == http.MethodPost && request.Revision != 0 {
		canvasError(c, model.ErrCanvasInvalid)
		return
	}
	if c.Request.Method == http.MethodPatch && request.Revision == 0 {
		canvasError(c, model.ErrCanvasInvalid)
		return
	}
	record, err := model.SaveCanvasProject(c.GetInt("id"), projectID, request.Revision, request.Document)
	if err != nil {
		canvasError(c, err)
		return
	}
	common.ApiSuccess(c, record)
}

func DeleteCanvasProject(c *gin.Context) {
	revision, _ := strconv.ParseInt(c.Query("revision"), 10, 64)
	result := model.DB.Model(&model.CanvasProject{}).Where("user_id = ? AND project_id = ? AND revision = ? AND deleted_at = 0", c.GetInt("id"), c.Param("project_id"), revision).
		Updates(map[string]any{"deleted_at": time.Now().UnixMilli(), "revision": revision + 1})
	if result.Error != nil {
		canvasError(c, result.Error)
		return
	}
	if result.RowsAffected != 1 {
		canvasError(c, model.ErrCanvasConflict)
		return
	}
	common.ApiSuccess(c, nil)
}

func RestoreCanvasProject(c *gin.Context) {
	result := model.DB.Model(&model.CanvasProject{}).Where("user_id = ? AND project_id = ? AND deleted_at > ?", c.GetInt("id"), c.Param("project_id"), time.Now().Add(-30*24*time.Hour).UnixMilli()).
		Updates(map[string]any{"deleted_at": 0, "revision": gorm.Expr("revision + 1")})
	if result.Error != nil {
		canvasError(c, result.Error)
		return
	}
	if result.RowsAffected != 1 {
		canvasError(c, gorm.ErrRecordNotFound)
		return
	}
	GetCanvasProject(c)
}

func CanvasSettings(c *gin.Context) {
	namespace, key := c.Param("namespace"), c.Param("entry_key")
	switch namespace {
	case "app_state", "image_generation_logs", "video_generation_logs", "prompt_cache":
	default:
		canvasError(c, model.ErrCanvasInvalid)
		return
	}
	if len(key) > 192 {
		canvasError(c, model.ErrCanvasInvalid)
		return
	}
	userID := c.GetInt("id")
	if c.Request.Method == http.MethodGet && key == "" {
		var rows []model.CanvasUserSetting
		if err := model.DB.Where("user_id = ? AND namespace = ?", userID, namespace).Find(&rows).Error; err != nil {
			canvasError(c, err)
			return
		}
		common.ApiSuccess(c, rows)
		return
	}
	record := model.CanvasUserSetting{}
	query := model.DB.Where("user_id = ? AND namespace = ? AND entry_key = ?", userID, namespace, key)
	if c.Request.Method == http.MethodGet {
		err := query.First(&record).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiSuccess(c, nil)
			return
		}
		if err != nil {
			canvasError(c, err)
			return
		}
		common.ApiSuccess(c, record)
		return
	}
	if c.Request.Method == http.MethodDelete {
		revision, _ := strconv.ParseInt(c.Query("revision"), 10, 64)
		if err := model.DeleteCanvasSetting(userID, namespace, key, revision); err != nil {
			canvasError(c, err)
			return
		}
		common.ApiSuccess(c, nil)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, model.CanvasMaxDocumentBytes+4096)
	var request struct {
		Revision int64 `json:"revision"`
		Value    any   `json:"value"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.Revision < 0 {
		canvasError(c, model.ErrCanvasInvalid)
		return
	}
	cleaned, _, err := model.SanitizeCanvasValue(request.Value)
	if err != nil {
		canvasError(c, err)
		return
	}
	raw, err := common.Marshal(cleaned)
	if err != nil {
		canvasError(c, err)
		return
	}
	record = model.CanvasUserSetting{UserID: userID, Namespace: namespace, EntryKey: key, Value: model.CanvasJSON(raw)}
	if err := model.SaveCanvasSetting(&record, request.Revision); err != nil {
		canvasError(c, err)
		return
	}
	common.ApiSuccess(c, record)
}

func ListCanvasAssets(c *gin.Context) {
	var records []model.CanvasAsset
	offset, _ := strconv.Atoi(c.Query("offset"))
	if err := model.DB.Where("user_id = ? AND deleted_at = 0", c.GetInt("id")).Order("id").Offset(max(0, offset)).Limit(100).Find(&records).Error; err != nil {
		canvasError(c, err)
		return
	}
	next := -1
	if len(records) == 100 {
		next = max(0, offset) + 100
	}
	common.ApiSuccess(c, gin.H{"items": records, "next_offset": next})
}

func UploadCanvasAsset(c *gin.Context) {
	var existing model.CanvasAsset
	err := model.DB.Where("user_id = ? AND asset_id = ? AND deleted_at = 0", c.GetInt("id"), c.Param("asset_id")).First(&existing).Error
	if err == nil {
		canvasError(c, model.ErrCanvasConflict)
		return
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		canvasError(c, err)
		return
	}
	asset, err := service.SaveCanvasAsset(c.GetInt("id"), c.Param("asset_id"), "", c.ContentType(), "", c.Request.Body)
	if err != nil {
		canvasError(c, err)
		return
	}
	common.ApiSuccess(c, asset)
}

func CanvasAsset(c *gin.Context) {
	var asset model.CanvasAsset
	if err := model.DB.Where("user_id = ? AND asset_id = ? AND deleted_at = 0", c.GetInt("id"), c.Param("asset_id")).First(&asset).Error; err != nil {
		canvasError(c, err)
		return
	}
	if c.Request.Method == http.MethodDelete {
		if err := model.DeleteCanvasAsset(asset.UserID, asset.AssetID); err != nil {
			canvasError(c, err)
			return
		}
		common.ApiSuccess(c, nil)
		return
	}
	if strings.HasSuffix(c.FullPath(), "/content") {
		if err := service.ServeCanvasFile(c.Writer, c.Request, asset.StorageKey, asset.MIME, asset.SHA256); err != nil {
			canvasError(c, gorm.ErrRecordNotFound)
		}
		return
	}
	common.ApiSuccess(c, asset)
}

func CanvasRuns(c *gin.Context) {
	var runs []model.CanvasGenerationRun
	offset, _ := strconv.Atoi(c.Query("offset"))
	if err := model.DB.Where("user_id = ?", c.GetInt("id")).Order("id DESC").Offset(max(0, offset)).Limit(100).Find(&runs).Error; err != nil {
		canvasError(c, err)
		return
	}
	for i := range runs {
		if runs[i].TaskID == "" {
			continue
		}
		task, exists, err := model.GetByTaskId(runs[i].UserID, runs[i].TaskID)
		if err != nil {
			canvasError(c, err)
			return
		}
		if !exists {
			continue
		}
		switch task.Status {
		case model.TaskStatusSuccess:
			runs[i].Status = "succeeded"
		case model.TaskStatusFailure:
			runs[i].Status = "failed"
		default:
			runs[i].Status = "running"
		}
	}
	next := -1
	if len(runs) == 100 {
		next = max(0, offset) + 100
	}
	common.ApiSuccess(c, gin.H{"items": runs, "next_offset": next})
}

func CanvasRun(c *gin.Context) {
	var run model.CanvasGenerationRun
	if err := model.DB.Where("user_id = ? AND run_id = ?", c.GetInt("id"), c.Param("run_id")).First(&run).Error; err != nil {
		canvasError(c, err)
		return
	}
	if strings.HasSuffix(c.FullPath(), "/result") {
		if run.ResultPath == "" || run.ArchiveStatus != "saved" {
			canvasError(c, gorm.ErrRecordNotFound)
			return
		}
		if err := service.ServeCanvasFile(c.Writer, c.Request, run.ResultPath, run.ResultMIME, ""); err != nil {
			canvasError(c, gorm.ErrRecordNotFound)
		}
		return
	}
	if c.Request.Method == http.MethodPost {
		cancelled := false
		if cancel, ok := canvasRunCancels.Load(run.RunID); ok {
			cancel.(context.CancelFunc)()
			cancelled = true
		}
		// Cancelling observation does not cancel an upstream durable task/refund.
		common.ApiSuccess(c, gin.H{"observation_cancelled": cancelled})
		return
	}
	if run.TaskID != "" {
		task, exists, err := model.GetByTaskId(run.UserID, run.TaskID)
		if err != nil {
			canvasError(c, err)
			return
		}
		if exists {
			common.ApiSuccess(c, gin.H{"run": run, "task_status": task.Status})
			return
		}
	}
	common.ApiSuccess(c, gin.H{"run": run})
}

// Only URLs in the recorded image response may be fetched. This is not an
// arbitrary URL importer, and no dashboard Authorization header is forwarded.
func ImportCanvasGeneratedAsset(c *gin.Context) {
	var request struct {
		RunID   string `json:"run_id"`
		URL     string `json:"url"`
		AssetID string `json:"asset_id"`
	}
	if common.DecodeJson(http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20), &request) != nil {
		canvasError(c, model.ErrCanvasInvalid)
		return
	}
	var run model.CanvasGenerationRun
	if err := model.DB.Where("user_id = ? AND run_id = ? AND archive_status = ?", c.GetInt("id"), request.RunID, "saved").First(&run).Error; err != nil {
		canvasError(c, err)
		return
	}
	if run.Operation != "/v1/images/generations" && run.Operation != "/v1/images/edits" {
		canvasError(c, model.ErrCanvasInvalid)
		return
	}
	path, err := service.CanvasStoragePath(run.ResultPath)
	if err != nil {
		canvasError(c, err)
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		canvasError(c, err)
		return
	}
	var response struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if common.Unmarshal(raw, &response) != nil {
		canvasError(c, model.ErrCanvasInvalid)
		return
	}
	allowed := false
	for _, image := range response.Data {
		if image.URL == request.URL && image.URL != "" {
			allowed = true
			break
		}
	}
	if !allowed {
		canvasError(c, model.ErrCanvasInvalid)
		return
	}
	data, mimeType, err := service.DownloadCanvasGeneratedImage(c.Request.Context(), request.URL)
	if err != nil {
		canvasError(c, err)
		return
	}
	asset, err := service.SaveCanvasAsset(c.GetInt("id"), request.AssetID, "", mimeType, run.RunID, bytes.NewReader(data))
	if err != nil {
		canvasError(c, err)
		return
	}
	common.ApiSuccess(c, asset)
}
