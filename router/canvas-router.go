package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"strings"
)

func SetCanvasRouter(router *gin.Engine) {
	canvas := router.Group("/api/canvas", middleware.RouteTag("canvas"), middleware.GlobalAPIRateLimit(), middleware.UserAuth(), middleware.CanvasSession())
	canvas.GET("/bootstrap", controller.CanvasBootstrap)
	canvas.GET("/models", controller.CanvasBootstrap)
	canvas.GET("/groups", controller.GetUserGroups)
	canvas.GET("/projects", controller.ListCanvasProjects)
	canvas.POST("/projects", controller.PutCanvasProject)
	canvas.GET("/projects/:project_id", controller.GetCanvasProject)
	canvas.PATCH("/projects/:project_id", controller.PutCanvasProject)
	canvas.DELETE("/projects/:project_id", controller.DeleteCanvasProject)
	canvas.POST("/projects/:project_id/restore", controller.RestoreCanvasProject)
	canvas.GET("/state/:namespace", controller.CanvasSettings)
	canvas.GET("/state/:namespace/:entry_key", controller.CanvasSettings)
	canvas.PUT("/state/:namespace/:entry_key", controller.CanvasSettings)
	canvas.DELETE("/state/:namespace/:entry_key", controller.CanvasSettings)
	canvas.GET("/assets", controller.ListCanvasAssets)
	canvas.POST("/assets/import", controller.ImportCanvasGeneratedAsset)
	canvas.PUT("/assets/:asset_id/content", controller.UploadCanvasAsset)
	canvas.GET("/assets/:asset_id", controller.CanvasAsset)
	canvas.DELETE("/assets/:asset_id", controller.CanvasAsset)
	canvas.GET("/assets/:asset_id/content", controller.CanvasAsset)
	canvas.HEAD("/assets/:asset_id/content", controller.CanvasAsset)
	canvas.GET("/runs", controller.CanvasRuns)
	canvas.GET("/runs/:run_id", controller.CanvasRun)
	canvas.GET("/runs/:run_id/result", controller.CanvasRun)
	canvas.POST("/runs/:run_id/cancel", controller.CanvasRun)

	for _, entry := range []struct {
		path, capability string
		format           types.RelayFormat
	}{
		{"/v1/responses", "text", types.RelayFormatOpenAIResponses},
		{"/v1/images/generations", "image", types.RelayFormatOpenAIImage},
		{"/v1/images/edits", "image", types.RelayFormatOpenAIImage},
		{"/v1/audio/speech", "audio", types.RelayFormatOpenAIAudio},
	} {
		format := entry.format
		canvas.POST(entry.path, middleware.SystemPerformanceCheck(), middleware.CanvasRelay(entry.capability), controller.CanvasGeneration(), middleware.ModelRequestRateLimit(), middleware.PinTaskPluginEndpoint(), middleware.PrepareTaskPluginEndpoint(), middleware.Distribute(), func(c *gin.Context) {
			controller.RelayTaskPluginEndpoint(c, func(c *gin.Context) { controller.Relay(c, format) })
		})
	}
	canvas.POST("/v1/videos", middleware.SystemPerformanceCheck(), middleware.CanvasRelay("video"), controller.CanvasGeneration(), middleware.ModelRequestRateLimit(), middleware.PinTaskPluginEndpoint(), middleware.PrepareTaskPluginEndpoint(), middleware.Distribute(), func(c *gin.Context) { controller.RelayTaskPluginEndpoint(c, controller.RelayTask) })
	canvas.GET("/v1/videos/:task_id", middleware.CanvasRelay(""), middleware.Distribute(), controller.RelayTaskFetch)
	canvas.GET("/v1/videos/:task_id/content", middleware.CanvasRelay(""), controller.VideoProxy)
	canvas.HEAD("/v1/videos/:task_id/content", middleware.CanvasRelay(""), controller.VideoProxy)
	canvas.GET("/v1/responses/:response_id", middleware.CanvasRelay(""), controller.RetrieveTaskPluginResponse)
	canvas.GET("/v1/tasks/:key/artifacts/:artifact_key/content", controller.TaskArtifactContent)
	canvas.GET("/v1/tasks/:key/artifacts", controller.GetTaskArtifacts)
	canvas.HEAD("/v1/tasks/:key/artifacts/:artifact_key/content", controller.TaskArtifactContent)
	for _, route := range router.Routes() {
		if strings.HasPrefix(route.Path, "/api/canvas/") {
			middleware.DeclareSessionOnlyDashboardRoute(route.Method, route.Path)
		}
	}
}
