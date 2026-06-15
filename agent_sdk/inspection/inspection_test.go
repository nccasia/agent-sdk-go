package inspection

import "testing"

func TestSuggestAxisOptimizationsFlowStepDisable(t *testing.T) {
	snap := EngineSnapshot{
		FlowSteps: []map[string]any{
			{"flow": "research", "step": "gather", "node_count": 0},
			{"flow": "research", "step": "synth", "node_count": 3},
		},
	}
	opts := SuggestAxisOptimizations(snap)
	if len(opts) != 1 || opts[0].Axis != "flow" || opts[0].Target != "research.gather" {
		t.Fatalf("opts = %+v", opts)
	}
	if opts[0].WeightPatch["flow_research__step_gather__disable"] != 1.0 {
		t.Fatalf("weight_patch = %v", opts[0].WeightPatch)
	}
}

func TestSuggestAxisOptimizationsLobePriorDrop(t *testing.T) {
	snap := EngineSnapshot{
		Lobes: []map[string]any{
			{"id": "skill_select", "state_node_summary": map[string]any{"total": 3, "enabled": 0}},
			{"id": "synthesize", "state_node_summary": map[string]any{"total": 2, "enabled": 2}},
		},
	}
	opts := SuggestAxisOptimizations(snap)
	if len(opts) != 1 || opts[0].Axis != "lobe" || opts[0].Target != "skill_select" {
		t.Fatalf("opts = %+v", opts)
	}
	if opts[0].WeightPatch["prior_skill_select"] != -0.1 {
		t.Fatalf("weight_patch = %v", opts[0].WeightPatch)
	}
}

func TestSuggestAxisOptimizationsEmpty(t *testing.T) {
	if got := SuggestAxisOptimizations(EngineSnapshot{}); len(got) != 0 {
		t.Fatalf("expected no suggestions, got %+v", got)
	}
}

func TestBuildMetaInputPackagesSnapshots(t *testing.T) {
	la := &LobeAxisSnapshot{Activated: []string{"synthesize"}}
	fa := &FlowAxisSnapshot{Flow: "research"}
	eng := &EngineSnapshot{}
	in := BuildMetaInput(la, fa, eng)
	if in["lobe_axis"] != la || in["flow_axis"] != fa || in["engine"] != eng {
		t.Fatalf("meta input = %+v", in)
	}
}
