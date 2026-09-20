package desktop

import (
	"testing"

	"goGame/internal/game"
)

// TestFanPositionsFitTable verifies the fan stays inside the table
// column for every legal hand size: the outermost card's edge must not
// cross the column, and cards must be ordered left to right.
func TestFanPositionsFitTable(t *testing.T) {
	const tableW = 570.0
	for n := 1; n <= game.HandSize; n++ {
		positions := fanPositions(n, tableW)
		if len(positions) != n {
			t.Fatalf("n=%d: got %d positions", n, len(positions))
		}
		for i, p := range positions {
			half := cardW / 2.0
			// Rotation can extend the footprint; give it slack.
			if p.X-half < -24 || p.X+half > tableW+24 {
				t.Fatalf("n=%d card %d: x=%.1f out of table [0, %.0f]", n, i, p.X, tableW)
			}
			if i > 0 && positions[i].X <= positions[i-1].X {
				t.Fatalf("n=%d: card %d not right of card %d", n, i, i-1)
			}
			if p.Y != 150+abs(float64(i)-float64(n-1)/2)*14 {
				t.Fatalf("n=%d card %d: unexpected arc drop y=%.1f", n, i, p.Y)
			}
		}
	}
}
