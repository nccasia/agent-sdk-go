package engine

import (
	"reflect"
	"testing"
)

// First-class Stage + builder + registry. Translated from tests/test_stages.py.

func TestBuilderDefaultsAlwaysOn(t *testing.T) {
	s := NewStage("plan", StageLobes("plan"))
	if s.ID() != "plan" {
		t.Fatalf("id = %q, want plan", s.ID())
	}
	if s.Name() != "plan" {
		t.Fatalf("name = %q, want plan", s.Name())
	}
	if s.Loop != "single" {
		t.Fatalf("loop = %q, want single", s.Loop)
	}
	if got := s.Signal(map[string]any{}); got != 1.0 {
		t.Fatalf("signal = %v, want 1.0", got)
	}
	if !reflect.DeepEqual(s.Lobes, []string{"plan"}) {
		t.Fatalf("lobes = %v, want [plan]", s.Lobes)
	}
}

func TestBuilderWithSignalGates(t *testing.T) {
	s := NewStage(
		"research",
		StageLobes("research"),
		StageLoop("agentic"),
		StageTools("search"),
		StageSignal(func(ctx map[string]any) float64 {
			if v, _ := ctx["needs_sources"].(bool); v {
				return 1.0
			}
			return 0.0
		}),
	)
	if got := s.Signal(map[string]any{"needs_sources": true}); got != 1.0 {
		t.Fatalf("signal(needs_sources) = %v, want 1.0", got)
	}
	if got := s.Signal(map[string]any{}); got != 0.0 {
		t.Fatalf("signal({}) = %v, want 0.0", got)
	}
}

func TestStageIsActivable(t *testing.T) {
	// Compile-time check that *Stage satisfies the Activable surface
	// (ID/Name/Signal). A failing assignment fails to compile.
	s := NewStage("plan", StageLobes("plan"))
	var _ interface {
		ID() string
		Name() string
		Signal(ctx map[string]any) float64
	} = s
}

func TestToFlowStepBridgesRuntime(t *testing.T) {
	s := NewStage(
		"research",
		StageLobes("research"),
		StageLoop("agentic"),
		StageTools("search"),
		StageSignal(func(map[string]any) float64 { return 0.0 }),
		StageThreshold(0.5),
		StageMaxTokens(2048),
	)
	fs := s.ToFlowStep()
	if fs.Name != "research" {
		t.Fatalf("name = %q, want research", fs.Name)
	}
	if fs.Loop != "agentic" {
		t.Fatalf("loop = %q, want agentic", fs.Loop)
	}
	if !reflect.DeepEqual(fs.Tools, []string{"search"}) {
		t.Fatalf("tools = %v, want [search]", fs.Tools)
	}
	if fs.MaxTokens == nil || *fs.MaxTokens != 2048 {
		t.Fatalf("max_tokens = %v, want 2048", fs.MaxTokens)
	}
	if got := fs.Signals(map[string]any{"x": 1})["research"]; got != 0.0 {
		t.Fatalf("signals[research] = %v, want 0.0", got)
	}
	if fs.MinActivation != 0.5 {
		t.Fatalf("min_activation = %v, want 0.5", fs.MinActivation)
	}
}

func TestRegistryResolveByReference(t *testing.T) {
	reg := NewStageRegistry(
		NewStage("plan", StageLobes("plan")),
		NewStage("synth", StageLobes("synthesize")),
	)
	resolved := reg.Resolve([]string{"plan", "synth", "missing"})
	ids := make([]string, len(resolved))
	for i, s := range resolved {
		ids[i] = s.ID()
	}
	if !reflect.DeepEqual(ids, []string{"plan", "synth"}) {
		t.Fatalf("resolved ids = %v, want [plan synth]", ids)
	}
	if !reflect.DeepEqual(reg.Get("plan").Lobes, []string{"plan"}) {
		t.Fatalf("plan lobes = %v, want [plan]", reg.Get("plan").Lobes)
	}
	gotIDs := map[string]bool{}
	for _, id := range reg.IDs() {
		gotIDs[id] = true
	}
	if !reflect.DeepEqual(gotIDs, map[string]bool{"plan": true, "synth": true}) {
		t.Fatalf("ids = %v, want {plan synth}", reg.IDs())
	}
}
