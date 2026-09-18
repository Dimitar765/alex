package web

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"goGame/internal/content"
	"goGame/internal/game"
)

func TestStorePersistsAndRestores(t *testing.T) {
	lib, err := content.Load(testContent)
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	dir := t.TempDir()

	st := NewStore(lib, dir)
	s := game.NewState(lib.CardList(), "title")
	s.Session = "sess1"
	s.Stats.Army = 7
	st.Put(s)

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
	lib, err := content.Load(testContent)
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	if got := NewStore(lib, "").Get("nope"); got != nil {
		t.Fatalf("Get(unknown) = %+v, want nil", got)
	}
}
