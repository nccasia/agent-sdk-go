// Package memory holds the pluggable session-state and durable-memory backends:
// SessionStore (protocol + in-memory default + a stdlib SQL/JSON-blob adapter)
// and MemoryStore (protocol + in-memory default). search is a deterministic
// token-overlap match (no embeddings) so it works with zero infra. Ported from
// agent_sdk/stores/{session,memory}.py.
//
// Deviation: Python's SessionStoreSQL uses the stdlib sqlite3 module. Go's
// standard library ships no SQL driver and this module pins no third-party
// deps, so SessionStoreSQL here is a stdlib-only JSON-blob store: an in-process
// map for ":memory:" and a JSON file for a path dsn. The contract (one JSON
// blob per id, load/append/compact/save) is identical.
package memory

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/nccasia/agent-sdk-go/agent_sdk/session"
)

// ── Session stores ───────────────────────────────────────────────────────────

// SessionStoreInMemory is a process-local SessionStore — the zero-infra default.
type SessionStoreInMemory struct {
	mu   sync.Mutex
	data map[string]*session.SessionState
}

// NewSessionStoreInMemory builds an empty in-memory session store.
func NewSessionStoreInMemory() *SessionStoreInMemory {
	return &SessionStoreInMemory{data: map[string]*session.SessionState{}}
}

func (s *SessionStoreInMemory) get(id string) *session.SessionState {
	st, ok := s.data[id]
	if !ok {
		st = &session.SessionState{}
		s.data[id] = st
	}
	return st
}

// Load returns the current state for id (creating an empty one on first use).
func (s *SessionStoreInMemory) Load(_ context.Context, id string) (session.SessionState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return *s.get(id), nil
}

// Append appends one turn to id's history.
func (s *SessionStoreInMemory) Append(_ context.Context, id string, turn session.Turn) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.get(id)
	st.History = append(st.History, turn)
	return nil
}

// Save persists the WHOLE state for id atomically.
func (s *SessionStoreInMemory) Save(_ context.Context, id string, state session.SessionState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := state
	s.data[id] = &cp
	return nil
}

// Compact folds older turns into the summary for id.
func (s *SessionStoreInMemory) Compact(_ context.Context, id string, summarizer session.Summarizer, keepLast int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return session.DoCompact(s.get(id), summarizer, keepLast)
}

// SessionStoreSQL is a stdlib-only JSON-blob SessionStore (one blob per id). See
// the package deviation note: ":memory:" is in-process; a path dsn persists to a
// JSON file on the filesystem.
type SessionStoreSQL struct {
	mu   sync.Mutex
	dsn  string
	data map[string]session.SessionState
}

// NewSessionStoreSQL opens (or creates) the store at dsn (":memory:" or a path).
func NewSessionStoreSQL(dsn string) (*SessionStoreSQL, error) {
	s := &SessionStoreSQL{dsn: dsn, data: map[string]session.SessionState{}}
	if dsn != ":memory:" && dsn != "" {
		if err := s.loadFile(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *SessionStoreSQL) loadFile() error {
	raw, err := os.ReadFile(s.dsn)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	blobs := map[string]map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &blobs); err != nil {
			return err
		}
	}
	for id, d := range blobs {
		s.data[id] = session.SessionStateFromJSON(d)
	}
	return nil
}

func (s *SessionStoreSQL) flush() error {
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
	return os.WriteFile(s.dsn, raw, 0o644)
}

// Load returns the stored state for id (empty when absent).
func (s *SessionStoreSQL) Load(_ context.Context, id string) (session.SessionState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data[id], nil
}

// Save persists the WHOLE state for id.
func (s *SessionStoreSQL) Save(_ context.Context, id string, state session.SessionState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[id] = state
	return s.flush()
}

// Append appends one turn to id's history.
func (s *SessionStoreSQL) Append(_ context.Context, id string, turn session.Turn) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.data[id]
	st.History = append(st.History, turn)
	s.data[id] = st
	return s.flush()
}

// Compact folds older turns into the summary for id.
func (s *SessionStoreSQL) Compact(_ context.Context, id string, summarizer session.Summarizer, keepLast int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.data[id]
	if err := session.DoCompact(&st, summarizer, keepLast); err != nil {
		return err
	}
	s.data[id] = st
	return s.flush()
}

// ── Memory store ─────────────────────────────────────────────────────────────

// MemoryItem is one scored durable-memory hit.
type MemoryItem struct {
	Scope string  `json:"scope"`
	Key   string  `json:"key"`
	Value any     `json:"value"`
	Score float64 `json:"score"`
}

// ToJSON renders the item as a wire-stable map.
func (m MemoryItem) ToJSON() map[string]any {
	return map[string]any{"scope": m.Scope, "key": m.Key, "value": m.Value, "score": m.Score}
}

var tokenSplit = regexp.MustCompile(`[\W_]+`)

func tokens(text string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, t := range tokenSplit.Split(strings.ToLower(text), -1) {
		if t != "" {
			out[t] = struct{}{}
		}
	}
	return out
}

func overlapSearch(items map[string]any, query string, k int) []MemoryItem {
	q := tokens(query)
	type scored struct {
		score float64
		key   string
		value any
	}
	var hits []scored
	for key, value := range items {
		hay := tokens(key + " " + toStr(value))
		n := 0
		for t := range q {
			if _, ok := hay[t]; ok {
				n++
			}
		}
		denom := len(q)
		if denom == 0 {
			denom = 1
		}
		score := float64(n) / float64(denom)
		if score > 0 {
			hits = append(hits, scored{score: score, key: key, value: value})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	if k < len(hits) {
		hits = hits[:k]
	}
	out := make([]MemoryItem, 0, len(hits))
	for _, h := range hits {
		out = append(out, MemoryItem{Scope: "", Key: h.key, Value: h.value, Score: round4(h.score)})
	}
	return out
}

// MemoryStore is the pluggable durable-memory backend.
type MemoryStore interface {
	Read(ctx context.Context, scope, key string) (any, error)
	Write(ctx context.Context, scope, key string, value any) error
	Search(ctx context.Context, scope, query string, k int) ([]MemoryItem, error)
	Forget(ctx context.Context, scope, key string) (bool, error)
}

// MemoryStoreInMemory is the zero-infra default MemoryStore.
type MemoryStoreInMemory struct {
	mu   sync.Mutex
	data map[string]map[string]any
}

// NewMemoryStoreInMemory builds an empty in-memory memory store.
func NewMemoryStoreInMemory() *MemoryStoreInMemory {
	return &MemoryStoreInMemory{data: map[string]map[string]any{}}
}

// Read returns the value for (scope, key), or nil when absent.
func (m *MemoryStoreInMemory) Read(_ context.Context, scope, key string) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.data[scope]; ok {
		return s[key], nil
	}
	return nil, nil
}

// Write stores value under (scope, key).
func (m *MemoryStoreInMemory) Write(_ context.Context, scope, key string, value any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.data[scope]
	if !ok {
		s = map[string]any{}
		m.data[scope] = s
	}
	s[key] = value
	return nil
}

// Search returns the top-k token-overlap matches in scope, each scope-stamped.
func (m *MemoryStoreInMemory) Search(_ context.Context, scope, query string, k int) ([]MemoryItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := overlapSearch(m.data[scope], query, k)
	for i := range items {
		items[i].Scope = scope
	}
	return items, nil
}

// Forget deletes (scope, key); returns whether a value was present.
func (m *MemoryStoreInMemory) Forget(_ context.Context, scope, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.data[scope]
	if !ok {
		return false, nil
	}
	if _, present := s[key]; !present {
		return false, nil
	}
	delete(s, key)
	return true, nil
}

func toStr(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

func round4(v float64) float64 {
	return float64(int64(v*1e4+0.5)) / 1e4
}
