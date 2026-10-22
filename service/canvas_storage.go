package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
)

var ErrCanvasAssetTooLarge = errors.New("Canvas file is too large")

func DownloadCanvasGeneratedImage(ctx context.Context, source string) ([]byte, string, error) {
	return downloadPlaygroundImage(ctx, source)
}

func CanvasStorageRoot() string {
	if root := strings.TrimSpace(os.Getenv("CANVAS_ASSET_STORAGE_PATH")); root != "" {
		return root
	}
	return "/data/canvas-assets"
}

func CanvasStoragePath(key string) (string, error) {
	if key == "" || filepath.IsAbs(key) || strings.Contains(key, "\\") {
		return "", model.ErrCanvasInvalid
	}
	clean := filepath.Clean(key)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", model.ErrCanvasInvalid
	}
	return filepath.Join(CanvasStorageRoot(), clean), nil
}

// WriteCanvasFile atomically persists bounded data. Only server-generated keys
// reach this function; user titles and asset IDs never become filesystem paths.
func WriteCanvasFile(key string, reader io.Reader, limit int64) (size int64, digest string, err error) {
	path, err := CanvasStoragePath(key)
	if err != nil {
		return 0, "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return 0, "", err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".canvas-*")
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = file.Close(); _ = os.Remove(file.Name()) }()
	hash := sha256.New()
	size, err = io.Copy(io.MultiWriter(file, hash), io.LimitReader(reader, limit+1))
	if err != nil {
		return 0, "", err
	}
	if size > limit {
		return 0, "", ErrCanvasAssetTooLarge
	}
	if err := file.Chmod(0640); err != nil {
		return 0, "", err
	}
	if err := file.Sync(); err != nil {
		return 0, "", err
	}
	if err := file.Close(); err != nil {
		return 0, "", err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return 0, "", err
	}
	return size, hex.EncodeToString(hash.Sum(nil)), nil
}

func SaveCanvasAsset(userID int, assetID, title, declaredMIME, runID string, reader io.Reader) (*model.CanvasAsset, error) {
	if userID <= 0 || !model.ValidCanvasID(assetID) {
		return nil, model.ErrCanvasInvalid
	}
	mimeType, _, err := mime.ParseMediaType(declaredMIME)
	if err != nil {
		return nil, model.ErrCanvasInvalid
	}
	kind, _, ok := strings.Cut(mimeType, "/")
	if !ok || (kind != "image" && kind != "video" && kind != "audio" && mimeType != "text/plain") || mimeType == "image/svg+xml" {
		return nil, model.ErrCanvasInvalid
	}
	// Keep uploads compatible with the gateway's existing media download limit.
	limit := int64(max(1, constant.MaxFileDownloadMB)) << 20
	if kind == "image" {
		limit = 50 << 20
	}
	key := fmt.Sprintf("user/%d/assets/%s", userID, common.GetUUID())
	size, hash, err := WriteCanvasFile(key, reader, limit)
	if err != nil {
		return nil, err
	}
	path, _ := CanvasStoragePath(key)
	record := &model.CanvasAsset{UserID: userID, AssetID: assetID, Title: title, Kind: kind, StorageKey: key, MIME: mimeType, Size: size, SHA256: hash, SourceRunID: runID}
	if err := model.DB.Create(record).Error; err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return record, nil
}

func ServeCanvasFile(w http.ResponseWriter, request *http.Request, key, mimeType, etag string) error {
	path, err := CanvasStoragePath(key)
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "inline")
	if etag != "" {
		w.Header().Set("ETag", `"`+etag+`"`)
	}
	http.ServeContent(w, request, "canvas-media", info.ModTime(), file)
	return nil
}

// Soft-deleted projects retain their refs throughout the trash period. Files
// are removed before metadata, making a crash retryable and idempotent.
func CleanupCanvasOnce() error {
	if constant.TaskTimeoutMinutes > 0 {
		cutoff := time.Now().Add(-time.Duration(constant.TaskTimeoutMinutes) * time.Minute).UnixMilli()
		if err := model.DB.Model(&model.CanvasGenerationRun{}).Where("status = ? AND updated_at < ?", "running", cutoff).Updates(map[string]any{"status": "interrupted", "archive_status": "failed"}).Error; err != nil {
			return err
		}
	}
	cutoff := time.Now().Add(-30 * 24 * time.Hour).UnixMilli()
	var projects []model.CanvasProject
	if err := model.DB.Where("deleted_at > 0 AND deleted_at < ?", cutoff).Limit(100).Find(&projects).Error; err != nil {
		return err
	}
	for _, project := range projects {
		if err := model.PurgeCanvasProject(project, cutoff); err != nil {
			return err
		}
	}
	var assets []model.CanvasAsset
	unused, err := model.ListUnreferencedCanvasAssets(cutoff)
	if err != nil {
		return err
	}
	for _, asset := range unused {
		err := model.DeleteCanvasAsset(asset.UserID, asset.AssetID)
		if err != nil && !errors.Is(err, model.ErrCanvasConflict) {
			return err
		}
	}
	if err := model.DB.Where("deleted_at > 0 AND deleted_at < ?", cutoff).Limit(100).Find(&assets).Error; err != nil {
		return err
	}
	for _, asset := range assets {
		var count int64
		if err := model.DB.Model(&model.CanvasProjectAssetRef{}).Where("user_id = ? AND asset_id = ?", asset.UserID, asset.AssetID).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			continue
		}
		path, err := CanvasStoragePath(asset.StorageKey)
		if err != nil {
			return err
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := model.DB.Where("id = ? AND deleted_at > 0", asset.ID).Delete(&model.CanvasAsset{}).Error; err != nil {
			return err
		}
	}
	var runs []model.CanvasGenerationRun
	if err := model.DB.Where("updated_at < ? AND status NOT IN ?", time.Now().Add(-180*24*time.Hour).UnixMilli(), []string{"queued", "running"}).Limit(100).Find(&runs).Error; err != nil {
		return err
	}
	for _, run := range runs {
		if run.ResultPath != "" {
			path, err := CanvasStoragePath(run.ResultPath)
			if err != nil {
				return err
			}
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		if err := model.DB.Delete(&run).Error; err != nil {
			return err
		}
	}
	return nil
}

func StartCanvasCleanup() {
	if !common.IsMasterNode {
		return
	}
	go func() {
		for {
			if err := CleanupCanvasOnce(); err != nil {
				common.SysError("canvas cleanup: " + err.Error())
			}
			time.Sleep(time.Hour)
		}
	}()
}
