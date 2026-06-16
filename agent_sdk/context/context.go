// Package context ports agent_sdk/context/context.py — AgentContext, the single
// shared-context handle every component holds. One scoped interface over all
// agent state: turn scope routes to the Scratchpad (RAM, dropped at turn end);
// the durable scopes route to the scoped Memory.
//
// Leaf-safe and opt-in — the default network is byte-identical until an
// integrator constructs and threads it.
package context

import (
	stdctx "context"
	"fmt"
	"strings"

	"github.com/nccasia/agent-sdk-go/agent_sdk/memory"
)

// Scope is the durability ladder — narrowest (RAM) to widest (durable, shared).
type Scope string

// The scope ladder.
const (
	ScopeTurn         Scope = "turn"
	ScopeConversation Scope = "conversation"
	ScopeChannel      Scope = "channel"
	ScopeUser         Scope = "user"
	ScopeBot          Scope = "bot"
)

// CoerceScope returns a Scope from a Scope or string.
func CoerceScope(scope string) Scope { return Scope(scope) }

// Evidence is the turn's shared evidence channel — retrieved chunks + a dedupe
// set. A view over the SAME two objects the engine threads into every call_tool.
type Evidence struct {
	RetrievedChunks []map[string]any
	AlreadyRead     map[string]struct{}
}

// NewEvidence builds an empty evidence channel.
func NewEvidence() *Evidence {
	return &Evidence{RetrievedChunks: []map[string]any{}, AlreadyRead: map[string]struct{}{}}
}

// Add appends a chunk, deduped by chunk_id (or id). Returns whether it was new.
func (e *Evidence) Add(chunk map[string]any) bool {
	cid := chunkID(chunk)
	if cid != "" {
		if _, seen := e.AlreadyRead[cid]; seen {
			return false
		}
		e.AlreadyRead[cid] = struct{}{}
	}
	cp := map[string]any{}
	for k, v := range chunk {
		cp[k] = v
	}
	e.RetrievedChunks = append(e.RetrievedChunks, cp)
	return true
}

// Len is the number of retrieved chunks.
func (e *Evidence) Len() int { return len(e.RetrievedChunks) }

func chunkID(chunk map[string]any) string {
	if v, ok := chunk["chunk_id"]; ok && v != nil {
		return fmt.Sprint(v)
	}
	if v, ok := chunk["id"]; ok && v != nil {
		return fmt.Sprint(v)
	}
	return ""
}

// AgentContext is the shared-context handle: one scoped interface over all agent
// state.
type AgentContext struct {
	Query       string
	StageID     string
	Path        string
	ActiveLobes []string

	scratchpad *memory.Scratchpad
	memory     *memory.Memory
	session    any
	identity   map[string]any
	channel    map[string]any
	evidence   *Evidence
}

// Options configures an AgentContext.
type Options struct {
	Query       string
	Scratchpad  *memory.Scratchpad
	Memory      *memory.Memory
	Session     any
	Identity    map[string]any
	Channel     map[string]any
	Evidence    *Evidence
	StageID     string
	Path        string
	ActiveLobes []string
}

// New builds an AgentContext. Memory is required only for the durable scopes.
func New(opts Options) *AgentContext {
	sp := opts.Scratchpad
	if sp == nil {
		sp = memory.NewScratchpad()
	}
	ev := opts.Evidence
	if ev == nil {
		ev = NewEvidence()
	}
	id := opts.Identity
	if id == nil {
		id = map[string]any{}
	}
	ch := opts.Channel
	if ch == nil {
		ch = map[string]any{}
	}
	return &AgentContext{
		Query:       opts.Query,
		StageID:     opts.StageID,
		Path:        opts.Path,
		ActiveLobes: opts.ActiveLobes,
		scratchpad:  sp,
		memory:      opts.Memory,
		session:     opts.Session,
		identity:    id,
		channel:     ch,
		evidence:    ev,
	}
}

// Turn is the duck-typed shape FromTurn reads off a live TurnContext.
type Turn struct {
	Query           string
	StageID         string
	ActivePath      string
	Identity        map[string]any
	Channel         map[string]any
	SessionMemory   any
	Scratchpad      *memory.Scratchpad
	RetrievedChunks []map[string]any
	AlreadyRead     map[string]struct{}
	ActiveLobes     []string
}

// FromTurn builds a context that wraps a live turn's state. The evidence view
// shares the turn's SAME RetrievedChunks / AlreadyRead objects.
func FromTurn(turn Turn, mem *memory.Memory) *AgentContext {
	chunks := turn.RetrievedChunks
	if chunks == nil {
		chunks = []map[string]any{}
	}
	seen := turn.AlreadyRead
	if seen == nil {
		seen = map[string]struct{}{}
	}
	ev := &Evidence{RetrievedChunks: chunks, AlreadyRead: seen}
	return New(Options{
		Query:       turn.Query,
		Scratchpad:  turn.Scratchpad,
		Memory:      mem,
		Session:     turn.SessionMemory,
		Identity:    turn.Identity,
		Channel:     turn.Channel,
		Evidence:    ev,
		StageID:     turn.StageID,
		Path:        turn.ActivePath,
		ActiveLobes: turn.ActiveLobes,
	})
}

func (c *AgentContext) mem() (*memory.Memory, error) {
	if c.memory == nil {
		return nil, fmt.Errorf("AgentContext has no durable Memory — only scope=turn is available. " +
			"Construct with memory=Memory(...) to use conversation/channel/user/bot scopes.")
	}
	return c.memory, nil
}

// Get reads one value at scope. turn → scratchpad; else → durable memory.
func (c *AgentContext) Get(ctx stdctx.Context, key string, scope Scope, def any) (any, error) {
	if scope == "" {
		scope = ScopeTurn
	}
	if scope == ScopeTurn {
		return c.scratchpad.Get(key, def), nil
	}
	m, err := c.mem()
	if err != nil {
		return nil, err
	}
	val, err := m.Read(ctx, string(scope), key)
	if err != nil {
		return nil, err
	}
	if val == nil {
		return def, nil
	}
	return val, nil
}

// Set writes one value at scope.
func (c *AgentContext) Set(ctx stdctx.Context, key string, value any, scope Scope) error {
	if scope == "" {
		scope = ScopeTurn
	}
	if scope == ScopeTurn {
		c.scratchpad.Set(key, value)
		return nil
	}
	m, err := c.mem()
	if err != nil {
		return err
	}
	return m.Write(ctx, string(scope), key, value)
}

// Delete drops one value at scope. Returns whether it existed.
func (c *AgentContext) Delete(ctx stdctx.Context, key string, scope Scope) (bool, error) {
	if scope == "" {
		scope = ScopeTurn
	}
	if scope == ScopeTurn {
		return c.scratchpad.Delete(key), nil
	}
	m, err := c.mem()
	if err != nil {
		return false, err
	}
	return m.Forget(ctx, string(scope), key)
}

// Search does a free-text find at scope. Turn scope scans the scratchpad;
// durable scopes delegate to the backend.
func (c *AgentContext) Search(ctx stdctx.Context, query string, scope Scope, k int) ([]memory.MemoryItem, error) {
	if scope == "" {
		scope = ScopeConversation
	}
	if k <= 0 {
		k = 5
	}
	if scope == ScopeTurn {
		return c.scratchpadSearch(query, k), nil
	}
	m, err := c.mem()
	if err != nil {
		return nil, err
	}
	return m.Search(ctx, string(scope), query, k)
}

func (c *AgentContext) scratchpadSearch(query string, k int) []memory.MemoryItem {
	q := strings.ToLower(query)
	var out []memory.MemoryItem
	for _, key := range c.scratchpad.Keys() {
		val := c.scratchpad.Get(key, nil)
		if q == "" || strings.Contains(strings.ToLower(key), q) || strings.Contains(strings.ToLower(fmt.Sprint(val)), q) {
			out = append(out, memory.MemoryItem{Scope: string(ScopeTurn), Key: key, Value: val, Score: 1.0})
		}
		if len(out) >= k {
			break
		}
	}
	return out
}

// Identity is who: principal / user / tenant. Host-provided, never model-forged.
func (c *AgentContext) Identity() map[string]any { return c.identity }

// Channel is where: room / workspace context.
func (c *AgentContext) Channel() map[string]any { return c.channel }

// Session is the conversation: history, summary, facts (or nil).
func (c *AgentContext) Session() any { return c.session }

// EvidenceCh is the turn's shared evidence channel.
func (c *AgentContext) EvidenceCh() *Evidence { return c.evidence }

// Scratchpad is the turn's RAM, for direct sync use inside a hot loop.
func (c *AgentContext) Scratchpad() *memory.Scratchpad { return c.scratchpad }

// HasDurable reports whether a durable backend is attached.
func (c *AgentContext) HasDurable() bool { return c.memory != nil }
