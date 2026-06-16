// Package sqlite is the SQLite-backed SessionStore. The Python reference
// uses the stdlib sqlite3 module. The Go port targets modernc.org/sqlite
// (pure-Go, no CGO) but the toolchain shipped with the project is Go 1.19
// and modernc.org/sqlite needs 1.21+. Per the rung's deviation note, this
// package falls back to a stdlib-only, file-based JSON-blob store when the
// modernc.org/sqlite toolchain is unavailable. The contract (one JSON blob
// per id, Load/Append/Compact/Save) is identical, and a future toolchain
// bump can swap the backend with no API change.
package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/mezon/agent-sdk-go/agent_sdk/session"
)

// Store is the JSON-blob-per-id SessionStore. ":memory:" is process-local;
// a path dsn persists to one JSON file on disk (atomic-rename).
type Store struct {
	mu   sync.Mutex
	dsn  string
	data map[string]session.SessionState
}

// NewStore opens (or creates) a store at dsn.
func NewStore(dsn string) (*Store, error) {
	s := &Store{dsn: dsn, data: map[string]session.SessionState{}}
	if dsn != ":memory:" && dsn != "" {
		if err := s.loadFile(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// Close flushes any pending state to disk.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.flush()
}

func (s *Store) loadFile() error {
	raw, err := os.ReadFile(s.dsn)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	blobs := map[string]map[string]any{}
	if err := json.Unmarshal(raw, &blobs); err != nil {
		return err
	}
	for id, d := range blobs {
		s.data[id] = session.SessionStateFromJSON(d)
	}
	return nil
}

func (s *Store) flush() error {
	if s.dsn == ":memory:" || s.dsn == "" {
		return nil
	}
	blobs := map[string]map[string]any{}
	for id, st := range s.data {
		blobs[id] = st.ToJSON()
	}
	raw, err := json.Marshal(blobs)
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.dsn)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp := s.dsn + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.dsn)
}

// Load returns the current SessionState for id (empty when absent).
func (s *Store) Load(_ context.Context, id string) (session.SessionState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data[id], nil
}

// Save persists the WHOLE state for id.
func (s *Store) Save(_ context.Context, id string, state session.SessionState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[id] = state
	return s.flush()
}

// Append appends one turn to id's history.
func (s *Store) Append(_ context.Context, id string, turn session.Turn) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.data[id]
	st.History = append(st.History, turn)
	s.data[id] = st
	return s.flush()
}

// Compact folds older turns into the summary for id.
func (s *Store) Compact(_ context.Context, id string, summarizer session.Summarizer, keepLast int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.data[id]
	if err := session.DoCompact(&st, summarizer, keepLast); err != nil {
		return err
	}
	s.data[id] = st
	return s.flush()
}
