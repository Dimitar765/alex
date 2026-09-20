// Package sim plays automated campaigns against the rules engine to prove
// content health: no soft-locked runs, every ending reachable, and a
// measured ending distribution for balance work.
package sim

import (
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"

	"goGame/internal/content"
	"goGame/internal/game"
)

// MaxTurns bounds one simulated run; hitting it counts as stuck.
const MaxTurns = 150

// cardPlayOdds is the chance per turn that the policy develops resources
// by playing a card instead of taking a choice.
const cardPlayOdds = 2 // out of 5

// Result summarizes one simulated campaign.
type Result struct {
	Ending  string // ending classification, or "stuck"
	SceneID string // final scene
	Turns   int
}

// Report aggregates simulation results.
type Report struct {
	Runs      int
	Endings   map[string]int
	TurnsAvg  float64
	TurnsMax  int
	Stuck     int
	StuckHere map[string]int // scene -> times stuck there
}

// Simulate plays n campaigns with the random-greedy policy, deterministically
// for the given seed, and returns the per-run results.
func Simulate(lib *content.Library, n int, seed uint64) []Result {
	results := make([]Result, 0, n)
	for i := 0; i < n; i++ {
		rng := rand.New(rand.NewPCG(seed, uint64(i)))
		results = append(results, play(NewRun(lib, rng), lib, rng))
	}
	return results
}

// NewRun starts a fresh state with an explicit random source.
func NewRun(lib *content.Library, rng *rand.Rand) *game.State {
	return game.NewStateWithRNG(lib, "title", rng)
}

// play drives one run to an ending (or the turn cap) and returns its result.
func play(st *game.State, lib *content.Library, rng *rand.Rand) Result {
	for st.Turns < MaxTurns {
		scene := lib.Scenes[st.SceneID]
		if scene.Ending != "" {
			return Result{Ending: scene.Ending, SceneID: scene.ID, Turns: st.Turns}
		}
		if act(st, lib, rng) {
			continue
		}
		return Result{Ending: "stuck", SceneID: st.SceneID, Turns: st.Turns}
	}
	return Result{Ending: "stuck", SceneID: st.SceneID, Turns: st.Turns}
}

// act takes one turn: with a fixed chance it plays a random playable card
// to develop resources, otherwise it takes a random available choice. It
// reports false only when neither is possible (a true deadlock).
func act(st *game.State, lib *content.Library, rng *rand.Rand) bool {
	playable := make([]string, 0, game.HandSize)
	for _, id := range st.Hand {
		if st.CanPlay(id, lib.Cards) {
			playable = append(playable, id)
		}
	}
	if len(playable) > 0 && rng.IntN(5) < cardPlayOdds {
		_ = st.PlayCard(playable[rng.IntN(len(playable))], lib)
		return true
	}
	scene := lib.Scenes[st.SceneID]
	available := make([]int, 0, len(scene.Choices))
	for i, ch := range scene.Choices {
		if st.CanChoose(ch) {
			available = append(available, i)
		}
	}
	if len(available) == 0 {
		if len(playable) == 0 {
			return false
		}
		_ = st.PlayCard(playable[rng.IntN(len(playable))], lib)
		return true
	}
	ch := scene.Choices[available[rng.IntN(len(available))]]
	_ = st.Choose(ch, lib)
	return true
}

// Aggregate summarizes simulation results.
func Aggregate(results []Result) Report {
	r := Report{Runs: len(results), Endings: map[string]int{}, StuckHere: map[string]int{}}
	total := 0
	for _, res := range results {
		r.Endings[res.Ending]++
		if res.Ending == "stuck" {
			r.Stuck++
			r.StuckHere[res.SceneID]++
		}
		total += res.Turns
		if res.Turns > r.TurnsMax {
			r.TurnsMax = res.Turns
		}
	}
	if r.Runs > 0 {
		r.TurnsAvg = float64(total) / float64(r.Runs)
	}
	return r
}

// String renders the report as an aligned plain-text block.
func (r Report) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "runs        %d\n", r.Runs)
	classes := make([]string, 0, len(r.Endings))
	for c := range r.Endings {
		classes = append(classes, c)
	}
	sort.Slice(classes, func(i, j int) bool {
		if classes[i] == "stuck" {
			return false // stuck always last
		}
		if classes[j] == "stuck" {
			return true
		}
		return r.Endings[classes[i]] > r.Endings[classes[j]]
	})
	for _, c := range classes {
		pct := 100 * float64(r.Endings[c]) / float64(r.Runs)
		fmt.Fprintf(&b, "%-11s %4d (%4.1f%%)\n", c, r.Endings[c], pct)
	}
	fmt.Fprintf(&b, "turns avg   %.1f, max %d\n", r.TurnsAvg, r.TurnsMax)
	if r.Stuck > 0 {
		fmt.Fprintf(&b, "STUCK RUNS at: %v\n", r.StuckHere)
	}
	return b.String()
}
