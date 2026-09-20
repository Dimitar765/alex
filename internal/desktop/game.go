// Package desktop implements the native Ebitengine client for Alexander.
// It shares the game rules (internal/game) and content (internal/content)
// with the web build; nothing here talks HTTP.
package desktop

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	assets "goGame"
	"goGame/internal/app"
	"goGame/internal/content"
	"goGame/internal/paths"
)

// Design-space dimensions; Ebitengine letterboxes the window onto them.
const (
	ScreenW = 1280
	ScreenH = 800
)

// Game is the root Ebitengine state: the screen stack over the shared
// view-model, plus per-tick input state the screens consume.
type Game struct {
	theme *Theme
	model *app.Model
	stack []screen

	// Per-tick input snapshot, refreshed in Update before screens run.
	cursorX, cursorY float64
	mouseClicked     bool
	keysPressed      map[ebiten.Key]bool
}

// New builds the root game: theme, model, and the landing screen.
func New() (*Game, error) {
	theme, err := NewTheme()
	if err != nil {
		return nil, err
	}
	lib, err := content.Load(assets.FS())
	if err != nil {
		return nil, err
	}
	dir, err := paths.UserSavesDir()
	if err != nil {
		dir = "" // memory-only rather than failing to start
	}
	g := &Game{
		theme:       theme,
		model:       app.NewModel(lib, dir),
		keysPressed: map[ebiten.Key]bool{},
	}
	g.pushScreen(&titleScreen{})
	return g, nil
}

// Update advances input, then the top screen by one tick.
func (g *Game) Update() error {
	g.refreshInput()
	if len(g.stack) == 0 {
		return ebiten.Termination
	}
	return g.stack[len(g.stack)-1].update(g)
}

// Draw renders the top screen.
func (g *Game) Draw(screen *ebiten.Image) {
	if len(g.stack) == 0 {
		screen.Fill(themeBackground)
		return
	}
	g.stack[len(g.stack)-1].draw(g, screen)
}

// Layout returns the fixed design-space size; Ebitengine scales the
// window onto it.
func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return ScreenW, ScreenH
}

// refreshInput snapshots cursor, click edge, and pressed keys for this tick.
func (g *Game) refreshInput() {
	cx, cy := ebiten.CursorPosition()
	g.cursorX, g.cursorY = float64(cx), float64(cy)
	g.mouseClicked = inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)

	clear(g.keysPressed)
	for _, k := range inpututil.AppendPressedKeys(nil) {
		g.keysPressed[k] = true
	}
}

// pushScreen puts a new screen on top of the stack.
func (g *Game) pushScreen(s screen) {
	g.stack = append(g.stack, s)
}

// popScreen removes the top screen; an empty stack ends the program.
func (g *Game) popScreen() {
	if n := len(g.stack); n > 0 {
		g.stack = g.stack[:n-1]
	}
}

// replaceScreen swaps the top screen in place (or pushes when empty).
func (g *Game) replaceScreen(s screen) {
	if n := len(g.stack); n > 0 {
		g.stack[n-1] = s
		return
	}
	g.stack = append(g.stack, s)
}

// consumeEffects feeds an action's FX events to the animation layer.
// Phase 5 turns this into tweens/particles/audio; for now the events are
// acknowledged so the view-model contract is exercised end to end.
func (g *Game) consumeEffects(r app.Result) {}
