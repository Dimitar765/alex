package game

import (
	"slices"
	"strings"
	"testing"

	"goGame/internal/content"
)

func testCards() ([]content.Card, map[string]content.Card) {
	mk := func(id, name string) content.Card {
		return content.Card{ID: id, Name: name, Text: "flavor " + id}
	}
	cards := []content.Card{
		mk("a", "Alpha"), mk("b", "Bravo"), mk("c", "Charlie"),
		mk("d", "Delta"), mk("e", "Echo"),
	}
	m := map[string]content.Card{}
	for _, c := range cards {
		m[c.ID] = c
	}
	return cards, m
}

func TestNewStateDealsOpeningHand(t *testing.T) {
	cards, _ := testCards()
	s := NewState(cards, "title")
	if s.SceneID != "title" {
		t.Fatalf("SceneID = %q, want title", s.SceneID)
	}
	if len(s.Hand) != HandSize {
		t.Fatalf("opening hand = %d cards, want %d", len(s.Hand), HandSize)
	}
	all := append(slices.Clone(s.Hand), s.Deck...)
	slices.Sort(all)
	want := []string{"a", "b", "c", "d", "e"}
	if !slices.Equal(all, want) {
		t.Fatalf("hand+deck = %v, want %v", all, want)
	}
	// Shuffle must actually spread the deck: hand is not the sorted prefix.
	if slices.IsSorted(s.Hand) {
		t.Log("warning: shuffle produced sorted hand (1/4! chance); not a failure")
	}
}

func TestCloneIsIndependent(t *testing.T) {
	s := &State{
		SceneID: "x",
		Stats:   Stats{Legacy: 1},
		Hand:    []string{"a"},
		Deck:    []string{"b"},
		Discard: []string{"c"},
		Log:     []string{"line"},
	}
	c := s.Clone()
	c.SceneID = "y"
	c.Stats.Army = 9
	c.Hand[0] = "z"
	c.Deck[0] = "z"
	c.Discard[0] = "z"
	c.Log[0] = "z"
	if s.SceneID != "x" || s.Stats.Army != 0 ||
		s.Hand[0] != "a" || s.Deck[0] != "b" || s.Discard[0] != "c" || s.Log[0] != "line" {
		t.Fatalf("original mutated via clone: %+v", s)
	}
}

func TestChooseRequiresStat(t *testing.T) {
	_, m := testCards()
	choice := content.Choice{
		Text:         "Break the center",
		RequiresStat: map[string]int{"army": 3, "legacy": 1},
	}
	s := &State{SceneID: "issus", Stats: Stats{Army: 2, Legacy: 5}, Hand: []string{"a"}}
	err := s.Choose(choice, m)
	if err == nil || !strings.Contains(err.Error(), "requires Army 3 (you have 2)") {
		t.Fatalf("Choose() error = %v, want Army shortfall named", err)
	}
	if s.SceneID != "issus" {
		t.Fatalf("rejected choice must not move the scene: %q", s.SceneID)
	}
	s.Stats.Army = 3
	if err := s.Choose(choice, m); err != nil {
		t.Fatalf("Choose() with met requirements failed: %v", err)
	}
}

func TestCanChooseReportsAvailability(t *testing.T) {
	choice := content.Choice{
		RequiresCard: "e",
		RequiresStat: map[string]int{"army": 1},
	}
	s := &State{Hand: []string{"a"}}
	if s.CanChoose(choice) {
		t.Fatal("choice must be unavailable without card or stat")
	}
	s.Stats.Army = 1
	if s.CanChoose(choice) {
		t.Fatal("choice must be unavailable without the card")
	}
	s.Hand = []string{"e", "a"}
	if !s.CanChoose(choice) {
		t.Fatal("choice must be available with card and stat")
	}
}

func TestLoseCardMovesToDiscard(t *testing.T) {
	_, m := testCards()
	// A stocked deck keeps drawUp from reshuffling the discard, so the lost
	// cards provably stay in the discard pile for now.
	s := &State{SceneID: "x", Hand: []string{"a", "b"}, Deck: []string{"c", "e", "e", "d", "d", "e"}}
	card := content.Card{ID: "a", Name: "Alpha", Effects: []content.Effect{
		{Kind: content.KindLoseCard, CardID: "c"},
		{Kind: content.KindLoseCard, CardID: "b"},
	}}
	m["a"] = card
	if err := s.PlayCard("a", m); err != nil {
		t.Fatalf("PlayCard() error = %v", err)
	}
	if slices.Contains(s.Hand, "b") || slices.Contains(s.Deck, "c") {
		t.Fatalf("lost cards must leave hand and deck: hand=%v deck=%v", s.Hand, s.Deck)
	}
	// Lost cards surface in the discard and stay in the run.
	for _, id := range []string{"a", "b", "c"} {
		if !slices.Contains(s.Discard, id) {
			t.Fatalf("discard must hold %q: %v", id, s.Discard)
		}
	}
	if !strings.Contains(strings.Join(s.Log, " | "), "lost Charlie") {
		t.Fatalf("log must name the lost card, got %v", s.Log)
	}
}

func TestRemoveCardExilesFromRun(t *testing.T) {
	_, m := testCards()
	s := &State{SceneID: "x", Hand: []string{"a", "b"}, Deck: []string{"c"}, Discard: []string{"d"}}
	choice := content.Choice{Text: "The horse dies", Effects: []content.Effect{
		{Kind: content.KindRemoveCard, CardID: "a"}, // from hand
		{Kind: content.KindRemoveCard, CardID: "c"}, // from deck
		{Kind: content.KindRemoveCard, CardID: "d"}, // from discard
	}}
	if err := s.Choose(choice, m); err != nil {
		t.Fatalf("Choose() error = %v", err)
	}
	piles := append(append(slices.Clone(s.Hand), s.Deck...), s.Discard...)
	for _, id := range []string{"a", "c", "d"} {
		if slices.Contains(piles, id) {
			t.Fatalf("removed card %q still present: %v", id, piles)
		}
	}
	if !strings.Contains(strings.Join(s.Log, " | "), "Alpha is gone for good") {
		t.Fatalf("log must name the exile, got %v", s.Log)
	}
}

func TestRandomSingleOutcomeIsDeterministic(t *testing.T) {
	_, m := testCards()
	s := &State{SceneID: "start", Hand: []string{"a"}}
	choice := content.Choice{Text: "Fate is kind", Effects: []content.Effect{
		{Kind: content.KindRandom, Outcomes: []content.Outcome{{
			Weight:  1,
			Effects: []content.Effect{{Kind: content.KindGoto, Next: "field"}},
		}}},
	}}
	if err := s.Choose(choice, m); err != nil {
		t.Fatalf("Choose() error = %v", err)
	}
	if s.SceneID != "field" {
		t.Fatalf("SceneID = %q, want field", s.SceneID)
	}
}

func TestRandomRespectsWeights(t *testing.T) {
	_, m := testCards()
	rare := 0
	const trials = 3000
	for i := 0; i < trials; i++ {
		s := &State{SceneID: "start", Hand: []string{"a"}}
		choice := content.Choice{Text: "Roll", Effects: []content.Effect{
			{Kind: content.KindRandom, Outcomes: []content.Outcome{
				{Weight: 3, Effects: []content.Effect{{Kind: content.KindStat, Delta: map[string]int{"army": 1}}}},
				{Weight: 1, Effects: []content.Effect{{Kind: content.KindStat, Delta: map[string]int{"legacy": 1}}}},
			}},
		}}
		if err := s.Choose(choice, m); err != nil {
			t.Fatalf("trial %d: %v", i, err)
		}
		if s.Stats.Legacy == 1 {
			rare++
		}
	}
	// Expected 25% of trials; generous bounds keep the test stable.
	if lo, hi := trials*15/100, trials*35/100; rare < lo || rare > hi {
		t.Fatalf("rare outcome hit %d/%d times, want %d..%d", rare, trials, lo, hi)
	}
}

func TestPlayCardCostsTreasury(t *testing.T) {
	_, m := testCards()
	m["a"] = content.Card{ID: "a", Name: "Alpha", Cost: 2, Effects: []content.Effect{
		{Kind: content.KindStat, Delta: map[string]int{"army": 2}},
	}}
	s := &State{SceneID: "x", Hand: []string{"a", "b"}, Stats: Stats{Treasury: 3}}
	if err := s.PlayCard("a", m); err != nil {
		t.Fatalf("PlayCard() error = %v", err)
	}
	if s.Stats.Treasury != 1 || s.Stats.Army != 2 {
		t.Fatalf("cost not paid correctly: %+v", s.Stats)
	}
	if !strings.Contains(strings.Join(s.Log, " | "), "Treasury -2") {
		t.Fatalf("log must name the cost, got %v", s.Log)
	}
}

func TestPlayCardUnaffordable(t *testing.T) {
	_, m := testCards()
	m["a"] = content.Card{ID: "a", Name: "Alpha", Cost: 2}
	s := &State{SceneID: "x", Hand: []string{"a", "b"}, Stats: Stats{Treasury: 1}}
	err := s.PlayCard("a", m)
	if err == nil || !strings.Contains(err.Error(), "cannot afford Alpha (costs 2 treasury)") {
		t.Fatalf("PlayCard() error = %v, want affordability error", err)
	}
	if len(s.Hand) != 2 || s.Stats.Treasury != 1 {
		t.Fatalf("refused play must not mutate state: hand=%v stats=%+v", s.Hand, s.Stats)
	}
	if s.CanPlay("a", m) {
		t.Fatal("CanPlay must report unaffordable")
	}
	s.Stats.Treasury = 2
	if !s.CanPlay("a", m) {
		t.Fatal("CanPlay must report affordable")
	}
}

func TestChooseConsumesRequiredCard(t *testing.T) {
	_, m := testCards()
	choice := content.Choice{
		Text:         "Ride him into the river",
		RequiresCard: "e",
		ConsumesCard: true,
		Effects:      []content.Effect{{Kind: content.KindStat, Delta: map[string]int{"legacy": 2}}},
	}
	// A stocked deck keeps drawUp from recycling the consumed card.
	s := &State{SceneID: "granicus", Hand: []string{"e", "a"}, Deck: []string{"b", "c", "d"}}
	if err := s.Choose(choice, m); err != nil {
		t.Fatalf("Choose() error = %v", err)
	}
	if slices.Contains(s.Hand, "e") || !slices.Contains(s.Discard, "e") {
		t.Fatalf("consumed card must move hand->discard: hand=%v discard=%v", s.Hand, s.Discard)
	}
	if !strings.Contains(strings.Join(s.Log, " | "), "spent Echo") {
		t.Fatalf("log must name the spent card, got %v", s.Log)
	}
}

func TestChooseRequiresCard(t *testing.T) {
	_, m := testCards()
	s := &State{SceneID: "river", Hand: []string{"a", "b"}}
	choice := content.Choice{Text: "Charge", RequiresCard: "e"}
	err := s.Choose(choice, m)
	if err == nil || !strings.Contains(err.Error(), "Echo") {
		t.Fatalf("Choose() error = %v, want error naming Echo", err)
	}
	if s.SceneID != "river" || len(s.Log) != 0 {
		t.Fatalf("rejected choice must not mutate state: scene=%q log=%v", s.SceneID, s.Log)
	}
}

func TestChooseAppliesEffectsAndRefills(t *testing.T) {
	_, m := testCards()
	s := &State{SceneID: "river", Hand: []string{"a", "b"}, Deck: []string{"c", "d"}}
	choice := content.Choice{
		Text: "March",
		Effects: []content.Effect{
			{Kind: content.KindStat, Delta: map[string]int{"legacy": 2, "army": -1}},
			{Kind: content.KindGoto, Next: "field"},
		},
	}
	if err := s.Choose(choice, m); err != nil {
		t.Fatalf("Choose() error = %v", err)
	}
	want := Stats{Legacy: 2, Army: -1}
	if s.Stats != want {
		t.Fatalf("Stats = %+v, want %+v (negatives must not clamp)", s.Stats, want)
	}
	if s.SceneID != "field" {
		t.Fatalf("SceneID = %q, want field", s.SceneID)
	}
	if len(s.Hand) != HandSize {
		t.Fatalf("hand = %d, want refilled to %d", len(s.Hand), HandSize)
	}
	if len(s.Log) != 1 || !strings.Contains(s.Log[0], "March") || !strings.Contains(s.Log[0], "Legacy +2") {
		t.Fatalf("Log = %v, want entry with choice text and deltas", s.Log)
	}
}

func TestPlayCardDeckExhaustionReshufflesDiscard(t *testing.T) {
	_, m := testCards()
	// 5-card deck, opening hand of 4: six plays force repeated reshuffles.
	s := &State{SceneID: "x", Hand: []string{"a", "b", "c", "d"}, Deck: []string{"e"}}
	plays := []string{"a", "b", "c", "d", "e", "a"}
	for i, id := range plays {
		if err := s.PlayCard(id, m); err != nil {
			t.Fatalf("play %d (%s): %v", i+1, id, err)
		}
		if len(s.Hand) != HandSize {
			t.Fatalf("after play %d: hand = %d, want %d (discard must reshuffle)", i+1, len(s.Hand), HandSize)
		}
		// Invariant: every card lives in exactly one pile — no loss, no dupes.
		piles := append(append(slices.Clone(s.Hand), s.Deck...), s.Discard...)
		slices.Sort(piles)
		if n := len(slices.Compact(piles)); n != 5 {
			t.Fatalf("after play %d: piles = %v, want 5 unique cards", i+1, piles)
		}
	}
	if len(s.Log) != len(plays) {
		t.Fatalf("Log = %d entries, want %d", len(s.Log), len(plays))
	}
}

func TestPlayCardNotInHand(t *testing.T) {
	_, m := testCards()
	s := &State{SceneID: "x", Hand: []string{"a"}}
	if err := s.PlayCard("z", m); err == nil {
		t.Fatal("PlayCard(unknown id) must error")
	}
}

func TestGainCardReachesHand(t *testing.T) {
	_, m := testCards()
	s := &State{SceneID: "x", Hand: []string{"a", "b", "c"}, Deck: nil, Discard: nil}
	card := content.Card{
		ID: "a", Name: "Alpha",
		Effects: []content.Effect{{Kind: content.KindGainCard, CardID: "e"}},
	}
	m["a"] = card
	if err := s.PlayCard("a", m); err != nil {
		t.Fatalf("PlayCard() error = %v", err)
	}
	if !slices.Contains(s.Hand, "e") {
		t.Fatalf("gained card e must be drawn (deck top), hand = %v", s.Hand)
	}
}

func TestLogCap(t *testing.T) {
	_, m := testCards()
	s := &State{SceneID: "x"}
	choice := content.Choice{Text: "Wait"}
	for i := 0; i < MaxLogLen+10; i++ {
		if err := s.Choose(choice, m); err != nil {
			t.Fatalf("Choose() error = %v", err)
		}
	}
	if len(s.Log) != MaxLogLen {
		t.Fatalf("Log = %d entries, want capped at %d", len(s.Log), MaxLogLen)
	}
	if !strings.Contains(s.Log[len(s.Log)-1], "Wait") {
		t.Fatalf("newest entry must be kept, got %q", s.Log[len(s.Log)-1])
	}
}
