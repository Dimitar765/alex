package desktop

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// stickDeadzone is the axis magnitude treated as "pressed".
const stickDeadzone = 0.5

// padTick is the just-pressed state of the standard gamepad controls the
// UI uses, aggregated over all connected pads (any pad acts). Mirrors
// pad.js: d-pad or left stick moves, A confirms, B backs out.
type padTick struct {
	left, right, up, down bool
	a, b                  bool
}

// padTick samples just-pressed controls across every connected gamepad.
// Stick axes use edge detection against the previous tick's position,
// stored on the Game.
func (g *Game) padTick() (t padTick) {
	ids := make([]ebiten.GamepadID, 0, 4)
	ids = ebiten.AppendGamepadIDs(ids)
	for _, id := range ids {
		just := func(b ebiten.StandardGamepadButton) bool {
			return inpututil.IsStandardGamepadButtonJustPressed(id, b)
		}
		t.left = t.left || just(ebiten.StandardGamepadButtonLeftLeft)
		t.right = t.right || just(ebiten.StandardGamepadButtonLeftRight)
		t.up = t.up || just(ebiten.StandardGamepadButtonLeftTop)
		t.down = t.down || just(ebiten.StandardGamepadButtonLeftBottom)
		t.a = t.a || just(ebiten.StandardGamepadButtonRightBottom)
		t.b = t.b || just(ebiten.StandardGamepadButtonLeftBottom)

		// Left stick edges: a crossing of the deadzone counts once.
		x := ebiten.StandardGamepadAxisValue(id, ebiten.StandardGamepadAxisLeftStickHorizontal)
		y := ebiten.StandardGamepadAxisValue(id, ebiten.StandardGamepadAxisLeftStickVertical)
		if g.stickX <= stickDeadzone && x > stickDeadzone {
			t.right = true
		}
		if g.stickX >= -stickDeadzone && x < -stickDeadzone {
			t.left = true
		}
		if g.stickY <= -stickDeadzone && y > stickDeadzone {
			t.down = true
		}
		if g.stickY >= stickDeadzone && y < -stickDeadzone {
			t.up = true
		}
		g.stickX, g.stickY = x, y
	}
	return t
}

// horizontalStep reports the -1/+1 horizontal step for this tick
// (arrows or pad d-pad/stick).
func (g *Game) horizontalStep() int {
	step := 0
	if g.keysPressed[ebiten.KeyLeft] {
		step--
	}
	if g.keysPressed[ebiten.KeyRight] {
		step++
	}
	p := g.padTick()
	if p.left {
		step--
	}
	if p.right {
		step++
	}
	return step
}

// verticalStep reports the -1/+1 vertical step (up = -1) for this tick.
func (g *Game) verticalStep() int {
	step := 0
	if g.keysPressed[ebiten.KeyUp] {
		step--
	}
	if g.keysPressed[ebiten.KeyDown] {
		step++
	}
	p := g.padTick()
	if p.up {
		step--
	}
	if p.down {
		step++
	}
	return step
}

// confirm reports whether Enter or gamepad A was just pressed.
func (g *Game) confirm() bool {
	if g.keysPressed[ebiten.KeyEnter] || g.keysPressed[ebiten.KeyNumpadEnter] {
		return true
	}
	return g.padTick().a
}

// back reports whether Escape, Backspace, or gamepad B was just pressed.
func (g *Game) back() bool {
	if g.keysPressed[ebiten.KeyEscape] || g.keysPressed[ebiten.KeyBackspace] {
		return true
	}
	return g.padTick().b
}
