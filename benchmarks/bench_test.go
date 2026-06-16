package benchmarks

import (
	"context"
	"testing"
)

// TestAttentionBenchReady is the cross-language parity gate. attentionbench
// ships NOT_READY in the Python source of truth: the qna/research grounding
// scenarios reference a cite lobe that does not activate without RAG, so two
// grounding checks fail (run.py exits 1). The Go bench reproduces that verdict.
func TestAttentionBenchReady(t *testing.T) {
	v, err := RunAttentionBench(context.Background(), "")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if v.Status != "NOT_READY" {
		t.Fatalf("status = %q, want NOT_READY (Python parity) — reasons: %v", v.Status, v.Reasons)
	}
}

// TestToolBenchReady is the parity gate for toolbench's free tier (spec/select/
// composite) — READY in Python without --live.
func TestToolBenchReady(t *testing.T) {
	v, err := RunToolBench(context.Background(), "")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if v.Status != "READY" {
		t.Fatalf("status = %q, want READY (reasons: %v)", v.Status, v.Reasons)
	}
}

// TestCorgictionBenchReady is the parity gate for the metacognition bench's
// deterministic floor (mirrors run.py without --live exiting 0, verdict READY).
func TestCorgictionBenchReady(t *testing.T) {
	v, err := RunCorgictionBench(context.Background(), "")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if v.Status != "READY" {
		t.Fatalf("status = %q, want READY (reasons: %v)", v.Status, v.Reasons)
	}
}

// TestFreeGatePasses mirrors ci-free-gates.sh / cmd/bench: every registered
// bench's deterministic floor reproduces its Python source-of-truth verdict, so
// the free-gate passes.
func TestFreeGatePasses(t *testing.T) {
	r := DefaultRegistry()
	rows, ok, err := r.FreeGate(context.Background())
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if !ok {
		for _, row := range rows {
			if !row.OK {
				t.Errorf("%s: status = %q, want %q (parity)", row.Name, row.Status, row.Expect)
			}
		}
		t.Fatalf("free-gate did not pass")
	}
}
