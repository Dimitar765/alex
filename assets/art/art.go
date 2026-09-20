// Package art embeds the gold card emblems and UI icons rasterized from
// internal/web/static/art.svg. Regenerate with:
//
//	go generate ./assets/art
package art

import (
	"embed"
	image "image"
	_ "image/png"
	"io/fs"
)

//go:generate go run ../../tools/svgtopng
//go:embed *.png
var files embed.FS

// Names returns every embedded sprite's base name (e.g. "art_phalanx").
func Names() []string {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		panic(err) // embed pattern guarantees the directory exists
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name()[:len(e.Name())-4]) // strip .png
	}
	return names
}

// Open returns a decoded sprite by base name (e.g. "art_phalanx").
func Open(name string) (image.Image, error) {
	f, err := files.Open(name + ".png")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}
