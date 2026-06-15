// Ported from agent-sdk/tests/test_blocks_smoke.py (propagate + resolve_path
// cases) plus merge_lobe_weights.
package activate

import (
	"reflect"
	"testing"

	"github.com/mezon/agent-sdk-go/agent_sdk/core/feature"
	"github.com/mezon/agent-sdk-go/agent_sdk/core/spec"
)

func lobe(id string, layer int, mut func(*spec.Lobe)) spec.Lobe {
	l := spec.Lobe{ID: id, Behavior: "custom", Layer: layer, MinActivation: 0.5}
	if mut != nil {
		mut(&l)
	}
	return l
}

func TestPropagateActivatesSignalLobe(t *testing.T) {
	classify := lobe("classify", spec.LayerCognition, func(l *spec.Lobe) {
		l.Signals = func(ctx map[string]any) map[string]float64 {
			v := 0.0
			if b, _ := ctx["is_question"].(bool); b {
				v = 1.0
			}
			return map[string]float64{"is_question": v}
		}
		l.SignalWeights = map[string]float64{"is_question": 1.0}
		l.MinActivation = 0.5
	})
	res, err := Propagate([]spec.Lobe{classify}, map[string]any{"is_question": true}, map[string]float64{}, PropagateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Activated, []string{"classify"}) {
		t.Errorf("activated = %v", res.Activated)
	}
	if !res.ByID()["classify"].Activated {
		t.Error("classify must be activated")
	}
}

func TestPropagateBelowThresholdStaysDark(t *testing.T) {
	classify := lobe("classify", spec.LayerCognition, func(l *spec.Lobe) {
		l.Signals = func(map[string]any) map[string]float64 { return map[string]float64{"x": 0.0} }
		l.SignalWeights = map[string]float64{"x": 1.0}
		l.MinActivation = 0.5
	})
	res, err := Propagate([]spec.Lobe{classify}, map[string]any{}, map[string]float64{}, PropagateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Activated) != 0 {
		t.Errorf("activated = %v, want empty", res.Activated)
	}
}

func TestRecognizeAndResolvePath(t *testing.T) {
	research := spec.Path{
		Name:    "research",
		Members: []string{"plan"},
		Recognizer: func(ctx map[string]any) float64 {
			if b, _ := ctx["complex"].(bool); b {
				return 1.0
			}
			return 0.0
		},
		Threshold: 0.5,
	}
	scores := feature.RecognizePaths(map[string]any{"complex": true}, []spec.Path{research})
	if scores["research"] != 1.0 {
		t.Errorf("research score = %v", scores["research"])
	}
	resolved := ResolvePath(scores, []spec.Path{research})
	if resolved["name"] != "research" {
		t.Errorf("resolved name = %v", resolved["name"])
	}
	if resolved["emergent"] != false {
		t.Errorf("emergent = %v", resolved["emergent"])
	}
}

func TestResolvePathEmergentWhenNothingClears(t *testing.T) {
	p := spec.Path{Name: "qna", Recognizer: func(map[string]any) float64 { return 0.0 }, Threshold: 0.5}
	resolved := ResolvePath(feature.RecognizePaths(map[string]any{}, []spec.Path{p}), []spec.Path{p})
	if resolved["emergent"] != true {
		t.Errorf("emergent = %v", resolved["emergent"])
	}
	if resolved["name"] != "emergent" {
		t.Errorf("name = %v", resolved["name"])
	}
}

func TestMergeLobeWeightsSparseOverride(t *testing.T) {
	merged := MergeLobeWeights(map[string]float64{"prior_a": 0.1}, map[string]float64{"prior_a": 0.9})
	if merged["prior_a"] != 0.9 {
		t.Errorf("prior_a = %v", merged["prior_a"])
	}
}

func TestPropagateRejectsBackwardEdge(t *testing.T) {
	// a (expression) -> b (cognition) is backward: Propagate must error.
	a := lobe("a", spec.LayerExpression, func(l *spec.Lobe) {
		l.Order = 1
		l.Edges = map[string]float64{"b": 0.5}
	})
	b := lobe("b", spec.LayerCognition, func(l *spec.Lobe) { l.Order = 0 })
	if _, err := Propagate([]spec.Lobe{a, b}, map[string]any{}, map[string]float64{}, PropagateOptions{}); err == nil {
		t.Error("expected error for backward edge")
	}
}

func BenchmarkPropagate(b *testing.B) {
	classify := lobe("classify", spec.LayerCognition, func(l *spec.Lobe) {
		l.Signals = func(map[string]any) map[string]float64 { return map[string]float64{"is_question": 1.0} }
		l.SignalWeights = map[string]float64{"is_question": 1.0}
	})
	synth := lobe("synthesize", spec.LayerExpression, func(l *spec.Lobe) { l.Prior = 1.0 })
	lobes := []spec.Lobe{classify, synth}
	ctx := map[string]any{"is_question": true}
	w := map[string]float64{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Propagate(lobes, ctx, w, PropagateOptions{})
	}
}
