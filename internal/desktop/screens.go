package desktop

import (
	"fmt"
	"image/color"

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

// tableScreen is the main play view; its full render lands in Phase 3.
type tableScreen struct{}

func (tableScreen) update(g *Game) error {
	if g.keysPressed[ebiten.KeyEscape] {
		g.popScreen()
	}
	return nil
}

func (s tableScreen) draw(g *Game, dst *ebiten.Image) {
	dst.Fill(themeBackground)
	v := g.model.View()
	drawTable(g, dst, v)
}

// drawTable renders the play view: fan, deck row, scene panel, log.
func drawTable(g *Game, dst *ebiten.Image, v *app.View) {
	l := newLayout()
	panel(dst, l.scenePanel)
	drawText(dst, v.Scene.Text, g.theme.Face(faceScene),
		l.scenePanel.X+20, l.scenePanel.Y+18, themeInk)

	statsY := l.scenePanel.Y + 20 + wrappedHeight(v.Scene.Text, g.theme.Face(faceScene), l.scenePanel.W-40) + 10
	stats := fmt.Sprintf("Legacy %d   Army %d   Treasury %d", v.Stats.Legacy, v.Stats.Army, v.Stats.Treasury)
	drawText(dst, stats, g.theme.Face(faceBody), l.scenePanel.X+20, statsY, themeGold)

	choicesY := statsY + 34
	for _, c := range v.Choices {
		clr := themeInk
		if !c.Available {
			clr = themeMuted
		}
		drawWrapped(dst, fmt.Sprintf("%d. %s", c.Index+1, c.Text),
			g.theme.Face(faceBody), l.scenePanel.X+20, choicesY, l.scenePanel.W-40, clr)
		choicesY += wrappedHeight(c.Text, g.theme.Face(faceBody), l.scenePanel.W-40) + 8
	}

	panel(dst, l.logPanel)
	logY := l.logPanel.Y + 16
	for i, line := range v.Log {
		clr := color.RGBA(themeMuted)
		if i == 0 {
			clr = color.RGBA(themeInk)
		}
		logY += drawWrapped(dst, line, g.theme.Face(faceSmall),
			l.logPanel.X+18, logY, l.logPanel.W-36, clr) + 6
		if logY > l.logPanel.Y+l.logPanel.H-24 {
			break
		}
	}
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
