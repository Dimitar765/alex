package app

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"goGame/internal/content"
	"goGame/internal/game"
)

// Player-facing refusals. The engine's own errors pass through verbatim.
var (
	errNoRun        = errPlayer("no campaign is underway")
	errCampaignOver = errPlayer("the campaign is over — start a new game")
	errNoSuchChoice = errPlayer("no such choice")
)

// errPlayer marks a refusal meant for display in the log pane.
type playerError struct{ msg string }

func (e playerError) Error() string { return e.msg }

func errPlayer(msg string) error { return playerError{msg} }

// EndingTile is one slot in the endings gallery.
type EndingTile struct {
	Class      string
	Discovered bool
}

// PileEntry is one aggregated deck/discard line.
type PileEntry struct {
	ID    string
	Name  string
	Count int
}

// CardView is one hand card as the UI draws it.
type CardView struct {
	ID       string
	Name     string
	Text     string
	Cost     int
	Playable bool
}

// ChoiceView is one scene option as the UI draws it.
type ChoiceView struct {
	Index     int
	Text      string
	Requires  string
	Available bool
}

// View is everything needed to draw one frame: a pure projection of the
// model, safe to hold while animations play.
type View struct {
	SceneID string
	Scene   content.Scene
	Stats   game.Stats
	Fate    bool // the last action resolved a random effect

	Choices []ChoiceView
	Hand    []CardView

	Deck         []PileEntry // every owned card across piles, sorted by ID
	DeckTotal    int
	DeckCount    int // draw pile size
	Discard      []PileEntry
	DiscardCount int

	Log         []string // newest first
	Turns       int
	CardsPlayed int

	Campaigns int
	Endings   []EndingTile
}

// view projects the live state for drawing.
func (m *Model) view() *View { return m.project(m.st, nil) }

// View returns the current projection without acting.
func (m *Model) View() *View { return m.view() }

// project builds draw data from st. A nil st projects the title-screen
// zero view. A non-nil err is surfaced as the newest log line only; the
// underlying state is unchanged.
func (m *Model) project(st *game.State, err error) *View {
	v := &View{
		Campaigns: len(m.runs),
		Endings:   m.gallery(),
	}
	if st == nil {
		return v
	}
	scene := m.lib.Scenes[st.SceneID]
	v.SceneID = st.SceneID
	v.Scene = scene
	v.Stats = st.Stats
	v.Fate = st.Rolled
	v.Turns = st.Turns
	v.CardsPlayed = st.CardsPlayed

	// Deck inspector: every card the run owns, across all piles.
	counts := map[string]int{}
	for _, pile := range [][]string{st.Hand, st.Deck, st.Discard} {
		for _, id := range pile {
			counts[id]++
		}
	}
	for _, id := range slices.Sorted(maps.Keys(counts)) {
		v.Deck = append(v.Deck, PileEntry{ID: id, Name: m.cardName(id), Count: counts[id]})
		v.DeckTotal += counts[id]
	}
	v.DeckCount = len(st.Deck)
	v.DiscardCount = len(st.Discard)

	dcounts := map[string]int{}
	for _, id := range st.Discard {
		dcounts[id]++
	}
	for _, id := range slices.Sorted(maps.Keys(dcounts)) {
		v.Discard = append(v.Discard, PileEntry{ID: id, Name: m.cardName(id), Count: dcounts[id]})
	}

	log := slices.Clone(st.Log)
	if err != nil {
		log = append(log, "Error: "+err.Error())
	}
	slices.Reverse(log)
	v.Log = log

	for i, c := range scene.Choices {
		v.Choices = append(v.Choices, ChoiceView{
			Index:     i,
			Text:      c.Text,
			Requires:  m.requires(c),
			Available: st.CanChoose(c),
		})
	}
	for _, id := range st.Hand {
		c := m.lib.Cards[id]
		v.Hand = append(v.Hand, CardView{
			ID:       id,
			Name:     c.Name,
			Text:     c.Text,
			Cost:     c.Cost,
			Playable: st.CanPlay(id, m.lib.Cards),
		})
	}
	return v
}

func (m *Model) cardName(id string) string {
	if c, ok := m.lib.Cards[id]; ok && c.Name != "" {
		return c.Name
	}
	return id
}

// requires formats a choice's unmet-requirement hint (e.g. "Phalanx · Legacy 2").
func (m *Model) requires(c content.Choice) string {
	var reqs []string
	if c.RequiresCard != "" {
		label := m.cardName(c.RequiresCard)
		if c.ConsumesCard {
			label += " (spent)"
		}
		reqs = append(reqs, label)
	}
	for _, k := range slices.Sorted(maps.Keys(c.RequiresStat)) {
		reqs = append(reqs, fmt.Sprintf("%s %d", game.StatLabel(k), c.RequiresStat[k]))
	}
	return strings.Join(reqs, " · ")
}

// gallery builds the endings tiles over content.EndingOrder.
func (m *Model) gallery() []EndingTile {
	found := map[string]bool{}
	for _, run := range m.runs {
		found[run.Ending] = true
	}
	tiles := make([]EndingTile, 0, len(content.EndingOrder))
	for _, class := range content.EndingOrder {
		tiles = append(tiles, EndingTile{Class: class, Discovered: found[class]})
	}
	return tiles
}

// Chronicle summarizes the completed runs for the stats screen.
type Chronicle struct {
	Runs    []RunSummary // newest first
	Endings []struct {
		Class string
		Count int
	}
	AvgTurns float64
	AvgCards float64
	Best     *RunSummary
}

// StatsPage projects the completed-run history.
func (m *Model) StatsPage() Chronicle {
	runs := slices.Clone(m.runs)
	slices.Reverse(runs)
	var c Chronicle
	c.Runs = runs
	counts := map[string]int{}
	var turns, cards int
	for _, run := range m.runs {
		counts[run.Ending]++
		turns += run.Turns
		cards += run.CardsPlayed
		if c.Best == nil || run.Stats.Legacy > c.Best.Stats.Legacy {
			b := run
			c.Best = &b
		}
	}
	if n := len(m.runs); n > 0 {
		c.AvgTurns = float64(turns) / float64(n)
		c.AvgCards = float64(cards) / float64(n)
	}
	for _, class := range content.EndingOrder {
		if counts[class] > 0 {
			c.Endings = append(c.Endings, struct {
				Class string
				Count int
			}{class, counts[class]})
		}
	}
	return c
}
