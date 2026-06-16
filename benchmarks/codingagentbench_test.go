package benchmarks

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func cabWriteFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func cabMkdir(t *testing.T, dir, rel string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, rel), 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestCodingAgentBench_Ready is the cross-language parity gate for
// coding-agent-bench's deterministic floor. coding-agent-bench has two tiers:
// a FREE replay tier (the scripted "understand" pipeline over a temp fixture —
// survey → plan → investigate → document, writing a real ARCHITECTURE.md) and a
// LIVE tier (the same flow against a real provider over a large repo). Python's
// run.py refuses to compose the live verdict without a provider token (exit 2),
// and the replay tier is its own pass/fail floor. The Go bench reproduces that:
// the live "understand" mode is missing evidence without a provider, so the
// composed verdict is UNMEASURED (no evidence is never READY) — even though the
// deterministic "replay" mode runs and passes.
func TestCodingAgentBench_Ready(t *testing.T) {
	v, err := RunCodingAgentBench(context.Background(), "")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if v.Status != "UNMEASURED" {
		t.Fatalf("status = %q, want UNMEASURED (Python parity — live bench, no provider) — reasons: %v",
			v.Status, v.Reasons)
	}
	// The live "understand" mode is missing without a provider — it gates as nil
	// (it did not run), never as a pass.
	if g, ok := v.Gates["understand_all_pass"]; ok && g != nil {
		t.Fatalf("understand_all_pass gate = %v, want absent/nil without a provider", *g)
	}
	// The FREE replay tier DID run deterministically and must pass — it gates as
	// a real pass (the CI floor for the whole understand pipeline).
	g, ok := v.Gates["replay_all_pass"]
	if !ok || g == nil {
		t.Fatalf("replay_all_pass gate = absent/nil, want a real pass (the free replay tier ran)")
	}
	if !*g {
		t.Fatalf("replay_all_pass = false, want true — the deterministic understand pipeline must score green")
	}
}

// TestCodingAgentBench_CheckIDParity asserts the Go bench emits the SAME
// check-id surface as the Python run.py score(): the 5 correctness + 3
// efficiency + 2 accuracy ids (10 total), in run.py order. Mirrors the check
// ids in benchmarks/coding-agent-bench/run.py:score(). The ids are a static,
// provider-independent property of the scoring instrument, so they are asserted
// on the deterministic-floor (replay) surface.
func TestCodingAgentBench_CheckIDParity(t *testing.T) {
	ids := codingAgentBenchCheckIDs()
	want := []string{
		// correctness
		"routed", "used_tools", "wrote_doc", "answered", "not_truncated",
		// efficiency (size-normalized)
		"hops_bounded", "tokens_bounded", "no_redundant_writes",
		// accuracy
		"paths_grounded", "covers_subsystems",
	}
	if len(ids) != len(want) {
		t.Fatalf("check ids = %d, want %d (%v)", len(ids), len(want), ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("check id[%d] = %q, want %q (order must match run.py score())", i, ids[i], want[i])
		}
	}
}

// TestCodingAgentBench_ReplayScores asserts the deterministic replay tier
// reproduces the full score() surface against the temp fixture: every one of
// the 10 checks passes, mirroring Python's `python run.py --replay` returning 0
// (all checks green). This is the free CI floor — routing + the four-stage flow
// + memory aggregation + grounded ARCHITECTURE.md, end to end, no provider.
func TestCodingAgentBench_ReplayScores(t *testing.T) {
	p, err := codingAgentBenchReplay(context.Background())
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !p.AllPass {
		failed := []string{}
		for _, c := range p.Checks {
			if !c.OK {
				failed = append(failed, c.ID+": "+c.Detail)
			}
		}
		t.Fatalf("replay did not all-pass; failing: %v", failed)
	}
	if len(p.Checks) != 10 {
		t.Fatalf("replay checks = %d, want 10", len(p.Checks))
	}
}

// TestCodingAgentBench_AccuracyMetrics mirrors test_scoring.py:
// test_accuracy_metrics_flags_phantom_paths — the doc's file references are
// resolved against the real tree; a cited path that does not exist drops the
// path-existence ratio.
func TestCodingAgentBench_AccuracyMetrics(t *testing.T) {
	dir := t.TempDir()
	cabWriteFile(t, dir, "engine.py", "x=1")
	cabMkdir(t, dir, "react")
	cabWriteFile(t, dir, "react/docguard.py", "x=1")
	doc := "The engine is in engine.py; the guard in react/docguard.py and guards/old.py."
	m := codingAgentBenchAccuracy(doc, dir)
	if m.RefCount != 3 {
		t.Fatalf("ref_count = %d, want 3", m.RefCount)
	}
	if len(m.MissingPaths) != 1 || m.MissingPaths[0] != "guards/old.py" {
		t.Fatalf("missing_paths = %v, want [guards/old.py]", m.MissingPaths)
	}
	if want := round3(2.0 / 3.0); m.PathExistRatio != want {
		t.Fatalf("path_exist_ratio = %v, want %v", m.PathExistRatio, want)
	}
}

// TestCodingAgentBench_Anchors mirrors test_scoring.py:
// test_anchors_are_real_subsystems — anchors are the real subsystems (top-level
// dirs + prominent root modules), excluding __init__; full coverage when the
// doc names them all.
func TestCodingAgentBench_Anchors(t *testing.T) {
	dir := t.TempDir()
	cabWriteFile(t, dir, "engine.py", "x=1")
	cabWriteFile(t, dir, "__init__.py", "")
	cabMkdir(t, dir, "lobes")
	cabMkdir(t, dir, "clients")
	anchors := codingAgentBenchAnchors(dir)
	have := map[string]bool{}
	for _, a := range anchors {
		have[a] = true
	}
	if !have["lobes"] || !have["clients"] || !have["engine"] {
		t.Fatalf("anchors = %v, want lobes/clients/engine present", anchors)
	}
	if have["__init__"] {
		t.Fatalf("anchors = %v, must exclude __init__", anchors)
	}
	m := codingAgentBenchAccuracy("It has lobes and clients and an engine.", dir)
	if m.AnchorCoverage != 1.0 {
		t.Fatalf("anchor_coverage = %v, want 1.0", m.AnchorCoverage)
	}
}
