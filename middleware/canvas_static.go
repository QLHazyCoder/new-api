package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

func CanvasFrontendPolicy(files static.ServeFileSystem) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !strings.HasPrefix(c.Request.URL.Path, "/canvas-app/") {
			return
		}
		// ES modules in an opaque-origin sandbox require anonymous CORS. This
		// permission applies to public build files only, never authenticated APIs.
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Cross-Origin-Resource-Policy", "cross-origin")
		c.Header("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' blob: data:; media-src blob: data:; font-src 'self' data:; connect-src blob: data:; frame-ancestors 'self'; base-uri 'none'; form-action 'none'")
		c.Header("Referrer-Policy", "same-origin")
		c.Header("X-Content-Type-Options", "nosniff")
		if !files.Exists("/", c.Request.URL.Path) {
			c.AbortWithStatus(http.StatusNotFound)
		}
	}
}
