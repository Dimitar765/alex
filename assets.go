// Package assets embeds the game content shipped in the repository's
// content/ directory so the binary is fully self-contained.
package assets

import (
	"embed"
	"io/fs"
)

//go:embed content
var embedded embed.FS

// FS returns the game content tree with cards.json and scenes.json at its root.
func FS() fs.FS {
	sub, err := fs.Sub(embedded, "content")
	if err != nil {
		// Can only happen if the embed path changes; that is a build-time bug.
		panic(err)
	}
	return sub
}
