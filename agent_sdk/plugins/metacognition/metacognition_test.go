// Metacognition plugin — install, recognizer, lobe, tool. Mirrors
// agent_sdk/plugins/metacognition/tests/*.
package metacognition

import (
	"context"
	"strings"
	"testing"

	"github.com/mezon/agent-sdk-go/agent_sdk/agent"
	"github.com/mezon/agent-sdk-go/agent_sdk/flows"
	"github.com/mezon/agent-sdk-go/agent_sdk/memory"
)

// TestInstallContributesLobeStagesFlowAndOneTool mirrors
// test_install_contributes_lobe_stages_flow_and_one_tool.
func TestInstallContributesLobeStagesFlowAndOneTool(t *testing.T) {
	setup := agent.NewAgentSetup()
	NewMetacognitionPlugin().Install(setup)
	// Lobes: meta_context, nav_brief
	got := map[string]struct{}{}
	for _, lb := range setup.Lobes {
		got[lb.ID] = struct{}{}
	}
	if _, ok := got["meta_context"]; !ok {
		t.Fatalf("expected lobe meta_context, got %v", got)
	}
	if _, ok := got["nav_brief"]; !ok {
		t.Fatalf("expected lobe nav_brief, got %v", got)
	}
	// Stage: meta_reflect
	stages := map[string]struct{}{}
	for _, s := range setup.Stages {
		if id := stageIDOf(s); id != "" {
			stages[id] = struct{}{}
		}
	}
	if _, ok := stages["meta_reflect"]; !ok {
		t.Fatalf("expected stage meta_reflect, got %v", stages)
	}
	// Flow: meta
	if len(setup.Flows) != 1 || setup.Flows[0].ID() != "meta" {
		t.Fatalf("expected flows=[meta], got %+v", setup.Flows)
	}
	// No plain tools, but one stateful runtime (meta_control).
	if len(setup.Tools) != 0 {
		t.Fatalf("expected no @tool tools, got %d", len(setup.Tools))
	}
	if len(setup.ToolRuntimes) != 1 {
		t.Fatalf("expected 1 tool runtime, got %d", len(setup.ToolRuntimes))
	}
	rt, ok := setup.ToolRuntimes[0].(*MetaControlToolRuntime)
	if !ok {
		t.Fatalf("expected *MetaControlToolRuntime, got %T", setup.ToolRuntimes[0])
	}
	specs := rt.GetToolSpecs()
	if len(specs) != 1 || specs[0]["name"] != "meta_control" {
		t.Fatalf("expected one `meta_control` tool, got %v", specs)
	}
}

// TestFlowFalseOmitsTheFlowButKeepsLobeStagesTool mirrors
// test_flow_false_omits_the_flow_but_keeps_lobe_stages_tool.
func TestFlowFalseOmitsTheFlowButKeepsLobeStagesTool(t *testing.T) {
	setup := agent.NewAgentSetup()
	NewMetacognitionPlugin(WithFlow(false)).Install(setup)
	if len(setup.Flows) != 0 {
		t.Fatalf("expected no flows, got %d", len(setup.Flows))
	}
	got := map[string]struct{}{}
	for _, lb := range setup.Lobes {
		got[lb.ID] = struct{}{}
	}
	if _, ok := got["meta_context"]; !ok {
		t.Fatalf("expected meta_context lobe, got %v", got)
	}
	if _, ok := got["nav_brief"]; !ok {
		t.Fatalf("expected nav_brief lobe, got %v", got)
	}
}

// TestRecognizerIsConservativeAndReadsNextTurnBias mirrors
// test_recognizer_is_conservative_and_reads_next_turn_bias.
func TestRecognizerIsConservativeAndReadsNextTurnBias(t *testing.T) {
	setup := agent.NewAgentSetup()
	NewMetacognitionPlugin().Install(setup)
	flow := setup.Flows[0]
	if got := flow.Signal(map[string]any{"query": "rethink your approach to this"}); got < 0.5 {
		t.Fatalf("expected rethink cue to score > 0.5, got %v", got)
	}
	if got := flow.Signal(map[string]any{"query": "what is the capital of France?"}); got != 0.0 {
		t.Fatalf("expected 0.0 for plain question, got %v", got)
	}
	if got := flow.Signal(map[string]any{"query": "hello", BiasFlag("meta"): true}); got != 1.0 {
		t.Fatalf("expected 1.0 for recorded bias, got %v", got)
	}
}

// TestRendersThinkingStateBlock mirrors test_renders_thinking_state_block.
func TestRendersThinkingStateBlock(t *testing.T) {
	ctx := map[string]any{
		"active_path": "research",
		"stage_id":    "meta_reflect",
		"active_lobes": map[string]struct{}{
			"synthesize":   {},
			"meta_context": {},
		},
		"lobe_outputs": map[string]any{
			"skills_in_use":     []any{"triage"},
			"meta_observations": []any{map[string]any{"kind": "low_confidence_path", "target": "research"}},
			"meta_flow_bias":    "qna",
		},
	}
	out := MetaContextRenderString(ctx)
	for _, sub := range []string{
		"How you are thinking",
		"Path (recognized intent): research",
		"Current step: meta_reflect",
		"Skills in use: triage",
		"low_confidence_path @ research",
		"applies to your NEXT turn",
	} {
		if !strings.Contains(out, sub) {
			t.Fatalf("expected %q in output, got:\n%s", sub, out)
		}
	}
}

// TestEmptyStateContributesNothing mirrors test_empty_state_contributes_nothing.
func TestEmptyStateContributesNothing(t *testing.T) {
	if got := MetaContextRenderString(map[string]any{}); got != "" {
		t.Fatalf("expected '' for empty ctx, got %q", got)
	}
	if got := MetaContextRenderString(map[string]any{"active_path": "", "stage_id": "", "lobe_outputs": map[string]any{}}); got != "" {
		t.Fatalf("expected '' for empty state, got %q", got)
	}
}

// TestActivationIsAlwaysOn mirrors test_activation_is_always_on.
func TestActivationIsAlwaysOn(t *testing.T) {
	if got := metaContextActivation(map[string]any{}); got != 1.0 {
		t.Fatalf("expected meta_context activation=1.0, got %v", got)
	}
}

// TestUseSkillsWritesSkillsInUseAndStripsPinned mirrors
// test_use_skills_writes_skills_in_use_and_strips_pinned.
func TestUseSkillsWritesSkillsInUseAndStripsPinned(t *testing.T) {
	turn := &turnState{LobeOutputs: map[string]any{}, Scratchpad: memory.NewScratchpad()}
	out := WithTurn(turn, func() string {
		rt := NewMetaControlToolRuntime()
		s, err := rt.CallTool(context.Background(), "meta_control", map[string]any{
			"action": "use_skills", "slugs": []any{"triage", "cite", "filter"},
		}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return s
	})
	if !strings.Contains(out, "triage") {
		t.Fatalf("expected 'triage' in output, got %q", out)
	}
	skills, _ := turn.LobeOutputs["skills_in_use"].([]string)
	// cite / filter pinned ⇒ stripped
	hasTriage := false
	for _, s := range skills {
		if s == "triage" {
			hasTriage = true
		}
		if s == "cite" || s == "filter" {
			t.Fatalf("pinned skill %q should be stripped, got %v", s, skills)
		}
	}
	if !hasTriage {
		t.Fatalf("expected 'triage' in skills_in_use, got %v", skills)
	}
}

// TestUseSkillsRequiresRealSlugs mirrors test_use_skills_requires_real_slugs.
func TestUseSkillsRequiresRealSlugs(t *testing.T) {
	turn := &turnState{LobeOutputs: map[string]any{}, Scratchpad: memory.NewScratchpad()}
	WithTurn(turn, func() string {
		rt := NewMetaControlToolRuntime()
		out, _ := rt.CallTool(context.Background(), "meta_control", map[string]any{
			"action": "use_skills", "slugs": []any{"cite"},
		}, nil, nil)
		if !strings.Contains(out, "Error") {
			t.Fatalf("expected 'Error' in output, got %q", out)
		}
		return out
	})
	if _, ok := turn.LobeOutputs["skills_in_use"]; ok {
		t.Fatalf("expected skills_in_use not to be set, got %v", turn.LobeOutputs["skills_in_use"])
	}
}

// TestBiasFlowRecordsNextTurnBias mirrors test_bias_flow_records_next_turn_bias.
func TestBiasFlowRecordsNextTurnBias(t *testing.T) {
	turn := &turnState{LobeOutputs: map[string]any{}, Scratchpad: memory.NewScratchpad()}
	out := WithTurn(turn, func() string {
		rt := NewMetaControlToolRuntime()
		s, _ := rt.CallTool(context.Background(), "meta_control", map[string]any{
			"action": "bias_flow", "path": "research",
		}, nil, nil)
		return s
	})
	if !strings.Contains(out, "NEXT turn") {
		t.Fatalf("expected 'NEXT turn' in output, got %q", out)
	}
	if turn.LobeOutputs["meta_flow_bias"] != "research" {
		t.Fatalf("expected meta_flow_bias=research, got %v", turn.LobeOutputs["meta_flow_bias"])
	}
}

// TestRegulateSkipRecordsRequest mirrors test_regulate_skip_records_request.
func TestRegulateSkipRecordsRequest(t *testing.T) {
	turn := &turnState{LobeOutputs: map[string]any{}, Scratchpad: memory.NewScratchpad()}
	out := WithTurn(turn, func() string {
		rt := NewMetaControlToolRuntime()
		s, _ := rt.CallTool(context.Background(), "meta_control", map[string]any{
			"action": "regulate", "request": "skip", "step": "research",
		}, nil, nil)
		return s
	})
	if !strings.Contains(out, "recorded") {
		t.Fatalf("expected 'recorded' in output, got %q", out)
	}
	req, _ := turn.Scratchpad.Get("meta_regulate_request", nil).(map[string]any)
	if req == nil {
		t.Fatalf("expected meta_regulate_request to be recorded")
	}
	if req["request"] != "skip" || req["step"] != "research" {
		t.Fatalf("expected request=skip step=research, got %v", req)
	}
}

// TestFanOutIsNoLongerAMetaAction mirrors test_fan_out_is_no_longer_a_meta_action.
func TestFanOutIsNoLongerAMetaAction(t *testing.T) {
	turn := &turnState{LobeOutputs: map[string]any{}, Scratchpad: memory.NewScratchpad()}
	out := WithTurn(turn, func() string {
		rt := NewMetaControlToolRuntime()
		s, _ := rt.CallTool(context.Background(), "meta_control", map[string]any{
			"action": "fan_out", "items": []any{map[string]any{"input": "x"}},
		}, nil, nil)
		return s
	})
	if !strings.Contains(out, "unknown action") {
		t.Fatalf("expected 'unknown action' in output, got %q", out)
	}
}

// TestRegulateNeverSkipsAPinnedStep mirrors test_regulate_never_skips_a_pinned_step.
func TestRegulateNeverSkipsAPinnedStep(t *testing.T) {
	turn := &turnState{LobeOutputs: map[string]any{}, Scratchpad: memory.NewScratchpad()}
	out := WithTurn(turn, func() string {
		rt := NewMetaControlToolRuntime()
		s, _ := rt.CallTool(context.Background(), "meta_control", map[string]any{
			"action": "regulate", "request": "skip", "step": "cite",
		}, nil, nil)
		return s
	})
	if !strings.Contains(out, "Refused") {
		t.Fatalf("expected 'Refused' in output, got %q", out)
	}
	if _, ok := turn.Scratchpad.Get("meta_regulate_request", nil).(map[string]any); ok {
		t.Fatalf("expected no meta_regulate_request recorded (pinned step)")
	}
}

// TestUnknownActionErrors mirrors test_unknown_action_errors.
func TestUnknownActionErrors(t *testing.T) {
	turn := &turnState{LobeOutputs: map[string]any{}, Scratchpad: memory.NewScratchpad()}
	out := WithTurn(turn, func() string {
		rt := NewMetaControlToolRuntime()
		s, _ := rt.CallTool(context.Background(), "meta_control", map[string]any{"action": "nonsense"}, nil, nil)
		return s
	})
	if !strings.Contains(out, "unknown action") {
		t.Fatalf("expected 'unknown action' in output, got %q", out)
	}
}

// TestNoTurnIsHandled mirrors test_no_turn_is_handled.
func TestNoTurnIsHandled(t *testing.T) {
	rt := NewMetaControlToolRuntime()
	out, _ := rt.CallTool(context.Background(), "meta_control", map[string]any{"action": "bias_flow", "path": "x"}, nil, nil)
	if !strings.Contains(out, "no active turn") {
		t.Fatalf("expected 'no active turn' in output, got %q", out)
	}
}

// TestRejectsOtherToolNames mirrors test_rejects_other_tool_names.
func TestRejectsOtherToolNames(t *testing.T) {
	rt := NewMetaControlToolRuntime()
	for _, name := range []string{"todos", "search"} {
		out, _ := rt.CallTool(context.Background(), name, map[string]any{}, nil, nil)
		if !strings.Contains(out, "unknown tool") {
			t.Fatalf("expected 'unknown tool' for %q, got %q", name, out)
		}
	}
}

func stageIDOf(s any) string {
	switch v := s.(type) {
	case interface{ StageID() string }:
		return v.StageID()
	case interface{ GetID() string }:
		return v.GetID()
	case flows.FlowStep:
		return v.Name
	case *flows.FlowStep:
		if v != nil {
			return v.Name
		}
	}
	return ""
}
