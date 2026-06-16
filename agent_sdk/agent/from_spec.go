package agent

import (
	"github.com/nccasia/agent-sdk-go/agent_sdk/core/spec"
	"github.com/nccasia/agent-sdk-go/agent_sdk/flows"
)

// FromSpec rebuilds a working PreactAgent from a serialized Spec — the inverse
// of (*PreactAgent).Spec() / Spec.ToJSON(). It rewires only the I/O seams
// (client + tools) afresh and reconstructs the lobes/stages/flows/skills
// network plus the weights/budgets surfaces. The round-trip holds: build agent
// A, then FromSpec(A.Spec()) yields an agent whose Spec().ToJSON() deep-equals
// A.Spec().ToJSON().
//
// Ported from agent_sdk/spec.py:agent_from_spec — including the named-alias
// field folding (flow_lobe_weights / flow_layer_budgets fold into
// weights / budgets, the named field winning a key collision).
func FromSpec(s *spec.Spec, client any, tools ...any) (*PreactAgent, error) {
	if s == nil {
		s = spec.NewSpec()
	}

	lobes := make([]spec.Lobe, 0, len(s.Lobes))
	for _, r := range s.Lobes {
		lobes = append(lobes, lobeFromRow(r))
	}

	// Stages are kept as map rows: Spec() renders a spec.Stage / map row
	// identically, and the engine reads stage fields from a map. Passing the
	// rows verbatim makes the stage half of the spec round-trip exactly.
	stages := make([]any, 0, len(s.Stages))
	for _, r := range s.Stages {
		stages = append(stages, cloneRow(r))
	}

	flowList := make([]flows.Flow, 0, len(s.Flows))
	for _, r := range s.Flows {
		flowList = append(flowList, flowFromRow(r))
	}

	skills := make([]any, 0, len(s.Skills))
	for _, r := range s.Skills {
		skills = append(skills, cloneRow(r))
	}

	// Fold the named authoring aliases into the canonical surfaces (the named
	// fields win a key collision — they are the explicit BotPolicy intent).
	weights := mergeMaps(s.Weights, s.FlowLobeWeights)
	budgets := mergeMaps(s.Budgets, s.FlowLayerBudgets)

	cfg := Config{
		Client:           client,
		Instructions:     s.Instructions,
		Lobes:            lobes,
		Stages:           stages,
		Flows:            flowList,
		Skills:           skills,
		Tools:            append([]any(nil), tools...),
		Weights:          weights,
		Budgets:          budgets,
		RequireCitations: s.RequireCitations,
		TZ:               s.TZ,
		Lang:             s.Lang,
	}
	return NewPreactAgent(cfg)
}

// lobeFromRow reconstructs a spec.Lobe from its serialized row, tolerating both
// native Go numbers (a freshly built Spec) and JSON float64s (a Spec restored
// from JSON). Mirrors agent_sdk/spec.py:_SpecLobe.__init__.
func lobeFromRow(r map[string]any) spec.Lobe {
	return spec.Lobe{
		ID:            rowStr(r, "id", ""),
		Behavior:      rowStr(r, "behavior", "recall"),
		Layer:         rowInt(r, "layer", 4),
		Order:         rowInt(r, "order", 0),
		Prior:         rowFloat(r, "prior", 0),
		Pinned:        rowBool(r, "pinned", false),
		MinActivation: rowFloat(r, "min_activation", 0.5),
		Writes:        rowStrSlice(r, "writes"),
	}
}

// flowFromRow reconstructs a flows.Flow from its serialized row.
func flowFromRow(r map[string]any) flows.Flow {
	opts := []flows.FlowOption{
		flows.FlowDescription(rowStr(r, "description", "")),
		flows.FlowUseWhen(rowStr(r, "use_when", "")),
		flows.FlowThreshold(rowFloat(r, "threshold", 0.5)),
		flows.FlowGrounds(rowBool(r, "grounds", true)),
	}
	if name := rowStr(r, "name", ""); name != "" {
		opts = append(opts, flows.FlowName(name))
	}
	if st := rowStrSlice(r, "stages"); st != nil {
		opts = append(opts, flows.FlowStages(st...))
	}
	if sig, ok := r["signal"]; ok && sig != nil {
		opts = append(opts, flows.FlowSignalExpr(sig))
	}
	return flows.NewFlow(rowStr(r, "id", ""), opts...)
}

// ── row coercion helpers (tolerate native Go + JSON-decoded numbers) ─────────

func cloneRow(r map[string]any) map[string]any {
	out := make(map[string]any, len(r))
	for k, v := range r {
		out[k] = v
	}
	return out
}

func mergeMaps(base, overlay map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(overlay))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}

func rowStr(r map[string]any, key, def string) string {
	if v, ok := r[key].(string); ok {
		return v
	}
	return def
}

func rowBool(r map[string]any, key string, def bool) bool {
	if v, ok := r[key].(bool); ok {
		return v
	}
	return def
}

func rowInt(r map[string]any, key string, def int) int {
	switch v := r[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return def
}

func rowFloat(r map[string]any, key string, def float64) float64 {
	switch v := r[key].(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	}
	return def
}

func rowStrSlice(r map[string]any, key string) []string {
	switch v := r[key].(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
