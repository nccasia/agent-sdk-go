package engine

import (
	"reflect"
	"testing"
)

// Declarative per-stage overrides + grounded-temp invariant.
// Translated from tests/test_stage_overrides.py.

func overrideStages() []*Stage {
	return []*Stage{
		NewStage("qna:research", StageName("research"), StageLoop("agentic"), StageTemperature(0.4), StageHops(5)),
		NewStage("qna:synthesize", StageName("synthesize"), StageTemperature(0.0)),
		NewStage("qna:cite", StageName("cite"), StageTemperature(0.0)),
	}
}

func findStage(stages []*Stage, id string) *Stage {
	for _, s := range stages {
		if s.IDField == id {
			return s
		}
	}
	return nil
}

func TestNoOverridesIsPassthrough(t *testing.T) {
	stages := overrideStages()
	out, err := ApplyStageOverrides(stages, nil)
	if err != nil {
		t.Fatal(err)
	}
	gotIDs := stageIDs(out)
	wantIDs := stageIDs(stages)
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("ids = %v, want %v", gotIDs, wantIDs)
	}
}

func TestBareNameOverrideAppliesToNamespacedStage(t *testing.T) {
	out, err := ApplyStageOverrides(overrideStages(), map[string]any{
		"research": map[string]any{"system_prompt": "be terse", "max_tokens": 99},
	})
	if err != nil {
		t.Fatal(err)
	}
	research := findStage(out, "qna:research")
	if research.SystemPrompt == nil || *research.SystemPrompt != "be terse" {
		t.Fatalf("system_prompt = %v, want 'be terse'", research.SystemPrompt)
	}
	if research.MaxTokens == nil || *research.MaxTokens != 99 {
		t.Fatalf("max_tokens = %v, want 99", research.MaxTokens)
	}
}

func TestOverridePreservesUnlistedFields(t *testing.T) {
	out, err := ApplyStageOverrides(overrideStages(), map[string]any{
		"research": map[string]any{"system_prompt": "x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	research := findStage(out, "qna:research")
	if research.Hops == nil || *research.Hops != 5 {
		t.Fatalf("hops = %v, want 5", research.Hops)
	}
	if research.Loop != "agentic" {
		t.Fatalf("loop = %q, want agentic", research.Loop)
	}
}

func TestBudgetHopsAndLoopOverride(t *testing.T) {
	out, err := ApplyStageOverrides(overrideStages(), map[string]any{
		"research": map[string]any{"loop": "single", "budget": map[string]any{"hops": 3}},
	})
	if err != nil {
		t.Fatal(err)
	}
	research := findStage(out, "qna:research")
	if research.Loop != "single" {
		t.Fatalf("loop = %q, want single", research.Loop)
	}
	if research.Hops == nil || *research.Hops != 3 {
		t.Fatalf("hops = %v, want 3", research.Hops)
	}
}

func TestGroundedStageInvariantRaisesOnBreach(t *testing.T) {
	bad := []*Stage{NewStage("qna:synthesize", StageName("synthesize"), StageTemperature(0.7))}
	if err := AssertGroundedStagesZeroTemp(bad); err == nil {
		t.Fatal("expected AssertGroundedStagesZeroTemp to error")
	}
}

func TestOverrideCannotBreakGroundedInvariant(t *testing.T) {
	_, err := ApplyStageOverrides(overrideStages(), map[string]any{
		"synthesize": map[string]any{"temperature": 0.9},
	})
	if err == nil {
		t.Fatal("expected ApplyStageOverrides to error on grounded breach")
	}
}

func TestGroundedStagesPassAtZero(t *testing.T) {
	if err := AssertGroundedStagesZeroTemp(overrideStages()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(GroundedStages[:], []string{"synthesize", "cite", "filter"}) {
		t.Fatalf("GroundedStages = %v", GroundedStages)
	}
}

func stageIDs(stages []*Stage) []string {
	out := make([]string, len(stages))
	for i, s := range stages {
		out[i] = s.IDField
	}
	return out
}
