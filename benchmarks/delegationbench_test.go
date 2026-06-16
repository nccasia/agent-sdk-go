package benchmarks

import (
	"context"
	"strings"
	"testing"
)

// TestDelegationBench_Ready is the cross-language parity gate for
// delegationbench's deterministic floor. delegationbench is the LIVE benchmark
// for plan-driven fan-out: every scenario drives the REAL PreactAgent against a
// real provider (plan → supervise → execute → fanin), so there is NO
// deterministic floor. Python's run.py refuses to compose a verdict without a
// provider token (payloads["live"] = None → UNMEASURED). The Go bench
// reproduces that: with no model the single "live" mode is missing evidence, so
// the verdict is UNMEASURED (no evidence is never READY).
func TestDelegationBench_Ready(t *testing.T) {
	v, err := RunDelegationBench(context.Background(), "")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if v.Status != "UNMEASURED" {
		t.Fatalf("status = %q, want UNMEASURED (Python parity — live bench, no provider) — reasons: %v",
			v.Status, v.Reasons)
	}
	// The missing "live" mode gates as nil (it did not run), never as a pass.
	if g, ok := v.Gates["live_all_pass"]; ok && g != nil {
		t.Fatalf("live_all_pass gate = %v, want absent/nil without a provider", *g)
	}
}

// TestDelegationBench_CheckIDParity asserts the Go bench emits the SAME static,
// provider-independent check-id surface as the Python run.py: for each scenario
// it emits exactly one of live.fanin.{id} (should-plan with a facet contract)
// or live.decision.{id} (otherwise), plus the four aggregate ids
// (live.planning.precision / live.planning.recall / live.exec.coverage /
// live.fanin.fidelity). The per-scenario live.exec.{id} ids are conditional on
// the runtime plan width (≥2 steps), so they are not part of the static floor.
// Mirrors the check ids built in benchmarks/delegationbench/run.py.
func TestDelegationBench_CheckIDParity(t *testing.T) {
	scns, err := delegationBenchScenarios()
	if err != nil {
		t.Fatalf("scenarios: %v", err)
	}
	if len(scns) != 41 {
		t.Fatalf("scenarios = %d, want 41 (dataset parity)", len(scns))
	}

	ids := delegationBenchStaticCheckIDs()
	have := map[string]bool{}
	for _, id := range ids {
		have[id] = true
	}

	fanin, decision := 0, 0
	for _, s := range scns {
		switch {
		case s.Want && len(s.AnswerContains) > 0:
			want := "live.fanin." + s.ID
			if !have[want] {
				t.Errorf("missing fanin check id %q", want)
			}
			fanin++
		default:
			want := "live.decision." + s.ID
			if !have[want] {
				t.Errorf("missing decision check id %q", want)
			}
			decision++
		}
	}
	if fanin != 26 {
		t.Errorf("fanin scenarios = %d, want 26", fanin)
	}
	if decision != 15 {
		t.Errorf("decision scenarios = %d, want 15", decision)
	}

	for _, agg := range []string{
		"live.planning.precision", "live.planning.recall",
		"live.exec.coverage", "live.fanin.fidelity",
	} {
		if !have[agg] {
			t.Errorf("missing aggregate check id %q", agg)
		}
	}

	// No per-scenario live.exec.{id} ids belong to the static floor (they are
	// runtime-conditional on the plan width).
	for _, id := range ids {
		if strings.HasPrefix(id, "live.exec.") && id != "live.exec.coverage" {
			t.Errorf("unexpected runtime-conditional id in static surface: %q", id)
		}
	}

	// Static surface size: one per scenario + four aggregates.
	if len(ids) != 41+4 {
		t.Fatalf("static check ids = %d, want %d", len(ids), 41+4)
	}
}

// TestDelegationBench_RegisteredLive asserts the bench is registered as a Live
// tier with the UNMEASURED parity target and does not break the free-gate.
func TestDelegationBench_RegisteredLive(t *testing.T) {
	r := DefaultRegistry()
	b, ok := r.Get("delegationbench")
	if !ok {
		t.Fatalf("delegationbench not registered")
	}
	if b.Tier != Live {
		t.Fatalf("tier = %v, want Live", b.Tier)
	}
	if b.ExpectStatus != "UNMEASURED" {
		t.Fatalf("ExpectStatus = %q, want UNMEASURED", b.ExpectStatus)
	}
	rows, allOK, err := r.FreeGate(context.Background())
	if err != nil {
		t.Fatalf("free-gate: %v", err)
	}
	if !allOK {
		t.Fatalf("free-gate failed with delegationbench registered: %v", rows)
	}
}
