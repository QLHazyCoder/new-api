package middleware

import (
	"io"
	"net/http"
	"strings"

	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

func setCanvasFrontendHeaders(c *gin.Context) {
	// ES modules in an opaque-origin sandbox require anonymous CORS. This
	// permission applies to public build files only, never authenticated APIs.
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Cross-Origin-Resource-Policy", "cross-origin")
	c.Header("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' blob: data:; media-src blob: data:; font-src 'self' data:; connect-src blob: data:; frame-ancestors 'self'; base-uri 'none'; form-action 'none'")
	c.Header("Referrer-Policy", "same-origin")
	c.Header("X-Content-Type-Options", "nosniff")
}

// CanvasFrontendIndex bypasses http.FileServer's special redirect for
// /index.html. That redirect turns /canvas-app/index.html into
// /canvas-app/index.html/ and leaves the sandboxed child unable to boot.
func CanvasFrontendIndex(files static.ServeFileSystem) gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.URL.Path {
		case "/canvas-app", "/canvas-app/", "/canvas-app/index.html":
		default:
			return
		}
		file, err := files.Open("/canvas-app/index.html")
		if err != nil {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		defer file.Close()
		body, err := io.ReadAll(file)
		if err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		setCanvasFrontendHeaders(c)
		c.Header("Cache-Control", "no-cache")
		c.Abort()
		if c.Request.Method == http.MethodHead {
			c.Status(http.StatusOK)
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", body)
	}
}

func CanvasFrontendPolicy(files static.ServeFileSystem) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path != "/canvas-app" && !strings.HasPrefix(c.Request.URL.Path, "/canvas-app/") {
			return
		}
		setCanvasFrontendHeaders(c)
		if !files.Exists("/", c.Request.URL.Path) {
			c.AbortWithStatus(http.StatusNotFound)
		}
	}
}
