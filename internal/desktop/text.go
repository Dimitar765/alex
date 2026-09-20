package desktop

import (
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// lineSpacing is the extra gap between lines as a fraction of the face
// size, on top of the font's ascent+descent.
const lineSpacing = 0.38

// measure returns the rendered width and height of s as one block
// (newlines honored).
func measure(s string, face *text.GoTextFace) (float64, float64) {
	return text.Measure(s, face, lineSpacing)
}

// lineHeight returns the vertical distance between consecutive lines.
func lineHeight(face *text.GoTextFace) float64 {
	m := face.Metrics()
	return m.HAscent + m.HDescent + m.HLineGap + face.Size*lineSpacing
}

// drawText draws s with its top-left corner at (x, y).
func drawText(dst *ebiten.Image, s string, face *text.GoTextFace, x, y float64, clr color.Color) {
	opts := &text.DrawOptions{}
	opts.GeoM.Translate(x, y)
	opts.ColorScale.ScaleWithColor(clr)
	text.Draw(dst, s, face, opts)
}

// drawTextAligned draws s in the rectangle (x, y, w, h). Horizontal
// alignment follows alignX; the text is vertically centered in the box.
func drawTextAligned(dst *ebiten.Image, s string, face *text.GoTextFace, x, y, w, h float64, clr color.Color, alignX text.Align) {
	opts := &text.DrawOptions{}
	opts.LayoutOptions.PrimaryAlign = alignX
	opts.LayoutOptions.SecondaryAlign = text.AlignCenter
	opts.GeoM.Translate(x, y+h/2)
	opts.ColorScale.ScaleWithColor(clr)
	text.Draw(dst, s, face, opts)
}

// drawWrapped draws s word-wrapped to maxW and returns the height used.
func drawWrapped(dst *ebiten.Image, s string, face *text.GoTextFace, x, y, maxW float64, clr color.Color) float64 {
	lines := wrap(s, face, maxW)
	lh := lineHeight(face)
	for i, ln := range lines {
		drawText(dst, ln, face, x, y+float64(i)*lh, clr)
	}
	return float64(len(lines)) * lh
}

// wrappedHeight returns the height drawWrapped would use.
func wrappedHeight(s string, face *text.GoTextFace, maxW float64) float64 {
	return float64(len(wrap(s, face, maxW))) * lineHeight(face)
}

// wrap breaks s into lines no wider than maxW, greedy word wrap.
func wrap(s string, face *text.GoTextFace, maxW float64) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		line := words[0]
		for _, w := range words[1:] {
			candidate := line + " " + w
			if width, _ := measure(candidate, face); width > maxW {
				out = append(out, line)
				line = w
				continue
			}
			line = candidate
		}
		out = append(out, line)
	}
	return out
}
