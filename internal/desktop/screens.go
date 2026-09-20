package desktop

import (
	"fmt"
	"math/rand"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"

	"goGame/internal/app"
	"goGame/internal/game"
)

// screen is one full-window view. Screens push and pop via the Game.
type screen interface {
	update(g *Game) error
	draw(g *Game, dst *ebiten.Image)
}

// layoutMetrics holds the play-screen geometry in design space, mirroring
// the web board: card table left, play side right.
type layoutMetrics struct {
	table      Rectangle // left column (fan + deck controls)
	play       Rectangle // right column (scene + log)
	scenePanel Rectangle
	logPanel   Rectangle
}

func newLayout() layoutMetrics {
	const (
		marginX  = 40.0
		marginY  = 46.0
		gap      = 36.0
		tableW   = 570.0
		controls = 96.0
	)
	playX := marginX + tableW + gap
	playW := float64(ScreenW) - 2*marginX - tableW - gap
	playH := float64(ScreenH) - 2*marginY
	_ = controls
	return layoutMetrics{
		table: Rectangle{X: marginX, Y: marginY, W: tableW, H: playH},
		play:  Rectangle{X: playX, Y: marginY, W: playW, H: playH},
		scenePanel: Rectangle{
			X: playX, Y: marginY, W: playW,
			H: 300,
		},
		logPanel: Rectangle{
			X: playX, Y: marginY + 300 + 24, W: playW,
			H: playH - 300 - 24,
		},
	}
}

// titleScreen is the landing view: campaign start, continue, chronicle.
type titleScreen struct {
	focus int // index into the current menu; -1 = mouse mode
}

// menu builds the button list for the current model state; used by both
// update (focus math) and draw (rendering) so they always agree.
func (s *titleScreen) menu(m *app.Model) []button {
	var labels []string
	if m.HasRun() {
		labels = append(labels, "Continue")
	}
	labels = append(labels, "New game", "Chronicle")

	menu := make([]button, 0, len(labels))
	for i, label := range labels {
		menu = append(menu, button{
			Rect:    Rectangle{X: 52, Y: 180 + float64(i)*56, W: 240, H: 44},
			Label:   label,
			primary: label == "New game",
		})
	}
	return menu
}

func (s *titleScreen) activate(g *Game, b button) {
	switch b.Label {
	case "New game":
		g.model.NewGame()
		g.replaceScreen(&tableScreen{})
	case "Continue":
		g.replaceScreen(&tableScreen{})
	case "Chronicle":
		g.pushScreen(&chronicleScreen{})
	}
}

func (s *titleScreen) update(g *Game) error {
	if g.back() {
		return ebiten.Termination
	}
	menu := s.menu(g.model)

	if step := g.verticalStep() + g.horizontalStep(); step != 0 {
		if s.focus < 0 {
			s.focus = 0
		} else {
			s.focus = (s.focus + step + len(menu)) % len(menu)
		}
	}
	if s.focus >= 0 && g.confirm() && s.focus < len(menu) {
		s.activate(g, menu[s.focus])
		return nil
	}
	for i := range menu {
		if menu[i].clicked(g) {
			s.activate(g, menu[i])
			s.focus = -1
		}
	}
	return nil
}

func (s *titleScreen) draw(g *Game, dst *ebiten.Image) {
	dst.Fill(themeBackground)
	drawText(dst, "Alexander", g.theme.DisplayFace(faceTitle), 48, 40, themeGold)
	drawText(dst, "a card adventure", g.theme.FaceItalic(faceScene), 52, 108, themeMuted)

	menu := s.menu(g.model)
	for i := range menu {
		b := &menu[i]
		focused := s.focus == i
		if focused {
			drawText(dst, "▸", g.theme.Face(faceBody), b.Rect.X-24, b.Rect.Y+8, themeGold)
		}
		b.draw(g, dst)
		if !b.clicked(g) {
			continue
		}
		s.focus = -1
		s.activate(g, menu[i])
	}

	// Endings gallery tiles under the menu.
	x0, y0 := 52.0, 180+float64(len(menu))*56+24
	for i, tile := range g.model.View().Endings {
		label := "???"
		if tile.Discovered {
			label = tile.Class
		}
		drawTextAligned(dst, label, g.theme.Face(faceHintSmall),
			x0+float64(i)*72, y0, 60, 28, themeMuted, text.AlignCenter)
	}
}

// tableScreen is the main play view: card fan left, scene and log right.
// Focus indices: -1 none, 0..len(hand)-1 a hand card, then Scout, then
// Shuffle.
type tableScreen struct {
	cards     *cardCache
	focus     int
	endingBtn Rectangle // set while drawing the ending summary

	anims    map[string]*cardAnim // hand card flight state, keyed by ID
	lastHand []string             // previous tick's hand IDs
	lastPos  []cardSlot           // their fan slots (for ghost spawns)

	fxGhosts []ghostCard
	fxFloats []floatText
	fxParts  []particle
	shakeT   float64

	statsX, statsY float64 // where the stat row landed last draw
	off            *ebiten.Image
}

func (s *tableScreen) ensureCache() {
	if s.cards == nil {
		s.cards = newCardCache()
	}
}

func (s *tableScreen) update(g *Game) error {
	s.stepFX(g.dt)
	if g.back() {
		g.popScreen()
		return nil
	}
	v := g.model.View()
	if v.Scene.Ending != "" {
		// Run over: the summary panel's button (or confirm) restarts.
		if (g.mouseClicked && s.endingBtn.Contains(g.cursorX, g.cursorY)) || g.confirm() {
			g.model.NewGame()
			s.reset()
		}
		return nil
	}

	// Track hand transitions: leaving cards become play ghosts, new
	// cards fly in from the deck side.
	pos := fanPositions(len(v.Hand), newLayout().table.W)
	cur := map[string]bool{}
	for _, c := range v.Hand {
		cur[c.ID] = true
	}
	if s.anims == nil {
		s.anims = map[string]*cardAnim{}
	}
	for i, id := range s.lastHand {
		if !cur[id] && i < len(s.lastPos) {
			s.spawnPlayGhost(id, s.lastPos[i])
		}
	}
	for i, c := range v.Hand {
		a, ok := s.anims[c.ID]
		if !ok {
			a = &cardAnim{}
			a.settle(pos[i], 0, true)
			s.anims[c.ID] = a
		}
		a.approach(pos[i], g.dt)
	}
	s.lastHand = make([]string, len(v.Hand))
	for i, c := range v.Hand {
		s.lastHand[i] = c.ID
	}
	s.lastPos = pos

	n := len(v.Hand)
	targets := n + 2 // hand cards + Scout + Shuffle
	if step := g.horizontalStep(); step != 0 {
		if s.focus < 0 {
			s.focus = 0
		} else {
			s.focus = (s.focus + step + targets) % targets
		}
	}
	if g.confirm() && s.focus >= 0 {
		switch {
		case s.focus < n:
			if r := g.model.PlayCard(v.Hand[s.focus].ID); r.Err == nil {
				g.sfx("card")
				g.consumeEffects(r)
				s.focus = -1
			} else {
				g.sfx("error")
			}
		case s.focus == n:
			g.model.Scout()
			g.sfx("scout")
		default:
			g.model.Shuffle()
			g.sfx("shuffle")
		}
	}
	if s.focus > targets {
		s.focus = -1
	}
	return nil
}

// reset clears transient FX state when a new campaign begins.
func (s *tableScreen) reset() {
	s.anims = map[string]*cardAnim{}
	s.lastHand = nil
	s.lastPos = nil
	s.fxGhosts = nil
	s.fxFloats = nil
	s.fxParts = nil
	s.focus = -1
}

func (s *tableScreen) draw(g *Game, dst *ebiten.Image) {
	s.ensureCache()
	v := g.model.View()
	l := newLayout()

	// While a shake is active, render the table into an offscreen image
	// and blit it with a decaying offset.
	target := dst
	if s.shakeT > 0 {
		if s.off == nil {
			s.off = ebiten.NewImage(ScreenW, ScreenH)
		}
		s.off.Fill(themeBackground)
		target = s.off
	}
	target.Fill(themeBackground)
	s.drawTable(g, target, v)

	if v.Scene.Ending == "" {
		n := len(v.Hand)
		for i, c := range v.Hand {
			if i >= len(s.lastPos) {
				break
			}
			a, ok := s.anims[c.ID]
			if !ok {
				continue
			}
			x, y := a.X, a.Y
			// Hover lift: cards rise toward the cursor like the web fan.
			hover := Rectangle{X: x - cardW/2, Y: y - cardH/2, W: cardW, H: cardH}.
				Contains(g.cursorX, g.cursorY)
			scale := a.Scale
			lift := 0.0
			if hover && c.Playable {
				lift = 16
			}
			if s.focus == i {
				lift = 12
				scale *= 1.05
			}
			s.cards.drawCard(target, s.cards.face(g, c), x, y-lift, a.Angle, scale, !c.Playable)
			if hover && c.Playable && g.mouseClicked {
				if r := g.model.PlayCard(c.ID); r.Err == nil {
					g.consumeEffects(r)
				}
			}
		}

		// Deck row under the fan.
		rowY := l.table.Y + l.table.H - 96
		drawText(target, fmt.Sprintf("Deck · %d", v.DeckCount), g.theme.Face(faceSmall),
			l.table.X+4, rowY, themeGold)
		drawText(target, fmt.Sprintf("Discard · %d", v.DiscardCount), g.theme.Face(faceSmall),
			l.table.X+4, rowY+30, themeMuted)

		shuffleLabel := fmt.Sprintf("Shuffle · %d", v.DiscardCount)
		shuffle := button{Rect: Rectangle{X: l.table.X + l.table.W - 110, Y: rowY - 4, W: 110, H: 36}, Label: shuffleLabel}
		scout := button{Rect: Rectangle{X: l.table.X + l.table.W - 228, Y: rowY - 4, W: 110, H: 36}, Label: "Scout"}
		if s.focus == n {
			drawText(dst, "▸", g.theme.Face(faceBody), scout.Rect.X-20, scout.Rect.Y+6, themeGold)
		}
		if s.focus == n+1 {
			drawText(dst, "▸", g.theme.Face(faceBody), shuffle.Rect.X-20, shuffle.Rect.Y+6, themeGold)
		}
		if scout.clicked(g) {
			g.model.Scout()
			g.sfx("scout")
		}
		if shuffle.clicked(g) {
			g.model.Shuffle()
			g.sfx("shuffle")
		}
		scout.draw(g, target)
		shuffle.draw(g, target)
	}

	// FX always render unshaken, on the real frame.
	for i := range s.fxGhosts {
		s.fxGhosts[i].draw(dst)
	}
	for i := range s.fxParts {
		s.fxParts[i].draw(dst)
	}
	for i := range s.fxFloats {
		s.fxFloats[i].draw(dst, g)
	}

	if s.shakeT > 0 {
		dx := sin64(s.shakeT*44) * 5 * s.shakeT
		dst.Fill(themeBackground)
		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Translate(dx, 0)
		dst.DrawImage(s.off, opts)
	}
}

// sin64 is math.Sin with a dependency-free call site.
func sin64(x float64) float64 {
	x = mod2pi(x)
	sq := x * x
	// Taylor around 0 is fine for the small range a shake uses.
	return x - x*sq/6 + x*sq*sq/120 - x*sq*sq*sq/5040
}

func mod2pi(x float64) float64 {
	const twoPi = 6.283185307179586
	for x > twoPi {
		x -= twoPi
	}
	for x < 0 {
		x += twoPi
	}
	return x
}

// consumeFX turns action effects into ghosts, floats, sparks, and shakes.
func (s *tableScreen) consumeFX(g *Game, effects []app.Effect) {
	l := newLayout()
	for _, e := range effects {
		switch e.Kind {
		case app.EffectStat:
			verb := "+"
			clr := themeGold
			if e.Delta < 0 {
				verb, clr = "", rgb(0xb0, 0x6a, 0x5a)
			}
			s.fxFloats = append(s.fxFloats, floatText{
				text: fmt.Sprintf("%s%+d %s", verb, e.Delta, game.StatLabel(e.Stat)),
				x:    s.statsX, y: s.statsY,
				life: 0.9, clr: clr,
			})
		case app.EffectCardLeft:
			if i := slices.Index(s.lastHand, e.CardID); i >= 0 && i < len(s.lastPos) {
				p := s.lastPos[i]
				if img := s.cards.faces[e.CardID]; img != nil {
					s.fxGhosts = append(s.fxGhosts, ghostCard{
						img: img, x: p.X, y: p.Y, angle: p.Angle, scale: 1,
						vx: 420, vy: -260, spin: -2.2, alpha: 1,
					})
					s.spawnDust(p.X, p.Y)
				}
			}
		case app.EffectCardGained:
			ln := newLayout()
			s.spawnDust(ln.table.X + ln.table.W/2 + 80)
		case app.EffectFate:
			if !reducedMotion {
				s.shakeT = 0.3
			}
		case app.EffectEnding:
			if e.Text == "defeat" || e.Text == "death" {
				g.sfx("defeat")
			} else {
				g.sfx("victory")
			}
			for i := 0; i < 24; i++ {
				s.fxParts = append(s.fxParts, particle{
					x:  l.play.X + l.play.W/2 + randSpread(180),
					y:  l.play.Y + 120,
					vx: randSpread(40), vy: -60 - randAbs(80),
					life: 1.2 + randAbs(0.5), maxLife: 1.6,
					size: 2.5, clr: themeGold,
				})
			}
		}
	}
}

// stepFX advances every transient effect by dt.
func (s *tableScreen) stepFX(dt float64) {
	for i := len(s.fxGhosts) - 1; i >= 0; i-- {
		if !s.fxGhosts[i].update(dt) {
			s.fxGhosts = append(s.fxGhosts[:i], s.fxGhosts[i+1:]...)
		}
	}
	for i := len(s.fxFloats) - 1; i >= 0; i-- {
		if !s.fxFloats[i].update(dt) {
			s.fxFloats = append(s.fxFloats[:i], s.fxFloats[i+1:]...)
		}
	}
	for i := len(s.fxParts) - 1; i >= 0; i-- {
		if !s.fxParts[i].update(dt) {
			s.fxParts = append(s.fxParts[:i], s.fxParts[i+1:]...)
		}
	}
	if s.shakeT > 0 {
		s.shakeT -= dt
		if s.shakeT < 0 {
			s.shakeT = 0
		}
	}
}

// spawnPlayGhost launches the leaving card from its old slot.
func (s *tableScreen) spawnPlayGhost(id string, p cardSlot) {
	if img := s.cards.faces[id]; img != nil {
		s.fxGhosts = append(s.fxGhosts, ghostCard{
			img: img, x: p.X, y: p.Y, angle: p.Angle, scale: 1,
			vx: 420, vy: -260, spin: -2.2, alpha: 1,
		})
	}
}

// spawnDust emits a little puff of gold specks at (x, y).
func (s *tableScreen) spawnDust(x float64, y ...float64) {
	py := 300.0
	if len(y) > 0 {
		py = y[0]
	}
	if reducedMotion {
		return
	}
	for i := 0; i < 8; i++ {
		s.fxParts = append(s.fxParts, particle{
			x: x + randSpread(30), y: py + randSpread(20),
			vx: randSpread(60), vy: -30 - randAbs(50),
			life: 0.5 + randAbs(0.3), maxLife: 0.8,
			size: 2, clr: themeGold,
		})
	}
}

// randSpread returns a random value in [-spread, spread].
func randSpread(spread float64) float64 { return (rand.Float64()*2 - 1) * spread }

// randAbs returns a random non-negative value below max.
func randAbs(max float64) float64 { return rand.Float64() * max }

// abs is math.Abs for the fan arc.
func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// cardSlot is one fan position: card center and tilt.
type cardSlot struct {
	X, Y, Angle float64
}

// fanPositions lays out n cards as a fan across tableW: centered, even
// spacing (shrinking for large hands), edges tilted and dropped — the 2D
// descendant of hand3d.js's fanSlot.
func fanPositions(n int, tableW float64) []cardSlot {
	if n <= 0 {
		return nil
	}
	const spacing = 118.0
	s := spacing
	if n > 1 {
		s = min(spacing, (tableW-cardW)/float64(n-1))
	}
	cx := tableW / 2
	out := make([]cardSlot, n)
	for i := range out {
		t := float64(i) - float64(n-1)/2
		out[i] = cardSlot{
			X:     cx + t*s,
			Y:     150 + abs(t)*14,
			Angle: -t * 0.085,
		}
	}
	return out
}

// drawTable renders the play side: scene panel, stats, choices, log.
func (s *tableScreen) drawTable(g *Game, dst *ebiten.Image, v *app.View) {
	l := newLayout()
	panel(dst, l.scenePanel)

	textH := wrappedHeight(v.Scene.Text, g.theme.Face(faceScene), l.scenePanel.W-40)
	drawWrapped(dst, v.Scene.Text, g.theme.Face(faceScene),
		l.scenePanel.X+20, l.scenePanel.Y+18, l.scenePanel.W-40, themeInk)

	statsY := l.scenePanel.Y + 18 + textH + 10
	stats := fmt.Sprintf("Legacy %d   Army %d   Treasury %d",
		v.Stats.Legacy, v.Stats.Army, v.Stats.Treasury)
	drawText(dst, stats, g.theme.Face(faceBody), l.scenePanel.X+20, statsY, themeGold)
	s.statsX, s.statsY = l.scenePanel.X+20, statsY

	if v.Scene.Ending != "" {
		// Terminal scene: badge, summary, and the way back in.
		drawTextAligned(dst, v.Scene.Ending, g.theme.Face(faceSmall),
			l.scenePanel.X+20, statsY+36, 140, 30, themeGold, text.AlignCenter)
		drawText(dst, fmt.Sprintf("The campaign ends after %d turns · %d cards played · %d campaigns completed",
			v.Turns, v.CardsPlayed, v.Campaigns),
			g.theme.Face(faceSmall), l.scenePanel.X+20, statsY+72, themeMuted)
		again := button{Rect: Rectangle{X: l.scenePanel.X + 20, Y: statsY + 104, W: 240, H: 42},
			Label: "Begin a new campaign", primary: true}
		again.draw(g, dst)
		s.endingBtn = again.Rect
		return
	}

	choicesY := statsY + 40
	for _, c := range v.Choices {
		clr := themeInk
		if !c.Available {
			clr = themeMuted
		}
		label := fmt.Sprintf("%d. %s", c.Index+1, c.Text)
		if c.Requires != "" {
			label += "  — requires " + c.Requires
		}
		h := wrappedHeight(label, g.theme.Face(faceBody), l.scenePanel.W-40)
		drawWrapped(dst, label, g.theme.Face(faceBody),
			l.scenePanel.X+20, choicesY, l.scenePanel.W-40, clr)
		// Number keys choose; a click on the line does too.
		line := Rectangle{X: l.scenePanel.X + 12, Y: choicesY - 2, W: l.scenePanel.W - 24, H: h + 6}
		if (line.Contains(g.cursorX, g.cursorY) && g.mouseClicked) ||
			(c.Available && g.keysPressed[digitKey(c.Index)]) {
			if c.Available {
				if r := g.model.Choose(c.Index); r.Err == nil {
					g.sfx("choice")
					g.consumeEffects(r)
				} else {
					g.sfx("error")
				}
			}
		}
		choicesY += h + 10
	}

	panel(dst, l.logPanel)
	logY := l.logPanel.Y + 16
	for i, line := range v.Log {
		clr := themeMuted
		if i == 0 {
			clr = themeInk
		}
		logY += drawWrapped(dst, line, g.theme.Face(faceSmall),
			l.logPanel.X+18, logY, l.logPanel.W-36, clr) + 6
		if logY > l.logPanel.Y+l.logPanel.H-24 {
			break
		}
	}
}

// digitKey maps choice index 0-8 to the number row keys.
func digitKey(i int) ebiten.Key {
	switch i {
	case 0:
		return ebiten.Key1
	case 1:
		return ebiten.Key2
	case 2:
		return ebiten.Key3
	case 3:
		return ebiten.Key4
	case 4:
		return ebiten.Key5
	case 5:
		return ebiten.Key6
	case 6:
		return ebiten.Key7
	case 7:
		return ebiten.Key8
	case 8:
		return ebiten.Key9
	}
	return ebiten.KeyEscape
}

// chronicleScreen lists completed runs.
type chronicleScreen struct{}

func (chronicleScreen) update(g *Game) error {
	if g.back() {
		g.popScreen()
	}
	return nil
}

func (chronicleScreen) draw(g *Game, dst *ebiten.Image) {
	dst.Fill(themeBackground)
	drawText(dst, "Chronicle", g.theme.DisplayFace(faceHeader), 48, 40, themeGold)
	c := g.model.StatsPage()
	if len(c.Runs) == 0 {
		drawText(dst, "No campaigns recorded yet.", g.theme.Face(faceBody), 52, 120, themeMuted)
		return
	}
	y := 120.0
	for i, run := range c.Runs {
		drawText(dst, fmt.Sprintf("%d. %s — legacy %d, %d turns",
			len(c.Runs)-i, run.Ending, run.Stats.Legacy, run.Turns),
			g.theme.Face(faceBody), 52, y, themeInk)
		y += 28
	}
}
