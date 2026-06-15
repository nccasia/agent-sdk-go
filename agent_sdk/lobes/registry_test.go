package lobes

import (
	"testing"

	"github.com/mezon/agent-sdk-go/agent_sdk/core/spec"
)

func baseLobes() []spec.Lobe {
	return []spec.Lobe{
		{ID: "synthesize", Behavior: "compose", Layer: spec.LayerCognition, Order: 5, Prior: 1.0, Pinned: true},
		{ID: "filter", Behavior: "verify", Layer: spec.LayerExpression, Order: 1},
	}
}

func TestNewRegistryValidatesAndSorts(t *testing.T) {
	r, err := NewRegistry(baseLobes(), nil)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	got := r.Lobes()
	if len(got) != 2 || got[0].ID != "synthesize" || got[1].ID != "filter" {
		t.Errorf("sorted lobes = %v", ids(got))
	}
}

func TestNewRegistryRejectsBackwardEdge(t *testing.T) {
	lobes := []spec.Lobe{
		{ID: "early", Layer: spec.LayerMemory, Order: 0, Edges: map[string]float64{"early": 1.0}},
	}
	if _, err := NewRegistry(lobes, nil); err == nil {
		t.Fatal("expected forward-DAG validation error")
	}
}

func TestAddRowRegistersDeclarativeLobe(t *testing.T) {
	r, _ := NewRegistry(baseLobes(), nil)
	lobe, err := r.AddRow(map[string]any{
		"id":       "toy",
		"behavior": "custom",
		"layer":    float64(spec.LayerCognition),
		"order":    float64(9),
		"signals":  map[string]any{"toy_sig": map[string]any{"flag": "toy_on"}},
		"edges":    map[string]any{},
	})
	if err != nil {
		t.Fatalf("AddRow: %v", err)
	}
	if lobe.ID != "toy" {
		t.Fatalf("lobe id = %q", lobe.ID)
	}
	got, ok := r.Get("toy")
	if !ok {
		t.Fatal("toy not registered")
	}
	// the compiled signal fires on the ctx flag
	if v := got.Signals(map[string]any{"toy_on": true})["toy_sig"]; v != 1.0 {
		t.Errorf("toy_sig on flag = %v, want 1.0", v)
	}
	if v := got.Signals(map[string]any{})["toy_sig"]; v != 0.0 {
		t.Errorf("toy_sig off flag = %v, want 0.0", v)
	}
}

func TestAddRowDefaults(t *testing.T) {
	r, _ := NewRegistry(baseLobes(), nil)
	lobe, err := r.AddRow(map[string]any{"id": "d"})
	if err != nil {
		t.Fatalf("AddRow: %v", err)
	}
	if lobe.Behavior != "custom" || lobe.Layer != spec.LayerCognition || lobe.Order != 99 || lobe.MinActivation != 0.5 {
		t.Errorf("defaults: behavior=%q layer=%d order=%d min=%v", lobe.Behavior, lobe.Layer, lobe.Order, lobe.MinActivation)
	}
}

func TestAddRowRejectsInhibitoryEdgeToPinned(t *testing.T) {
	r, _ := NewRegistry(baseLobes(), nil)
	// an early lobe with a negative edge to the pinned synthesize must fail.
	_, err := r.AddRow(map[string]any{
		"id":    "bad",
		"layer": float64(spec.LayerMemory),
		"order": float64(0),
		"edges": map[string]any{"synthesize": -1.0},
	})
	if err == nil {
		t.Fatal("expected pinned-edge protection error")
	}
	if _, ok := r.Get("bad"); ok {
		t.Error("bad lobe must not remain registered after a failed validation")
	}
}

func TestFromRowsAppliesLobeAndPathRows(t *testing.T) {
	r, err := FromRows(
		[]map[string]any{{"id": "x", "layer": float64(spec.LayerCognition), "order": float64(8)}},
		[]map[string]any{{"name": "p", "members": []any{"x"}, "recognizer": map[string]any{"flag": "p_on"}}},
	)
	if err != nil {
		t.Fatalf("FromRows: %v", err)
	}
	if _, ok := r.Get("x"); !ok {
		t.Error("lobe row not applied")
	}
	p, ok := r.GetPath("p")
	if !ok {
		t.Fatal("path row not applied")
	}
	if p.Recognizer(map[string]any{"p_on": true}) != 1.0 {
		t.Error("path recognizer did not fire on flag")
	}
}

func TestRemove(t *testing.T) {
	r, _ := NewRegistry(baseLobes(), nil)
	if err := r.Remove("filter"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, ok := r.Get("filter"); ok {
		t.Error("filter still present after Remove")
	}
}

func ids(ls []spec.Lobe) []string {
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = l.ID
	}
	return out
}

func BenchmarkAddRow(b *testing.B) {
	row := map[string]any{"id": "toy", "layer": float64(spec.LayerCognition), "order": float64(9),
		"signals": map[string]any{"s": map[string]any{"flag": "on"}}}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r, _ := NewRegistry(baseLobes(), nil)
		_, _ = r.AddRow(row)
	}
}
