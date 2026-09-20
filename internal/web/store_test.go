package web

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"goGame/internal/content"
	"goGame/internal/game"
)

func testLib(t *testing.T) *content.Library {
	t.Helper()
	lib, err := content.Load(testContent)
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	return lib
}

func TestStorePersistsAndRestores(t *testing.T) {
	lib := testLib(t)
	dir := t.TempDir()

	st := NewStore(lib, dir)
	s := game.NewState(lib, "title")
	s.Session = "sess1"
	s.Stats.Army = 7
	if err := st.Put(s); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "sess1.json")); err != nil {
		t.Fatalf("save file missing: %v", err)
	}

	// A fresh store (simulating restart) restores from disk.
	st2 := NewStore(lib, dir)
	got := st2.Get("sess1")
	if got == nil {
		t.Fatal("Get() = nil after restart")
	}
	if got.SceneID != "title" || got.Stats.Army != 7 || !slices.Equal(got.Hand, s.Hand) {
		t.Fatalf("restored state mismatch: %+v", got)
	}
}

func TestStoreGetUnknownSession(t *testing.T) {
	if got := NewStore(testLib(t), "").Get("nope"); got != nil {
		t.Fatalf("Get(unknown) = %+v, want nil", got)
	}
}

func TestUpdateCommitsOnSuccess(t *testing.T) {
	dir := t.TempDir()
	lib := testLib(t)
	st := NewStore(lib, dir)
	s := game.NewState(lib, "title")
	s.Session = "sess"
	if err := st.Put(s); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	got, err := st.Update("sess", func(next *game.State) error {
		next.Stats.Army = 5
		return nil
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got.Stats.Army != 5 {
		t.Fatalf("returned state Army = %d, want 5", got.Stats.Army)
	}
	b, err := os.ReadFile(filepath.Join(dir, "sess.json"))
	if err != nil {
		t.Fatalf("save file missing: %v", err)
	}
	if !strings.Contains(string(b), `"Army": 5`) {
		t.Fatalf("save file not committed, got:\n%s", b)
	}
}

func TestUpdateUnknownSessionReturnsNil(t *testing.T) {
	got, err := NewStore(testLib(t), "").Update("nope", func(*game.State) error {
		t.Fatal("fn must not run without a state")
		return nil
	})
	if got != nil || err != nil {
		t.Fatalf("Update(unknown) = (%v, %v), want (nil, nil)", got, err)
	}
}

// A failed action must leave both the live state and the save file
// untouched, even when fn mutated the clone before failing.
func TestUpdateFailureLeavesStateUntouched(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(testLib(t), dir)
	s := &game.State{Session: "sess", SceneID: "title", Hand: []string{"phalanx"}}
	if err := st.Put(s); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	got, err := st.Update("sess", func(next *game.State) error {
		next.Stats.Army = 99 // partial mutation...
		next.Hand = nil
		return errors.New("boom") // ...then the action fails
	})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("Update() error = %v, want boom", err)
	}
	if got.Stats.Army != 0 || len(got.Hand) != 1 {
		t.Fatalf("rendered state must be the untouched original: %+v", got)
	}
	if after := st.Get("sess"); after.Stats.Army != 0 || len(after.Hand) != 1 {
		t.Fatalf("live state mutated by failed update: %+v", after)
	}
	b, err := os.ReadFile(filepath.Join(dir, "sess.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"Army": 0`) {
		t.Fatalf("save file mutated by failed update:\n%s", b)
	}
}

// Concurrent updates to one session must serialize: none are lost.
func TestUpdateSerializesConcurrentActions(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(testLib(t), dir)
	s := &game.State{Session: "sess", SceneID: "title"}
	if err := st.Put(s); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	const n = 32
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = st.Update("sess", func(next *game.State) error {
				next.Stats.Army++
				return nil
			})
		}()
	}
	wg.Wait()
	if got := st.Get("sess").Stats.Army; got != n {
		t.Fatalf("Army = %d, want %d (updates must serialize)", got, n)
	}
}

func TestAtomicSaveLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(testLib(t), dir)
	s := &game.State{Session: "sess", SceneID: "title"}
	if err := st.Put(s); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	leftovers, err := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(leftovers) != 0 {
		t.Fatalf("temp files left behind: %v", leftovers)
	}
}

func TestHistoryPersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	lib := testLib(t)
	st := NewStore(lib, dir)

	// Reach the demo ending scene and record two runs for the session.
	endState := &game.State{Session: "sess", SceneID: "end_demo", Stats: game.Stats{Legacy: 9}, Turns: 12, CardsPlayed: 5}
	if err := st.CompleteRun("sess", endState); err != nil {
		t.Fatalf("CompleteRun(): %v", err)
	}
	endState.Stats = game.Stats{Legacy: 4}
	if err := st.CompleteRun("sess", endState); err != nil {
		t.Fatalf("CompleteRun(): %v", err)
	}

	// A fresh store (simulating restart) restores the history.
	runs := NewStore(lib, dir).History("sess")
	if len(runs) != 2 {
		t.Fatalf("History() = %d runs, want 2", len(runs))
	}
	if runs[0].Ending != "triumph" || runs[1].Stats.Legacy != 4 || runs[0].Turns != 12 {
		t.Fatalf("restored runs mismatch: %+v", runs)
	}

	if got := st.History("unknownsession"); got != nil {
		t.Fatalf("History(unknown) = %+v, want nil", got)
	}
}

func TestSweepRemovesOnlyStaleSaves(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(testLib(t), dir)
	fresh := &game.State{Session: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SceneID: "title"}
	stale := &game.State{Session: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", SceneID: "title"}
	if err := st.Put(fresh); err != nil {
		t.Fatalf("Put(fresh): %v", err)
	}
	if err := st.Put(stale); err != nil {
		t.Fatalf("Put(stale): %v", err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(st.path(stale.Session), old, old); err != nil {
		t.Fatal(err)
	}

	if n := st.Sweep(24 * time.Hour); n != 1 {
		t.Fatalf("Sweep() removed %d saves, want 1", n)
	}
	if _, err := os.Stat(st.path(fresh.Session)); err != nil {
		t.Fatalf("fresh save must survive: %v", err)
	}
	if _, err := os.Stat(st.path(stale.Session)); !os.IsNotExist(err) {
		t.Fatalf("stale save must be gone, got: %v", err)
	}
	if got := st.Get(stale.Session); got != nil {
		t.Fatalf("swept session must not resurrect from cache: %+v", got)
	}
}
