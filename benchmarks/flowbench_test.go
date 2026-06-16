package benchmarks

import (
	"context"
	"testing"
)

// TestFlowBench_Ready is the cross-language parity gate for flowbench's
// deterministic floor. The Python source of truth (benchmarks/flowbench/run.py,
// no provider) certifies every default flow is wired + works — routing, tiers,
// states, grounding, coverage, determinism, subject, execution — and ships
// READY (63/63 checks, verdicts/flowbench.md). The Go bench reproduces that
// verdict: routing is a pure function of (spec, context), so the deterministic
// floor is READY.
func TestFlowBench_Ready(t *testing.T) {
	v, err := RunFlowBench(context.Background(), "")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if v.Status != "READY" {
		t.Fatalf("status = %q, want READY (Python parity) — reasons: %v", v.Status, v.Reasons)
	}
}

// TestFlowBench_CheckIDParity asserts the Go bench emits the SAME mode/check-id
// surface as the Python run.py (the 63 check ids across the 8 modes), so the
// verdict is comparable id-for-id. Mirrors verdicts/flowbench.md.
func TestFlowBench_CheckIDParity(t *testing.T) {
	payloads := flowPayloads(context.Background())

	// Per-mode check counts from the Python verdict baseline.
	wantCounts := map[string]int{
		"routing":     11,
		"tiers":       12,
		"states":      11,
		"grounding":   11,
		"coverage":    1,
		"determinism": 4,
		"subject":     2,
		"execution":   11,
	}
	got := 0
	for mode, want := range wantCounts {
		p := payloads[mode]
		if p == nil {
			t.Fatalf("mode %q missing", mode)
		}
		if len(p.Checks) != want {
			t.Errorf("mode %q: %d checks, want %d", mode, len(p.Checks), want)
		}
		got += len(p.Checks)
	}
	if len(payloads) != len(wantCounts) {
		t.Errorf("mode count = %d, want %d (%v)", len(payloads), len(wantCounts), payloads)
	}
	if got != 63 {
		t.Errorf("total checks = %d, want 63 (Python parity)", got)
	}

	// A spot-check of canonical check ids that must appear verbatim.
	wantIDs := []string{
		"routing.research-compare", "routing.adv-imperative",
		"tiers.spectrum_covered", "states.clarify-followup",
		"grounding.research-tradeoffs", "coverage.all_flows_tested",
		"determinism.qna-fact", "subject.threaded", "subject.tagged",
		"execution.onboarding-steward",
	}
	have := map[string]bool{}
	for _, p := range payloads {
		for _, c := range p.Checks {
			have[c.ID] = true
		}
	}
	for _, id := range wantIDs {
		if !have[id] {
			t.Errorf("missing check id %q", id)
		}
	}
}
