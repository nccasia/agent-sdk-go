package cognition

import (
	"testing"

	"github.com/nccasia/agent-sdk-go/agent_sdk/core/spec"
)

func TestCognitionLobeMetadata(t *testing.T) {
	byID := map[string]spec.Lobe{}
	for _, l := range Lobes {
		byID[l.ID] = l.Spec()
	}
	for _, id := range []string{"classify", "condense", "plan", "research", "scope_check", "synthesize"} {
		if _, ok := byID[id]; !ok {
			t.Fatalf("missing cognition lobe %q", id)
		}
		if byID[id].Layer != spec.LayerCognition {
			t.Fatalf("%s layer = %d", id, byID[id].Layer)
		}
	}
	if !byID["synthesize"].Pinned {
		t.Fatal("synthesize must be pinned")
	}
	if byID["plan"].MinActivation != 1.5 {
		t.Fatalf("plan threshold = %v", byID["plan"].MinActivation)
	}
}

func TestClassifyExcitesPlanAndSynthesize(t *testing.T) {
	s := Classify.Spec()
	if s.Edges["plan"] != 1.0 || s.Edges["synthesize"] != 1.0 {
		t.Fatalf("classify edges = %v", s.Edges)
	}
}

func TestClassifySimpleShapeInhibits(t *testing.T) {
	// has_stage_classify gates the router; a strongly-qna simple query also sets
	// the inhibitory simple_shape signal.
	signals := Classify.Spec().Signals(map[string]any{
		"query":  "what is the capital of France?",
		"stages": []any{"classify"},
	})
	if signals["has_stage_classify"] != 1.0 {
		t.Fatalf("has_stage_classify = %v", signals["has_stage_classify"])
	}
	if signals["simple_shape"] != 1.0 {
		t.Fatalf("simple_shape should fire on a strong qna query, got %v", signals["simple_shape"])
	}
	// a complex/comparative query is NOT simple-shaped → router runs.
	cplx := Classify.Spec().Signals(map[string]any{
		"query":  "compare the GDP of Canada vs Brazil",
		"stages": []any{"classify"},
	})
	if cplx["simple_shape"] != 0.0 {
		t.Fatalf("comparative query should not be simple-shaped, got %v", cplx["simple_shape"])
	}
}

func TestPlanRouteComplexSignal(t *testing.T) {
	hot := Plan.Spec().Signals(map[string]any{"route": "complex"})
	if hot["route_complex"] != 1.0 {
		t.Fatalf("route_complex = %v", hot["route_complex"])
	}
	cold := Plan.Spec().Signals(map[string]any{"route": "simple"})
	if cold["route_complex"] != 0.0 {
		t.Fatalf("route_complex = %v", cold["route_complex"])
	}
}

func TestCondenseAnaphoraAndShortQuery(t *testing.T) {
	s := Condense.Spec().Signals(map[string]any{
		"query":       "what about that one",
		"stages":      []any{"condense"},
		"has_history": true,
	})
	if s["anaphora"] != 1.0 {
		t.Fatalf("anaphora = %v", s["anaphora"])
	}
	// without history the lobe is ineligible → no anaphora/short signals.
	noHist := Condense.Spec().Signals(map[string]any{
		"query":  "what about that one",
		"stages": []any{"condense"},
	})
	if noHist["anaphora"] != 0.0 || noHist["short_query"] != 0.0 {
		t.Fatalf("ineligible should be dark, got %v", noHist)
	}
}

func TestScopeCheckGatedOnFlag(t *testing.T) {
	on := ScopeCheck.Spec().Signals(map[string]any{"scope_gate": true})
	if on["scope_check"] != 1.0 {
		t.Fatalf("scope_check on = %v", on["scope_check"])
	}
	off := ScopeCheck.Spec().Signals(map[string]any{})
	if off["scope_check"] != 0.0 {
		t.Fatalf("scope_check off = %v", off["scope_check"])
	}
}

func TestFormatMemos(t *testing.T) {
	memos := []Memo{
		{AspectID: "a", Claims: []Claim{{Text: "x"}, {Text: "y"}}},
		{AspectID: "b", Claims: []Claim{{Text: "z"}}},
	}
	got := FormatMemos(memos)
	want := "## a\n- x\n- y\n## b\n- z"
	if got != want {
		t.Fatalf("FormatMemos = %q, want %q", got, want)
	}
}

func TestSystemPromptsNonEmpty(t *testing.T) {
	prompts := []string{
		ClassifySystemPrompt, CondenseSystemPrompt, PlanSystemPrompt,
		ResearchSystemPrompt, ScopeCheckSystemPrompt, SynthesizeSystemPrompt,
		SynthesizeSimpleSystemPrompt,
	}
	for i, p := range prompts {
		if p == "" {
			t.Fatalf("system prompt %d is empty", i)
		}
	}
}
