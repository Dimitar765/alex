package sim

import (
	"testing"

	"goGame/internal/content"

	assets "goGame"
)

func shippedLib(t *testing.T) *content.Library {
	t.Helper()
	lib, err := content.Load(assets.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	return lib
}

// The safety proof: random-greedy play must never soft-lock on the shipped
// campaign — every run reaches a declared ending within the turn cap.
func TestNoStuckRunsOnShippedContent(t *testing.T) {
	lib := shippedLib(t)
	report := Aggregate(Simulate(lib, 500, 1))
	if report.Stuck != 0 {
		t.Fatalf("soft-locked runs detected at %v", report.StuckHere)
	}
	if report.Runs != 500 || len(report.Endings) == 0 {
		t.Fatalf("bad report: %+v", report)
	}
	if report.TurnsAvg <= 0 || report.TurnsMax >= MaxTurns {
		t.Fatalf("implausible turn counts: avg %.1f max %d (cap %d)", report.TurnsAvg, report.TurnsMax, MaxTurns)
	}
}

// Every ending class that exists in the shipped content must be reachable
// by undirected play.
func TestAllShippedEndingsReachable(t *testing.T) {
	lib := shippedLib(t)
	report := Aggregate(Simulate(lib, 500, 7))
	want := map[string]bool{}
	for _, sc := range lib.Scenes {
		if sc.Ending != "" {
			want[sc.Ending] = true
		}
	}
	for class := range want {
		if report.Endings[class] == 0 {
			t.Fatalf("ending %q never observed in 500 runs; distribution:\n%s", class, report)
		}
	}
}

// Same seed must reproduce the same campaign outcomes exactly.
func TestSimulateDeterministic(t *testing.T) {
	lib := shippedLib(t)
	a := Simulate(lib, 25, 42)
	b := Simulate(lib, 25, 42)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("run %d differs between simulations: %+v vs %+v", i, a[i], b[i])
		}
	}
}

func TestAggregateCounts(t *testing.T) {
	r := Aggregate([]Result{
		{Ending: "triumph", Turns: 10},
		{Ending: "triumph", Turns: 20},
		{Ending: "stuck", SceneID: "vault", Turns: MaxTurns},
	})
	if r.Runs != 3 || r.Endings["triumph"] != 2 || r.Stuck != 1 || r.StuckHere["vault"] != 1 {
		t.Fatalf("bad aggregate: %+v", r)
	}
	if r.TurnsAvg != float64(10+20+MaxTurns)/3 {
		t.Fatalf("bad average: %.2f", r.TurnsAvg)
	}
	if r.TurnsMax != MaxTurns {
		t.Fatalf("bad max: %d", r.TurnsMax)
	}
}
