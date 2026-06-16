package lobes

import (
	"testing"

	"github.com/nccasia/agent-sdk-go/agent_sdk/core/spec"
)

func TestLobeSpecCompilesSingleSignal(t *testing.T) {
	l := Lobe{
		ID:       "router",
		Behavior: "select",
		Layer:    spec.LayerCognition,
		Order:    2,
		Excites:  map[string]float64{"plan": 1.0},
		Activation: func(ctx map[string]any) float64 {
			if ctxBool(ctx, "go") {
				return 1.0
			}
			return 0.0
		},
	}
	s := l.Spec()
	if s.ID != "router" || s.Behavior != "select" || s.Layer != spec.LayerCognition || s.Order != 2 {
		t.Fatalf("spec metadata: %+v", s)
	}
	if s.Edges["plan"] != 1.0 {
		t.Errorf("edge plan = %v", s.Edges["plan"])
	}
	// default single signal named by id.
	if v := s.Signals(map[string]any{"go": true})["router"]; v != 1.0 {
		t.Errorf("router signal on = %v, want 1.0", v)
	}
	if v := s.Signals(map[string]any{})["router"]; v != 0.0 {
		t.Errorf("router signal off = %v, want 0.0", v)
	}
}

func TestLobeSpecNilActivationIsDark(t *testing.T) {
	l := Lobe{ID: "plan", Layer: spec.LayerCognition, Order: 3}
	s := l.Spec()
	if v := s.Signals(map[string]any{"anything": true})["plan"]; v != 0.0 {
		t.Errorf("nil activation = %v, want 0.0 (dark)", v)
	}
}

func TestLobeSpecMultiSignalOverride(t *testing.T) {
	l := Lobe{
		ID:    "classify",
		Layer: spec.LayerCognition,
		Order: 2,
		Signals: func(ctx map[string]any) map[string]float64 {
			return map[string]float64{"a": 1.0, "b": 0.0}
		},
	}
	s := l.Spec()
	got := s.Signals(map[string]any{})
	if got["a"] != 1.0 || got["b"] != 0.0 {
		t.Errorf("multi-signal override = %v", got)
	}
}

func TestLobeSpecValidates(t *testing.T) {
	l := Lobe{ID: "synthesize", Layer: spec.LayerCognition, Order: 5}
	if err := l.Spec().Validate(); err != nil {
		t.Errorf("cognition lobe should validate: %v", err)
	}
	bad := Lobe{ID: "x", Layer: spec.LayerInstinct}
	if err := bad.Spec().Validate(); err == nil {
		t.Error("instinct-layer lobe should fail Validate")
	}
}
