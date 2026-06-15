// Package contracts holds the narrow per-turn data contracts the engine and
// lobe behaviors share — the injectable seams (LlmCall, LobeServices,
// ToolRuntime), the turn context, the citation/memo models, and the canonical
// pinned-lobe set. Ported from agent_sdk/contracts/{llm,memo,pins,services,
// tools,turn}.py.
package contracts

import (
	"context"
	"sort"
	"strings"
)

// PinnedLobes is the canonical set of output-contract lobe ids that bypass the
// activation threshold and can never be disabled *when present* (filter from
// SafetyPlugin; cite from RagPlugin). Mirrors PINNED_LOBES in
// agent_sdk/contracts/pins.py.
var PinnedLobes = map[string]struct{}{
	"cite":   {},
	"filter": {},
}

// IsPinned reports whether a lobe id is in the canonical pinned set.
func IsPinned(id string) bool {
	_, ok := PinnedLobes[id]
	return ok
}

// SortedPinnedLobes returns the pinned ids in deterministic sorted order.
func SortedPinnedLobes() []string {
	out := make([]string, 0, len(PinnedLobes))
	for id := range PinnedLobes {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// ── LLM seam (contracts/llm.py) ──────────────────────────────────────────────

// LlmRequest is one LLM call on behalf of a lobe behavior. stage selects the
// policy stage whose model config applies; system may be a plain string or a
// cache-split block array.
type LlmRequest struct {
	Stage       string
	System      any // string | []any
	Messages    []map[string]any
	MaxTokens   int
	Temperature *float64
	Tools       []map[string]any
	CountUsage  bool
}

// LlmCall is the narrow, injectable seam a lobe behavior calls. The production
// implementation wraps per-stage model resolution + the provider create call +
// usage roll-up.
type LlmCall interface {
	Call(ctx context.Context, req LlmRequest) (any, error)
}

// ── Side-effect seams (contracts/services.py) ────────────────────────────────

// LobeServices are the injected side-effect seams available to executable lobe
// classes. Fields are nil when a service is not wired for the turn.
type LobeServices struct {
	Llm                 LlmCall
	ExecuteTools        func(ctx context.Context, args ...any) (any, error)
	Embed               func(args ...any) (any, error)
	PostInternalContext func(ctx context.Context, args ...any) (any, error)
	SessionFactory      func(args ...any) any
	Redis               any
	Emit                func(ctx context.Context, args ...any) (any, error)
}

// ── Tool runtime (contracts/tools.py) ────────────────────────────────────────

// ToolRuntime is the runtime boundary between the agent harness and executable
// tools.
type ToolRuntime interface {
	// GetToolSpecs returns provider-compatible tool specs available this turn.
	GetToolSpecs() []map[string]any
	// CallTool executes one tool call and returns model-visible tool output.
	CallTool(ctx context.Context, name string, inp map[string]any, retrievedChunks []map[string]any, alreadyRead map[string]struct{}) (string, error)
}

// CompositeToolRuntime combines built-in tools with zero or more MCP runtimes.
type CompositeToolRuntime struct {
	runtimes  []ToolRuntime
	toolOwner map[string]ToolRuntime
}

// NewCompositeToolRuntime builds a composite over the given runtimes.
func NewCompositeToolRuntime(runtimes []ToolRuntime) *CompositeToolRuntime {
	return &CompositeToolRuntime{runtimes: runtimes, toolOwner: map[string]ToolRuntime{}}
}

// GetToolSpecs returns the merged, de-duplicated specs across runtimes (first
// owner of a name wins).
func (c *CompositeToolRuntime) GetToolSpecs() []map[string]any {
	specs := []map[string]any{}
	c.toolOwner = map[string]ToolRuntime{}
	for _, rt := range c.runtimes {
		for _, spec := range rt.GetToolSpecs() {
			name, _ := spec["name"].(string)
			if name == "" {
				continue
			}
			if _, dup := c.toolOwner[name]; dup {
				continue
			}
			c.toolOwner[name] = rt
			specs = append(specs, spec)
		}
	}
	return specs
}

// CallTool dispatches a call to the runtime that owns the named tool.
func (c *CompositeToolRuntime) CallTool(ctx context.Context, name string, inp map[string]any, retrievedChunks []map[string]any, alreadyRead map[string]struct{}) (string, error) {
	rt := c.toolOwner[name]
	if rt == nil {
		c.GetToolSpecs()
		rt = c.toolOwner[name]
	}
	if rt == nil {
		return "Error: unknown tool '" + name + "'. Use only the provided tools.", nil
	}
	return rt.CallTool(ctx, name, inp, retrievedChunks, alreadyRead)
}

// ── Citation / memo models (contracts/memo.py) ───────────────────────────────

// Citation grounds a span of an answer in a source chunk.
type Citation struct {
	ChunkID        string `json:"chunk_id"`
	SourceRef      string `json:"source_ref"`
	SupportingSpan [2]int `json:"supporting_span"` // (start, end)
}

// Claim is one supported assertion with its supporting chunk ids.
type Claim struct {
	Text               string   `json:"text"`
	SupportingChunkIDs []string `json:"supporting_chunk_ids"`
	Confidence         float64  `json:"confidence"` // 0..1
}

// Memo is an aspect's resolved claims plus what remains unresolved.
type Memo struct {
	AspectID   string   `json:"aspect_id"`
	Claims     []Claim  `json:"claims"`
	Unresolved []string `json:"unresolved"`
	TokensUsed int      `json:"tokens_used"`
}

// Section is one composed answer section and its source memos.
type Section struct {
	Content     string   `json:"content"`
	SourceMemos []string `json:"source_memos"`
}

// Synthesis is the composed answer across sections.
type Synthesis struct {
	Sections    []Section `json:"sections"`
	OpenThreads []string  `json:"open_threads"`
}

// FinalEnvelope is the turn's final answer-or-refusal contract.
type FinalEnvelope struct {
	Status         string           `json:"status"` // "answered" | "refused"
	AnswerMarkdown *string          `json:"answer_markdown"`
	Citations      []Citation       `json:"citations"`
	RefusalReason  *string          `json:"refusal_reason"` // no_citations|budget_exceeded|policy_violation
	TraceID        string           `json:"trace_id"`
	RefusalMessage *string          `json:"refusal_message,omitempty"`
	MemoryUpdates  []map[string]any `json:"memory_updates"`
}

var footerPrefixes = []string{
	"• Đã ghi nhớ:", "• đã quên:", "• Memory updated:", "• Forgotten:",
}

// StripMemoryFooter drops the trailing memory-confirmation line from an answer.
// The footer is chat-UI chrome, not conversation content.
func StripMemoryFooter(text string) string {
	if text == "" {
		return text
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > 0 {
		last := strings.TrimSpace(lines[len(lines)-1])
		for _, p := range footerPrefixes {
			if strings.HasPrefix(last, p) {
				return strings.TrimRight(strings.Join(lines[:len(lines)-1], "\n"), " \t\n")
			}
		}
	}
	return text
}

// ── Per-turn context (contracts/turn.py) ─────────────────────────────────────

// PromptContribution is one lobe-owned prompt block. stability is consumed by
// the interpreter's cache-aware composition.
type PromptContribution struct {
	Text      string
	Stability string // "stable" | "slow" | "volatile"
	StageIDs  []string
	Source    string
}

// TurnContext is the narrow per-turn data passed to lobe classes — conversation
// state plus injected services, so lobes can be tested without the interpreter.
type TurnContext struct {
	Query           string
	Policy          map[string]any
	Services        LobeServices
	StageID         string
	ActivePath      string
	PreviousPath    string
	ActiveLobes     map[string]struct{}
	Blackboard      any
	Scratchpad      any
	LobeOutputs     map[string]any
	Identity        map[string]any
	Channel         map[string]any
	SessionMemory   any
	MemoryItems     []map[string]any
	TaskItems       []map[string]any
	CatalogItems    []map[string]any
	RetrievedChunks []map[string]any
	AlreadyRead     map[string]struct{}
	Degraded        []string
}

// NewTurnContext builds a TurnContext with the empty-collection defaults the
// Python dataclass uses via field(default_factory=...).
func NewTurnContext(query string) *TurnContext {
	return &TurnContext{
		Query:           query,
		Policy:          map[string]any{},
		ActiveLobes:     map[string]struct{}{},
		LobeOutputs:     map[string]any{},
		Identity:        map[string]any{},
		Channel:         map[string]any{},
		RetrievedChunks: []map[string]any{},
		AlreadyRead:     map[string]struct{}{},
		Degraded:        []string{},
	}
}

// LobeResult is the standard result envelope for class-based lobe execution.
type LobeResult struct {
	Value    any
	Nodes    []any
	Prompt   []PromptContribution
	Metadata map[string]any
}

// StageResult is the result envelope for a single stage's execution.
type StageResult struct {
	StageName    string
	Path         string
	Text         string
	ContextNodes []any
	ToolCalls    []any
	TokensIn     int
	TokensOut    int
	LatencyMs    float64
	Metadata     map[string]any
}
