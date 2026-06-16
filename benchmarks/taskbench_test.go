package benchmarks

import (
	"context"
	"strings"
	"testing"
)

// TestTaskBench_Ready is the cross-language parity gate for taskbench's
// deterministic floor. taskbench is a LIVE bench: every capability is measured
// by driving the REAL task agent (TaskPlugin + the bench's SqlitePlugin) over a
// seeded SQLite DB against a real provider, then grading the final answer vs
// reference SQL. Python's run.py refuses to run without --live and a provider
// token (exit 2). With no model the capability matrix is empty — every mode is
// missing evidence — so the composed verdict is UNMEASURED (no evidence is
// never READY). The Go bench reproduces that.
func TestTaskBench_Ready(t *testing.T) {
	v, err := RunTaskBench(context.Background(), "")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if v.Status != "UNMEASURED" {
		t.Fatalf("status = %q, want UNMEASURED (Python parity — live bench, no provider) — reasons: %v",
			v.Status, v.Reasons)
	}
	// No mode ran, so no *_all_pass gate may be a pass.
	for k, g := range v.Gates {
		if g != nil && *g {
			t.Fatalf("gate %q = true without a provider, want no passing gates", k)
		}
	}
}

// TestTaskBench_CheckIDParity asserts the Go bench emits the SAME gating
// check-id surface as the Python run.py score(): per task the three gating
// checks answered / answer_correct / bounded (the diag checks routed_task /
// planned are shown, not gating). Mirrors the gating ids in
// benchmarks/taskbench/run.py:score(). The ids are a static,
// provider-independent property of the bench, so they are asserted on the
// deterministic-floor surface.
func TestTaskBench_CheckIDParity(t *testing.T) {
	ids := taskBenchGatingCheckIDs()
	want := []string{"answered", "answer_correct", "bounded"}
	if len(ids) != len(want) {
		t.Fatalf("gating check ids = %d, want %d (%v)", len(ids), len(want), ids)
	}
	have := map[string]bool{}
	for _, id := range ids {
		have[id] = true
	}
	for _, id := range want {
		if !have[id] {
			t.Errorf("missing gating check id %q", id)
		}
	}
}

// TestTaskBench_CapabilityModes asserts the capability matrix mode labels mirror
// run.py:build_matrix — one cap{N}_{name} per dataset capability, plus the
// cross-cutting cap2_drive_to_completion. cap10_error_recovery is UNMEASURED
// without a provider (no db.query errored), so it is not a gating mode label.
func TestTaskBench_CapabilityModes(t *testing.T) {
	modes := taskBenchModeNames()
	for _, want := range []string{
		"cap1_decompose", "cap4_tool_orchestration", "cap3_state_carry",
		"cap2_drive_to_completion", "cap6_dependency_order", "cap8_branching",
		"cap11_long_horizon", "cap5_predefined_fastpath",
	} {
		found := false
		for _, m := range modes {
			if m == want {
				found = true
			}
		}
		if !found {
			t.Errorf("missing capability mode %q (have %v)", want, modes)
		}
	}
	// cap10_error_recovery is UNMEASURED without a provider — not a gating mode.
	for _, m := range modes {
		if m == "cap10_error_recovery" {
			t.Errorf("cap10_error_recovery must be UNMEASURED, not a gating mode")
		}
	}
}

// TestTaskBench_SeedDeterministic mirrors test_taskbench.py:
// test_seed_is_deterministic — the seeded SQLite DB is reproducible run-to-run,
// so every task's ground truth is stable.
func TestTaskBench_SeedDeterministic(t *testing.T) {
	a, err := taskBenchBuildDB()
	if err != nil {
		t.Fatalf("build a: %v", err)
	}
	defer a.Close()
	b, err := taskBenchBuildDB()
	if err != nil {
		t.Fatalf("build b: %v", err)
	}
	defer b.Close()

	q := "SELECT COUNT(*), SUM(quantity*unit_price) FROM order_items"
	var ca, cb int
	var sa, sb float64
	if err := a.QueryRow(q).Scan(&ca, &sa); err != nil {
		t.Fatalf("scan a: %v", err)
	}
	if err := b.QueryRow(q).Scan(&cb, &sb); err != nil {
		t.Fatalf("scan b: %v", err)
	}
	if ca == 0 || ca != cb || sa != sb {
		t.Fatalf("seed not deterministic: a=(%d,%v) b=(%d,%v)", ca, sa, cb, sb)
	}
}

// TestTaskBench_EveryTaskGroundTruth mirrors
// test_every_task_reference_sql_yields_ground_truth — each task's answer_sql
// returns at least one non-null fact against the same seeded DB.
func TestTaskBench_EveryTaskGroundTruth(t *testing.T) {
	db, err := taskBenchBuildDB()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	defer db.Close()
	for _, task := range taskBenchTasks() {
		facts, err := taskBenchGroundTruth(db, task.AnswerSQL)
		if err != nil {
			t.Fatalf("%s: ground_truth: %v", task.ID, err)
		}
		if len(facts) == 0 {
			t.Fatalf("%s: answer_sql returned no rows", task.ID)
		}
		any := false
		for _, f := range facts {
			if f != nil {
				any = true
			}
		}
		if !any {
			t.Fatalf("%s: all ground-truth facts are NULL", task.ID)
		}
	}
}

// TestTaskBench_DBQueryToolRunsAndErrors mirrors
// test_db_query_tool_runs_and_errors_cleanly — db.query runs read-only SELECTs,
// logs/returns clean errors on a bad query, and refuses writes.
func TestTaskBench_DBQueryToolRunsAndErrors(t *testing.T) {
	db, err := taskBenchBuildDB()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	defer db.Close()
	store := &taskBenchStore{db: db}
	ctx := context.Background()

	ok, err := store.query(ctx, "SELECT COUNT(*) FROM customers")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if !strings.Contains(ok, "60") {
		t.Fatalf("count query = %q, want contains 60", ok)
	}
	bad, err := store.query(ctx, "SELECT nope FROM customers")
	if err != nil {
		t.Fatalf("bad query unexpected go error: %v", err)
	}
	if !strings.HasPrefix(bad, "Error:") {
		t.Fatalf("bad query = %q, want Error: prefix", bad)
	}
	if len(store.queries) != 2 {
		t.Fatalf("queries logged = %d, want 2", len(store.queries))
	}
	if _, hasErr := store.queries[1]["error"]; !hasErr {
		t.Fatalf("second query log missing error key: %v", store.queries[1])
	}
	wr, err := store.query(ctx, "DELETE FROM customers")
	if err != nil {
		t.Fatalf("write query: %v", err)
	}
	if !strings.Contains(wr, "read-only") {
		t.Fatalf("write query = %q, want read-only refusal", wr)
	}
}

// TestTaskBench_Grader mirrors test_grader_matches_facts_with_tolerance — string
// facts by case-insensitive containment, numbers by 3%/±1 tolerance.
func TestTaskBench_Grader(t *testing.T) {
	ok, _ := taskBenchGradeAnswer(
		"The top products are Laptop Pro 14, Ultrawide Monitor and 27in 4K Monitor.",
		[]any{"Laptop Pro 14", "Ultrawide Monitor", "27in 4K Monitor"})
	if !ok {
		t.Fatalf("string facts should grade correct")
	}
	if okN, _ := taskBenchGradeAnswer("Total revenue is about $123,450.", []any{123400.0}); !okN {
		t.Fatalf("number within 3%% should grade correct")
	}
	bad, detail := taskBenchGradeAnswer("It is Germany.", []any{"US", 99})
	if bad {
		t.Fatalf("wrong answer should grade incorrect")
	}
	anyFail := false
	for _, d := range detail {
		if !d.OK {
			anyFail = true
		}
	}
	if !anyFail {
		t.Fatalf("expected at least one failing fact in detail")
	}
}
