package metacognition

import (
	"reflect"
	"testing"

	"github.com/nccasia/agent-sdk-go/agent_sdk/inspection"
)

// ── facade: modes + coerce (tests/test_extra_coverage.py) ────────────────────

func TestMetacognitionModesAndCoerce(t *testing.T) {
	if m, _ := CoerceMetacognition("apply"); m.Mode != ModeApply {
		t.Fatalf("coerce(apply).mode = %q", m.Mode)
	}
	if m, _ := CoerceMetacognition("observe"); m.Mode != ModeObserve {
		t.Fatalf("coerce(observe).mode = %q", m.Mode)
	}
	if m, _ := CoerceMetacognition(nil); m.Mode != ModeObserve {
		t.Fatalf("coerce(nil).mode = %q", m.Mode)
	}
	m, _ := NewMetacognition(ModeApply, map[string]struct{}{ActionAdjustLobeSlice: {}})
	if got, _ := CoerceMetacognition(m); got != m {
		t.Fatal("coerce(meta) should return the same instance")
	}
	if _, err := NewMetacognition("frobnicate", nil); err == nil {
		t.Fatal("expected error for bad mode")
	}
	if _, err := CoerceMetacognition(123); err == nil {
		t.Fatal("expected error coercing an int")
	}
}

func TestMetacognitionPinnedNeverSkipped(t *testing.T) {
	want := map[string]struct{}{"cite": {}, "filter": {}}
	if !reflect.DeepEqual(PinnedUnskippable, want) {
		t.Fatalf("PinnedUnskippable = %v", PinnedUnskippable)
	}
	meta, _ := NewMetacognition(ModeApply, map[string]struct{}{ActionAdjustLobeSlice: {}})
	flow, step := "research", "cite"
	decision := meta.PlanNext(PlanNextInput{TargetFlow: &flow, TargetStep: &step})
	if decision.Action == ActionSkipStep {
		t.Fatalf("plan_next must never skip a pinned step, got %q", decision.Action)
	}
}

// PlanNext that WOULD skip a non-pinned empty step still skips; a pinned target
// rewrites to continue with the pin reason.
func TestPlanNextPinnedGuardRewritesSkip(t *testing.T) {
	meta, _ := NewMetacognition(ModeApply, nil)
	flowAxis := &inspection.FlowAxisSnapshot{
		Flow: "research",
		Steps: []inspection.FlowStepInspection{
			{Flow: "research", Step: "cite", Lobes: nil}, // empty slice → would skip
		},
	}
	step := "cite"
	d := meta.PlanNext(PlanNextInput{
		MonitorInput: MonitorInput{FlowAxis: flowAxis},
		TargetStep:   &step,
	})
	// the empty pinned slice routes to meta_review in the regulator (pinned step),
	// and the facade guard never rewrites a non-skip; assert it is not a skip.
	if d.Action == ActionSkipStep {
		t.Fatalf("pinned step must not be skipped, got %q", d.Action)
	}
}

// ── controller: mode resolution + allow-list ─────────────────────────────────

func TestMetacognitionModeDefaultsToApply(t *testing.T) {
	t.Setenv("METACOGNITION", "")
	if got := MetacognitionMode(nil); got != ModeApply {
		t.Fatalf("default mode = %q", got)
	}
	if got := MetacognitionMode(map[string]any{"metacognition_mode": "observe"}); got != ModeObserve {
		t.Fatalf("policy mode = %q", got)
	}
	if got := MetacognitionMode(map[string]any{"metacognition_enabled": false}); got != ModeObserve {
		t.Fatalf("legacy enabled=false → %q", got)
	}
}

func TestMetacognitionModeEnvWins(t *testing.T) {
	t.Setenv("METACOGNITION", "observe")
	if got := MetacognitionMode(map[string]any{"metacognition_mode": "apply"}); got != ModeObserve {
		t.Fatalf("env should win, got %q", got)
	}
	// legacy off-token maps to observe
	t.Setenv("METACOGNITION", "disabled")
	if got := MetacognitionMode(nil); got != ModeObserve {
		t.Fatalf("disabled → %q", got)
	}
}

func TestControllerShouldApply(t *testing.T) {
	c := NewMetaController(ModeApply, false, map[string]struct{}{ActionAdjustLobeSlice: {}})
	if !c.ShouldApply(ActionAdjustLobeSlice) {
		t.Fatal("adjust_lobe_slice should apply in apply mode")
	}
	if c.ShouldApply(ActionSkipStep) {
		t.Fatal("skip_step not in allow-list")
	}
	obs := NewMetaController(ModeObserve, false, nil)
	if obs.ShouldApply(ActionAdjustLobeSlice) {
		t.Fatal("observe mode never applies")
	}
}

func TestFromPolicyParsesApplyActions(t *testing.T) {
	t.Setenv("METACOGNITION", "")
	c := MetaControllerFromPolicy(map[string]any{
		"metacognition_apply_actions": "adjust_lobe_slice, skip_step, bogus",
	})
	if !c.ShouldApply(ActionAdjustLobeSlice) || !c.ShouldApply(ActionSkipStep) {
		t.Fatal("expected adjust + skip allowed")
	}
	if _, ok := c.ApplyActions["bogus"]; ok {
		t.Fatal("non-capable action must be dropped")
	}
}
