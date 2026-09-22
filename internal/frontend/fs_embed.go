//go:build embed

package frontend

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Assets returns the production frontend bundle embedded at build time.
func Assets() fs.FS {
	root, err := fs.Sub(dist, "dist")
	if err != nil {
		panic("frontend: embedded dist: " + err.Error())
	}

	return root
}
