package web

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"goGame/internal/content"
	"goGame/internal/game"
)

// Store holds committed game states and their run history, mirroring both
// to JSON files in the save directory. Session IDs reaching these methods
// must already be validated by the cookie layer (sessionRe); the store only
// concatenates them into filenames. An empty dir means memory-only (used by
// tests).
//
// Stored states are immutable once committed: Update works on a clone and
// is the only path that replaces an entry, and persistence happens before
// publication, so memory and disk can never diverge on a failed action.
type Store struct {
	mu      sync.Mutex
	games   map[string]*game.State
	history map[string][]RunSummary
	lib     *content.Library
	dir     string
}

// RunSummary records one completed campaign for the endings gallery.
type RunSummary struct {
	Ending      string     `json:"ending"`
	Stats       game.Stats `json:"stats"`
	Turns       int        `json:"turns"`
	CardsPlayed int        `json:"cardsPlayed"`
	Date        time.Time  `json:"date"`
}

// NewStore builds a store persisting to dir ("" disables persistence).
func NewStore(lib *content.Library, dir string) *Store {
	return &Store{
		games:   map[string]*game.State{},
		history: map[string][]RunSummary{},
		lib:     lib,
		dir:     dir,
	}
}

// Get returns a snapshot of the session's state, loading it from disk on a
// cache miss. It returns nil when no state exists (caller renders the title
// page). The snapshot is safe to read while other actions run.
func (st *Store) Get(session string) *game.State {
	st.mu.Lock()
	defer st.mu.Unlock()
	s := st.getLocked(session)
	if s == nil {
		return nil
	}
	return s.Clone()
}

// Update applies fn to a clone of the session's state and commits the
// result — memory and save file together — only when fn succeeds. On
// failure it returns the untouched current state alongside fn's error so
// the caller can render the refusal. It returns (nil, nil) when no state
// exists for the session.
func (st *Store) Update(session string, fn func(*game.State) error) (*game.State, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	cur := st.getLocked(session)
	if cur == nil {
		return nil, nil
	}
	next := cur.Clone()
	if err := fn(next); err != nil {
		return cur, err
	}
	if err := st.putLocked(next); err != nil {
		return cur, fmt.Errorf("save failed: %w", err)
	}
	return next, nil
}

// Put stores a fresh state, persisting it before it becomes live.
func (st *Store) Put(s *game.State) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.putLocked(s)
}

func (st *Store) getLocked(session string) *game.State {
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

// putLocked persists s atomically and only then publishes it as the live
// state, so a failed write never half-applies.
func (st *Store) putLocked(s *game.State) error {
	if st.dir != "" {
		if err := os.MkdirAll(st.dir, 0o755); err != nil {
			return err
		}
		b, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			return err
		}
		if err := writeFileAtomic(st.path(s.Session), b); err != nil {
			return err
		}
	}
	st.games[s.Session] = s
	return nil
}

// writeFileAtomic writes b to path via a temp file in the same directory
// followed by rename, so readers never observe a torn file.
func writeFileAtomic(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// CompleteRun appends a run that just reached an ending scene to the
// session's history and persists it.
func (st *Store) CompleteRun(session string, s *game.State) error {
	run := RunSummary{
		Ending:      st.lib.Scenes[s.SceneID].Ending,
		Stats:       s.Stats,
		Turns:       s.Turns,
		CardsPlayed: s.CardsPlayed,
		Date:        time.Now(),
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	runs := append(st.historyLocked(session), run)
	st.history[session] = runs
	if st.dir == "" {
		return nil
	}
	if err := os.MkdirAll(st.dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(runs, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(st.historyPath(session), b)
}

// History returns the session's completed runs, oldest first, loading them
// from disk on a cache miss. A corrupt history file reads as empty.
func (st *Store) History(session string) []RunSummary {
	st.mu.Lock()
	defer st.mu.Unlock()
	return slices.Clone(st.historyLocked(session))
}

func (st *Store) historyLocked(session string) []RunSummary {
	if runs, ok := st.history[session]; ok {
		return runs
	}
	if st.dir == "" {
		return nil
	}
	b, err := os.ReadFile(st.historyPath(session))
	if err != nil {
		return nil
	}
	var runs []RunSummary
	if json.Unmarshal(b, &runs) != nil {
		return nil // corrupt history: start over rather than fail the request
	}
	st.history[session] = runs
	return runs
}

func (st *Store) historyPath(session string) string {
	return filepath.Join(st.dir, "history-"+session+".json")
}

func (st *Store) path(session string) string {
	return filepath.Join(st.dir, session+".json")
}

// Sweep deletes save files whose session has been idle longer than maxAge
// and drops their cached states. It returns the number of saves removed.
func (st *Store) Sweep(maxAge time.Duration) int {
	if st.dir == "" {
		return 0
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	entries, err := os.ReadDir(st.dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || filepath.Ext(name) != ".json" {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		session := strings.TrimSuffix(name, filepath.Ext(name))
		if os.Remove(st.path(session)) == nil {
			delete(st.games, session)
			n++
		}
	}
	return n
}
