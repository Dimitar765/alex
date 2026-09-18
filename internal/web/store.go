package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"goGame/internal/content"
	"goGame/internal/game"
)

// Store holds live game states and mirrors them to one JSON save file per
// session. Session IDs reaching Put/Get must already be validated by the
// cookie layer (sessionRe); the store only concatenates them into filenames.
// An empty dir means memory-only (used by tests).
type Store struct {
	mu    sync.Mutex
	games map[string]*game.State
	lib   *content.Library
	dir   string
}

// NewStore builds a store persisting to dir ("" disables persistence).
func NewStore(lib *content.Library, dir string) *Store {
	return &Store{games: map[string]*game.State{}, lib: lib, dir: dir}
}

// Get returns the state for session, loading it from disk on a cache miss.
// It returns nil when no state exists (caller renders the title page).
func (st *Store) Get(session string) *game.State {
	st.mu.Lock()
	defer st.mu.Unlock()
	if s, ok := st.games[session]; ok {
		return s
	}
	if st.dir == "" {
		return nil
	}
	b, err := os.ReadFile(st.path(session))
	if err != nil {
		return nil
	}
	var s game.State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil // corrupt save: start over rather than fail the request
	}
	s.Session = session
	st.games[session] = &s
	return &s
}

// Put stores the state in memory and writes its save file.
func (st *Store) Put(s *game.State) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.games[s.Session] = s
	if st.dir == "" {
		return
	}
	_ = os.MkdirAll(st.dir, 0o755)
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(st.path(s.Session), b, 0o644)
}

func (st *Store) path(session string) string {
	return filepath.Join(st.dir, session+".json")
}
