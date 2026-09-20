package desktop

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// reducedMotion disables ambient animation when the player asks for a
// calmer table. Wired to a settings toggle in Phase 7.
var reducedMotion = false

// cardAnim is the animated state of one hand-card slot. Values ease
// toward their targets every tick, so layout changes read as motion.
type cardAnim struct {
	X, Y, Angle, Scale float64
	spawned            bool
}

// settle eases the slot toward its target. First appearances snap to the
// deck-side spawn point so new cards visibly fly in from the pile.
func (c *cardAnim) settle(t cardSlot, dt float64, fromDeck bool) {
	if !c.spawned {
		if fromDeck {
			c.X, c.Y, c.Angle, c.Scale = ScreenW+80, t.Y+60, 0.35, 0.7
		} else {
			c.X, c.Y, c.Angle, c.Scale = t.X, t.Y, t.Angle, 1
		}
		c.spawned = true
		return
	}
	c.approach(t, dt)
}

// snapTo jumps straight to the target (used when motion is disabled).
func (c *cardAnim) snapTo(t cardSlot) {
	c.X, c.Y, c.Angle, c.Scale = t.X, t.Y, t.Angle, 1
	c.spawned = true
}

// approach eases X/Y/Angle/Scale toward the slot targets.
func (c *cardAnim) approach(t cardSlot, dt float64) {
	k := approachK(dt, 14)
	c.X += (t.X - c.X) * k
	c.Y += (t.Y - c.Y) * k
	c.Angle += (t.Angle - c.Angle) * k
	c.Scale += (1 - c.Scale) * k
}

// approachK converts a half-life-ish speed into the per-tick blend factor.
func approachK(dt, speed float64) float64 {
	if dt <= 0 {
		return 0
	}
	k := 1 - expNeg(speed*dt)
	if k > 1 {
		return 1
	}
	return k
}

func expNeg(x float64) float64 {
	// exp(-x) via squaring keeps it dependency-free and is plenty for FX.
	n := 1 + x/16
	x2 := n * n
	x4 := x2 * x2
	x8 := x4 * x4
	x16 := x8 * x8
	return 1 / x16
}

// ghostCard is a card that left the hand: it flies toward the play side
// while fading out.
type ghostCard struct {
	img                *ebiten.Image
	x, y, angle, scale float64
	vx, vy, spin       float64
	alpha              float64
}

func (gc *ghostCard) update(dt float64) bool {
	gc.x += gc.vx * dt
	gc.y += gc.vy * dt
	gc.angle += gc.spin * dt
	gc.alpha -= dt * 2.6
	return gc.alpha > 0
}

func (gc *ghostCard) draw(dst *ebiten.Image) {
	opts := &ebiten.DrawImageOptions{}
	opts.Filter = ebiten.FilterLinear
	opts.GeoM.Translate(-cardW/2, -cardH/2)
	opts.GeoM.Scale(gc.scale, gc.scale)
	opts.GeoM.Rotate(gc.angle)
	opts.GeoM.Translate(gc.x, gc.y)
	opts.ColorScale.Scale(1, 1, 1, float32(gc.alpha))
	dst.DrawImage(gc.img, opts)
}

// floatText is a rising "+1" style note.
type floatText struct {
	text string
	x, y float64
	life float64
	clr  color.RGBA
}

func (f *floatText) update(dt float64) bool {
	f.y -= 34 * dt
	f.life -= dt
	return f.life > 0
}

func (f *floatText) draw(dst *ebiten.Image, g *Game) {
	a := f.life
	if a > 1 {
		a = 1
	}
	clr := f.clr
	clr.A = uint8(float64(clr.A) * a)
	drawText(dst, f.text, g.theme.Face(faceBody), f.x, f.y, clr)
}

// particle is one tiny spark: dust, gold rise, or embers.
type particle struct {
	x, y, vx, vy  float64
	life, maxLife float64
	size          float32
	clr           color.RGBA
}

func (p *particle) update(dt float64) bool {
	p.x += p.vx * dt
	p.y += p.vy * dt
	p.life -= dt
	return p.life > 0
}

func (p *particle) draw(dst *ebiten.Image) {
	k := p.life / p.maxLife
	clr := p.clr
	clr.A = uint8(float64(clr.A) * k * 0.8)
	vector.DrawFilledCircle(dst, float32(p.x), float32(p.y), float32(float64(p.size)*k+0.5), clr, true)
}
