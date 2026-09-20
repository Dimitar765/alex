// Package desktop implements the native Ebitengine client for Alexander.
// It shares the game rules (internal/game) and content (internal/content)
// with the web build; nothing here talks HTTP.
package desktop

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// Design-space dimensions; Ebitengine letterboxes the window onto them.
const (
	ScreenW = 1280
	ScreenH = 800
)

// Palette mirrors the web client's style.css tokens.
var (
	themeBackground = rgb(0x15, 0x13, 0x12) // --bg
	themePanel      = rgb(0x21, 0x1d, 0x1a) // --panel
	themeLine       = rgb(0x35, 0x30, 0x2a) // --line
	themeGold       = rgb(0xc9, 0xa2, 0x27) // --gold
	themeInk        = rgb(0xe8, 0xe0, 0xd0) // --ink
	themeMuted      = rgb(0x9a, 0x8f, 0x7d) // --muted
)

// Game is the root Ebitengine state: the current screen and shared
// resources. Screens arrive in later phases; Phase 0 proves the shell.
type Game struct{}

// New creates the root Game.
func New() *Game {
	return &Game{}
}

// Update advances the game by one tick. No rendering happens here.
func (g *Game) Update() error {
	if ebiten.IsKeyPressed(ebiten.KeyEscape) {
		return ebiten.Termination
	}
	return nil
}

// Draw renders the current frame.
func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(themeBackground)
}

// Layout returns the fixed design-space size; Ebitengine scales the
// window onto it.
func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return ScreenW, ScreenH
}

// rgb packs 8-bit components into an opaque color.
func rgb(r, g, b uint8) color.RGBA {
	return color.RGBA{R: r, G: g, B: b, A: 0xff}
}
