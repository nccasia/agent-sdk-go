package memory

import (
	"context"
	"fmt"

	storemem "github.com/mezon/agent-sdk-go/agent_sdk/stores/memory"
)

// DefaultScopes are the durable scopes a Memory allows by default.
var DefaultScopes = []string{"conversation", "channel", "user", "bot"}

// MemoryItem is one scored durable-memory hit (re-exported from stores/memory).
type MemoryItem = storemem.MemoryItem

// Memory is durable, scoped agent memory over a pluggable backend store.
type Memory struct {
	Store  storemem.MemoryStore
	Scopes []string
}

// NewMemory builds a Memory over store (a zero-infra in-memory store if nil),
// restricted to scopes (DefaultScopes if empty).
func NewMemory(store storemem.MemoryStore, scopes []string) *Memory {
	if store == nil {
		store = storemem.NewMemoryStoreInMemory()
	}
	sc := scopes
	if len(sc) == 0 {
		sc = append([]string(nil), DefaultScopes...)
	}
	return &Memory{Store: store, Scopes: sc}
}

func (m *Memory) check(scope string) error {
	for _, s := range m.Scopes {
		if s == scope {
			return nil
		}
	}
	return fmt.Errorf("scope %q not in allowed scopes %v", scope, m.Scopes)
}

// Read returns the value at (scope, key).
func (m *Memory) Read(ctx context.Context, scope, key string) (any, error) {
	if err := m.check(scope); err != nil {
		return nil, err
	}
	return m.Store.Read(ctx, scope, key)
}

// Write stores value at (scope, key).
func (m *Memory) Write(ctx context.Context, scope, key string, value any) error {
	if err := m.check(scope); err != nil {
		return err
	}
	return m.Store.Write(ctx, scope, key, value)
}

// Search returns the top-k matches in scope for query.
func (m *Memory) Search(ctx context.Context, scope, query string, k int) ([]MemoryItem, error) {
	if err := m.check(scope); err != nil {
		return nil, err
	}
	return m.Store.Search(ctx, scope, query, k)
}

// Forget deletes (scope, key); returns whether a value was present.
func (m *Memory) Forget(ctx context.Context, scope, key string) (bool, error) {
	if err := m.check(scope); err != nil {
		return false, err
	}
	return m.Store.Forget(ctx, scope, key)
}

// ToolRuntime builds a MemoryToolRuntime over this Memory.
func (m *Memory) ToolRuntime() *MemoryToolRuntime {
	return &MemoryToolRuntime{Memory: m}
}

// MemoryToolRuntime exposes one memory tool (remember/recall/forget).
type MemoryToolRuntime struct {
	Memory  *Memory
	Updates []map[string]any
}

// GetToolSpecs returns the memory tool spec.
func (r *MemoryToolRuntime) GetToolSpecs() []map[string]any {
	return []map[string]any{
		{
			"name": "memory",
			"description": "Durable memory across the conversation. Use action=remember to save a " +
				"fact, recall to look one up (by key or free-text query), forget to delete.",
			"input_schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{"type": "string", "enum": []string{"remember", "recall", "forget"}},
					"scope":  map[string]any{"type": "string", "enum": append([]string(nil), r.Memory.Scopes...)},
					"key":    map[string]any{"type": "string"},
					"value":  map[string]any{"type": "string"},
					"query":  map[string]any{"type": "string"},
				},
				"required": []string{"action", "scope"},
			},
		},
	}
}

// CallTool dispatches the memory tool.
func (r *MemoryToolRuntime) CallTool(ctx context.Context, name string, inp map[string]any) (string, error) {
	if name != "memory" {
		return "Error: unknown tool '" + name + "'.", nil
	}
	action := asString(inp["action"])
	scope := asString(inp["scope"])
	if scope == "" {
		scope = "conversation"
	}
	key := asString(inp["key"])
	switch action {
	case "remember":
		if err := r.Memory.Write(ctx, scope, key, asString(inp["value"])); err != nil {
			return "Error: " + err.Error(), nil
		}
		r.Updates = append(r.Updates, map[string]any{"action": "remember", "scope": scope, "key": key})
		return fmt.Sprintf("Remembered %q in %s.", key, scope), nil
	case "recall":
		if q := asString(inp["query"]); q != "" {
			items, err := r.Memory.Search(ctx, scope, q, 5)
			if err != nil {
				return "Error: " + err.Error(), nil
			}
			if len(items) == 0 {
				return "(nothing found)", nil
			}
			var lines []string
			for _, i := range items {
				lines = append(lines, fmt.Sprintf("- %s: %v", i.Key, i.Value))
			}
			return joinLines(lines), nil
		}
		val, err := r.Memory.Read(ctx, scope, key)
		if err != nil {
			return "Error: " + err.Error(), nil
		}
		if val == nil {
			return "(not set)", nil
		}
		return fmt.Sprintf("%s: %v", key, val), nil
	case "forget":
		ok, err := r.Memory.Forget(ctx, scope, key)
		if err != nil {
			return "Error: " + err.Error(), nil
		}
		if ok {
			r.Updates = append(r.Updates, map[string]any{"action": "forget", "scope": scope, "key": key})
			return "Forgotten.", nil
		}
		return "(nothing to forget)", nil
	}
	return fmt.Sprintf("Error: unknown action %q.", action), nil
}

func joinLines(lines []string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}
