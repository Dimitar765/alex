package desktop

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2/text/v2"

	"goGame/assets/fonts"
)

// Palette mirrors the web client's style.css tokens.
var (
	themeBackground = rgb(0x15, 0x13, 0x12) // --bg
	themePanel      = rgb(0x21, 0x1d, 0x1a) // --panel
	themeLine       = rgb(0x35, 0x30, 0x2a) // --line
	themeGold       = rgb(0xc9, 0xa2, 0x27) // --gold
	themeInk        = rgb(0xe8, 0xe0, 0xd0) // --ink
	themeMuted      = rgb(0x9a, 0x8f, 0x7d) // --muted
	themeCardFace   = rgb(0x26, 0x21, 0x1c)
	themeCardEdge   = rgb(0x4a, 0x3f, 0x2e)
)

func rgb(r, g, b uint8) color.RGBA {
	return color.RGBA{R: r, G: g, B: b, A: 0xff}
}

// Theme owns the loaded typefaces and hands out cached faces per size.
// EB Garamond carries body text; Cinzel is the engraved display face for
// headers; the italic is for scene flavor text.
type Theme struct {
	serif, serifItalic, display *text.GoTextFaceSource

	faces map[faceKey]*text.GoTextFace
}

type faceKey struct {
	src  *text.GoTextFaceSource
	size float64
}

// Face sizes in design-space units.
const (
	faceBody      = 17.0
	faceSmall     = 14.0
	faceScene     = 19.0
	faceCardName  = 15.0
	faceCardText  = 13.0
	faceHeader    = 30.0
	faceTitle     = 44.0
	faceHintSmall = 12.0
)

// NewTheme loads the embedded typefaces.
func NewTheme() (*Theme, error) {
	serif, err := text.NewGoTextFaceSource(bytesReader(fonts.MustOpen("EBGaramond.ttf")))
	if err != nil {
		return nil, err
	}
	italic, err := text.NewGoTextFaceSource(bytesReader(fonts.MustOpen("EBGaramond-Italic.ttf")))
	if err != nil {
		return nil, err
	}
	display, err := text.NewGoTextFaceSource(bytesReader(fonts.MustOpen("Cinzel.ttf")))
	if err != nil {
		return nil, err
	}
	return &Theme{
		serif: serif, serifItalic: italic, display: display,
		faces: map[faceKey]*text.GoTextFace{},
	}, nil
}

// Face returns a cached serif face at the given size.
func (t *Theme) Face(size float64) *text.GoTextFace {
	return t.face(t.serif, size)
}

// FaceItalic returns a cached italic serif face at the given size.
func (t *Theme) FaceItalic(size float64) *text.GoTextFace {
	return t.face(t.serifItalic, size)
}

// DisplayFace returns the Cinzel display face at the given size.
func (t *Theme) DisplayFace(size float64) *text.GoTextFace {
	return t.face(t.display, size)
}

func (t *Theme) face(src *text.GoTextFaceSource, size float64) *text.GoTextFace {
	key := faceKey{src: src, size: size}
	if f, ok := t.faces[key]; ok {
		return f
	}
	f := &text.GoTextFace{Source: src, Size: size}
	t.faces[key] = f
	return f
}
