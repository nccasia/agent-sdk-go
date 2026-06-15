// Package inspection holds read-only inspection + optimization helpers for the
// lobe/flow axes — the serializable OX/OY snapshots the metacognition layer
// monitors. Ported from agent_sdk/inspection.py.
//
// State nodes are carried as map[string]any to mirror the Python dicts the
// activation network emits (id, activated, weight, …).
package inspection

// LobeInspection is one lobe's snapshot: its layer, activation, per-node state.
type LobeInspection struct {
	ID               string           `json:"id"`
	Layer            int              `json:"layer"`
	Activated        bool             `json:"activated"`
	StateNodes       []map[string]any `json:"state_nodes"`
	ContextNodeCount int              `json:"context_node_count"`
	WriteMeta        map[string]any   `json:"write_meta"`
}

// LobeAxisSnapshot is the lobe-axis (OX) snapshot: every lobe + the activated ids.
type LobeAxisSnapshot struct {
	Lobes     []LobeInspection `json:"lobes"`
	Activated []string         `json:"activated"`
}

// FlowStepInspection is one flow step's snapshot.
type FlowStepInspection struct {
	Flow       string           `json:"flow"`
	Step       string           `json:"step"`
	Loop       string           `json:"loop"`
	Tools      []string         `json:"tools"`
	Lobes      []string         `json:"lobes"`
	Type       string           `json:"type"` // RFC 0017: react/simple/map/none; default "simple"
	Disabled   bool             `json:"disabled"`
	StateNodes []map[string]any `json:"state_nodes"`
}

// FlowAxisSnapshot is the flow-axis (OY) snapshot for one path.
type FlowAxisSnapshot struct {
	Flow     string               `json:"flow"`
	Disabled bool                 `json:"disabled"`
	Steps    []FlowStepInspection `json:"steps"`
}

// EngineSnapshot combines trace + optional blackboard state into one
// serializable picture. Path/Flow are nil when absent.
type EngineSnapshot struct {
	Path       map[string]any   `json:"path"`
	Flow       map[string]any   `json:"flow"`
	Lobes      []map[string]any `json:"lobes"`
	FlowSteps  []map[string]any `json:"flow_steps"`
	Blackboard map[string]any   `json:"blackboard"`
	Response   map[string]any   `json:"response"`
}

// AxisOptimization is a pure optimization proposal; callers decide whether to
// apply it.
type AxisOptimization struct {
	Axis        string             `json:"axis"`
	Target      string             `json:"target"`
	Reason      string             `json:"reason"`
	WeightPatch map[string]float64 `json:"weight_patch"`
}

// BuildMetaInput packages object-level OX/OY snapshots for the metacognition
// layer. Any of the snapshots may be nil.
func BuildMetaInput(lobeAxis *LobeAxisSnapshot, flowAxis *FlowAxisSnapshot, engine *EngineSnapshot) map[string]any {
	return map[string]any{
		"lobe_axis": lobeAxis,
		"flow_axis": flowAxis,
		"engine":    engine,
	}
}

// SuggestAxisOptimizations returns pure optimization proposals; callers decide
// whether to apply them. A flow step that produced no lobe context nodes is
// proposed for disable; a lobe whose every state node stayed inactive gets a
// small prior decrement.
func SuggestAxisOptimizations(snapshot EngineSnapshot) []AxisOptimization {
	var out []AxisOptimization
	for _, step := range snapshot.FlowSteps {
		flow := asString(step["flow"])
		name := asString(step["step"])
		nodeCount := asInt(step["node_count"])
		if flow != "" && name != "" && nodeCount == 0 {
			out = append(out, AxisOptimization{
				Axis:   "flow",
				Target: flow + "." + name,
				Reason: "step produced no lobe context nodes in this snapshot",
				WeightPatch: map[string]float64{
					"flow_" + flow + "__step_" + name + "__disable": 1.0,
				},
			})
		}
	}
	for _, lobe := range snapshot.Lobes {
		lobeID := asString(lobe["id"])
		summary, _ := lobe["state_node_summary"].(map[string]any)
		total := asInt(summary["total"])
		enabled := asInt(summary["enabled"])
		if lobeID != "" && total != 0 && enabled == 0 {
			out = append(out, AxisOptimization{
				Axis:        "lobe",
				Target:      lobeID,
				Reason:      "all lobe state nodes stayed inactive in this snapshot",
				WeightPatch: map[string]float64{"prior_" + lobeID: -0.1},
			})
		}
	}
	return out
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	}
	return 0
}
