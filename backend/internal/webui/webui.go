// Package webui serves the compiled single-page application from the same
// origin as the API, so the whole application lives on one port.
package webui

import (
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Register installs the SPA handler as the router's fallback.
//
// Unknown /api paths still return JSON 404s so a typo in an API call does not
// silently return index.html, which would defeat client error handling.
func Register(r *gin.Engine) {
	assets := FS()
	fileServer := http.FileServer(http.FS(assets))

	r.NoRoute(func(c *gin.Context) {
		requestPath := c.Request.URL.Path

		if strings.HasPrefix(requestPath, "/api/") || requestPath == "/api" {
			c.JSON(http.StatusNotFound, gin.H{"error": "接口不存在"})
			return
		}

		name := strings.TrimPrefix(requestPath, "/")
		if name == "" {
			serveIndex(c, assets)
			return
		}

		f, err := assets.Open(name)
		if err != nil {
			// Not a real file: hand it to the SPA router so client-side routes
			// like /admin/questionnaires/3 survive a page refresh.
			if looksLikeAsset(name) {
				c.Status(http.StatusNotFound)
				return
			}
			serveIndex(c, assets)
			return
		}
		_ = f.Close()

		// Vite emits content-hashed filenames under /assets, so those can be
		// cached forever. Everything else gets revalidated, which matters most
		// for index.html: a stale entrypoint would pair last week's codec
		// against today's server.
		if strings.HasPrefix(name, "assets/") {
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			c.Header("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
}

func serveIndex(c *gin.Context, assets fs.FS) {
	f, err := assets.Open("index.html")
	if err != nil {
		c.String(http.StatusInternalServerError, "frontend bundle is missing from this build")
		return
	}
	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(f)
	if err != nil {
		c.String(http.StatusInternalServerError, "cannot read index.html")
		return
	}
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}

// looksLikeAsset reports whether a missing path was probably meant to be a
// file, so it should 404 rather than return the SPA shell.
func looksLikeAsset(name string) bool {
	if strings.HasPrefix(name, "assets/") {
		return true
	}
	lastSlash := strings.LastIndex(name, "/")
	lastDot := strings.LastIndex(name, ".")
	return lastDot > lastSlash
}
