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
