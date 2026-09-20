package desktop

import (
	"bytes"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// bytesReader wraps b for text face sources.
func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }

// Rectangle is an axis-aligned box in design space.
type Rectangle struct {
	X, Y, W, H float64
}

// Contains reports whether the point (x, y) lies inside the rectangle.
func (r Rectangle) Contains(x, y float64) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// button is a clickable labeled box with hover feedback. It is stateless
// about actions: the owning screen checks Clicked.
type button struct {
	Rect  Rectangle
	Label string

	primary bool // gold border for the emphasized action
}

// hovered reports whether the cursor is over the button.
func (b *button) hovered(g *Game) bool {
	return b.Rect.Contains(g.cursorX, g.cursorY)
}

// clicked reports whether the button was pressed this tick.
func (b *button) clicked(g *Game) bool {
	return b.hovered(g) && g.mouseClicked
}

func (b *button) draw(g *Game, dst *ebiten.Image) {
	fill := themeCardFace
	if b.hovered(g) {
		fill = rgb(0x3a, 0x32, 0x29) // hover tone from the web build
	}
	border := themeLine
	if b.primary {
		border = themeGold
	}
	const antialias = true
	vector.DrawFilledRect(dst, float32(b.Rect.X), float32(b.Rect.Y),
		float32(b.Rect.W), float32(b.Rect.H), fill, antialias)
	vector.StrokeRect(dst, float32(b.Rect.X)+0.5, float32(b.Rect.Y)+0.5,
		float32(b.Rect.W)-1, float32(b.Rect.H)-1, 1, border, antialias)
	drawTextAligned(dst, b.Label, g.theme.Face(faceBody),
		b.Rect.X, b.Rect.Y, b.Rect.W, b.Rect.H, themeInk, text.AlignCenter)
}

// panel draws the dark panel the scene and log live in.
func panel(dst *ebiten.Image, r Rectangle) {
	const antialias = true
	vector.DrawFilledRect(dst, float32(r.X), float32(r.Y),
		float32(r.W), float32(r.H), themePanel, antialias)
	vector.StrokeRect(dst, float32(r.X)+0.5, float32(r.Y)+0.5,
		float32(r.W)-1, float32(r.H)-1, 1, themeLine, antialias)
}
