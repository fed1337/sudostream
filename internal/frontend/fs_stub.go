//go:build !embed

package frontend

import "io/fs"

// Assets returns nil when the server is built without the embed tag.
func Assets() fs.FS {
	return nil
}
