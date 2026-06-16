// Package redis is the Redis-backed SessionStore. The Python reference
// uses redis.asyncio with JSON-blob storage under “session:<id>“. The Go
// port speaks to a small client interface (Get/Set/Del on string keys),
// so tests can use an in-process fake and production can use a real
// go-redis client (or any compatible implementation).
//
// Contract: one JSON blob per id (the whole SessionState). Load returns an
// empty SessionState when absent. Save persists the whole state. Append
// loads + mutates + saves. Compact loads + compacts in place + saves.
package redis

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/mezon/agent-sdk-go/agent_sdk/session"
)

// Client is the small subset of go-redis's command surface this package
// uses. A real *redis.Client satisfies it (it has Get/Set/Del methods
// whose signatures match when you ignore variadic args).
type Client interface {
	Get(ctx context.Context, key string) (string, bool, error)
	Set(ctx context.Context, key, value string) error
	Del(ctx context.Context, key string) (bool, error)
}

// Store is the JSON-blob-per-id SessionStore.
type Store struct {
	mu     sync.Mutex
	client Client
	prefix string
}

// NewStoreWithClient builds a store over the given client and key prefix.
func NewStoreWithClient(client Client, prefix string) *Store {
	return &Store{client: client, prefix: prefix}
}

func (s *Store) key(id string) string {
	return s.prefix + id
}

// Load returns the current SessionState for id (empty when absent).
func (s *Store) Load(ctx context.Context, id string) (session.SessionState, error) {
	raw, ok, err := s.client.Get(ctx, s.key(id))
	if err != nil {
		return session.SessionState{}, err
	}
	if !ok || raw == "" {
		return session.SessionState{}, nil
	}
	var d map[string]any
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return session.SessionState{}, err
	}
	return session.SessionStateFromJSON(d), nil
}

func (s *Store) save(ctx context.Context, id string, state session.SessionState) error {
	raw, err := json.Marshal(state.ToJSON())
	if err != nil {
		return err
	}
	return s.client.Set(ctx, s.key(id), string(raw))
}

// Save persists the WHOLE state for id.
func (s *Store) Save(ctx context.Context, id string, state session.SessionState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save(ctx, id, state)
}

// Append appends one turn to id's history.
func (s *Store) Append(ctx context.Context, id string, turn session.Turn) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.Load(ctx, id)
	if err != nil {
		return err
	}
	state.History = append(state.History, turn)
	return s.save(ctx, id, state)
}

// Compact folds older turns into the summary for id.
func (s *Store) Compact(ctx context.Context, id string, summarizer session.Summarizer, keepLast int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.Load(ctx, id)
	if err != nil {
		return err
	}
	if err := session.DoCompact(&state, summarizer, keepLast); err != nil {
		return err
	}
	return s.save(ctx, id, state)
}
