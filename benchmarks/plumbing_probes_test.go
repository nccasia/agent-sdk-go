package benchmarks

import (
	"context"
	"testing"

	"github.com/mezon/agent-sdk-go/agent_sdk/probe"
)

// populated asserts a probe record carries a real, non-empty trace (a flow name
// and at least one executed stage) — i.e. the agent ran end-to-end so the viewer
// inspection is populated, not empty.
func populated(t *testing.T, rec *probe.Record, what string) {
	t.Helper()
	if rec == nil {
		t.Fatalf("%s: nil record", what)
	}
	if rec.Flow == "" {
		t.Errorf("%s: empty flow (trace not populated)", what)
	}
	if len(rec.Stages) == 0 {
		t.Errorf("%s: no executed stages (trace not populated)", what)
	}
}

// TestStatelessBenchProbesPopulated asserts the statelessbench probe drives the
// real agent end-to-end across the stateless hop, so the inspection is populated.
func TestStatelessBenchProbesPopulated(t *testing.T) {
	recs, err := RunStatelessBenchProbes(context.Background(), "")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2 (store + recall hop)", len(recs))
	}
	for _, r := range recs {
		populated(t, r, "statelessbench probe "+r.Label)
	}
}

// TestToolBenchProbesPopulated asserts the toolbench probe drives a real agentic
// tool loop: a populated trace that captured the get_weather tool call.
func TestToolBenchProbesPopulated(t *testing.T) {
	recs, err := RunToolBenchProbes(context.Background(), "")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1 (loop · weather)", len(recs))
	}
	populated(t, recs[0], "toolbench probe")
	called := false
	for _, c := range recs[0].ToolCalls {
		if n, _ := c["name"].(string); n == "get_weather" {
			called = true
		}
	}
	if !called {
		t.Errorf("toolbench probe: get_weather not called; tool_calls=%v", recs[0].ToolCalls)
	}
}

// TestAttentionBenchProbesPopulated asserts the attentionbench probe captures
// 1-2 scenarios as real end-to-end traces (not the empty [] the Python passes).
func TestAttentionBenchProbesPopulated(t *testing.T) {
	recs, err := RunAttentionBenchProbes(context.Background(), "")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2 (qna + research)", len(recs))
	}
	for _, r := range recs {
		populated(t, r, "attentionbench probe "+r.Label)
	}
}
