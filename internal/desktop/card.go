package desktop

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"goGame/assets/art"
	"goGame/internal/app"
)

// Card geometry in design space.
const (
	cardW = 150
	cardH = 210
)

// cardCache renders each distinct card face once and reuses the image.
// Faces include the emblem, name, flavor text, and cost chip; the layout
// mirrors the web build's paintFront.
type cardCache struct {
	faces map[string]*ebiten.Image
}

func newCardCache() *cardCache {
	return &cardCache{faces: map[string]*ebiten.Image{}}
}

// face returns the cached face image for a card view, rendering it on
// first use.
func (cc *cardCache) face(g *Game, c app.CardView) *ebiten.Image {
	if img, ok := cc.faces[c.ID]; ok {
		return img
	}
	img := ebiten.NewImage(cardW, cardH)
	g.paintFace(img, c)
	cc.faces[c.ID] = img
	return img
}

// paintFace draws one card face onto dst. Fixed bands keep text from
// colliding: emblem 14–106, name centered at 118, flavor capped to the
// band 150–200.
func (g *Game) paintFace(dst *ebiten.Image, c app.CardView) {
	const corner = 10
	// Base plate: rect + four corner circles reads as a rounded card.
	vector.DrawFilledRect(dst, 0, corner, cardW, cardH-2*corner, themeCardFace, true)
	vector.DrawFilledRect(dst, corner, 0, cardW-2*corner, cardH, themeCardFace, true)
	for _, p := range [][2]float32{
		{corner, corner}, {cardW - corner, corner},
		{corner, cardH - corner}, {cardW - corner, cardH - corner},
	} {
		vector.DrawFilledCircle(dst, p[0], p[1], corner, themeCardFace, true)
	}
	vector.StrokeRect(dst, 2, 2, cardW-4, cardH-4, 1.5, themeCardEdge, true)

	// Emblem, gold line art centered in the upper band.
	if emblem, err := art.Open("art_" + c.ID); err == nil {
		src := ebiten.NewImageFromImage(emblem)
		const box = 88
		s := float64(box) / float64(src.Bounds().Dx())
		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Scale(s, s)
		opts.GeoM.Translate((cardW-box)/2, 14)
		opts.ColorScale.ScaleWithColor(themeGold)
		dst.DrawImage(src, opts)
	}

	// Name, centered; shrinks to fit long names.
	name := c.Name
	nameFace := g.theme.Face(faceCardName)
	if w, _ := measure(name, nameFace); w > cardW-16 {
		nameFace = g.theme.Face(faceCardText)
	}
	nw, _ := measure(name, nameFace)
	nameY := 114.0
	drawText(dst, name, nameFace, (cardW-nw)/2, nameY, themeGold)

	// Cost chip: gold circle with the number, top-right.
	if c.Cost > 0 {
		cx, cy, r := float64(cardW-24), float64(24), float64(14)
		vector.DrawFilledCircle(dst, float32(cx), float32(cy), float32(r), themeGold, true)
		chip := fmt.Sprint(c.Cost)
		drawTextAligned(dst, chip, g.theme.Face(faceCardText),
			cx-r, cy-r, r*2, r*2, rgb(0x15, 0x13, 0x12), text.AlignCenter)
	}

	// Flavor: at most three lines in the band under the name, centered.
	flavorFace := g.theme.Face(faceCardText)
	lines := wrap(c.Text, flavorFace, cardW-16)
	const flavorTop, maxLines = 148.0, 3
	lh := lineHeight(flavorFace)
	for i, ln := range lines {
		if i >= maxLines {
			break
		}
		w, _ := measure(ln, flavorFace)
		drawText(dst, ln, flavorFace, (cardW-w)/2, flavorTop+float64(i)*lh, themeMuted)
	}
}

// drawCard blits a cached face at a slot: center (x, y), rotation, scale,
// and dimming for unaffordable cards.
func (cc *cardCache) drawCard(dst *ebiten.Image, face *ebiten.Image, x, y, angle, scale float64, dim bool) {
	opts := &ebiten.DrawImageOptions{}
	opts.Filter = ebiten.FilterLinear
	opts.GeoM.Translate(-cardW/2, -cardH/2)
	opts.GeoM.Scale(scale, scale)
	opts.GeoM.Rotate(angle)
	opts.GeoM.Translate(x, y)
	if dim {
		opts.ColorScale.Scale(0.45, 0.45, 0.45, 1)
	}
	dst.DrawImage(face, opts)
}
