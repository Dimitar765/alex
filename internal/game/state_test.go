package game

import (
	"slices"
	"strings"
	"testing"

	"goGame/internal/content"
)

func testLib() *content.Library {
	lib := &content.Library{Cards: map[string]content.Card{}, Scenes: map[string]content.Scene{}}
	for _, c := range []content.Card{
		{ID: "a", Name: "Alpha", Text: "flavor a", Start: true},
		{ID: "b", Name: "Bravo", Text: "flavor b", Start: true},
		{ID: "c", Name: "Charlie", Text: "flavor c", Start: true},
		{ID: "d", Name: "Delta", Text: "flavor d", Start: true},
		{ID: "e", Name: "Echo", Text: "flavor e", Start: true},
	} {
		lib.Cards[c.ID] = c
	}
	return lib
}

func TestNewStateDealsOpeningHand(t *testing.T) {
	lib := testLib()
	s := NewState(lib, "title")
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

func TestPeekRevealsTopWithoutTouchingPiles(t *testing.T) {
	lib := testLib()
	s := &State{SceneID: "x", Hand: []string{"a"}, Deck: []string{"b", "c"}}
	top, err := s.Peek(lib)
	if err != nil {
		t.Fatalf("Peek() error = %v", err)
	}
	if top != "c" {
		t.Fatalf("Peek() = %q, want the deck top c", top)
	}
	if s.Turns != 1 {
		t.Fatalf("peek must spend the turn: %d", s.Turns)
	}
	if len(s.Deck) != 2 || s.Deck[1] != "c" || len(s.Hand) != 1 {
		t.Fatalf("peek must not touch piles: deck=%v hand=%v", s.Deck, s.Hand)
	}
	if !strings.Contains(strings.Join(s.Log, " | "), "Scouted the deck: next card is Charlie") {
		t.Fatalf("log must name the scouted card, got %v", s.Log)
	}
}

func TestPeekEmptyDeckRefused(t *testing.T) {
	s := &State{SceneID: "x", Hand: []string{"a"}}
	_, err := s.Peek(testLib())
	if err == nil || !strings.Contains(err.Error(), "nothing to scout") {
		t.Fatalf("Peek() error = %v, want refusal", err)
	}
	if s.Turns != 0 {
		t.Fatalf("refused peek must not spend the turn: %d", s.Turns)
	}
}

func TestShuffleRecyclesDiscardAndSpendsTurn(t *testing.T) {
	lib := testLib()
	s := &State{SceneID: "x", Hand: []string{"a"}, Deck: []string{"b"}, Discard: []string{"c", "d"}}
	turns := s.Turns
	if err := s.Shuffle(); err != nil {
		t.Fatalf("Shuffle() error = %v", err)
	}
	if s.Turns != turns+1 || s.CardsPlayed != 0 {
		t.Fatalf("shuffle must spend the turn without counting a card play: %+v", s)
	}
	if len(s.Discard) != 0 || len(s.Deck) != 3 { // b + recycled c, d
		t.Fatalf("discard must recycle into the deck: deck=%v discard=%v", s.Deck, s.Discard)
	}
	all := append(slices.Clone(s.Deck), s.Hand...)
	slices.Sort(all)
	if !slices.Equal(all, []string{"a", "b", "c", "d"}) {
		t.Fatalf("cards must be conserved: %v", all)
	}
	if !strings.Contains(strings.Join(s.Log, " | "), "Shuffled the discard") {
		t.Fatalf("log must record the shuffle, got %v", s.Log)
	}
	_ = lib
}

func TestShuffleEmptyDiscardRefused(t *testing.T) {
	s := &State{SceneID: "x", Hand: []string{"a"}, Deck: []string{"b"}}
	err := s.Shuffle()
	if err == nil || !strings.Contains(err.Error(), "nothing to shuffle") {
		t.Fatalf("Shuffle() error = %v, want refusal", err)
	}
	if s.Turns != 0 || len(s.Deck) != 1 {
		t.Fatalf("refused shuffle must not mutate state: %+v", s)
	}
}

func TestActionCounters(t *testing.T) {
	lib := testLib()
	s := &State{SceneID: "x", Hand: []string{"a"}, Deck: []string{"b", "c", "d", "e"}}
	choice := content.Choice{Text: "March"}
	if err := s.Choose(choice, lib); err != nil {
		t.Fatalf("Choose() error = %v", err)
	}
	if err := s.PlayCard("a", lib); err != nil {
		t.Fatalf("PlayCard() error = %v", err)
	}
	if err := s.PlayCard("ghost", lib); err == nil {
		t.Fatal("refused play must not count")
	}
	if s.Turns != 2 || s.CardsPlayed != 1 {
		t.Fatalf("Turns = %d, CardsPlayed = %d; want 2 and 1 (refusals excluded)", s.Turns, s.CardsPlayed)
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
	lib := testLib()
	choice := content.Choice{
		Text:         "Break the center",
		RequiresStat: map[string]int{"army": 3, "legacy": 1},
	}
	s := &State{SceneID: "issus", Stats: Stats{Army: 2, Legacy: 5}, Hand: []string{"a"}}
	err := s.Choose(choice, lib)
	if err == nil || !strings.Contains(err.Error(), "requires Army 3 (you have 2)") {
		t.Fatalf("Choose() error = %v, want Army shortfall named", err)
	}
	if s.SceneID != "issus" {
		t.Fatalf("rejected choice must not move the scene: %q", s.SceneID)
	}
	s.Stats.Army = 3
	if err := s.Choose(choice, lib); err != nil {
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
	lib := testLib()
	// A stocked deck keeps drawUp from reshuffling the discard, so the lost
	// cards provably stay in the discard pile for now.
	s := &State{SceneID: "x", Hand: []string{"a", "b"}, Deck: []string{"c", "e", "e", "d", "d", "e"}}
	card := content.Card{ID: "a", Name: "Alpha", Effects: []content.Effect{
		{Kind: content.KindLoseCard, CardID: "c"},
		{Kind: content.KindLoseCard, CardID: "b"},
	}}
	lib.Cards["a"] = card
	if err := s.PlayCard("a", lib); err != nil {
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
	lib := testLib()
	s := &State{SceneID: "x", Hand: []string{"a", "b"}, Deck: []string{"c"}, Discard: []string{"d"}}
	choice := content.Choice{Text: "The horse dies", Effects: []content.Effect{
		{Kind: content.KindRemoveCard, CardID: "a"}, // from hand
		{Kind: content.KindRemoveCard, CardID: "c"}, // from deck
		{Kind: content.KindRemoveCard, CardID: "d"}, // from discard
	}}
	if err := s.Choose(choice, lib); err != nil {
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
	lib := testLib()
	s := &State{SceneID: "start", Hand: []string{"a"}}
	choice := content.Choice{Text: "Fate is kind", Effects: []content.Effect{
		{Kind: content.KindRandom, Outcomes: []content.Outcome{{
			Weight:  1,
			Effects: []content.Effect{{Kind: content.KindGoto, Next: "field"}},
		}}},
	}}
	if err := s.Choose(choice, lib); err != nil {
		t.Fatalf("Choose() error = %v", err)
	}
	if s.SceneID != "field" {
		t.Fatalf("SceneID = %q, want field", s.SceneID)
	}
}

func TestRandomRespectsWeights(t *testing.T) {
	lib := testLib()
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
		if err := s.Choose(choice, lib); err != nil {
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
	lib := testLib()
	lib.Cards["a"] = content.Card{ID: "a", Name: "Alpha", Cost: 2, Effects: []content.Effect{
		{Kind: content.KindStat, Delta: map[string]int{"army": 2}},
	}}
	s := &State{SceneID: "x", Hand: []string{"a", "b"}, Stats: Stats{Treasury: 3}}
	if err := s.PlayCard("a", lib); err != nil {
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
	lib := testLib()
	lib.Cards["a"] = content.Card{ID: "a", Name: "Alpha", Cost: 2}
	s := &State{SceneID: "x", Hand: []string{"a", "b"}, Stats: Stats{Treasury: 1}}
	err := s.PlayCard("a", lib)
	if err == nil || !strings.Contains(err.Error(), "cannot afford Alpha (costs 2 treasury)") {
		t.Fatalf("PlayCard() error = %v, want affordability error", err)
	}
	if len(s.Hand) != 2 || s.Stats.Treasury != 1 {
		t.Fatalf("refused play must not mutate state: hand=%v stats=%+v", s.Hand, s.Stats)
	}
	if s.CanPlay("a", lib.Cards) {
		t.Fatal("CanPlay must report unaffordable")
	}
	s.Stats.Treasury = 2
	if !s.CanPlay("a", lib.Cards) {
		t.Fatal("CanPlay must report affordable")
	}
}

func TestChooseConsumesRequiredCard(t *testing.T) {
	lib := testLib()
	choice := content.Choice{
		Text:         "Ride him into the river",
		RequiresCard: "e",
		ConsumesCard: true,
		Effects:      []content.Effect{{Kind: content.KindStat, Delta: map[string]int{"legacy": 2}}},
	}
	// A stocked deck keeps drawUp from recycling the consumed card.
	s := &State{SceneID: "granicus", Hand: []string{"e", "a"}, Deck: []string{"b", "c", "d"}}
	if err := s.Choose(choice, lib); err != nil {
		t.Fatalf("Choose() error = %v", err)
	}
	if slices.Contains(s.Hand, "e") || !slices.Contains(s.Discard, "e") {
		t.Fatalf("consumed card must move hand->discard: hand=%v discard=%v", s.Hand, s.Discard)
	}
	if !strings.Contains(strings.Join(s.Log, " | "), "spent Echo") {
		t.Fatalf("log must name the spent card, got %v", s.Log)
	}
}

func TestNewStateDealsStartingCardsOnly(t *testing.T) {
	lib := testLib()
	regional := lib.Cards["e"]
	regional.Start = false // Echo is regional now
	lib.Cards["e"] = regional
	s := NewState(lib, "title")
	all := append(slices.Clone(s.Hand), s.Deck...)
	for _, id := range all {
		if id == "e" {
			t.Fatalf("non-start card dealt into opening run: %v", all)
		}
	}
	if len(all) != 4 {
		t.Fatalf("opening piles = %d cards, want the 4 start cards", len(all))
	}
}

func TestArrivalGrantsScenePoolOnce(t *testing.T) {
	lib := testLib()
	lib.Scenes["field"] = content.Scene{ID: "field", Cards: []string{"e"}}
	s := &State{SceneID: "start", Hand: []string{"a"}, Deck: []string{"b"}}
	enter := content.Choice{Text: "March", Effects: []content.Effect{{Kind: content.KindGoto, Next: "field"}}}

	if err := s.Choose(enter, lib); err != nil {
		t.Fatalf("first entry: %v", err)
	}
	if !slices.Contains(s.Granted, "field") {
		t.Fatalf("field must be marked granted: %v", s.Granted)
	}
	// The granted card sits on the deck top, so the refill drew it.
	if !slices.Contains(s.Hand, "e") {
		t.Fatalf("granted card must be drawn first: hand=%v deck=%v", s.Hand, s.Deck)
	}
	if !strings.Contains(strings.Join(s.Log, " | "), "gained Echo") {
		t.Fatalf("log must name the grant, got %v", s.Log)
	}

	// Leave and re-enter: the pool never grants twice.
	exit := content.Choice{Text: "Back", Effects: []content.Effect{{Kind: content.KindGoto, Next: "start"}}}
	enter2 := content.Choice{Text: "March again", Effects: []content.Effect{{Kind: content.KindGoto, Next: "field"}}}
	before := len(s.Hand) + len(s.Deck) + len(s.Discard)
	if err := s.Choose(exit, lib); err != nil {
		t.Fatalf("exit: %v", err)
	}
	if err := s.Choose(enter2, lib); err != nil {
		t.Fatalf("re-entry: %v", err)
	}
	after := len(s.Hand) + len(s.Deck) + len(s.Discard)
	if after != before {
		t.Fatalf("re-entry must not duplicate grants: piles %d -> %d", before, after)
	}
	if strings.Count(strings.Join(s.Log, " | "), "gained Echo") != 1 {
		t.Fatalf("grant must be logged exactly once: %v", s.Log)
	}
}

func TestChooseRequiresCard(t *testing.T) {
	lib := testLib()
	s := &State{SceneID: "river", Hand: []string{"a", "b"}}
	choice := content.Choice{Text: "Charge", RequiresCard: "e"}
	err := s.Choose(choice, lib)
	if err == nil || !strings.Contains(err.Error(), "Echo") {
		t.Fatalf("Choose() error = %v, want error naming Echo", err)
	}
	if s.SceneID != "river" || len(s.Log) != 0 {
		t.Fatalf("rejected choice must not mutate state: scene=%q log=%v", s.SceneID, s.Log)
	}
}

func TestChooseAppliesEffectsAndRefills(t *testing.T) {
	lib := testLib()
	s := &State{SceneID: "river", Hand: []string{"a", "b"}, Deck: []string{"c", "d"}}
	choice := content.Choice{
		Text: "March",
		Effects: []content.Effect{
			{Kind: content.KindStat, Delta: map[string]int{"legacy": 2, "army": -1}},
			{Kind: content.KindGoto, Next: "field"},
		},
	}
	if err := s.Choose(choice, lib); err != nil {
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
	lib := testLib()
	// 5-card deck, opening hand of 4: six plays force repeated reshuffles.
	s := &State{SceneID: "x", Hand: []string{"a", "b", "c", "d"}, Deck: []string{"e"}}
	plays := []string{"a", "b", "c", "d", "e", "a"}
	for i, id := range plays {
		if err := s.PlayCard(id, lib); err != nil {
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
	lib := testLib()
	s := &State{SceneID: "x", Hand: []string{"a"}}
	if err := s.PlayCard("z", lib); err == nil {
		t.Fatal("PlayCard(unknown id) must error")
	}
}

func TestGainCardReachesHand(t *testing.T) {
	lib := testLib()
	s := &State{SceneID: "x", Hand: []string{"a", "b", "c"}, Deck: nil, Discard: nil}
	card := content.Card{
		ID: "a", Name: "Alpha",
		Effects: []content.Effect{{Kind: content.KindGainCard, CardID: "e"}},
	}
	lib.Cards["a"] = card
	if err := s.PlayCard("a", lib); err != nil {
		t.Fatalf("PlayCard() error = %v", err)
	}
	if !slices.Contains(s.Hand, "e") {
		t.Fatalf("gained card e must be drawn (deck top), hand = %v", s.Hand)
	}
}

func TestLogCap(t *testing.T) {
	lib := testLib()
	s := &State{SceneID: "x"}
	choice := content.Choice{Text: "Wait"}
	for i := 0; i < MaxLogLen+10; i++ {
		if err := s.Choose(choice, lib); err != nil {
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
