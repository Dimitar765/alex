// Command svgtopng rasterizes the card emblems and UI icons from
// internal/web/static/art.svg into gold-on-transparent PNGs sized for the
// Ebitengine client. Run via go generate (see assets/art/art.go).
//
//	go run ./tools/svgtopng
package main

import (
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

const (
	// source sprite sheet, shared with the web client
	src = "internal/web/static/art.svg"
	// output directory for the generated PNGs
	outDir = "assets/art"
	// raster scale: the 64x64 grid renders at 4x (256px) so cards can
	// scale up without visible aliasing
	scale = 4
	// gold ink matching the web client's --gold token
	gold = "#c9a227"
)

var idRe = regexp.MustCompile(`<symbol id="([^"]+)"`)

func main() {
	raw, err := os.ReadFile(src)
	if err != nil {
		log.Fatalf("read %s: %v", src, err)
	}
	srcSVG := string(raw)

	ids := idRe.FindAllStringSubmatch(srcSVG, -1)
	if len(ids) == 0 {
		log.Fatalf("no <symbol> elements found in %s", src)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatal(err)
	}

	var written int
	for _, m := range ids {
		id := m[1]
		svg := wrapSymbol(srcSVG, id)

		icon, err := oksvg.ReadIconStream(strings.NewReader(svg))
		if err != nil {
			log.Fatalf("parse %s: %v", id, err)
		}
		w := int(icon.ViewBox.W * scale)
		h := int(icon.ViewBox.H * scale)
		icon.SetTarget(0, 0, float64(w), float64(h))

		img := image.NewRGBA(image.Rect(0, 0, w, h))
		scanner := rasterx.NewScannerGV(w, h, img, img.Bounds())
		raster := rasterx.NewDasher(w, h, scanner)
		icon.Draw(raster, 1.0)

		out := filepath.Join(outDir, id+".png")
		f, err := os.Create(out)
		if err != nil {
			log.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			f.Close()
			log.Fatal(err)
		}
		f.Close()
		written++
		fmt.Printf("wrote %s (%dx%d)\n", out, w, h)
	}
	fmt.Printf("done: %d PNGs from %s\n", written, src)
}

// wrapSymbol extracts one <symbol> body into a standalone SVG document.
// oksvg treats <symbol> like <g>, so the inner markup renders fine once it
// is wrapped in a <g> with the gold ink substituted for currentColor.
func wrapSymbol(srcSVG, id string) string {
	open := `<symbol id="` + id + `"`
	start := strings.Index(srcSVG, open)
	if start < 0 {
		log.Fatalf("symbol %s not found", id)
	}
	bodyStart := strings.Index(srcSVG[start:], ">") + start + 1
	end := strings.Index(srcSVG[bodyStart:], "</symbol>")
	if end < 0 {
		log.Fatalf("symbol %s has no closing tag", id)
	}
	body := srcSVG[bodyStart : bodyStart+end]
	body = strings.Replace(body, "currentColor", gold, -1)
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64">` +
		`<g>` + body + `</g></svg>`
}
