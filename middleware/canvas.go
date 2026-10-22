package middleware

import (
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func CanvasSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := GetSessionAuthIdentity(c); !ok || c.GetBool("use_access_token") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "canvas_session_required", "message": "Session expired!"}})
			return
		}
		common.OptionMapRWMutex.RLock()
		raw := common.OptionMap["SidebarModulesAdmin"]
		common.OptionMapRWMutex.RUnlock()
		if raw != "" {
			var modules map[string]map[string]bool
			if common.UnmarshalJsonStr(raw, &modules) != nil {
				c.AbortWithStatus(http.StatusServiceUnavailable)
				return
			}
			if chat, ok := modules["chat"]; ok {
				enabled, hasEnabled := chat["enabled"]
				canvas, hasCanvas := chat["canvas"]
				if hasEnabled && !enabled || hasCanvas && !canvas {
					c.AbortWithStatus(http.StatusForbidden)
					return
				}
			}
		}
		c.Set(common.CanvasOriginContextKey, common.CanvasOrigin)
		c.Header("Cache-Control", "private, no-store")
		c.Next()
	}
}

// The canonical protocol path is created by the host after authentication.
// It is not a client-controlled forwarding target.
func CanvasRelay(capability string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString(common.CanvasOriginContextKey) != common.CanvasOrigin {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		group := c.GetHeader("X-Canvas-Group")
		userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
		if group == "" {
			group = userGroup
		}
		if !service.GroupInUserUsableGroups(userGroup, group) {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		if c.Request.Method == http.MethodPost {
			var modelName string
			if strings.HasPrefix(c.ContentType(), "multipart/form-data") {
				storage, err := common.GetBodyStorage(c)
				if err != nil {
					c.AbortWithStatus(http.StatusBadRequest)
					return
				}
				if _, err = c.MultipartForm(); err != nil {
					c.AbortWithStatus(http.StatusBadRequest)
					return
				}
				modelName = c.PostForm("model")
				_, _ = storage.Seek(0, io.SeekStart)
				c.Request.Body = io.NopCloser(storage)
			} else {
				var request struct {
					Model string `json:"model"`
				}
				if err := common.UnmarshalBodyReusable(c, &request); err != nil {
					c.AbortWithStatus(http.StatusBadRequest)
					return
				}
				modelName = request.Model
			}
			allowed := false
			catalog, err := service.CanvasModelCatalog(userGroup)
			if err != nil {
				c.AbortWithStatus(http.StatusServiceUnavailable)
				return
			}
			for _, option := range catalog {
				if option.Group == group && option.Name == modelName && option.Capability == capability {
					allowed = true
					break
				}
			}
			if !allowed {
				c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{"error": gin.H{"code": "canvas_model_unavailable", "message": "Model not found"}})
				return
			}
			c.Set("canvas_model", modelName)
		}
		common.SetContextKey(c, constant.ContextKeyUsingGroup, group)
		common.SetContextKey(c, constant.ContextKeyTokenGroup, group)
		common.SetContextKey(c, constant.ContextKeyTokenId, 0)
		common.SetContextKey(c, constant.ContextKeyTokenKey, "")
		c.Set("token_name", "canvas")
		common.SetContextKey(c, constant.ContextKeyTokenUnlimited, true)
		c.Set("canvas_group", group)
		c.Set("canvas_original_path", c.Request.URL.Path)
		c.Request.URL.Path = strings.TrimPrefix(c.Request.URL.Path, "/api/canvas")
		c.Request.RequestURI = c.Request.URL.RequestURI()
		// Auth is already validated; a dashboard bearer must never become an
		// upstream passthrough header or a plugin request descriptor field.
		c.Request.Header.Del("Authorization")
		c.Request.Header.Del("Cookie")
		c.Request.Header.Del("X-Canvas-Group")
		c.Next()
	}
}
