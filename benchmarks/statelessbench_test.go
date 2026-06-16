package benchmarks

import (
	"context"
	"sort"
	"testing"
)

// TestStatelessBenchReady is the cross-language parity gate for statelessbench's
// deterministic floor. The Python source of truth (benchmarks/statelessbench/
// run.py) runs every mode against a FakeClient + auto-establish, so all checks
// pass → verdict READY (run.py exits 0). The Go bench reproduces that verdict.
func TestStatelessBenchReady(t *testing.T) {
	v, err := RunStatelessBench(context.Background(), "")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if v.Status != "READY" {
		t.Fatalf("status = %q, want READY (Python parity) — reasons: %v", v.Status, v.Reasons)
	}
}

// TestStatelessBenchCheckIDParity pins the exact per-mode check ids the bench
// emits to the Python run.py source-of-truth ids, so a rename/drop on either
// side fails the gate (the verdict alone would not catch a silently missing
// check). The ids are taken verbatim from run.py's _check(...) calls for the
// five ported modes (snapshot/store/isolation/spec/schema).
func TestStatelessBenchCheckIDParity(t *testing.T) {
	ctx := context.Background()
	mode := func(p *ModePayload) []string {
		ids := make([]string, 0, len(p.Checks))
		for _, c := range p.Checks {
			ids = append(ids, c.ID)
		}
		sort.Strings(ids)
		return ids
	}
	want := map[string][]string{
		"snapshot": {
			"snapshot.fact_stored", "snapshot.history_carried", "snapshot.history_grew",
			"snapshot.memory_survived_hop", "snapshot.versioned",
		},
		"store": {
			"store.history_continued", "store.history_persisted",
			"store.memory_persisted", "store.resumes_across_agents",
		},
		"isolation": {
			"isolation.alice_only_own", "isolation.bob_only_own", "isolation.carol_only_own",
			"isolation.dave_only_own", "isolation.erin_only_own", "isolation.frank_only_own",
		},
		"spec":   {"spec.has_network", "spec.roundtrips"},
		"schema": {"schema.carries_memory", "schema.tolerates_missing", "schema.tolerates_unknown", "schema.versioned"},
	}
	got := map[string]*ModePayload{
		"snapshot":  statelessRunSnapshot(ctx),
		"store":     statelessRunStore(ctx),
		"isolation": statelessRunIsolation(ctx),
		"spec":      statelessRunSpec(),
		"schema":    statelessRunSchema(),
	}
	for m, ids := range want {
		gotIDs := mode(got[m])
		if len(gotIDs) != len(ids) {
			t.Fatalf("mode %q: %d check ids %v, want %d %v", m, len(gotIDs), gotIDs, len(ids), ids)
		}
		for i := range ids {
			if gotIDs[i] != ids[i] {
				t.Errorf("mode %q: check id %q, want %q (parity)", m, gotIDs[i], ids[i])
			}
		}
		for _, c := range got[m].Checks {
			if !c.OK {
				t.Errorf("mode %q: check %q failed: %s", m, c.ID, c.Detail)
			}
		}
	}
}
