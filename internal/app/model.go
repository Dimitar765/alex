// Package app is the UI-agnostic view-model for Alexander: it wraps the
// game rules and content with everything a client needs to draw and act —
// projected views, structured FX events, run history, and persistence.
// The web build and the Ebitengine build are both thin skins over this.
package app

import (
	"slices"
	"time"

	"goGame/internal/content"
	"goGame/internal/game"
)

// StartScene is the ID of the campaign's opening scene.
const StartScene = "title"

// Model is one local player's game: the live state (nil until a campaign
// starts), completed-run history, and the save directory ("" keeps
// everything in memory, which tests use).
type Model struct {
	lib        *content.Library
	st         *game.State
	runs       []RunSummary
	dir        string
	startScene string
}

// RunSummary records one completed campaign for the gallery and chronicle.
type RunSummary struct {
	Ending      string     `json:"ending"`
	Stats       game.Stats `json:"stats"`
	Turns       int        `json:"turns"`
	CardsPlayed int        `json:"cardsPlayed"`
	Date        time.Time  `json:"date"`
}

// NewModel builds a model persisting to dir. An existing save and history
// are loaded; a corrupt pair reads as empty rather than failing.
func NewModel(lib *content.Library, dir string) *Model {
	m := &Model{lib: lib, dir: dir, startScene: StartScene}
	m.st = m.loadState()
	m.runs = m.loadHistory()
	return m
}

// HasRun reports whether a campaign is underway (even one that ended).
func (m *Model) HasRun() bool { return m.st != nil }

// History returns the completed runs, oldest first.
func (m *Model) History() []RunSummary { return slices.Clone(m.runs) }

// Result is the outcome of an action: the projected view (always present,
// reflecting the unchanged state on refusal), the engine error (nil on
// success), and the derived FX effects (empty on refusal).
type Result struct {
	View    *View
	Err     error
	Effects []Effect
}

// NewGame starts a fresh campaign, replacing any current one. An abandoned
// run is not recorded in history — only endings are.
func (m *Model) NewGame() Result {
	m.st = game.NewState(m.lib, m.startScene)
	m.saveState()
	return Result{View: m.view()}
}

// PlayCard plays a hand card by ID.
func (m *Model) PlayCard(id string) Result {
	return m.act(func(st *game.State) error {
		return st.PlayCard(id, m.lib)
	})
}

// Choose takes the scene choice at index i.
func (m *Model) Choose(i int) Result {
	return m.act(func(st *game.State) error {
		scene := m.lib.Scenes[st.SceneID]
		if i < 0 || i >= len(scene.Choices) {
			return errNoSuchChoice
		}
		return st.Choose(scene.Choices[i], m.lib)
	})
}

// Scout reveals the deck's top card; the turn is spent.
func (m *Model) Scout() Result {
	return m.act(func(st *game.State) error {
		_, err := st.Peek(m.lib)
		return err
	})
}

// Shuffle recycles the discard pile into the deck for one treasury.
func (m *Model) Shuffle() Result {
	return m.act(func(st *game.State) error {
		return st.Shuffle()
	})
}

// act applies fn to a clone and, on success, commits it, persists it, and
// derives FX effects from the before/after diff. On failure the state is
// untouched and the refusal surfaces in the view's log.
func (m *Model) act(fn func(*game.State) error) Result {
	if m.st == nil {
		return Result{View: m.project(nil, errNoRun), Err: errNoRun}
	}
	if m.lib.Scenes[m.st.SceneID].Ending != "" {
		return Result{View: m.project(m.st, errCampaignOver), Err: errCampaignOver}
	}
	next := m.st.Clone()
	err := fn(next)
	if err != nil {
		return Result{View: m.project(m.st, err), Err: err}
	}

	effects := diffEffects(m.lib, m.st, next)
	if m.lib.Scenes[next.SceneID].Ending != "" {
		effects = append(effects, Effect{Kind: EffectEnding, Text: m.lib.Scenes[next.SceneID].Ending})
		m.recordRun(next)
	}
	m.st = next
	m.saveState()
	return Result{View: m.project(next, nil), Effects: effects}
}

// recordRun appends a completed run to the history and persists it.
func (m *Model) recordRun(st *game.State) {
	m.runs = append(m.runs, RunSummary{
		Ending:      m.lib.Scenes[st.SceneID].Ending,
		Stats:       st.Stats,
		Turns:       st.Turns,
		CardsPlayed: st.CardsPlayed,
		Date:        time.Now(),
	})
	m.saveHistory()
}

// diffEffects compares the pre-action and post-action states and reports
// everything a client might animate, in deterministic order.
func diffEffects(lib *content.Library, prev, next *game.State) []Effect {
	var fx []Effect
	for _, key := range []string{content.StatLegacy, content.StatArmy, content.StatTreasury} {
		d := next.Stat(key) - prev.Stat(key)
		if d != 0 {
			fx = append(fx, Effect{Kind: EffectStat, Stat: key, Delta: d})
		}
	}
	prevHand, nextHand := counts(prev.Hand), counts(next.Hand)
	for _, id := range sortedKeys(nextHand, prevHand) {
		if d := nextHand[id] - prevHand[id]; d < 0 {
			fx = append(fx, Effect{Kind: EffectCardLeft, CardID: id})
		}
	}
	prevPool := counts(append(append(slices.Clone(prev.Hand), prev.Deck...), prev.Discard...))
	nextPool := counts(append(append(slices.Clone(next.Hand), next.Deck...), next.Discard...))
	for _, id := range sortedKeys(nextPool, prevPool) {
		if d := nextPool[id] - prevPool[id]; d > 0 {
			fx = append(fx, Effect{Kind: EffectCardGained, CardID: id})
		} else if d < 0 {
			fx = append(fx, Effect{Kind: EffectCardGone, CardID: id})
		}
	}
	if next.SceneID != prev.SceneID {
		fx = append(fx, Effect{Kind: EffectSceneChanged, From: prev.SceneID, To: next.SceneID})
	}
	if next.Rolled {
		fx = append(fx, Effect{Kind: EffectFate})
	}
	return fx
}

// counts tallies multiset membership.
func counts(ids []string) map[string]int {
	c := make(map[string]int, len(ids))
	for _, id := range ids {
		c[id]++
	}
	return c
}

// sortedKeys returns the union of both maps' keys in stable order.
func sortedKeys(a, b map[string]int) []string {
	seen := map[string]bool{}
	var keys []string
	for _, m := range []*map[string]int{&a, &b} {
		for k := range *m {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	return slices.Sorted(slices.Values(keys))
}
