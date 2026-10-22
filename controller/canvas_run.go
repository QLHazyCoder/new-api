package controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"
)

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
