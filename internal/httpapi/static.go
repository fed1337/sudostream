package httpapi

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// RegisterFrontend serves embedded SPA assets and falls back to index.html for client routes.
// Register API routes before calling this so /api keeps precedence.
func RegisterFrontend(router *gin.Engine, root fs.FS) {
	handler := spaHandler{root: root}
	router.NoRoute(handler.serve)
}

type spaHandler struct {
	root fs.FS
}

func (h spaHandler) serve(c *gin.Context) {
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		c.Status(http.StatusNotFound)

		return
	}

	path := c.Request.URL.Path
	if strings.HasPrefix(path, "/api") {
		c.Status(http.StatusNotFound)

		return
	}

	filePath := strings.TrimPrefix(path, "/")
	if filePath == "" {
		h.serveIndex(c)

		return
	}

	_, err := fs.Stat(h.root, filePath)
	if err == nil {
		c.FileFromFS(filePath, http.FS(h.root))

		return
	}

	h.serveIndex(c)
}

func (h spaHandler) serveIndex(c *gin.Context) {
	data, err := fs.ReadFile(h.root, "index.html")
	if err != nil {
		c.Status(http.StatusNotFound)

		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}
