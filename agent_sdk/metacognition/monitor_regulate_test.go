package metacognition

import (
	"testing"

	"github.com/mezon/agent-sdk-go/agent_sdk/inspection"
)

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ── monitor ──────────────────────────────────────────────────────────────────

func TestMonitorEmptyLobeSlice(t *testing.T) {
	fa := &inspection.FlowAxisSnapshot{
		Flow: "research",
		Steps: []inspection.FlowStepInspection{
			{Flow: "research", Step: "gather", Lobes: nil},
		},
	}
	obs := Monitor(MonitorInput{FlowAxis: fa})
	if len(obs) != 1 || obs[0].Kind != "empty_lobe_slice" {
		t.Fatalf("obs = %+v", obs)
	}
	if obs[0].Severity != 0.8 || obs[0].Target != "research.gather" {
		t.Fatalf("obs detail = %+v", obs[0])
	}
}

func TestMonitorDisabledAndContextTight(t *testing.T) {
	fa := &inspection.FlowAxisSnapshot{
		Flow: "research",
		Steps: []inspection.FlowStepInspection{
			{Flow: "research", Step: "s1", Lobes: []string{"x"}, Disabled: true},
			{Flow: "research", Step: "s2", Lobes: []string{"x"}, StateNodes: []map[string]any{
				{"id": "context:tight", "activated": true},
			}},
			{Flow: "research", Step: "s3", Lobes: []string{"x"}, StateNodes: []map[string]any{
				{"id": "context:open", "activated": true},
			}},
		},
	}
	obs := Monitor(MonitorInput{FlowAxis: fa})
	kinds := map[string]bool{}
	for _, o := range obs {
		kinds[o.Kind] = true
	}
	for _, want := range []string{"step_disabled", "context_tight", "context_open"} {
		if !kinds[want] {
			t.Fatalf("missing kind %q in %+v", want, obs)
		}
	}
}

func TestMonitorLowConfidencePathAndEmptyStepContext(t *testing.T) {
	eng := &inspection.EngineSnapshot{
		Path:      map[string]any{"emergent": true, "name": "research"},
		FlowSteps: []map[string]any{{"flow": "research", "step": "gather", "node_count": 0}},
	}
	obs := Monitor(MonitorInput{Engine: eng})
	kinds := map[string]bool{}
	for _, o := range obs {
		kinds[o.Kind] = true
	}
	if !kinds["low_confidence_path"] || !kinds["empty_step_context"] {
		t.Fatalf("obs = %+v", obs)
	}
}

func TestMonitorLowScorePath(t *testing.T) {
	eng := &inspection.EngineSnapshot{Path: map[string]any{"score": 0.4, "name": "qna"}}
	obs := Monitor(MonitorInput{Engine: eng})
	if len(obs) != 1 || obs[0].Kind != "low_confidence_path" || obs[0].Target != "qna" {
		t.Fatalf("obs = %+v", obs)
	}
	// score above threshold → no observation
	eng2 := &inspection.EngineSnapshot{Path: map[string]any{"score": 0.9}}
	if got := Monitor(MonitorInput{Engine: eng2}); len(got) != 0 {
		t.Fatalf("expected no obs, got %+v", got)
	}
}

func TestMonitorInactiveLobeGroup(t *testing.T) {
	la := &inspection.LobeAxisSnapshot{
		Lobes: []inspection.LobeInspection{
			{ID: "skill_select", StateNodes: []map[string]any{{"id": "n", "activated": false}}},
		},
	}
	obs := Monitor(MonitorInput{LobeAxis: la})
	if len(obs) != 1 || obs[0].Kind != "inactive_lobe_group" {
		t.Fatalf("obs = %+v", obs)
	}
}

// ── regulate ─────────────────────────────────────────────────────────────────

func TestRegulateHealthyContinue(t *testing.T) {
	d := Regulate(nil, RegulateInput{})
	if d.Action != ActionContinue || d.Confidence != 1.0 {
		t.Fatalf("decision = %+v", d)
	}
	if d.Reason != "object-level state is healthy" {
		t.Fatalf("reason = %q", d.Reason)
	}
}

func TestRegulateLowConfidenceMetaReview(t *testing.T) {
	obs := []MetaObservation{{ID: "x", Kind: "low_confidence_path", Target: "research", Severity: 0.7, Detail: "low"}}
	d := Regulate(obs, RegulateInput{})
	if d.Action != ActionMetaReview || d.Confidence != 0.45 {
		t.Fatalf("decision = %+v", d)
	}
}

func TestRegulateContextTightAdjust(t *testing.T) {
	obs := []MetaObservation{{ID: "x", Kind: "context_tight", Target: "research.gather", Severity: 0.75}}
	d := Regulate(obs, RegulateInput{CurrentLobes: []string{"synthesize", "memory_recall", "skill_select"}})
	if d.Action != ActionAdjustLobeSlice {
		t.Fatalf("action = %q", d.Action)
	}
	if len(d.TargetLobes) != 1 || d.TargetLobes[0] != "synthesize" {
		t.Fatalf("target_lobes = %v", d.TargetLobes)
	}
	if deref(d.TargetFlow) != "research" || deref(d.TargetStep) != "gather" {
		t.Fatalf("target = %s.%s", deref(d.TargetFlow), deref(d.TargetStep))
	}
}

// when nothing is trimmable, context_tight falls through to the default continue.
func TestRegulateContextTightNoTrimmableFallsThrough(t *testing.T) {
	obs := []MetaObservation{{ID: "x", Kind: "context_tight", Target: "research.gather", Severity: 0.75}}
	d := Regulate(obs, RegulateInput{CurrentLobes: []string{"synthesize"}})
	if d.Action != ActionContinue {
		t.Fatalf("action = %q", d.Action)
	}
}

func TestRegulateEmptyLobeSliceSkipsNonPinned(t *testing.T) {
	obs := []MetaObservation{{ID: "x", Kind: "empty_lobe_slice", Target: "research.gather", Severity: 0.8, Detail: "no lobes"}}
	d := Regulate(obs, RegulateInput{})
	if d.Action != ActionSkipStep || d.Confidence != 0.75 {
		t.Fatalf("decision = %+v", d)
	}
}

func TestRegulateEmptyLobeSlicePinnedReviews(t *testing.T) {
	obs := []MetaObservation{{ID: "x", Kind: "empty_lobe_slice", Target: "research.cite", Severity: 0.8}}
	d := Regulate(obs, RegulateInput{})
	if d.Action != ActionMetaReview || d.Confidence != 0.4 {
		t.Fatalf("pinned empty slice must review, got %+v", d)
	}
}

func TestRegulateEmptyStepContextRetries(t *testing.T) {
	obs := []MetaObservation{{ID: "x", Kind: "empty_step_context", Target: "research.gather", Severity: 0.65, Detail: "empty"}}
	d := Regulate(obs, RegulateInput{})
	if d.Action != ActionRetryStep || d.Confidence != 0.65 {
		t.Fatalf("decision = %+v", d)
	}
}

func TestRegulateUnknownKindContinues(t *testing.T) {
	obs := []MetaObservation{{ID: "x", Kind: "context_open", Target: "a.b", Severity: 0.2}}
	d := Regulate(obs, RegulateInput{})
	if d.Action != ActionContinue || d.Confidence != 0.9 {
		t.Fatalf("decision = %+v", d)
	}
	if len(d.Queue) != 1 {
		t.Fatalf("queue should carry the observation, got %v", d.Queue)
	}
}

// ── CompileStatePlan ─────────────────────────────────────────────────────────

func TestCompileStatePlanSingleAspect(t *testing.T) {
	plan := CompileStatePlan([]any{"only one"}, false)
	if len(plan) != 1 || plan[0].State != "act" || plan[0].Subject == nil || *plan[0].Subject != "only one" {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestCompileStatePlanEmptyDegradesToAct(t *testing.T) {
	plan := CompileStatePlan(nil, false)
	if len(plan) != 1 || plan[0].State != "act" || plan[0].Subject != nil {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestCompileStatePlanFanOutAndSynthesize(t *testing.T) {
	plan := CompileStatePlan([]any{"a", "b", "c"}, false)
	if len(plan) != 4 {
		t.Fatalf("plan len = %d: %+v", len(plan), plan)
	}
	for i, subj := range []string{"a", "b", "c"} {
		if plan[i].State != "act" || plan[i].Subject == nil || *plan[i].Subject != subj {
			t.Fatalf("plan[%d] = %+v", i, plan[i])
		}
	}
	if plan[3].State != "synthesize" || plan[3].Subject != nil {
		t.Fatalf("plan[3] = %+v", plan[3])
	}
}

func TestCompileStatePlanGroundsAppendsCiteFilter(t *testing.T) {
	plan := CompileStatePlan([]any{map[string]any{"question": "q1"}, map[string]any{"subject": "q2"}}, true)
	// act, act, synthesize, cite, filter
	if len(plan) != 5 {
		t.Fatalf("plan = %+v", plan)
	}
	if plan[3].State != "cite" || plan[4].State != "filter" {
		t.Fatalf("grounding tail = %+v", plan[3:])
	}
}
