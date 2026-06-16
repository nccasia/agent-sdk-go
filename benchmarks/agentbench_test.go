package benchmarks

import (
	"context"
	"testing"
)

// TestAgentBench_Ready is the cross-language parity gate for agentbench's
// deterministic floor. agentbench is the LIVE benchmark: every behavior (the
// integrated mission + the hard cases) drives the REAL PreactAgent against a
// real provider — there is NO deterministic floor. Python's run.py refuses to
// compose a verdict without a provider token ("agentbench is a LIVE bench — set
// a provider token …", exit 2). The Go bench reproduces that: with no model the
// single "agentbench" mode is missing evidence, so the verdict is UNMEASURED
// (no evidence is never READY).
func TestAgentBench_Ready(t *testing.T) {
	v, err := RunAgentBench(context.Background(), "")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if v.Status != "UNMEASURED" {
		t.Fatalf("status = %q, want UNMEASURED (Python parity — live bench, no provider) — reasons: %v",
			v.Status, v.Reasons)
	}
	// The missing mode gates as nil (it did not run), never as a pass.
	if g, ok := v.Gates["agentbench_all_pass"]; ok && g != nil {
		t.Fatalf("agentbench_all_pass gate = %v, want absent/nil without a provider", *g)
	}
}

// TestAgentBench_CheckIDParity asserts the Go bench emits the SAME mode +
// check-id surface as the Python run.py: ONE mode "agentbench" carrying the 7
// mission.* checks and the 2 hard.* checks (9 total). Mirrors the b.check(...)
// ids in benchmarks/agentbench/run.py. The ids are a static, provider-
// independent property of the bench, so they are asserted on the
// deterministic-floor surface.
func TestAgentBench_CheckIDParity(t *testing.T) {
	ids := agentBenchCheckIDs()

	want := []string{
		"mission.memorized",
		"mission.recall_current_supersession",
		"mission.no_double_greeting",
		"mission.distractor_entity",
		"mission.needle_recall",
		"mission.synthesize_from_memory",
		"mission.cross_session_recall",
		"hard.bounded_context",
		"hard.recall_at_scale",
	}
	if len(ids) != len(want) {
		t.Fatalf("check ids = %d, want %d (%v)", len(ids), len(want), ids)
	}
	have := map[string]bool{}
	for _, id := range ids {
		have[id] = true
	}
	for _, id := range want {
		if !have[id] {
			t.Errorf("missing check id %q", id)
		}
	}
	mission, hard := 0, 0
	for _, id := range ids {
		switch {
		case len(id) >= 8 && id[:8] == "mission.":
			mission++
		case len(id) >= 5 && id[:5] == "hard.":
			hard++
		}
	}
	if mission != 7 {
		t.Errorf("mission.* checks = %d, want 7", mission)
	}
	if hard != 2 {
		t.Errorf("hard.* checks = %d, want 2", hard)
	}
}
