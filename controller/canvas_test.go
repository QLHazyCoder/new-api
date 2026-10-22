package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func canvasControllerFixture(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := setupPlaygroundImageControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.CanvasProject{}, &model.CanvasAsset{}, &model.CanvasProjectAssetRef{}, &model.CanvasGenerationRun{}, &model.CanvasUserSetting{}, &model.Task{}))
	t.Setenv("CANVAS_ASSET_STORAGE_PATH", t.TempDir())
	engine := gin.New()
	engine.Use(middleware.BodyStorageCleanup(), func(c *gin.Context) {
		userID := 1
		if c.GetHeader("X-Test-User") == "2" {
			userID = 2
		}
		c.Set("id", userID)
		c.Set("role", common.RoleAdminUser)
		c.Set(common.CanvasOriginContextKey, common.CanvasOrigin)
		c.Next()
	})
	engine.GET("/api/canvas/projects", ListCanvasProjects)
	engine.POST("/api/canvas/projects", PutCanvasProject)
	engine.GET("/api/canvas/projects/:project_id", GetCanvasProject)
	engine.PATCH("/api/canvas/projects/:project_id", PutCanvasProject)
	engine.DELETE("/api/canvas/projects/:project_id", DeleteCanvasProject)
	engine.POST("/api/canvas/projects/:project_id/restore", RestoreCanvasProject)
	engine.GET("/api/canvas/state/:namespace", CanvasSettings)
	engine.GET("/api/canvas/state/:namespace/:entry_key", CanvasSettings)
	engine.PUT("/api/canvas/state/:namespace/:entry_key", CanvasSettings)
	engine.DELETE("/api/canvas/state/:namespace/:entry_key", CanvasSettings)
	engine.GET("/api/canvas/assets", ListCanvasAssets)
	engine.PUT("/api/canvas/assets/:asset_id/content", UploadCanvasAsset)
	engine.GET("/api/canvas/assets/:asset_id/content", CanvasAsset)
	engine.DELETE("/api/canvas/assets/:asset_id", CanvasAsset)
	engine.GET("/api/canvas/runs", CanvasRuns)
	engine.GET("/api/canvas/runs/:run_id", CanvasRun)
	engine.GET("/api/canvas/runs/:run_id/result", CanvasRun)
	return engine
}

func TestCanvasProjectsAssetsAndConflicts(t *testing.T) {
	engine := canvasControllerFixture(t)
	perform := func(method, path, body, contentType, user string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", contentType)
		request.Header.Set("X-Test-User", user)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		return response
	}
	png := "\x89PNG\r\n\x1a\nimage-bytes"
	response := perform(http.MethodPut, "/api/canvas/assets/image:one/content", png, "image/png", "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	doc := `{"project_id":"project","revision":0,"document":{"id":"project","title":"中文🙂","nodes":[{"id":"node","type":"image","position":{"x":0,"y":0},"width":200,"height":100,"storageKey":"image:one","content":"blob:temporary"}],"connections":[],"chatSessions":[],"backgroundMode":"lines","viewport":{"x":0,"y":0,"k":1}}}`
	response = perform(http.MethodPost, "/api/canvas/projects", doc, "application/json", "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	response = perform(http.MethodPost, "/api/canvas/projects", doc, "application/json", "")
	assert.Equal(t, http.StatusConflict, response.Code)
	response = perform(http.MethodGet, "/api/canvas/projects/project", "", "", "2")
	assert.Equal(t, http.StatusNotFound, response.Code)
	response = perform(http.MethodDelete, "/api/canvas/assets/image:one", "", "", "")
	assert.Equal(t, http.StatusConflict, response.Code, "referenced media is retained")
	response = perform(http.MethodGet, "/api/canvas/assets/image:one/content", "", "", "2")
	assert.Equal(t, http.StatusNotFound, response.Code)
	response = perform(http.MethodGet, "/api/canvas/assets/image:one/content", "", "", "")
	assert.Equal(t, []byte(png), response.Body.Bytes())
	assert.Equal(t, "nosniff", response.Header().Get("X-Content-Type-Options"))
	response = perform(http.MethodDelete, "/api/canvas/projects/project?revision=1", "", "", "")
	assert.Equal(t, http.StatusOK, response.Code)
	response = perform(http.MethodPost, "/api/canvas/projects/project/restore", "", "", "")
	assert.Equal(t, http.StatusOK, response.Code)
	response = perform(http.MethodGet, "/api/canvas/projects/project", "", "", "")
	assert.Contains(t, response.Body.String(), "中文🙂")
	assert.NotContains(t, response.Body.String(), "blob:temporary")
	response = perform(http.MethodPut, "/api/canvas/state/app_state/config", `{"revision":0,"value":"{\"apiKey\":\"secret-canary\"}"}`, "application/json", "")
	assert.Equal(t, http.StatusUnprocessableEntity, response.Code)
}

func TestCanvasGenerationIdempotencyAndArchive(t *testing.T) {
	engine := canvasControllerFixture(t)
	submissions := 0
	engine.POST("/api/canvas/v1/responses", CanvasGeneration(), func(c *gin.Context) {
		submissions++
		storage, err := common.GetBodyStorage(c)
		require.NoError(t, err)
		body, err := storage.Bytes()
		require.NoError(t, err)
		assert.NotContains(t, string(body), "group")
		assert.Empty(t, c.GetHeader("X-Canvas-Request-ID"))
		c.Header("Content-Type", "text/event-stream")
		_, err = c.Writer.WriteString("data: {\"text\":\"中文🙂\"}\n\n")
		require.NoError(t, err)
		c.Writer.Flush()
		_, err = c.Writer.WriteString("data: [DONE]\n\n")
		require.NoError(t, err)
	})
	perform := func(body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/responses", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Canvas-Request-ID", "unique-request")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		return response
	}
	response := perform(`{"model":"example","input":"中文","group":"internal-routing-only"}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	runID := response.Header().Get("X-Canvas-Run-ID")
	require.NotEmpty(t, runID)
	second := perform(`{"model":"example","input":"changed"}`)
	assert.Equal(t, http.StatusConflict, second.Code)
	assert.Equal(t, 1, submissions, "retry cannot generate/charge again")
	var run model.CanvasGenerationRun
	require.NoError(t, model.DB.Where("user_id = ? AND run_id = ?", 1, runID).First(&run).Error)
	assert.Equal(t, "succeeded", run.Status)
	assert.Equal(t, "saved", run.ArchiveStatus)
	path, err := service.CanvasStoragePath(run.ResultPath)
	require.NoError(t, err)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, response.Body.Bytes(), raw, "SSE UTF-8 bytes are archived intact")
}

// This harness is opt-in and serves only a temporary test database. It never
// starts the production service or touches live users/channels.
func TestCanvasBrowserHarness(t *testing.T) {
	if os.Getenv("CANVAS_BROWSER_TEST") != "1" {
		t.Skip("browser harness not requested")
	}
	engine := canvasControllerFixture(t)
	build, err := filepath.Abs("../web/dist/canvas-app")
	require.NoError(t, err)
	testBridge, err := os.ReadFile(os.Getenv("CANVAS_BROWSER_BRIDGE_FILE"))
	require.NoError(t, err, "bundle integrations/infinite-canvas/browser-host.ts for the fixture")
	engine.GET("/api/canvas/bootstrap", func(c *gin.Context) {
		common.ApiSuccess(c, gin.H{"user_id": 1, "groups": gin.H{"default": "Default"}, "models": []gin.H{{"name": "image-test", "group": "default", "capability": "image"}, {"name": "text-test", "group": "default", "capability": "text"}, {"name": "video-test", "group": "default", "capability": "video"}, {"name": "audio-test", "group": "default", "capability": "audio"}}})
	})
	engine.GET("/api/canvas/test/project-count", func(c *gin.Context) {
		var count int64
		_ = model.DB.Model(&model.CanvasProject{}).Count(&count).Error
		common.ApiSuccess(c, count)
	})
	engine.POST("/api/canvas/v1/images/generations", CanvasGeneration(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"created": 1, "data": []gin.H{{"b64_json": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="}}})
	})
	engine.Static("/canvas-app", build)
	engine.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
	})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/canvas-test-host.js" {
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			_, _ = w.Write(testBridge)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/canvas-app/") {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' blob: data:; media-src blob: data:; font-src 'self' data:; connect-src blob: data:; frame-ancestors 'self'; base-uri 'none'; form-action 'none'")
		}
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, `<!doctype html><html><head><meta charset="UTF-8"></head><body><script type="module" src="/canvas-test-host.js"></script></body></html>`)
			return
		}
		engine.ServeHTTP(w, r)
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	t.Log("CANVAS_BROWSER_URL=" + server.URL)
	marker := os.Getenv("CANVAS_BROWSER_READY_FILE")
	if marker != "" {
		require.NoError(t, os.WriteFile(marker, []byte(server.URL), 0600))
	}
	stop := os.Getenv("CANVAS_BROWSER_STOP_FILE")
	deadline := time.NewTimer(10 * time.Minute)
	defer deadline.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("browser harness timed out")
		case <-time.After(250 * time.Millisecond):
		}
		if stop != "" {
			if _, err := os.Stat(stop); err == nil {
				return
			}
		}
	}
}
