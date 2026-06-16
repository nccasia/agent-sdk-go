// Package probe — run one real turn through a PreactAgent and capture the
// engine's internals as a structured, JSON-able record.
//
// A good benchmark *sees* what actually fired, not just the final answer. Probe
// drives a PreactAgent through one turn, subscribes to its event stream to
// collect tool calls + answer text, and pairs that with the engine's own
// last-trace snapshot (path + flow stages + per-stage timeline) so the
// downstream report/viewer can render the full inspect detail.
//
// Ported from agent_sdk/probe.py.
package probe

import (
	"context"
	"fmt"

	"github.com/mezon/agent-sdk-go/agent_sdk/agent"
	"github.com/mezon/agent-sdk-go/agent_sdk/events"
	"github.com/mezon/agent-sdk-go/agent_sdk/result"
)

// Record is the structured internals of one probed turn (JSON-able). Mirrors
// agent_sdk/probe.py:ProbeRecord.
type Record struct {
	Label          string           `json:"label"`
	Query          string           `json:"query"`
	Status         string           `json:"status"`
	Answer         string           `json:"answer"`
	Flow           string           `json:"flow"`
	FlowScore      float64          `json:"flow_score"`
	Path           map[string]any   `json:"path"`
	Lobes          []map[string]any `json:"lobes"`
	Stages         []map[string]any `json:"stages"`
	LlmCalls       []map[string]any `json:"llm_calls"`
	ToolCalls      []map[string]any `json:"tool_calls"`
	Usage          map[string]any   `json:"usage"`
	MetaActions    []map[string]any `json:"meta_actions"`
	Hints          []map[string]any `json:"hints"`
	Attention      map[string]any   `json:"attention"`
	Blackboard     map[string]any   `json:"blackboard"`
	SkillSelection []map[string]any `json:"skill_selection"`
	ToolSelection  []map[string]any `json:"tool_selection"`
	Degraded       []string         `json:"degraded"`
	Error          string           `json:"error,omitempty"`
}

// ActivatedLobes returns the ids of lobes that were activated. Mirrors the
// Python @property activated_lobes.
func (r *Record) ActivatedLobes() []string {
	out := []string{}
	for _, lb := range r.Lobes {
		if on, _ := lb["activated"].(bool); on {
			if id, _ := lb["id"].(string); id != "" {
				out = append(out, id)
			}
		}
	}
	return out
}

// ToJSON returns the record as a wire-stable map (passthrough; the struct is
// already JSON-tagged).
func (r *Record) ToJSON() map[string]any {
	// Round-trip through the JSON tags to produce a plain map.
	// (Records are JSON-serialised; calling .ToJSON() returns the same
	// map[string]any shape Python uses.)
	return recordToMap(r)
}

// ProbeOption configures a Probe call.
type ProbeOption func(*probeOpts)

type probeOpts struct {
	label string
}

// WithLabel sets the record label (defaults to the first 48 chars of the
// query).
func WithLabel(s string) ProbeOption { return func(o *probeOpts) { o.label = s } }

// Probe runs one turn through agent and captures its internals. It never
// raises on a turn failure — the error is recorded so the report still
// renders what got that far. Mirrors agent_sdk/probe.py:probe (async).
func Probe(ctx context.Context, a *agent.PreactAgent, query string, opts ...ProbeOption) (*Record, error) {
	o := probeOpts{}
	for _, fn := range opts {
		fn(&o)
	}
	label := o.label
	if label == "" {
		if len(query) > 48 {
			label = query[:48]
		} else {
			label = query
		}
	}
	rec := &Record{Label: label, Query: query}

	// Drive the agent and subscribe to its event stream to capture tool
	// calls (the engine's last-trace doesn't carry them yet).
	stream := a.Act(ctx, query)
	toolIn := map[string]map[string]any{}
	collectedFinal := false
	for ev := range stream.Iter() {
		switch e := ev.(type) {
		case *events.ToolCall:
			toolIn[e.ID] = map[string]any{"name": e.Name, "input": e.Input}
		case *events.ToolResult:
			call, ok := toolIn[e.ID]
			if !ok {
				call = map[string]any{"name": e.Name, "input": map[string]any{}}
			}
			merged := map[string]any{}
			for k, v := range call {
				merged[k] = v
			}
			merged["output"] = e.Output
			rec.ToolCalls = append(rec.ToolCalls, merged)
			delete(toolIn, e.ID)
		case *events.Final:
			if r, ok := e.Result.(*result.AgentResult); ok {
				rec.Status = r.Status
				if rec.Status == "" {
					rec.Status = "answered"
				}
				rec.Answer = r.Text
				rec.Usage = r.Usage.ToJSON()
			}
			collectedFinal = true
		}
	}
	if !collectedFinal {
		// The stream ended without a Final — query directly as a fallback.
		res, err := a.Query(ctx, query)
		if err != nil {
			rec.Status = "error"
			rec.Error = fmt.Sprintf("%s: %s", errType(err), err.Error())
			return rec, nil
		}
		if res != nil {
			rec.Status = res.Status
			if rec.Status == "" {
				rec.Status = "answered"
			}
			rec.Answer = res.Text
			rec.Usage = res.Usage.ToJSON()
		}
	}

	// Pull the engine's own last-trace for path + flow stages.
	if tr := a.LastTrace(); tr != nil {
		rec.Path = dictOrEmpty(tr.Path)
		if name, _ := rec.Path["name"].(string); name != "" {
			rec.Flow = name
		} else {
			rec.Flow = "emergent"
		}
		if s, ok := rec.Path["score"].(float64); ok {
			rec.FlowScore = s
		}
		rec.Stages = listOrEmpty(tr.FlowStages)
		rec.Lobes = listOrEmpty(tr.Lobes)
		rec.LlmCalls = listOrEmpty(tr.LlmCalls)
		rec.MetaActions = listOrEmpty(tr.MetaActions)
		rec.Usage = mapOrEmpty(tr.Usage.ToJSON())
		rec.Attention = dictOrEmpty(tr.Attention)
		rec.SkillSelection = listOrEmpty(tr.SkillSelection)
		rec.ToolSelection = listOrEmpty(tr.ToolSelection)
		rec.Degraded = strsOrEmpty(tr.Degraded)
	}
	if rec.Flow == "" {
		rec.Flow = "emergent"
	}
	// Optimization hotspots — best-effort; if the agent's
	// SuggestOptimizations is callable, use it.
	if suggest, ok := any(a).(interface {
		SuggestOptimizations() []result.Optimization
	}); ok {
		// Defer to the agent's own optimizer.
		for _, h := range suggest.SuggestOptimizations() {
			rec.Hints = append(rec.Hints, h.ToJSON())
		}
	}
	return rec, nil
}

func errType(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%T", err)
}

// dictOrEmpty normalises nil maps to empty (so JSON serialises as {}).
func dictOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func listOrEmpty(xs []map[string]any) []map[string]any {
	if xs == nil {
		return []map[string]any{}
	}
	return xs
}

func strsOrEmpty(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}

func mapOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

// recordToMap converts a *Record to a generic map for the ToJSON contract.
// (Records use JSON tags, so the shape matches Python's asdict().)
func recordToMap(r *Record) map[string]any {
	if r == nil {
		return nil
	}
	m := map[string]any{
		"label":           r.Label,
		"query":           r.Query,
		"status":          r.Status,
		"answer":          r.Answer,
		"flow":            r.Flow,
		"flow_score":      r.FlowScore,
		"path":            dictOrEmpty(r.Path),
		"lobes":           listOrEmpty(r.Lobes),
		"stages":          listOrEmpty(r.Stages),
		"llm_calls":       listOrEmpty(r.LlmCalls),
		"tool_calls":      listOrEmpty(r.ToolCalls),
		"usage":           mapOrEmpty(r.Usage),
		"meta_actions":    listOrEmpty(r.MetaActions),
		"hints":           listOrEmpty(r.Hints),
		"attention":       dictOrEmpty(r.Attention),
		"blackboard":      dictOrEmpty(r.Blackboard),
		"skill_selection": listOrEmpty(r.SkillSelection),
		"tool_selection":  listOrEmpty(r.ToolSelection),
		"degraded":        strsOrEmpty(r.Degraded),
	}
	if r.Error != "" {
		m["error"] = r.Error
	}
	return m
}
