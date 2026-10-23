package controller

import (
	"bufio"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"
)

// HTTP 200 also carries Responses SSE errors and JSON business failures. This
// describes the observed result only; settlement remains in the existing relay.
func canvasObservedResult(file *os.File, mime string) string {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "interrupted"
	}
	if strings.Contains(mime, "text/event-stream") {
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 4096), model.CanvasMaxDocumentBytes)
		complete := false
		for scanner.Scan() {
			data, ok := strings.CutPrefix(scanner.Text(), "data:")
			if !ok {
				continue
			}
			data = strings.TrimSpace(data)
			if data == "[DONE]" {
				complete = true
				continue
			}
			if data == "" {
				continue
			}
			var event struct {
				Type     string `json:"type"`
				Error    any    `json:"error"`
				Response struct {
					Status string `json:"status"`
					Error  any    `json:"error"`
				} `json:"response"`
			}
			if common.UnmarshalJsonStr(data, &event) != nil {
				return "interrupted"
			}
			if event.Error != nil || event.Response.Error != nil || event.Type == "error" || event.Type == "response.failed" || event.Response.Status == "failed" {
				return "failed"
			}
			if event.Type == "response.incomplete" {
				return "interrupted"
			}
			if event.Type == "response.completed" || event.Type == "image_generation.completed" || event.Type == "image_edit.completed" {
				complete = true
			}
		}
		if scanner.Err() != nil || !complete {
			return "interrupted"
		}
		return "succeeded"
	}
	if strings.Contains(mime, "json") {
		var result struct {
			Error   any    `json:"error"`
			Success *bool  `json:"success"`
			Code    any    `json:"code"`
			Status  string `json:"status"`
		}
		if common.DecodeJson(io.LimitReader(file, int64(max(1, constant.MaxFileDownloadMB))<<20), &result) != nil {
			return "interrupted"
		}
		if result.Error != nil || result.Success != nil && !*result.Success || result.Status == "failed" || result.Status == "cancelled" {
			return "failed"
		}
		if result.Code != nil && fmt.Sprint(result.Code) != "0" && fmt.Sprint(result.Code) != "200" {
			return "failed"
		}
	}
	return "succeeded"
}

var canvasRunCancels sync.Map

type canvasRunWriter struct {
	gin.ResponseWriter
	file  *os.File
	size  int64
	limit int64
	err   error
}

func (w *canvasRunWriter) Write(data []byte) (int, error) {
	if w.err == nil {
		if int64(len(data)) > w.limit-w.size {
			w.err = service.ErrCanvasAssetTooLarge
		} else {
			_, w.err = w.file.Write(data)
			w.size += int64(len(data))
		}
	}
	return w.ResponseWriter.Write(data)
}
func (w *canvasRunWriter) WriteString(data string) (int, error) { return w.Write([]byte(data)) }

// CanvasGeneration is a durable idempotency barrier, not a second billing
// pipeline. A lost response never causes the same request to run upstream twice.
func CanvasGeneration() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Canvas-Request-ID")
		if !model.ValidCanvasID(requestID) {
			canvasError(c, model.ErrCanvasInvalid)
			c.Abort()
			return
		}
		if err := stripPlaygroundInternalFields(c); err != nil {
			canvasError(c, model.ErrCanvasInvalid)
			c.Abort()
			return
		}
		c.Request.Header.Del("X-Canvas-Request-ID")
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			canvasError(c, err)
			c.Abort()
			return
		}
		hash := sha256.New()
		_, _ = fmt.Fprintf(hash, "%s\n%s\n", c.Request.URL.Path, c.GetString("canvas_group"))
		if _, err = io.Copy(hash, storage); err != nil {
			canvasError(c, err)
			c.Abort()
			return
		}
		_, _ = storage.Seek(0, io.SeekStart)
		c.Request.Body = io.NopCloser(storage)
		run := model.CanvasGenerationRun{UserID: c.GetInt("id"), RunID: common.GetUUID(), ClientRequestID: requestID, RequestHash: fmt.Sprintf("%x", hash.Sum(nil)), Operation: c.Request.URL.Path, Model: c.GetString("canvas_model"), GroupName: c.GetString("canvas_group"), Status: "running", ArchiveStatus: "pending"}
		result := model.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&run)
		if result.Error != nil {
			canvasError(c, result.Error)
			c.Abort()
			return
		}
		if result.RowsAffected == 0 {
			var existing model.CanvasGenerationRun
			if err := model.DB.Where("user_id = ? AND client_request_id = ?", run.UserID, requestID).First(&existing).Error; err != nil {
				canvasError(c, err)
			} else {
				c.Header("X-Canvas-Run-ID", existing.RunID)
				common.ApiErrorStatus(c, http.StatusConflict, common.NewMessage("Canvas request already exists"), gin.H{"data": existing})
			}
			c.Abort()
			return
		}
		key := fmt.Sprintf("user/%d/runs/%s", run.UserID, run.RunID)
		path, err := service.CanvasStoragePath(key)
		if err == nil {
			err = os.MkdirAll(filepath.Dir(path), 0750)
		}
		var file *os.File
		if err == nil {
			file, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0640)
		}
		if err != nil {
			_ = model.DB.Model(&run).Updates(map[string]any{"status": "failed", "archive_status": "failed"}).Error
			canvasError(c, err)
			c.Abort()
			return
		}
		defer file.Close()
		ctx, cancel := context.WithCancel(c.Request.Context())
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		canvasRunCancels.Store(run.RunID, cancel)
		defer canvasRunCancels.Delete(run.RunID)
		c.Set("canvas_run_id", run.RunID)
		c.Header("X-Canvas-Run-ID", run.RunID)
		wrapped := &canvasRunWriter{ResponseWriter: c.Writer, file: file, limit: int64(max(1, constant.MaxFileDownloadMB)) << 20}
		c.Writer = wrapped
		c.Next()
		status := "succeeded"
		if wrapped.Status() >= 400 {
			status = "failed"
		} else if wrapped.err == nil {
			status = canvasObservedResult(file, wrapped.Header().Get("Content-Type"))
		}
		if ctx.Err() != nil {
			status = "interrupted"
		}
		archive := "saved"
		if ctx.Err() != nil || wrapped.err != nil || file.Sync() != nil {
			archive = "failed"
		}
		updates := map[string]any{"status": status, "http_status": wrapped.Status(), "result_mime": wrapped.Header().Get("Content-Type"), "archive_status": archive, "result_path": key, "updated_at": time.Now().UnixMilli()}
		if err := model.DB.Model(&model.CanvasGenerationRun{}).Where("user_id = ? AND run_id = ?", run.UserID, run.RunID).Updates(updates).Error; err != nil {
			common.SysError("canvas run persistence failed: " + err.Error())
		}
	}
}

func attachCanvasTask(c *gin.Context, task *model.Task) {
	if c.GetString(common.CanvasOriginContextKey) != common.CanvasOrigin || task == nil {
		return
	}
	runID := c.GetString("canvas_run_id")
	if runID == "" {
		return
	}
	if err := model.DB.Model(&model.CanvasGenerationRun{}).Where("user_id = ? AND run_id = ?", task.UserId, runID).Update("task_id", task.TaskID).Error; err != nil {
		common.SysError("canvas task link failed: " + err.Error())
	}
}
