package app

import (
	"encoding/json"
	"os"
	"path/filepath"

	"goGame/internal/game"
)

// Save file names inside the save directory. Distinct from the web
// build's per-session files so both can coexist in one directory.
const (
	saveFile    = "save.json"
	historyFile = "history.json"
)

// loadState reads a persisted campaign, returning nil when absent or
// corrupt — the client then shows the title screen rather than failing.
func (m *Model) loadState() *game.State {
	if m.dir == "" {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(m.dir, saveFile))
	if err != nil {
		return nil
	}
	var st game.State
	if json.Unmarshal(b, &st) != nil {
		return nil
	}
	return &st
}

// loadHistory reads the completed-run history; corrupt reads as empty.
func (m *Model) loadHistory() []RunSummary {
	if m.dir == "" {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(m.dir, historyFile))
	if err != nil {
		return nil
	}
	var runs []RunSummary
	if json.Unmarshal(b, &runs) != nil {
		return nil
	}
	return runs
}

// saveState persists the live state atomically ("" disables persistence).
func (m *Model) saveState() error {
	if m.dir == "" || m.st == nil {
		return nil
	}
	b, err := json.MarshalIndent(m.st, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(m.dir, saveFile), b)
}

// saveHistory persists the completed-run history atomically.
func (m *Model) saveHistory() error {
	if m.dir == "" {
		return nil
	}
	b, err := json.MarshalIndent(m.runs, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(m.dir, historyFile), b)
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
