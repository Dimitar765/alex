package desktop

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"

	"goGame/internal/app"
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
type titleScreen struct{}

func (titleScreen) update(g *Game) error {
	if g.keysPressed[ebiten.KeyEscape] {
		return ebiten.Termination
	}
	return nil
}

func (titleScreen) draw(g *Game, dst *ebiten.Image) {
	dst.Fill(themeBackground)
	drawText(dst, "Alexander", g.theme.DisplayFace(faceTitle), 48, 40, themeGold)
	drawText(dst, "a card adventure", g.theme.FaceItalic(faceScene), 52, 108, themeMuted)

	// Menu: Continue first when a campaign is underway.
	var labels []string
	if g.model.HasRun() {
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
	for i := range menu {
		b := &menu[i]
		b.draw(g, dst)
		if !b.clicked(g) {
			continue
		}
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
type tableScreen struct {
	cards *cardCache
}

func (s *tableScreen) ensureCache() {
	if s.cards == nil {
		s.cards = newCardCache()
	}
}

func (tableScreen) update(g *Game) error {
	if g.keysPressed[ebiten.KeyEscape] {
		g.popScreen()
	}
	return nil
}

func (s tableScreen) draw(g *Game, dst *ebiten.Image) {
	s.ensureCache()
	dst.Fill(themeBackground)
	v := g.model.View()
	l := newLayout()
	drawTable(g, dst, v)

	// Fan of hand cards in the table column, below the scene panel band.
	if v.Scene.Ending == "" {
		positions := fanPositions(len(v.Hand), l.table.W)
		for i, c := range v.Hand {
			p := positions[i]
			// Hover lift: cards rise toward the cursor like the web fan.
			hover := Rectangle{X: p.X - cardW/2, Y: p.Y - cardH/2, W: cardW, H: cardH}.
				Contains(g.cursorX, g.cursorY)
			if hover && c.Playable {
				p.Y -= 16
			}
			s.cards.drawCard(dst, s.cards.face(g, c), p.X, p.Y, p.Angle, 1, !c.Playable)
			if hover && c.Playable && g.mouseClicked {
				if r := g.model.PlayCard(c.ID); r.Err == nil {
					g.consumeEffects(r)
				}
			}
		}

		// Deck row under the fan.
		rowY := l.table.Y + l.table.H - 96
		drawText(dst, fmt.Sprintf("Deck · %d", v.DeckCount), g.theme.Face(faceSmall),
			l.table.X+4, rowY, themeGold)
		drawText(dst, fmt.Sprintf("Discard · %d", v.DiscardCount), g.theme.Face(faceSmall),
			l.table.X+4, rowY+30, themeMuted)

		shuffleLabel := fmt.Sprintf("Shuffle · %d", v.DiscardCount)
		shuffle := button{Rect: Rectangle{X: l.table.X + l.table.W - 110, Y: rowY - 4, W: 110, H: 36}, Label: shuffleLabel}
		scout := button{Rect: Rectangle{X: l.table.X + l.table.W - 228, Y: rowY - 4, W: 110, H: 36}, Label: "Scout"}
		if scout.clicked(g) {
			g.model.Scout()
		}
		if shuffle.clicked(g) {
			g.model.Shuffle()
		}
		scout.draw(g, dst)
		shuffle.draw(g, dst)
	}
}

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
func drawTable(g *Game, dst *ebiten.Image, v *app.View) {
	l := newLayout()
	panel(dst, l.scenePanel)

	textH := wrappedHeight(v.Scene.Text, g.theme.Face(faceScene), l.scenePanel.W-40)
	drawWrapped(dst, v.Scene.Text, g.theme.Face(faceScene),
		l.scenePanel.X+20, l.scenePanel.Y+18, l.scenePanel.W-40, themeInk)

	statsY := l.scenePanel.Y + 18 + textH + 10
	stats := fmt.Sprintf("Legacy %d   Army %d   Treasury %d",
		v.Stats.Legacy, v.Stats.Army, v.Stats.Treasury)
	drawText(dst, stats, g.theme.Face(faceBody), l.scenePanel.X+20, statsY, themeGold)

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
		if again.clicked(g) {
			g.model.NewGame()
		}
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
					g.consumeEffects(r)
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

// endingScreen shows the run summary once the campaign is over.
type endingScreen struct{}

func (endingScreen) update(g *Game) error {
	if g.keysPressed[ebiten.KeyEscape] {
		g.popScreen()
	}
	return nil
}

func (endingScreen) draw(g *Game, dst *ebiten.Image) {
	dst.Fill(themeBackground)
	drawText(dst, "The campaign ends", g.theme.DisplayFace(faceHeader), 48, 60, themeGold)
}

// chronicleScreen lists completed runs.
type chronicleScreen struct{}

func (chronicleScreen) update(g *Game) error {
	if g.keysPressed[ebiten.KeyEscape] || g.keysPressed[ebiten.KeyBackspace] {
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
