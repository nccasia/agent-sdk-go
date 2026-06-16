// Package result holds the ergonomic result + trace types — the public output
// surface. AgentResult is the ergonomic wrapper over the engine's FinalEnvelope
// plus the Trace; Trace is the full, JSON-able picture of a run;
// ActivationSnapshot is the dry, no-LLM routing probe; Optimization is a pure
// weight-patch proposal. Ported from agent_sdk/result.py.
package result

import (
	"math"

	"github.com/nccasia/agent-sdk-go/agent_sdk/contracts"
)

// DefaultCostPerMTok is the rough $/Mtok default (input, output) — overridable;
// only an estimate.
var DefaultCostPerMTok = [2]float64{3.0, 15.0}

// ProviderUsage mirrors agent_sdk.clients.messages.ProviderUsage — the raw
// provider token counts Usage is projected from.
type ProviderUsage struct {
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
}

// Usage is the per-turn token + cost roll-up.
type Usage struct {
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	CacheReadTokens  int     `json:"cache_read_tokens"`
	CacheWriteTokens int     `json:"cache_write_tokens"`
	EstimatedCost    float64 `json:"estimated_cost"`
}

// UsageFromProvider projects raw provider counts into a Usage, estimating cost
// from the per-Mtok rate.
func UsageFromProvider(pu ProviderUsage, costPerMTok [2]float64) Usage {
	cost := (float64(pu.InputTokens)/1e6)*costPerMTok[0] + (float64(pu.OutputTokens)/1e6)*costPerMTok[1]
	return Usage{
		InputTokens:      pu.InputTokens,
		OutputTokens:     pu.OutputTokens,
		CacheReadTokens:  pu.CacheReadTokens,
		CacheWriteTokens: pu.CacheWriteTokens,
		EstimatedCost:    round6(cost),
	}
}

func round6(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}

// ToJSON renders the usage as a wire-stable map.
func (u Usage) ToJSON() map[string]any {
	return map[string]any{
		"input_tokens":       u.InputTokens,
		"output_tokens":      u.OutputTokens,
		"cache_read_tokens":  u.CacheReadTokens,
		"cache_write_tokens": u.CacheWriteTokens,
		"estimated_cost":     u.EstimatedCost,
	}
}

// Refusal is a structured refusal: reason is one of "no_citations",
// "budget_exceeded", "policy_violation".
type Refusal struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// ToJSON renders the refusal as a wire-stable map.
func (r Refusal) ToJSON() map[string]any {
	return map[string]any{"reason": r.Reason, "message": r.Message}
}

// MemoryUpdate records one durable-memory mutation applied this turn.
type MemoryUpdate struct {
	Action string `json:"action"`
	Scope  string `json:"scope"`
	Key    string `json:"key"`
}

// ToJSON renders the memory update as a wire-stable map.
func (m MemoryUpdate) ToJSON() map[string]any {
	return map[string]any{"action": m.Action, "scope": m.Scope, "key": m.Key}
}

// Trace is the full, JSON-able picture of a run.
type Trace struct {
	TraceID        string           `json:"trace_id"`
	Path           map[string]any   `json:"path"`
	Lobes          []map[string]any `json:"lobes"`
	FlowStages     []map[string]any `json:"flow_stages"`
	Blackboard     map[string]any   `json:"blackboard"`
	Usage          Usage            `json:"usage"`
	Steps          []map[string]any `json:"steps"`
	MetaActions    []map[string]any `json:"meta_actions"`
	LlmCalls       []map[string]any `json:"llm_calls"`
	Attention      map[string]any   `json:"attention"`
	ToolSelection  []map[string]any `json:"tool_selection"`
	SkillSelection []map[string]any `json:"skill_selection"`
	Degraded       []string         `json:"degraded"`
}

// Timeline returns the ReAct sub-steps across the run (thinking / tool_use /
// tool_result / answer), bracketed by stage_start / stage_end markers.
func (t Trace) Timeline() []map[string]any {
	out := []map[string]any{}
	for _, stage := range t.FlowStages {
		out = append(out, map[string]any{"kind": "stage_start", "stage": stage["stage"]})
		if steps, ok := stage["steps"].([]map[string]any); ok {
			out = append(out, steps...)
		}
		out = append(out, map[string]any{"kind": "stage_end", "stage": stage["stage"]})
	}
	return out
}

// ToJSON renders the trace as a wire-stable map.
func (t Trace) ToJSON() map[string]any {
	return map[string]any{
		"trace_id":        t.TraceID,
		"path":            orEmptyMap(t.Path),
		"lobes":           orEmptyList(t.Lobes),
		"flow_stages":     orEmptyList(t.FlowStages),
		"blackboard":      orEmptyMap(t.Blackboard),
		"usage":           t.Usage.ToJSON(),
		"meta_actions":    orEmptyList(t.MetaActions),
		"llm_calls":       orEmptyList(t.LlmCalls),
		"attention":       orEmptyMap(t.Attention),
		"tool_selection":  orEmptyList(t.ToolSelection),
		"skill_selection": orEmptyList(t.SkillSelection),
		"degraded":        orEmptyStrs(t.Degraded),
	}
}

// AgentResult is the ergonomic public output of a turn.
type AgentResult struct {
	Text          string               `json:"text"`
	Status        string               `json:"status"` // "answered" | "refused"
	Citations     []contracts.Citation `json:"citations"`
	Refusal       *Refusal             `json:"refusal"`
	Usage         Usage                `json:"usage"`
	MemoryUpdates []MemoryUpdate       `json:"memory_updates"`
	Trace         Trace                `json:"trace"`
}

// String returns the answer text (mirrors Python __str__).
func (r AgentResult) String() string { return r.Text }

// status returns the effective status, defaulting to "answered".
func (r AgentResult) status() string {
	if r.Status == "" {
		return "answered"
	}
	return r.Status
}

// ToJSON renders the result as a wire-stable map.
func (r AgentResult) ToJSON() map[string]any {
	cits := make([]map[string]any, 0, len(r.Citations))
	for _, c := range r.Citations {
		cits = append(cits, citationJSON(c))
	}
	var ref any
	if r.Refusal != nil {
		ref = r.Refusal.ToJSON()
	}
	mems := make([]map[string]any, 0, len(r.MemoryUpdates))
	for _, m := range r.MemoryUpdates {
		mems = append(mems, m.ToJSON())
	}
	return map[string]any{
		"text":           r.Text,
		"status":         r.status(),
		"citations":      cits,
		"refusal":        ref,
		"usage":          r.Usage.ToJSON(),
		"memory_updates": mems,
		"trace_id":       r.Trace.TraceID,
	}
}

func citationJSON(c contracts.Citation) map[string]any {
	return map[string]any{
		"chunk_id":        c.ChunkID,
		"source_ref":      c.SourceRef,
		"supporting_span": []int{c.SupportingSpan[0], c.SupportingSpan[1]},
	}
}

// ActivationSnapshot is the dry, no-LLM routing probe (agent.inspect).
type ActivationSnapshot struct {
	Path   PathScore        `json:"path"`
	Lobes  []map[string]any `json:"lobes"`
	Flow   []string         `json:"flow"`
	Budget map[string]any   `json:"budget"`
}

// PathScore is the (name, score) routing pick.
type PathScore struct {
	Name  string
	Score float64
}

// ToJSON renders the snapshot as a wire-stable map.
func (a ActivationSnapshot) ToJSON() map[string]any {
	name := a.Path.Name
	if name == "" {
		name = "emergent"
	}
	return map[string]any{
		"path":   []any{name, a.Path.Score},
		"lobes":  orEmptyList(a.Lobes),
		"flow":   orEmptyStrs(a.Flow),
		"budget": orEmptyMap(a.Budget),
	}
}

// Optimization is a pure weight-patch proposal.
type Optimization struct {
	Axis        string             `json:"axis"`
	Target      string             `json:"target"`
	Reason      string             `json:"reason"`
	WeightPatch map[string]float64 `json:"weight_patch"`
}

// ToJSON renders the optimization as a wire-stable map.
func (o Optimization) ToJSON() map[string]any {
	patch := o.WeightPatch
	if patch == nil {
		patch = map[string]float64{}
	}
	return map[string]any{
		"axis":         o.Axis,
		"target":       o.Target,
		"reason":       o.Reason,
		"weight_patch": patch,
	}
}

func orEmptyMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func orEmptyList(l []map[string]any) []map[string]any {
	if l == nil {
		return []map[string]any{}
	}
	return l
}

func orEmptyStrs(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}
