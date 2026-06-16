package agent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/mezon/agent-sdk-go/agent_sdk/clients"
	"github.com/mezon/agent-sdk-go/agent_sdk/contracts"
	"github.com/mezon/agent-sdk-go/agent_sdk/core/spec"
	"github.com/mezon/agent-sdk-go/agent_sdk/events"
	"github.com/mezon/agent-sdk-go/agent_sdk/flows"
	"github.com/mezon/agent-sdk-go/agent_sdk/metacognition"
	"github.com/mezon/agent-sdk-go/agent_sdk/result"
	"github.com/mezon/agent-sdk-go/agent_sdk/session"
)

// Engine is the minimal turn driver the PreactAgent façade uses. It owns the
// per-turn state, runs the LLM, dispatches tool calls, and emits the typed
// event stream. It mirrors the role of the Python `Engine` (the full kernel
// lives across the engine/network/lobes/flows Go packages) — what the agent
// surface needs is: a stable call site to drive a turn, a final
// AgentResult-shaped envelope, and a set of seam fields plugins patch
// (finalize hooks, tool-result hooks, prefetch hooks, tool filters, the
// answer-retry).
type Engine struct {
	mu sync.Mutex

	Client           clients.LlmCall
	Lobes            []spec.Lobe
	Stages           []any
	Flows            []flows.Flow
	Paths            []spec.Path
	Skills           []any
	Tools            any
	Instructions     string
	Metacognition    *metacognition.Metacognition
	Memory           Memory
	MemoryStore      any
	SystemAddendum   string
	RequireCitations bool
	ShareHistory     bool
	ToolsInPrompt    bool
	Funnel           bool
	TZ               string
	Lang             string
	PromptFormat     string
	Context          any
	Budgets          map[string]any

	// Plugin deep-hooks (populated by PreactAgent from AgentSetup).
	PrefetchHooks   []PrefetchHookFn
	ToolFilters     []ToolFilterFn
	FinalizeHooks   []FinalizeHookFn
	ToolResultHooks []ToolResultHookFn

	// The anti-hedge answer-retry seam (host-injected).
	AnswerRetry func(answer string) *string

	// Pre-turn gate: returns non-nil to short-circuit the turn.
	PreTurnGate func(query string, state session.SessionState) *result.AgentResult

	// Internal.
	resolvedPaths []spec.Path
}

// RunRequest is the input to Engine.Run.
type RunRequest struct {
	Query   string
	State   session.SessionState
	Session *session.Session // optional — when present, Run is stateless otherwise
	TraceID string
}

// RunResult is the output of Engine.Run.
type RunResult struct {
	Result *result.AgentResult
	Events []events.AgentEvent
	Trace  result.Trace
	State  session.SessionState
}

// NewEngine builds an Engine from explicit args (the PreactAgent calls this
// once at construction time).
func NewEngine(c clients.LlmCall) *Engine {
	return &Engine{
		Client:        c,
		TZ:            "UTC",
		Lang:          "en",
		PromptFormat:  "xml",
		Budgets:       map[string]any{},
		Metacognition: mustObserveMeta(),
	}
}

// Inspect is the no-LLM routing probe — recognize the path, surface the
// flow, list the active lobes. Mirrors Engine.inspect.
func (e *Engine) Inspect(query string) result.ActivationSnapshot {
	ctx := buildContext(query, session.SessionState{}, e.Context)
	scores := map[string]float64{}
	for _, p := range e.Paths {
		if p.Recognizer != nil {
			scores[p.Name] = p.Recognizer(ctx)
		}
	}
	resolved := map[string]any{"name": "emergent", "score": 0.0}
	if len(scores) > 0 {
		bestName := "emergent"
		bestScore := 0.0
		for n, s := range scores {
			if s > bestScore {
				bestName = n
				bestScore = s
			}
		}
		resolved = map[string]any{"name": bestName, "score": bestScore}
	}
	flow := e.selectFlow(resolved)
	lobeRows := make([]map[string]any, 0, len(e.Lobes))
	for _, l := range e.Lobes {
		score := 0.0
		if l.Signals != nil {
			for _, v := range l.Signals(ctx) {
				score += v
			}
		}
		lobeRows = append(lobeRows, map[string]any{
			"id": l.ID, "layer": l.Layer, "activated": score > l.MinActivation,
			"score": score, "reason": "",
		})
	}
	flowNames := []string{}
	if flow != nil {
		for _, sid := range flow.Stages {
			flowNames = append(flowNames, sid)
		}
	}
	score := 0.0
	if s, ok := resolved["score"].(float64); ok {
		score = s
	}
	name, _ := resolved["name"].(string)
	return result.ActivationSnapshot{
		Path:   result.PathScore{Name: name, Score: score},
		Lobes:  lobeRows,
		Flow:   flowNames,
		Budget: e.Budgets,
	}
}

// Run is the single-turn driver: pre-checks, resolve flow, run each stage's
// LLM call (with tool dispatch), finalize, emit the typed event stream.
func (e *Engine) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	traceID := req.TraceID
	if traceID == "" {
		traceID = newTraceID()
	}
	// Pre-turn gate short-circuit.
	if e.PreTurnGate != nil {
		if early := e.PreTurnGate(req.Query, req.State); early != nil {
			ev := events.Stamp(&events.Final{Result: early}, traceID).(*events.Final)
			return &RunResult{Result: early, Events: []events.AgentEvent{ev}}, nil
		}
	}
	// 1) Path resolution.
	pathCtx := buildContext(req.Query, req.State, e.Context)
	scores := map[string]float64{}
	for _, p := range e.Paths {
		if p.Recognizer != nil {
			scores[p.Name] = p.Recognizer(pathCtx)
		}
	}
	path := resolvePath(scores, e.Paths)
	flow := e.selectFlow(path)
	out := []events.AgentEvent{}
	out = append(out, events.Stamp(&events.RunStart{}, traceID))
	pathName, _ := path["name"].(string)
	pathScore, _ := path["score"].(float64)
	out = append(out, events.Stamp(&events.PathResolved{Path: pathName, Score: pathScore}, traceID))
	if flow == nil {
		// No matching flow — emit a refusal.
		r := &result.AgentResult{
			Status:  "refused",
			Text:    "",
			Refusal: &result.Refusal{Reason: "no_flow", Message: "no flow recognized this turn"},
		}
		out = append(out, events.Stamp(&events.Final{Result: r}, traceID))
		return &RunResult{Result: r, Events: out}, nil
	}
	// 2) Run each stage's loop.
	stageRegistry := buildStageRegistry(e.Stages)
	flowStages := []map[string]any{}
	citations := []contracts.Citation{}
	toolCalls := []string{}
	finalAnswer := ""
	refusal := (*result.Refusal)(nil)
	memoryUpdates := []result.MemoryUpdate{}
	usage := result.Usage{}
	stageOrder := flow.Stages
	for _, sid := range stageOrder {
		_ = stageRegistry.Get(sid) // resolved per-stage
		out = append(out, events.Stamp(&events.StageStart{Flow: flow.ID(), Stage: sid}, traceID))
		steps := []map[string]any{}
		// Make the LLM call for this stage.
		messages := buildMessages(req.Query, req.State, e.ShareHistory)
		system := e.composeSystem(sid)
		tools := e.toolSpecs()
		msg, err := e.callLLM(ctx, sid, system, messages, tools)
		if err != nil {
			return nil, err
		}
		if msg.Usage.InputTokens > 0 || msg.Usage.OutputTokens > 0 {
			usage = usageFromProvider(usage, msg.Usage)
		}
		// 3) Process tool calls.
		for _, tu := range msg.ToolUses() {
			toolCalls = append(toolCalls, tu.Name)
			out = append(out, events.Stamp(&events.ToolCall{ID: tu.ID, Name: tu.Name, Input: tu.Input}, traceID))
			if msg, _ := e.runToolFilters(sid, tu); msg != "" {
				// short-circuit with the filter's message
				out = append(out, events.Stamp(&events.ToolResult{ID: tu.ID, Name: tu.Name, Output: msg}, traceID))
				continue
			}
			out, memoryUpdates = e.dispatchTool(ctx, tu, citations, out, traceID, memoryUpdates)
		}
		// 4) Collect text answer.
		text := msg.Text()
		if text != "" {
			steps = append(steps, map[string]any{"kind": "answer", "text": text})
			finalAnswer = text
			// Emit a text-delta so the AgentStream.TextStream() channel
			// surfaces the answer (matches the Python engine.stream behavior).
			out = append(out, events.Stamp(&events.TextDelta{Text: text}, traceID))
		}
		out = append(out, events.Stamp(&events.StageEnd{Flow: flow.ID(), Stage: sid, Usage: usage}, traceID))
		flowStages = append(flowStages, map[string]any{
			"flow": flow.ID(), "stage": sid, "steps": steps, "skipped": false,
		})
	}
	// 5) Finalize hooks: rewrite answer / augment citations / force refusal.
	for _, hook := range e.FinalizeHooks {
		if hook == nil {
			continue
		}
		newAnswer, newCites, refusalReason := hook(finalAnswer, citations, nil, e.isGrounded(flow), e.RequireCitations)
		finalAnswer = newAnswer
		citations = newCites
		if refusalReason != "" {
			refusal = &result.Refusal{Reason: refusalReason, Message: "policy_violation"}
		}
	}
	// 6) Grounding check (require_citations).
	if e.RequireCitations && len(citations) == 0 && refusal == nil {
		refusal = &result.Refusal{Reason: "no_citations", Message: "no citations"}
	}
	res := &result.AgentResult{
		Text:          finalAnswer,
		Usage:         usage,
		Citations:     citations,
		MemoryUpdates: memoryUpdates,
	}
	if refusal != nil {
		res.Status = "refused"
		res.Refusal = refusal
	} else {
		res.Status = "answered"
	}
	res.Trace = result.Trace{
		TraceID:    traceID,
		Path:       path,
		FlowStages: flowStages,
	}
	out = append(out, events.Stamp(&events.Final{Result: res}, traceID))
	return &RunResult{Result: res, Events: out, State: req.State, Trace: res.Trace}, nil
}

// dispatchTool runs a tool call (after the filters) and appends the
// result event. Citations discovered by tool-result hooks are added to the
// citations list. Memory updates from memory tools are recorded.
func (e *Engine) dispatchTool(
	ctx context.Context, tu clients.ToolUseBlock,
	citations []contracts.Citation,
	out []events.AgentEvent, traceID string,
	updates []result.MemoryUpdate,
) ([]events.AgentEvent, []result.MemoryUpdate) {
	// Memory tool: record the update regardless of whether the runtime is
	// available. The engine's "memory" tool is a special-cased side channel
	// that the runtime owns; the agent's Memory also writes through here.
	if tu.Name == "memory" {
		updates = append(updates, result.MemoryUpdate{
			Action: asString(tu.Input["action"]),
			Scope:  asString(tu.Input["scope"]),
			Key:    asString(tu.Input["key"]),
		})
	}
	if e.Tools == nil {
		out = append(out, events.Stamp(&events.ToolResult{ID: tu.ID, Name: tu.Name, Output: "no tools available"}, traceID))
		return out, updates
	}
	rt, ok := e.Tools.(contracts.ToolRuntime)
	if !ok {
		out = append(out, events.Stamp(&events.ToolResult{ID: tu.ID, Name: tu.Name, Output: "tool runtime unavailable"}, traceID))
		return out, updates
	}
	output, _ := rt.CallTool(ctx, tu.Name, tu.Input, nil, map[string]struct{}{})
	out = append(out, events.Stamp(&events.ToolResult{ID: tu.ID, Name: tu.Name, Output: output}, traceID))
	// Tool-result hooks: extract citations a tool emits in its output.
	for _, hook := range e.ToolResultHooks {
		if hook == nil {
			continue
		}
		for _, c := range hook(tu.Name, output) {
			citations = append(citations, c)
		}
	}
	return out, updates
}

func (e *Engine) runToolFilters(stageID string, tu clients.ToolUseBlock) (string, bool) {
	if len(e.ToolFilters) == 0 {
		return "", false
	}
	for _, f := range e.ToolFilters {
		if f == nil {
			continue
		}
		if msg := f(stageID, tu.Name, tu.Input); msg != "" {
			return msg, true
		}
	}
	return "", false
}

func (e *Engine) callLLM(ctx context.Context, stage string, system any, messages []map[string]any, tools []map[string]any) (clients.Message, error) {
	if e.Client == nil {
		return clients.Message{}, fmt.Errorf("engine: no client")
	}
	out, err := e.Client.Call(ctx, clients.Request{
		Stage:    stage,
		System:   system,
		Messages: messages,
		Tools:    tools,
	})
	if err != nil {
		return clients.Message{}, err
	}
	if msg, ok := out.(clients.Message); ok {
		return msg, nil
	}
	return clients.Message{}, fmt.Errorf("engine: client returned %T, expected clients.Message", out)
}

func (e *Engine) toolSpecs() []map[string]any {
	if e.Tools == nil {
		return nil
	}
	rt, ok := e.Tools.(contracts.ToolRuntime)
	if !ok {
		return nil
	}
	return rt.GetToolSpecs()
}

func (e *Engine) composeSystem(stage string) string {
	out := e.Instructions
	if e.SystemAddendum != "" {
		out += "\n\n" + e.SystemAddendum
	}
	return out
}

func (e *Engine) isGrounded(flow *flows.Flow) bool {
	if flow == nil {
		return false
	}
	return flow.Grounds
}

func (e *Engine) selectFlow(path map[string]any) *flows.Flow {
	name, _ := path["name"].(string)
	if name == "" {
		name = "emergent"
	}
	for i, f := range e.Flows {
		if f.ID() == name || f.Name() == name {
			return &e.Flows[i]
		}
	}
	// Fallback: qna (when it exists) or the first flow.
	for i, f := range e.Flows {
		if f.ID() == "qna" {
			return &e.Flows[i]
		}
	}
	if len(e.Flows) > 0 {
		return &e.Flows[0]
	}
	return nil
}

// ── helpers ─────────────────────────────────────────────────────────────────

func buildContext(query string, state session.SessionState, hostCtx any) map[string]any {
	out := map[string]any{
		"query":       query,
		"is_question": endsWithQuestion(query),
		"word_count":  len(splitWords(query)),
		"has_history": len(state.History) > 0,
		"ambiguous":   false,
	}
	if m, ok := hostCtx.(map[string]any); ok {
		for k, v := range m {
			if _, set := out[k]; !set {
				switch v.(type) {
				case bool, int, int64, float64, string:
					out[k] = v
				}
			}
		}
	}
	return out
}

func resolvePath(scores map[string]float64, paths []spec.Path) map[string]any {
	bestName := "emergent"
	bestScore := 0.0
	for n, s := range scores {
		if s > bestScore {
			bestName = n
			bestScore = s
		}
	}
	return map[string]any{"name": bestName, "score": bestScore}
}

func buildStageRegistry(stages []any) *StageRegistryLite {
	r := &StageRegistryLite{stages: map[string]any{}}
	for _, s := range stages {
		id := stageID(s)
		if id != "" {
			r.stages[id] = s
		}
	}
	return r
}

func stageID(s any) string {
	switch v := s.(type) {
	case spec.Stage:
		return v.ID
	case *spec.Stage:
		if v != nil {
			return v.ID
		}
	case flows.FlowStep:
		return v.Name
	case *flows.FlowStep:
		if v != nil {
			return v.Name
		}
	case map[string]any:
		if id, ok := v["id"].(string); ok {
			return id
		}
		if name, ok := v["name"].(string); ok {
			return name
		}
	}
	return ""
}

// StageRegistryLite is a minimal id → stage lookup the engine builds from the
// configured Stages slice.
type StageRegistryLite struct {
	stages map[string]any
}

// Get returns the stage with the given id, or nil.
func (r *StageRegistryLite) Get(id string) any { return r.stages[id] }

// IDs returns the registered stage ids in insertion order.
func (r *StageRegistryLite) IDs() []string {
	out := make([]string, 0, len(r.stages))
	for k := range r.stages {
		out = append(out, k)
	}
	return out
}

func buildMessages(query string, state session.SessionState, share bool) []map[string]any {
	msgs := state.Messages(3, 6, 8000)
	if !share {
		// Per-stage isolation: only the last user message is included.
		msgs = nil
	}
	msgs = append(msgs, map[string]any{"role": "user", "content": query})
	return msgs
}

func usageFromProvider(u result.Usage, p clients.ProviderUsage) result.Usage {
	return result.UsageFromProvider(result.ProviderUsage{
		InputTokens:      u.InputTokens + p.InputTokens,
		OutputTokens:     u.OutputTokens + p.OutputTokens,
		CacheReadTokens:  u.CacheReadTokens + p.CacheReadTokens,
		CacheWriteTokens: u.CacheWriteTokens + p.CacheWriteTokens,
	}, result.DefaultCostPerMTok)
}

func mustObserveMeta() *metacognition.Metacognition {
	m, err := metacognition.NewMetacognition(metacognition.ModeObserve, nil)
	if err != nil {
		panic(err)
	}
	return m
}

func endsWithQuestion(s string) bool {
	if s == "" {
		return false
	}
	return s[len(s)-1] == '?'
}

func splitWords(s string) []string {
	out := []string{}
	cur := ""
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\t' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func newTraceID() string {
	return fmt.Sprintf("tr-%d", traceSeq.Add(1))
}

var traceSeq counter

// counter is a small atomic counter the engine uses for trace-id / job-id
// generation. The legacy Add-based interface is preserved; the underlying
// storage is an atomic so concurrent turns don't race.
type counter struct{ n atomic.Int64 }

func (c *counter) Add(d int64) int64 {
	return c.n.Add(d)
}
