//go:build !dev

package webui

import (
	"embed"
	"io/fs"
)

// The frontend build is embedded so the deployment is a single static binary:
// the server needs no Node, no reverse proxy and no network access to fetch
// assets at run time.
//
//go:embed all:dist
var embedded embed.FS

// FS returns the embedded frontend, rooted at the build output.
func FS() fs.FS {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		panic("webui: embedded dist missing: " + err.Error())
	}
	return sub
}
