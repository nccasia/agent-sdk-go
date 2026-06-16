// Package sqlite is the SQLite-backed SessionStore. The Python reference
// (agent_sdk/stores/session.py: SessionStoreSQL) uses the stdlib sqlite3
// module — one JSON blob per id in a `sessions(id, state)` table. The Go port
// uses modernc.org/sqlite (pure-Go, no CGO) so the same contract holds on disk
// and in memory: Load (empty when absent), Save (whole-state atomic upsert),
// Append (load+mutate+save), Compact (load+fold+save), with a file DSN
// round-tripping across processes.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/mezon/agent-sdk-go/agent_sdk/session"
	_ "modernc.org/sqlite"
)

const schema = `CREATE TABLE IF NOT EXISTS sessions (id TEXT PRIMARY KEY, state TEXT)`

// Store is the SQLite-backed SessionStore. ":memory:" is process-local (a single
// pooled connection so the in-memory database is shared across calls); a path
// dsn persists to one SQLite file on disk.
type Store struct {
	db *sql.DB
}

// NewStore opens (or creates) a store at dsn and ensures the schema exists.
func NewStore(dsn string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// An in-memory database lives only as long as its single connection, so
	// pin the pool to one connection to keep state across calls.
	if dsn == ":memory:" || dsn == "" {
		db.SetMaxOpenConns(1)
	}
	if _, err := db.ExecContext(context.Background(), schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close releases the underlying database handle.
func (s *Store) Close() error {
	return s.db.Close()
}

// Load returns the current SessionState for id (empty when absent).
func (s *Store) Load(ctx context.Context, id string) (session.SessionState, error) {
	var blob string
	err := s.db.QueryRowContext(ctx, "SELECT state FROM sessions WHERE id=?", id).Scan(&blob)
	if err == sql.ErrNoRows {
		return session.SessionState{}, nil
	}
	if err != nil {
		return session.SessionState{}, err
	}
	var d map[string]any
	if err := json.Unmarshal([]byte(blob), &d); err != nil {
		return session.SessionState{}, err
	}
	return session.SessionStateFromJSON(d), nil
}

// Save persists the WHOLE state for id (atomic upsert).
func (s *Store) Save(ctx context.Context, id string, state session.SessionState) error {
	raw, err := json.Marshal(state.ToJSON())
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		"INSERT INTO sessions(id, state) VALUES(?, ?) "+
			"ON CONFLICT(id) DO UPDATE SET state=excluded.state",
		id, string(raw))
	return err
}

// Append appends one turn to id's history.
func (s *Store) Append(ctx context.Context, id string, turn session.Turn) error {
	st, err := s.Load(ctx, id)
	if err != nil {
		return err
	}
	st.History = append(st.History, turn)
	return s.Save(ctx, id, st)
}

// Compact folds older turns into the summary for id.
func (s *Store) Compact(ctx context.Context, id string, summarizer session.Summarizer, keepLast int) error {
	st, err := s.Load(ctx, id)
	if err != nil {
		return err
	}
	if err := session.DoCompact(&st, summarizer, keepLast); err != nil {
		return err
	}
	return s.Save(ctx, id, st)
}
