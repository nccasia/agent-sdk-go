package metacognition

import (
	"fmt"

	"github.com/nccasia/agent-sdk-go/agent_sdk/inspection"
)

// MonitorInput carries the optional OX/OY snapshots Monitor observes. A nil
// pointer means "not provided" (mirrors the Python keyword defaults).
type MonitorInput struct {
	LobeAxis *inspection.LobeAxisSnapshot
	FlowAxis *inspection.FlowAxisSnapshot
	Engine   *inspection.EngineSnapshot
}

// Monitor observes object-level thinking state without mutating it, returning
// the observations in declaration order (flow steps, then lobes, then engine).
func Monitor(in MonitorInput) []MetaObservation {
	var obs []MetaObservation

	if in.FlowAxis != nil {
		for _, step := range in.FlowAxis.Steps {
			target := step.Flow + "." + step.Step
			if step.Disabled {
				obs = append(obs, MetaObservation{
					ID:       fmt.Sprintf("flow:%s:disabled", target),
					Kind:     "step_disabled",
					Target:   target,
					Severity: 0.3,
					Detail:   "flow customization disabled this step",
				})
			}
			if len(step.Lobes) == 0 {
				obs = append(obs, MetaObservation{
					ID:       fmt.Sprintf("flow:%s:empty_lobe_slice", target),
					Kind:     "empty_lobe_slice",
					Target:   target,
					Severity: 0.8,
					Detail:   "step has no lobes to consult",
				})
			}
			for _, node := range step.StateNodes {
				id, _ := node["id"].(string)
				activated, _ := node["activated"].(bool)
				if id == "context:tight" && activated {
					obs = append(obs, MetaObservation{
						ID:       fmt.Sprintf("flow:%s:context_tight", target),
						Kind:     "context_tight",
						Target:   target,
						Severity: 0.75,
						Detail:   "context window pressure detected",
					})
				}
				if id == "context:open" && activated {
					obs = append(obs, MetaObservation{
						ID:       fmt.Sprintf("flow:%s:context_open", target),
						Kind:     "context_open",
						Target:   target,
						Severity: 0.2,
						Detail:   "context window has room",
					})
				}
			}
		}
	}

	if in.LobeAxis != nil {
		for _, lobe := range in.LobeAxis.Lobes {
			if len(lobe.StateNodes) > 0 && !anyActivated(lobe.StateNodes) {
				obs = append(obs, MetaObservation{
					ID:       fmt.Sprintf("lobe:%s:inactive_group", lobe.ID),
					Kind:     "inactive_lobe_group",
					Target:   lobe.ID,
					Severity: 0.4,
					Detail:   "all state nodes for this lobe stayed inactive",
				})
			}
		}
	}

	if in.Engine != nil {
		path := in.Engine.Path
		if path == nil {
			path = map[string]any{}
		}
		emergent, _ := path["emergent"].(bool)
		score, hasScore := floatVal(path["score"])
		if emergent || (hasScore && score < 0.55) {
			obs = append(obs, MetaObservation{
				ID:       "engine:path:low_confidence",
				Kind:     "low_confidence_path",
				Target:   nameOr(path["name"], "unknown"),
				Severity: 0.7,
				Detail:   "path recognition is emergent or low-confidence",
			})
		}
		for _, step := range in.Engine.FlowSteps {
			flow, _ := step["flow"].(string)
			name, _ := step["step"].(string)
			if flow != "" && name != "" && intVal(step["node_count"]) == 0 {
				target := flow + "." + name
				obs = append(obs, MetaObservation{
					ID:       fmt.Sprintf("engine:%s:empty_step_context", target),
					Kind:     "empty_step_context",
					Target:   target,
					Severity: 0.65,
					Detail:   "executed step produced no lobe context nodes",
				})
			}
		}
	}

	return obs
}

func anyActivated(nodes []map[string]any) bool {
	for _, n := range nodes {
		if a, _ := n["activated"].(bool); a {
			return true
		}
	}
	return false
}

func floatVal(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	}
	return 0, false
}

func intVal(v any) int {
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

func nameOr(v any, fallback string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return fallback
}
