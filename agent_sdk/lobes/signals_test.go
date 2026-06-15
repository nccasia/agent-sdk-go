// Per-lobe free signal extraction parity (the OY perception layer): each
// production lobe emits the same signal map Python's signals(ctx) does.
package lobes

import "testing"

func TestMemoryRecallSignals(t *testing.T) {
	got := memoryRecallSignals(map[string]any{
		"memory_enabled": true,
		"memory_scopes":  map[string]any{"conversation": 1, "user": 1},
	})
	want := map[string]float64{
		"memory_enabled": 1.0, "mem_conversation": 1.0, "mem_channel": 0.0,
		"mem_user": 1.0, "mem_bot": 0.0,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
}

func TestMemoryRecallEnabledDefaultsTrue(t *testing.T) {
	got := memoryRecallSignals(map[string]any{})
	if got["memory_enabled"] != 1.0 {
		t.Errorf("memory_enabled default = %v, want 1.0", got["memory_enabled"])
	}
}

func TestCondenseSignals(t *testing.T) {
	// eligible: condense stage present + history; query is anaphoric and short.
	ctx := map[string]any{"query": "this?", "stages": []any{"condense"}, "has_history": true}
	got := condenseSignals(ctx)
	if got["anaphora"] != 1.0 {
		t.Errorf("anaphora = %v, want 1.0", got["anaphora"])
	}
	if got["short_query"] != 1.0 {
		t.Errorf("short_query = %v, want 1.0", got["short_query"])
	}
	if got["has_history"] != 1.0 {
		t.Errorf("has_history = %v", got["has_history"])
	}
}

func TestCondenseSignalsIneligibleWithoutStage(t *testing.T) {
	ctx := map[string]any{"query": "this?", "has_history": true}
	got := condenseSignals(ctx)
	if got["anaphora"] != 0.0 || got["short_query"] != 0.0 {
		t.Errorf("ineligible should zero anaphora/short: %v", got)
	}
}

func TestClassifySignals(t *testing.T) {
	got := classifySignals(map[string]any{"query": "what is the capital of France?", "stages": []any{"classify"}})
	if got["has_stage_classify"] != 1.0 {
		t.Errorf("has_stage_classify = %v", got["has_stage_classify"])
	}
	// interrogative+short with no anaphora/comparative ⇒ simple shape.
	if got["simple_shape"] != 1.0 {
		t.Errorf("simple_shape = %v, want 1.0", got["simple_shape"])
	}
}

func TestClassifySimpleShapeFalseOnAnaphora(t *testing.T) {
	// "it" is an anaphoric referent — a simple-looking query may hide multi-hop work.
	got := classifySignals(map[string]any{"query": "what time is it?", "stages": []any{"classify"}})
	if got["simple_shape"] != 0.0 {
		t.Errorf("anaphoric query simple_shape = %v, want 0.0", got["simple_shape"])
	}
}

func TestClassifySimpleShapeFalseOnComparative(t *testing.T) {
	got := classifySignals(map[string]any{
		"query":  "compare microservices versus monoliths in great detail across all dimensions",
		"stages": []any{"classify"},
	})
	if got["simple_shape"] != 0.0 {
		t.Errorf("comparative query simple_shape = %v, want 0.0", got["simple_shape"])
	}
}

func TestPlanSignals(t *testing.T) {
	if v := planSignals(map[string]any{"route": "complex"})["route_complex"]; v != 1.0 {
		t.Errorf("route_complex = %v, want 1.0", v)
	}
	if v := planSignals(map[string]any{"route": "simple"})["route_complex"]; v != 0.0 {
		t.Errorf("route_complex simple = %v, want 0.0", v)
	}
}

func TestSimpleSignalLobes(t *testing.T) {
	if v := skillSelectSignals(map[string]any{"skills_declared": true})["skill_select"]; v != 1.0 {
		t.Errorf("skill_select = %v", v)
	}
	if v := toolSelectSignals(map[string]any{"tool_strategy": "adaptive"})["tool_select"]; v != 1.0 {
		t.Errorf("tool_select = %v", v)
	}
	if v := scopeCheckSignals(map[string]any{"scope_gate": true})["scope_check"]; v != 1.0 {
		t.Errorf("scope_check = %v", v)
	}
	if v := respondSignals(nil)["respond"]; v != 1.0 {
		t.Errorf("respond = %v", v)
	}
}

func TestSignalFor(t *testing.T) {
	if SignalFor("memory_recall") == nil {
		t.Fatal("memory_recall has no signal fn")
	}
	// unknown id -> empty-signal extractor (not nil).
	if got := SignalFor("nope")(map[string]any{}); len(got) != 0 {
		t.Errorf("unknown id signals = %v, want empty", got)
	}
}

func BenchmarkClassifySignals(b *testing.B) {
	ctx := map[string]any{"query": "what time is it?", "stages": []any{"classify"}}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = classifySignals(ctx)
	}
}
