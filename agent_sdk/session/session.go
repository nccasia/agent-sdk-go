// Package session holds Session — persisted conversation state over a pluggable
// backend. A Session is a small handle bundling an id and a backing store; it
// carries the rolling conversation (history + summary + facts + injected
// context + a memory snapshot), loaded at turn start, appended + compacted at
// turn end. SessionState.ToJSON/FromJSON is the forward/backward-tolerant
// stateless-serving contract. Ported from agent_sdk/session.py.
package session

import (
	"context"
	"fmt"
)

// SnapshotVersion stamps the snapshot schema version (the stateless-serving
// contract). Bumped only on a breaking change to SessionState.ToJSON.
// FromJSON is forward/backward tolerant — unknown keys ignored, missing keys
// default — so most additive extensions need no bump.
const SnapshotVersion = 1

// clip shortens a long turn to limit chars, keeping head + tail with an elision
// marker.
func clip(text string, limit int) string {
	r := []rune(text)
	if limit <= 0 || len(r) <= limit {
		return text
	}
	head := limit / 2
	tail := limit - head
	return fmt.Sprintf("%s\n[… %d chars elided …]\n%s", string(r[:head]), len(r)-limit, string(r[len(r)-tail:]))
}

// Turn is one conversation turn (user | assistant) with optional metadata.
type Turn struct {
	Role     string         `json:"role"`
	Content  string         `json:"content"`
	Metadata map[string]any `json:"metadata"`
}

// ToMessage renders the turn as a provider message.
func (t Turn) ToMessage() map[string]any {
	return map[string]any{"role": t.Role, "content": t.Content}
}

// ToJSON renders the turn as a wire-stable map.
func (t Turn) ToJSON() map[string]any {
	md := t.Metadata
	if md == nil {
		md = map[string]any{}
	}
	return map[string]any{"role": t.Role, "content": t.Content, "metadata": md}
}

// TurnFromJSON rebuilds a Turn from its JSON map (tolerant of missing keys).
func TurnFromJSON(d map[string]any) Turn {
	md, _ := d["metadata"].(map[string]any)
	if md == nil {
		md = map[string]any{}
	}
	return Turn{
		Role:     asString(d["role"]),
		Content:  asString(d["content"]),
		Metadata: md,
	}
}

// SessionState is the full per-session state (the snapshot contract).
type SessionState struct {
	History      []Turn         `json:"history"`
	Summary      string         `json:"summary"`
	Facts        []string       `json:"facts"`
	Context      []string       `json:"context"`
	SkillsInUse  []string       `json:"skills_in_use"`
	MetaFlowBias string         `json:"meta_flow_bias"`
	Memory       map[string]any `json:"memory"`
}

// Messages renders the conversation as provider messages — a trimmed transcript
// (primacy + recency). A short conversation renders verbatim; a long one folds
// n-first turns + the rolling summary into one block, blurs the middle, and
// keeps n-last turns capped to maxTurnChars.
func (s SessionState) Messages(firstN, lastM, maxTurnChars int) []map[string]any {
	h := s.History
	if s.Summary == "" && len(h) <= lastM {
		out := make([]map[string]any, 0, len(h))
		for _, t := range h {
			out = append(out, t.ToMessage())
		}
		return out
	}

	out := []map[string]any{}
	blocks := []string{}
	if s.Summary != "" {
		blocks = append(blocks, trimSpace(s.Summary))
	}
	var tail []Turn
	if len(h) > lastM {
		older := h[:len(h)-lastM]
		tail = h[len(h)-lastM:]
		n := firstN
		if n < 0 {
			n = 0
		}
		if n > len(older) {
			n = len(older)
		}
		first := older[:n]
		if len(first) > 0 {
			lines := make([]string, 0, len(first))
			for _, t := range first {
				role := "A"
				if t.Role == "user" {
					role = "U"
				}
				lines = append(lines, fmt.Sprintf("%s: %s", role, clip(t.Content, maxTurnChars)))
			}
			blocks = append(blocks, join(lines, "\n"))
		}
		elided := len(older) - len(first)
		if elided > 0 {
			blocks = append(blocks, fmt.Sprintf("[… %d earlier turns elided …]", elided))
		}
	} else {
		tail = h
	}
	if len(blocks) > 0 {
		out = append(out, map[string]any{"role": "user", "content": "[Conversation so far]\n" + join(blocks, "\n")})
	}
	for _, t := range tail {
		out = append(out, map[string]any{"role": t.Role, "content": clip(t.Content, maxTurnChars)})
	}
	return out
}

// Transcript renders the trimmed conversation as a U:/A: transcript with the
// same primacy/recency shaping as Messages.
func (s SessionState) Transcript(firstN, lastM, maxTurnChars int) string {
	lines := []string{}
	for _, m := range s.Messages(firstN, lastM, maxTurnChars) {
		tag := "A"
		if m["role"] == "user" {
			tag = "U"
		}
		content := trimSpace(asString(m["content"]))
		if content != "" {
			lines = append(lines, fmt.Sprintf("%s: %s", tag, content))
		}
	}
	return join(lines, "\n")
}

// ToJSON renders the full per-session snapshot (the stateless-serving contract).
func (s SessionState) ToJSON() map[string]any {
	hist := make([]map[string]any, 0, len(s.History))
	for _, t := range s.History {
		hist = append(hist, t.ToJSON())
	}
	return map[string]any{
		"v":              SnapshotVersion,
		"history":        hist,
		"summary":        s.Summary,
		"facts":          orEmptyStrs(s.Facts),
		"context":        orEmptyStrs(s.Context),
		"skills_in_use":  orEmptyStrs(s.SkillsInUse),
		"meta_flow_bias": s.MetaFlowBias,
		"memory":         orEmptyMap(s.Memory),
	}
}

// SessionStateFromJSON rebuilds a SessionState from ToJSON output. Tolerant by
// design: unknown keys ignored, missing keys defaulted.
func SessionStateFromJSON(d map[string]any) SessionState {
	if d == nil {
		d = map[string]any{}
	}
	hist := []Turn{}
	if raw, ok := d["history"].([]map[string]any); ok {
		for _, t := range raw {
			hist = append(hist, TurnFromJSON(t))
		}
	} else if raw, ok := d["history"].([]any); ok {
		for _, t := range raw {
			if tm, ok := t.(map[string]any); ok {
				hist = append(hist, TurnFromJSON(tm))
			}
		}
	}
	return SessionState{
		History:      hist,
		Summary:      asString(d["summary"]),
		Facts:        asStrings(d["facts"]),
		Context:      asStrings(d["context"]),
		SkillsInUse:  asStrings(d["skills_in_use"]),
		MetaFlowBias: asString(d["meta_flow_bias"]),
		Memory:       asMap(d["memory"]),
	}
}

// Summarizer folds a window of turns into a summary string.
type Summarizer func(turns []Turn) (string, error)

// SessionStore is the pluggable conversation-state backend.
type SessionStore interface {
	Load(ctx context.Context, id string) (SessionState, error)
	Append(ctx context.Context, id string, turn Turn) error
	Compact(ctx context.Context, id string, summarizer Summarizer, keepLast int) error
}

// SessionStoreSaver is the optional capability to persist the WHOLE state
// atomically (so a snapshot survives a stateless hop).
type SessionStoreSaver interface {
	Save(ctx context.Context, id string, state SessionState) error
}

// Session is a conversation handle: an id + a backing SessionStore. Defaults to
// an in-memory store when none is given (zero infra).
type Session struct {
	ID    string
	Store SessionStore
}

// New builds a Session over store, defaulting to an in-memory store when nil.
func New(id string, store SessionStore) *Session {
	if store == nil {
		store = newDefaultStore()
	}
	return &Session{ID: id, Store: store}
}

// Load returns the current SessionState.
func (s *Session) Load(ctx context.Context) (SessionState, error) {
	return s.Store.Load(ctx, s.ID)
}

// Append appends one turn.
func (s *Session) Append(ctx context.Context, turn Turn) error {
	return s.Store.Append(ctx, s.ID, turn)
}

// Save persists the WHOLE state atomically if the store supports it; returns
// false when the store only does append (the caller can fall back).
func (s *Session) Save(ctx context.Context, state SessionState) (bool, error) {
	saver, ok := s.Store.(SessionStoreSaver)
	if !ok {
		return false, nil
	}
	if err := saver.Save(ctx, s.ID, state); err != nil {
		return false, err
	}
	return true, nil
}

// Compact rolls older turns into the summary, keeping keepLast.
func (s *Session) Compact(ctx context.Context, summarizer Summarizer, keepLast int) error {
	return s.Store.Compact(ctx, s.ID, summarizer, keepLast)
}

// DoCompact applies the rolling-summary compaction in place: fold all but the
// last keepLast turns into the summary. Shared by every store implementation.
func DoCompact(state *SessionState, summarizer Summarizer, keepLast int) error {
	if len(state.History) <= keepLast {
		return nil
	}
	old := state.History[:len(state.History)-keepLast]
	recent := state.History[len(state.History)-keepLast:]
	newSummary, err := summarizer(old)
	if err != nil {
		return err
	}
	if state.Summary != "" {
		state.Summary = trimSpace(state.Summary + "\n" + newSummary)
	} else {
		state.Summary = newSummary
	}
	state.History = append([]Turn(nil), recent...)
	return nil
}

// ── default in-memory store (avoids an import cycle with stores/memory) ───────

type defaultStore struct {
	data map[string]*SessionState
}

func newDefaultStore() *defaultStore {
	return &defaultStore{data: map[string]*SessionState{}}
}

func (d *defaultStore) get(id string) *SessionState {
	st, ok := d.data[id]
	if !ok {
		st = &SessionState{}
		d.data[id] = st
	}
	return st
}

func (d *defaultStore) Load(_ context.Context, id string) (SessionState, error) {
	return *d.get(id), nil
}

func (d *defaultStore) Append(_ context.Context, id string, turn Turn) error {
	st := d.get(id)
	st.History = append(st.History, turn)
	return nil
}

func (d *defaultStore) Save(_ context.Context, id string, state SessionState) error {
	cp := state
	d.data[id] = &cp
	return nil
}

func (d *defaultStore) Compact(_ context.Context, id string, summarizer Summarizer, keepLast int) error {
	return DoCompact(d.get(id), summarizer, keepLast)
}

// ── helpers ──────────────────────────────────────────────────────────────────

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asStrings(v any) []string {
	switch x := v.(type) {
	case []string:
		return append([]string(nil), x...)
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			out = append(out, asString(e))
		}
		return out
	}
	return []string{}
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func orEmptyStrs(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

func orEmptyMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func join(parts []string, sep string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += sep
		}
		out += p
	}
	return out
}

func trimSpace(s string) string {
	start := 0
	for start < len(s) && isSpace(s[start]) {
		start++
	}
	end := len(s)
	for end > start && isSpace(s[end-1]) {
		end--
	}
	return s[start:end]
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\v' || b == '\f'
}
