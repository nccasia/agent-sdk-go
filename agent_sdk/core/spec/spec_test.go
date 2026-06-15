// Ported from agent-sdk/tests/test_blocks_smoke.py (validate_network cases) and
// agent-sdk/tests/test_spec.py (serializable spec round-trip — the portions
// independent of the agent façade).
package spec

import (
	"encoding/json"
	"reflect"
	"testing"
)

func specLobe(id string, layer int, mut func(*Lobe)) Lobe {
	l := Lobe{ID: id, Behavior: "custom", Layer: layer, MinActivation: 0.5}
	if mut != nil {
		mut(&l)
	}
	return l
}

func TestForwardDAGValidationAcceptsForwardEdge(t *testing.T) {
	a := specLobe("a", LayerExpression, func(l *Lobe) { l.Order = 1 })
	b := specLobe("b", LayerCognition, func(l *Lobe) {
		l.Order = 0
		l.Edges = map[string]float64{"a": 0.5}
	})
	// b (cognition) -> a (expression) is forward: fine.
	if err := ValidateNetwork([]Lobe{a, b}); err != nil {
		t.Errorf("forward edge rejected: %v", err)
	}
}

func TestForwardDAGValidationRejectsBackwardEdge(t *testing.T) {
	b := specLobe("b", LayerCognition, func(l *Lobe) { l.Order = 0 })
	// a (expression) -> b (cognition) is backward: rejected.
	bad := specLobe("a", LayerExpression, func(l *Lobe) {
		l.Order = 1
		l.Edges = map[string]float64{"b": 0.5}
	})
	if err := ValidateNetwork([]Lobe{bad, b}); err == nil {
		t.Error("expected error for backward edge")
	}
}

func TestValidateNetworkRejectsInhibitoryEdgeToPinned(t *testing.T) {
	cite := specLobe("cite", LayerExpression, func(l *Lobe) {
		l.Order = 1
		l.Pinned = true
	})
	src := specLobe("src", LayerCognition, func(l *Lobe) {
		l.Order = 0
		l.Edges = map[string]float64{"cite": -0.5}
	})
	if err := ValidateNetwork([]Lobe{src, cite}); err == nil {
		t.Error("expected error for inhibitory edge to pinned lobe")
	}
}

func TestValidateNetworkRejectsDuplicateID(t *testing.T) {
	a := specLobe("dup", LayerCognition, nil)
	b := specLobe("dup", LayerExpression, nil)
	if err := ValidateNetwork([]Lobe{a, b}); err == nil {
		t.Error("expected error for duplicate lobe id")
	}
}

func TestLobeValidateRejectsCoreLayer(t *testing.T) {
	l := Lobe{ID: "x", Layer: LayerInstinct}
	if err := l.Validate(); err == nil {
		t.Error("a lobe at layer 0 must be rejected (lobes live in B2..B5)")
	}
	ok := Lobe{ID: "y", Layer: LayerCognition}
	if err := ok.Validate(); err != nil {
		t.Errorf("cognition lobe rejected: %v", err)
	}
}

func TestSpecDefaults(t *testing.T) {
	s := NewSpec()
	if s.Version != "1" {
		t.Errorf("version = %q", s.Version)
	}
	if s.TZ != "UTC" || s.Lang != "en" {
		t.Errorf("tz/lang = %q/%q", s.TZ, s.Lang)
	}
	if !reflect.DeepEqual(s.PinnedLobes, []string{"cite", "filter"}) {
		t.Errorf("pinned_lobes = %v", s.PinnedLobes)
	}
}

func TestSpecJSONRoundTrips(t *testing.T) {
	s := NewSpec()
	s.Instructions = "You are helpful."
	s.Lobes = []map[string]any{{"id": "classify"}, {"id": "synthesize"}}
	blob, err := s.ToJSONStr()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := FromJSON([]byte(blob))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.ToJSON(), s.ToJSON()) {
		t.Errorf("round-trip mismatch:\n got %v\nwant %v", restored.ToJSON(), s.ToJSON())
	}
}

func TestSpecFromJSONIgnoresUnknownKeys(t *testing.T) {
	raw := `{"version":"1","instructions":"hi","unknown_field":42,"tz":"Asia/Bangkok"}`
	s, err := FromJSON([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if s.Instructions != "hi" || s.TZ != "Asia/Bangkok" {
		t.Errorf("from_json = %+v", s)
	}
}

func TestSpecToJSONContainsCanonicalKeys(t *testing.T) {
	s := NewSpec()
	m := s.ToJSON()
	for _, k := range []string{"version", "instructions", "lobes", "weights", "budgets", "pinned_lobes"} {
		if _, ok := m[k]; !ok {
			t.Errorf("to_json missing key %q", k)
		}
	}
	// pinned set parity with the contracts source of truth.
	b, _ := json.Marshal(SortedPinnedLobes())
	if string(b) != `["cite","filter"]` {
		t.Errorf("SortedPinnedLobes = %s", b)
	}
}
