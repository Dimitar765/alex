package app

import (
	"os"
	"path/filepath"
	"testing"

	"goGame/internal/content"
	"goGame/internal/game"
)

// testLib builds a tiny campaign:
//
//	title --(free choice)--> field --(win)--> ending "triumph"
//	  |                        |
//	  └ cards: free, costly    └ arrival grants: gift
//
// "free" costs 0 and grants Legacy +1; "costly" costs 3 (unaffordable at
// the start); "gift" is only obtainable via field's arrival pool.
func testLib() *content.Library {
	return &content.Library{
		Cards: map[string]content.Card{
			"free":   {ID: "free", Name: "Free Blade", Text: "Swing.", Start: true, Effects: []content.Effect{{Kind: "stat", Delta: map[string]int{"legacy": 1}}}},
			"costly": {ID: "costly", Name: "Costly Guard", Text: "Pricy.", Cost: 3, Start: true},
			"gift":   {ID: "gift", Name: "Field Gift", Text: "From the field."},
		},
		Scenes: map[string]content.Scene{
			"title": {ID: "title", Text: "The beginning.", Choices: []content.Choice{
				{Text: "March", Effects: []content.Effect{{Kind: "goto", Next: "field"}}},
			}},
			"field": {ID: "field", Text: "The plain.", Cards: []string{"gift"}, Choices: []content.Choice{
				{Text: "Win", Effects: []content.Effect{{Kind: "goto", Next: "ending"}}},
			}},
			"ending": {ID: "ending", Text: "It is done.", Ending: "triumph"},
		},
	}
}

func newTestModel() *Model { return NewModel(testLib(), "") }

// findHand returns the view's card with the given ID.
func findHand(t *testing.T, v *View, id string) CardView {
	t.Helper()
	for _, c := range v.Hand {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("card %q not in hand", id)
	return CardView{}
}

// effectsOf filters a result's effects by kind.
func effectsOf(r Result, kind EffectKind) []Effect {
	var out []Effect
	for _, e := range r.Effects {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

func TestNewGameStartsFreshRun(t *testing.T) {
	m := newTestModel()
	if m.HasRun() {
		t.Fatal("fresh model should have no run")
	}
	r := m.NewGame()
	if !m.HasRun() || r.Err != nil {
		t.Fatalf("NewGame: err=%v hasRun=%v", r.Err, m.HasRun())
	}
	v := r.View
	if v.SceneID != StartScene {
		t.Fatalf("scene = %q, want %q", v.SceneID, StartScene)
	}
	if len(v.Hand) != 2 { // only free + costly are start cards
		t.Fatalf("hand = %d cards, want 2", len(v.Hand))
	}
	if v.DeckCount != 0 || v.DiscardCount != 0 {
		t.Fatalf("piles = deck %d discard %d, want 0/0", v.DeckCount, v.DiscardCount)
	}
	if len(v.Choices) != 1 {
		t.Fatalf("choices = %d, want 1", len(v.Choices))
	}
}

func TestActionsWithoutRunAreRefused(t *testing.T) {
	m := newTestModel()
	for name, act := range map[string]func() Result{
		"play":    func() Result { return m.PlayCard("free") },
		"choose":  func() Result { return m.Choose(0) },
		"scout":   func() Result { return m.Scout() },
		"shuffle": func() Result { return m.Shuffle() },
	} {
		if r := act(); r.Err == nil {
			t.Errorf("%s without a run: want error", name)
		}
	}
}

func TestPlayCardEmitsStatEffectAndRefills(t *testing.T) {
	m := newTestModel()
	m.NewGame()
	r := m.PlayCard("free")
	if r.Err != nil {
		t.Fatalf("play free: %v", r.Err)
	}
	stats := effectsOf(r, EffectStat)
	if len(stats) != 1 || stats[0].Stat != "legacy" || stats[0].Delta != 1 {
		t.Fatalf("stat effects = %+v, want legacy +1", stats)
	}
	v := r.View
	if v.Stats.Legacy != 1 {
		t.Fatalf("legacy = %d, want 1", v.Stats.Legacy)
	}
	if v.Turns != 1 || v.CardsPlayed != 1 {
		t.Fatalf("turns/cards = %d/%d, want 1/1", v.Turns, v.CardsPlayed)
	}
	// The tiny deck recycles the discard immediately on refill, so the
	// run still owns both cards and the hand is full again.
	if len(v.Hand) != 2 || v.DeckTotal != 2 {
		t.Fatalf("hand = %d, total owned = %d, want 2/2", len(v.Hand), v.DeckTotal)
	}
	if len(v.Log) == 0 || v.Log[0] != "Played Free Blade. Legacy +1, Threat +1." {
		t.Fatalf("log head = %q", v.Log[0])
	}
}

func TestPlayCardRefusalLeavesStateUntouched(t *testing.T) {
	m := newTestModel()
	m.NewGame()
	before := m.View()
	r := m.PlayCard("costly") // costs 3, treasury is 0
	if r.Err == nil {
		t.Fatal("want refusal for unaffordable card")
	}
	if len(r.Effects) != 0 {
		t.Fatalf("refused action emitted effects: %+v", r.Effects)
	}
	after := m.View()
	if after.Turns != before.Turns || after.Stats != before.Stats {
		t.Fatal("state changed on refused action")
	}
	// The refusal is part of the action's own view, never persisted.
	if len(r.View.Log) == 0 || r.View.Log[0] != "Error: "+r.Err.Error() {
		t.Fatalf("error not surfaced in result log (len %d)", len(r.View.Log))
	}
	if len(m.View().Log) != len(before.Log) {
		t.Fatal("refusal leaked into the persisted log")
	}
	// The refused card must render as unplayable in the view.
	if c := findHand(t, after, "costly"); c.Playable {
		t.Fatal("costly card should not be playable at 0 treasury")
	}
}

func TestChooseTriggersSceneArrivalAndGain(t *testing.T) {
	m := newTestModel()
	m.NewGame()
	r := m.Choose(0)
	if r.Err != nil {
		t.Fatalf("choose: %v", r.Err)
	}
	if v := r.View; v.SceneID != "field" {
		t.Fatalf("scene = %q, want field", v.SceneID)
	}
	gains := effectsOf(r, EffectCardGained)
	if len(gains) != 1 || gains[0].CardID != "gift" {
		t.Fatalf("gains = %+v, want gift", gains)
	}
	scenes := effectsOf(r, EffectSceneChanged)
	if len(scenes) != 1 || scenes[0].From != StartScene || scenes[0].To != "field" {
		t.Fatalf("scene effects = %+v", scenes)
	}
}

func TestEndingRecordsRunOnceAndLocksActions(t *testing.T) {
	m := newTestModel()
	m.NewGame()
	if r := m.Choose(0); r.Err != nil { // title -> field (grants gift)
		t.Fatalf("march: %v", r.Err)
	}
	r := m.Choose(0) // field -> ending
	if r.Err != nil {
		t.Fatalf("win: %v", r.Err)
	}
	if endings := effectsOf(r, EffectEnding); len(endings) != 1 || endings[0].Text != "triumph" {
		t.Fatalf("ending effects = %+v", endings)
	}
	if len(m.History()) != 1 {
		t.Fatalf("history = %d runs, want 1", len(m.History()))
	}
	if got := m.History()[0].Ending; got != "triumph" {
		t.Fatalf("recorded ending = %q", got)
	}
	// The ending scene offers no choices; a card play is refused and
	// must not record a second run.
	if r := m.PlayCard("free"); r.Err == nil {
		t.Fatal("post-ending play should be refused")
	}
	if len(m.History()) != 1 {
		t.Fatalf("history grew to %d", len(m.History()))
	}
}

func TestNewGameAfterEndingKeepsHistory(t *testing.T) {
	m := newTestModel()
	m.NewGame()
	m.Choose(0)
	m.Choose(0) // reach ending
	m.NewGame()
	if len(m.History()) != 1 {
		t.Fatalf("history = %d, want 1 (only completed runs)", len(m.History()))
	}
	if v := m.View(); v.SceneID != StartScene {
		t.Fatalf("scene = %q, want fresh %q", v.SceneID, StartScene)
	}
}

func TestScoutAndShuffleRefusals(t *testing.T) {
	m := newTestModel()
	m.NewGame()
	// The two-card opening deck is dealt entirely into the hand, so both
	// the deck and the discard start empty: scout and shuffle refuse.
	if r := m.Shuffle(); r.Err == nil {
		t.Fatal("shuffle with empty discard should fail")
	}
	if r := m.Scout(); r.Err == nil {
		t.Fatal("scout on empty deck should fail")
	}
	if m.View().Turns != 0 {
		t.Fatalf("refused actions must not spend turns; turns = %d", m.View().Turns)
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	lib := testLib()
	m := NewModel(lib, dir)
	m.NewGame()
	m.PlayCard("free")
	want := m.View()

	// A second model over the same directory resumes the run.
	m2 := NewModel(lib, dir)
	if !m2.HasRun() {
		t.Fatal("save not loaded")
	}
	got := m2.View()
	if got.SceneID != want.SceneID || got.Turns != want.Turns || got.Stats != want.Stats {
		t.Fatalf("resumed view mismatch: %+v vs %+v", got, want)
	}
	if got.DeckTotal != want.DeckTotal || len(got.Hand) != len(want.Hand) {
		t.Fatal("piles/hand not restored")
	}
}

func TestHistoryPersistence(t *testing.T) {
	dir := t.TempDir()
	lib := testLib()
	m := NewModel(lib, dir)
	m.NewGame()
	m.Choose(0)
	m.Choose(0) // ending
	m2 := NewModel(lib, dir)
	runs := m2.History()
	if len(runs) != 1 || runs[0].Ending != "triumph" {
		t.Fatalf("persisted history = %+v", runs)
	}
	if v := m2.View(); v.Campaigns != 1 {
		t.Fatalf("campaigns = %d, want 1", v.Campaigns)
	}
	// The gallery marks triumph discovered.
	for _, tile := range vGallery(m2) {
		if tile.Class == "triumph" && !tile.Discovered {
			t.Fatal("triumph tile should be discovered")
		}
	}
}

func vGallery(m *Model) []EndingTile { return m.View().Endings }

func TestCorruptSaveReadsAsEmpty(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(saveFile, "{not json")
	write(historyFile, "nope")
	m := NewModel(testLib(), dir)
	if m.HasRun() {
		t.Fatal("corrupt save should read as no run")
	}
	if len(m.History()) != 0 {
		t.Fatal("corrupt history should read as empty")
	}
}

func TestSpeculativeCloneKeepsModelImmutable(t *testing.T) {
	// Diffing must never leak next-state into the model: play on a clone,
	// refuse to commit, then confirm the model still shows the old view.
	m := newTestModel()
	m.NewGame()
	st := m.st
	clone := st.Clone()
	if err := clone.PlayCard("free", testLib()); err != nil {
		t.Fatal(err)
	}
	if m.View().Stats.Legacy != 0 {
		t.Fatal("clone mutated the live model")
	}
	if game.HandSize != 4 {
		t.Fatal("HandSize changed unexpectedly")
	}
}

func TestThreatEffectsAndView(t *testing.T) {
	m := newTestModel()
	m.NewGame()
	m.PlayCard("free") // threat +1 from the turn advance
	v := m.View()
	if v.Threat != 1 || v.MaxThreat != game.ThreatMax {
		t.Fatalf("threat = %d/%d, want 1/%d", v.Threat, v.MaxThreat, game.ThreatMax)
	}
	if r := m.PlayCard("free"); r.Err == nil {
		found := false
		for _, e := range r.Effects {
			if e.Kind == EffectThreat {
				found = true
			}
		}
		if !found {
			t.Fatal("threat delta not reported in effects")
		}
	}
}
