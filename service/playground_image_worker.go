package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
)

const (
	playgroundImageStorageEnv       = "PLAYGROUND_IMAGE_STORAGE_PATH"
	playgroundImageDefaultRoot      = "/data/playground-images"
	playgroundImageMaxResultBytes   = int64(50 * 1024 * 1024)
	playgroundImageMaxReferenceSize = int64(20 * 1024 * 1024)
	playgroundImageLeaseDuration    = 45 * time.Second
	playgroundImageHeartbeatPeriod  = 10 * time.Second
	playgroundImagePollPeriod       = 500 * time.Millisecond
	playgroundImageCleanupPeriod    = time.Minute
	playgroundImageClaimLimit       = 500
)

type PlaygroundImageReferenceFile struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size"`
}

type PlaygroundImageEditPayload struct {
	Fields map[string][]string `json:"fields"`
}

type PlaygroundImageExecutionResult struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

type PlaygroundImageExecutor func(context.Context, *model.PlaygroundImageTask, *model.PlaygroundImageBatch) (*PlaygroundImageExecutionResult, error)

var ExecutePlaygroundImageTask PlaygroundImageExecutor

var playgroundImageURLPattern = regexp.MustCompile(`https?://[^\s"'<>]+`)

type playgroundImageRelayResponse struct {
	Data []struct {
		URL           string `json:"url"`
		B64JSON       string `json:"b64_json"`
		RevisedPrompt string `json:"revised_prompt"`
		MimeType      string `json:"mime_type"`
	} `json:"data"`
}

type playgroundImageRelayError struct {
	Error struct {
		Message string `json:"message"`
		Code    any    `json:"code"`
	} `json:"error"`
	Message string `json:"message"`
}

func GetPlaygroundImageStorageRoot() string {
	if value := strings.TrimSpace(os.Getenv(playgroundImageStorageEnv)); value != "" {
		return value
	}
	return playgroundImageDefaultRoot
}

func resolvePlaygroundImagePath(relativePath string) (string, error) {
	if relativePath == "" || filepath.IsAbs(relativePath) {
		return "", errors.New("invalid playground image path")
	}
	root, err := filepath.Abs(GetPlaygroundImageStorageRoot())
	if err != nil {
		return "", err
	}
	resolved, err := filepath.Abs(filepath.Join(root, filepath.Clean(relativePath)))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("playground image path escapes storage root")
	}
	return resolved, nil
}

func OpenPlaygroundImageResult(relativePath string) (*os.File, error) {
	path, err := resolvePlaygroundImagePath(relativePath)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

func OpenPlaygroundImageStoredFile(relativePath string) (*os.File, error) {
	return OpenPlaygroundImageResult(relativePath)
}

func RemovePlaygroundImageResult(relativePath string) error {
	if relativePath == "" {
		return nil
	}
	path, err := resolvePlaygroundImagePath(relativePath)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func allowedPlaygroundImageMimeType(value string) bool {
	value = strings.ToLower(strings.TrimSpace(strings.Split(value, ";")[0]))
	switch value {
	case "image/png", "image/jpeg", "image/jpg", "image/webp", "image/gif":
		return true
	default:
		return false
	}
}

func playgroundImageExtension(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0])) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".png"
	}
}

func validatePlaygroundImageBytes(data []byte, declaredMimeType string) (string, error) {
	if len(data) == 0 {
		return "", errors.New("image data is empty")
	}
	detected := http.DetectContentType(data)
	if !allowedPlaygroundImageMimeType(detected) {
		return "", fmt.Errorf("unsupported image content type: %s", detected)
	}
	normalizedDeclared := strings.ToLower(strings.TrimSpace(strings.Split(declaredMimeType, ";")[0]))
	if normalizedDeclared != "" && normalizedDeclared != "application/octet-stream" && !allowedPlaygroundImageMimeType(normalizedDeclared) {
		return "", fmt.Errorf("unsupported declared image content type: %s", declaredMimeType)
	}
	return strings.ToLower(strings.TrimSpace(strings.Split(detected, ";")[0])), nil
}

func writePlaygroundImageFile(relativePath string, data []byte) error {
	path, err := resolvePlaygroundImagePath(relativePath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".playground-image-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o640); err != nil {
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func SavePlaygroundImageReference(batchID string, index int, header *multipart.FileHeader) (PlaygroundImageReferenceFile, error) {
	if header == nil || header.Size <= 0 || header.Size > playgroundImageMaxReferenceSize {
		return PlaygroundImageReferenceFile{}, errors.New("reference image must be between 1 byte and 20 MB")
	}
	file, err := header.Open()
	if err != nil {
		return PlaygroundImageReferenceFile{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, playgroundImageMaxReferenceSize+1))
	if err != nil {
		return PlaygroundImageReferenceFile{}, err
	}
	if int64(len(data)) > playgroundImageMaxReferenceSize {
		return PlaygroundImageReferenceFile{}, errors.New("reference image exceeds 20 MB")
	}
	mimeType, err := validatePlaygroundImageBytes(data, header.Header.Get("Content-Type"))
	if err != nil {
		return PlaygroundImageReferenceFile{}, err
	}
	relativePath := filepath.Join("references", batchID, fmt.Sprintf("%d%s", index, playgroundImageExtension(mimeType)))
	if err := writePlaygroundImageFile(relativePath, data); err != nil {
		return PlaygroundImageReferenceFile{}, err
	}
	return PlaygroundImageReferenceFile{
		Path:     relativePath,
		Name:     filepath.Base(header.Filename),
		MimeType: mimeType,
		Size:     int64(len(data)),
	}, nil
}

func RemovePlaygroundImageReferencesJSON(referenceJSON string) error {
	if referenceJSON == "" {
		return nil
	}
	var references []PlaygroundImageReferenceFile
	if err := common.Unmarshal([]byte(referenceJSON), &references); err != nil {
		return err
	}
	var firstErr error
	directories := make(map[string]struct{})
	for _, reference := range references {
		path, err := resolvePlaygroundImagePath(reference.Path)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		directories[filepath.Dir(path)] = struct{}{}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) && firstErr == nil {
			firstErr = err
		}
	}
	for directory := range directories {
		_ = os.Remove(directory)
	}
	return firstErr
}

func parsePlaygroundImageRelayError(result *PlaygroundImageExecutionResult, fallback error) (string, string) {
	if result != nil && len(result.Body) > 0 {
		var relayError playgroundImageRelayError
		if err := common.Unmarshal(result.Body, &relayError); err == nil {
			message := relayError.Error.Message
			if message == "" {
				message = relayError.Message
			}
			if message != "" {
				code := ""
				if relayError.Error.Code != nil {
					code = fmt.Sprint(relayError.Error.Code)
				}
				return sanitizePlaygroundImageError(message), sanitizePlaygroundImageError(code)
			}
		}
	}
	if fallback != nil {
		return sanitizePlaygroundImageError(fallback.Error()), "worker_error"
	}
	if result != nil {
		return fmt.Sprintf("Image generation failed with status %d", result.StatusCode), "upstream_error"
	}
	return "Image generation failed", "worker_error"
}

func sanitizePlaygroundImageError(message string) string {
	return playgroundImageURLPattern.ReplaceAllString(message, "[upstream URL redacted]")
}

func decodePlaygroundBase64Image(value string) ([]byte, string, error) {
	value = strings.TrimSpace(value)
	declaredMimeType := ""
	if strings.HasPrefix(value, "data:") {
		comma := strings.IndexByte(value, ',')
		if comma < 0 {
			return nil, "", errors.New("invalid image data URL")
		}
		metadata := strings.TrimPrefix(value[:comma], "data:")
		declaredMimeType = strings.Split(metadata, ";")[0]
		if !strings.Contains(strings.ToLower(metadata), ";base64") {
			return nil, "", errors.New("image data URL is not base64 encoded")
		}
		value = value[comma+1:]
	}
	if int64(base64.StdEncoding.DecodedLen(len(value))) > playgroundImageMaxResultBytes {
		return nil, "", errors.New("generated image exceeds 50 MB")
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(value)
	}
	if err != nil {
		return nil, "", fmt.Errorf("invalid base64 image: %w", err)
	}
	if int64(len(data)) > playgroundImageMaxResultBytes {
		return nil, "", errors.New("generated image exceeds 50 MB")
	}
	return data, declaredMimeType, nil
}

func downloadPlaygroundImage(ctx context.Context, imageURL string) ([]byte, string, error) {
	parsed, err := url.Parse(imageURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, "", errors.New("generated image URL is invalid")
	}
	downloadContext, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(downloadContext, http.MethodGet, imageURL, nil)
	if err != nil {
		common.SysError(fmt.Sprintf("create playground image download request: %v", err))
		return nil, "", errors.New("failed to download generated image")
	}
	req.Header.Set("Accept", "image/*")
	client := GetSSRFProtectedHTTPClient()
	if client == nil {
		common.SysError("download generated playground image: HTTP client is not initialized")
		return nil, "", errors.New("failed to download generated image")
	}
	resp, err := client.Do(req)
	if err != nil {
		common.SysError(fmt.Sprintf("download generated playground image: %v", err))
		return nil, "", errors.New("failed to download generated image")
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, "", fmt.Errorf("generated image download returned status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, playgroundImageMaxResultBytes+1))
	if err != nil {
		common.SysError(fmt.Sprintf("read generated playground image: %v", err))
		return nil, "", errors.New("failed to download generated image")
	}
	if int64(len(data)) > playgroundImageMaxResultBytes {
		return nil, "", errors.New("generated image exceeds 50 MB")
	}
	return data, resp.Header.Get("Content-Type"), nil
}

func savePlaygroundImageExecutionResult(ctx context.Context, task *model.PlaygroundImageTask, result *PlaygroundImageExecutionResult) (string, string, int64, error) {
	if result == nil || result.StatusCode < http.StatusOK || result.StatusCode >= http.StatusMultipleChoices {
		return "", "", 0, errors.New("image relay did not return a successful response")
	}
	var relayResponse playgroundImageRelayResponse
	if err := common.Unmarshal(result.Body, &relayResponse); err != nil {
		return "", "", 0, fmt.Errorf("invalid image relay response: %w", err)
	}
	if len(relayResponse.Data) == 0 {
		return "", "", 0, errors.New("image relay response contains no image")
	}
	image := relayResponse.Data[0]
	var data []byte
	declaredMimeType := image.MimeType
	var err error
	if image.B64JSON != "" {
		var dataURLMimeType string
		data, dataURLMimeType, err = decodePlaygroundBase64Image(image.B64JSON)
		if declaredMimeType == "" {
			declaredMimeType = dataURLMimeType
		}
	} else if image.URL != "" {
		if strings.HasPrefix(strings.TrimSpace(image.URL), "data:") {
			data, declaredMimeType, err = decodePlaygroundBase64Image(image.URL)
		} else {
			data, declaredMimeType, err = downloadPlaygroundImage(ctx, image.URL)
		}
	} else {
		err = errors.New("image relay response contains neither url nor b64_json")
	}
	if err != nil {
		return "", "", 0, err
	}
	mimeType, err := validatePlaygroundImageBytes(data, declaredMimeType)
	if err != nil {
		return "", "", 0, err
	}
	createdAt := time.Unix(task.CreatedAt, 0).UTC()
	relativePath := filepath.Join(
		"results",
		fmt.Sprintf("%d", task.UserID),
		createdAt.Format("2006/01/02"),
		task.TaskID+playgroundImageExtension(mimeType),
	)
	if err := writePlaygroundImageFile(relativePath, data); err != nil {
		return "", "", 0, err
	}
	return relativePath, mimeType, int64(len(data)), nil
}

func cleanupPlaygroundImageBatchReferences(batchRecordID int64) {
	referenceJSON, err := model.GetPlaygroundImageBatchReferencesIfTerminal(batchRecordID)
	if err != nil {
		common.SysError(fmt.Sprintf("release playground image references: %v", err))
		return
	}
	if referenceJSON == "" {
		return
	}
	if err := RemovePlaygroundImageReferencesJSON(referenceJSON); err != nil {
		common.SysError(fmt.Sprintf("remove playground image references: %v", err))
		return
	}
	if err := model.ClearPlaygroundImageBatchReferences(batchRecordID, referenceJSON); err != nil {
		common.SysError(fmt.Sprintf("clear playground image reference metadata: %v", err))
	}
}

func CleanupPlaygroundImageBatchReferences(batchRecordID int64) {
	cleanupPlaygroundImageBatchReferences(batchRecordID)
}

func executeClaimedPlaygroundImageTask(owner string, task model.PlaygroundImageTask) {
	stopHeartbeat := make(chan struct{})
	defer func() {
		close(stopHeartbeat)
		if recovered := recover(); recovered != nil {
			common.SysError(fmt.Sprintf("playground image task %s panicked: %v", task.TaskID, recovered))
			batchRecordID, err := model.InterruptPlaygroundImageTask(
				task.TaskID,
				owner,
				"Generation was interrupted by an internal worker error",
				"worker_panic",
				common.GetTimestamp(),
			)
			if err == nil {
				cleanupPlaygroundImageBatchReferences(batchRecordID)
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(playgroundImageHeartbeatPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				now := common.GetTimestamp()
				if err := model.HeartbeatPlaygroundImageTask(task.TaskID, owner, now+int64(playgroundImageLeaseDuration.Seconds()), now); err != nil {
					common.SysError(fmt.Sprintf("heartbeat playground image task %s: %v", task.TaskID, err))
				}
			case <-stopHeartbeat:
				return
			}
		}
	}()

	batch, err := model.GetPlaygroundImageBatchByRecordID(task.BatchRecordID)
	if err != nil {
		now := common.GetTimestamp()
		batchRecordID, failErr := model.FailPlaygroundImageTask(task.TaskID, owner, err.Error(), "batch_not_found", now)
		if failErr != nil {
			common.SysError(fmt.Sprintf("fail playground image task %s: %v", task.TaskID, failErr))
			return
		}
		cleanupPlaygroundImageBatchReferences(batchRecordID)
		return
	}
	if ExecutePlaygroundImageTask == nil {
		now := common.GetTimestamp()
		batchRecordID, failErr := model.FailPlaygroundImageTask(task.TaskID, owner, "Image task executor is not configured", "executor_unavailable", now)
		if failErr == nil {
			cleanupPlaygroundImageBatchReferences(batchRecordID)
		}
		return
	}

	now := common.GetTimestamp()
	if err := model.MarkPlaygroundImageTaskUpstreamStarted(task.TaskID, owner, now); err != nil {
		common.SysError(fmt.Sprintf("mark playground image task %s started: %v", task.TaskID, err))
		return
	}
	result, executionErr := ExecutePlaygroundImageTask(context.Background(), &task, batch)
	if executionErr != nil || result == nil || result.StatusCode < http.StatusOK || result.StatusCode >= http.StatusMultipleChoices {
		message, code := parsePlaygroundImageRelayError(result, executionErr)
		batchRecordID, failErr := model.FailPlaygroundImageTask(task.TaskID, owner, message, code, common.GetTimestamp())
		if failErr != nil {
			common.SysError(fmt.Sprintf("record playground image task %s failure: %v", task.TaskID, failErr))
			return
		}
		cleanupPlaygroundImageBatchReferences(batchRecordID)
		return
	}
	if err := model.MarkPlaygroundImageTaskSaving(task.TaskID, owner, common.GetTimestamp()); err != nil {
		common.SysError(fmt.Sprintf("mark playground image task %s saving: %v", task.TaskID, err))
		return
	}
	resultPath, mimeType, resultSize, saveErr := savePlaygroundImageExecutionResult(context.Background(), &task, result)
	if saveErr != nil {
		message, code := parsePlaygroundImageRelayError(nil, saveErr)
		batchRecordID, failErr := model.FailPlaygroundImageTask(task.TaskID, owner, message, code, common.GetTimestamp())
		if failErr != nil {
			common.SysError(fmt.Sprintf("record playground image save failure %s: %v", task.TaskID, failErr))
			return
		}
		cleanupPlaygroundImageBatchReferences(batchRecordID)
		return
	}
	discard, batchRecordID, err := model.CompletePlaygroundImageTask(task.TaskID, owner, resultPath, mimeType, resultSize, common.GetTimestamp())
	if err != nil {
		_ = RemovePlaygroundImageResult(resultPath)
		common.SysError(fmt.Sprintf("complete playground image task %s: %v", task.TaskID, err))
		return
	}
	if discard {
		if err := RemovePlaygroundImageResult(resultPath); err != nil {
			common.SysError(fmt.Sprintf("remove discarded playground image result: %v", err))
		} else if err := model.ClearPlaygroundImageTaskResult(task.TaskID, resultPath); err != nil {
			common.SysError(fmt.Sprintf("clear discarded playground image result metadata: %v", err))
		}
	}
	cleanupPlaygroundImageBatchReferences(batchRecordID)
}

func RunPlaygroundImageCleanupOnce() {
	batches, err := model.ListPlaygroundImageBatchesWithReferences(500)
	if err != nil {
		common.SysError(fmt.Sprintf("list playground image references: %v", err))
	} else {
		for _, batch := range batches {
			cleanupPlaygroundImageBatchReferences(batch.ID)
		}
	}
	now := common.GetTimestamp()
	tasks, err := model.ListExpiredPlaygroundImageTasks(now, 1000)
	if err != nil {
		common.SysError(fmt.Sprintf("cleanup expired playground image tasks: %v", err))
		return
	}
	for _, task := range tasks {
		if err := RemovePlaygroundImageResult(task.ResultPath); err != nil {
			common.SysError(fmt.Sprintf("remove expired playground image result: %v", err))
			continue
		}
		if err := model.DeleteExpiredPlaygroundImageTask(task.TaskID, now); err != nil {
			common.SysError(fmt.Sprintf("delete expired playground image task: %v", err))
		}
	}
	batches, err = model.ListPlaygroundImageBatchesWithReferences(500)
	if err == nil {
		for _, batch := range batches {
			cleanupPlaygroundImageBatchReferences(batch.ID)
		}
	}
	if err := model.DeleteExpiredPlaygroundImageBatches(now); err != nil {
		common.SysError(fmt.Sprintf("delete expired playground image batches: %v", err))
	}
}

func StartPlaygroundImageTaskRunner() {
	owner := fmt.Sprintf("%s-%s", common.NodeName, common.GetRandomString(12))
	go func() {
		pollTicker := time.NewTicker(playgroundImagePollPeriod)
		cleanupTicker := time.NewTicker(playgroundImageCleanupPeriod)
		defer pollTicker.Stop()
		defer cleanupTicker.Stop()
		RunPlaygroundImageCleanupOnce()
		for {
			select {
			case <-pollTicker.C:
				for {
					maxConcurrency := setting.GetPlaygroundImageMaxConcurrency()
					now := common.GetTimestamp()
					claimed, err := model.ClaimPlaygroundImageTasks(
						owner,
						maxConcurrency,
						playgroundImageClaimLimit,
						now,
						now+int64(playgroundImageLeaseDuration.Seconds()),
					)
					if err != nil {
						common.SysError(fmt.Sprintf("claim playground image tasks: %v", err))
						break
					}
					for _, task := range claimed {
						go executeClaimedPlaygroundImageTask(owner, task)
					}
					if maxConcurrency > 0 || len(claimed) < playgroundImageClaimLimit {
						break
					}
				}
			case <-cleanupTicker.C:
				RunPlaygroundImageCleanupOnce()
			}
		}
	}()
}
