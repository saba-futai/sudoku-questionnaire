//go:build dev

package webui

import (
	"io/fs"
	"os"
)

// In development mode the frontend is served from disk, so a template or style
// change does not require rebuilding the Go binary. The Vite dev server is the
// better loop; this exists for serving a real production bundle without
// recompiling.
func FS() fs.FS {
	dir := os.Getenv("QRFORQA_WEB_DIR")
	if dir == "" {
		dir = "../frontend/dist"
	}
	return os.DirFS(dir)
}
