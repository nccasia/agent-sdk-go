package benchmarks

import "testing"

// TestComposeVerdictReadyWhenAllPass mirrors verdict.py: every mode present and
// all_pass ⇒ READY, with a per-mode <mode>_all_pass gate that is true.
func TestComposeVerdictReadyWhenAllPass(t *testing.T) {
	payloads := map[string]*ModePayload{
		"a": {Checks: []Check{{ID: "a.x", OK: true}}, AllPass: true},
		"b": {Checks: []Check{{ID: "b.y", OK: true}}, AllPass: true},
	}
	v := ComposeVerdict(payloads, nil)
	if v.Status != "READY" {
		t.Fatalf("status = %q, want READY", v.Status)
	}
	if g, ok := v.Gates["a_all_pass"]; !ok || g == nil || *g != true {
		t.Fatalf("gate a_all_pass = %v, want true", g)
	}
}

// TestComposeVerdictNotReadyOnFailingMode mirrors: a present mode that fails ⇒
// NOT_READY and a false gate with a reason listing the failing check ids.
func TestComposeVerdictNotReadyOnFailingMode(t *testing.T) {
	payloads := map[string]*ModePayload{
		"a": {Checks: []Check{{ID: "a.x", OK: false}, {ID: "a.y", OK: true}}, AllPass: false},
	}
	v := ComposeVerdict(payloads, nil)
	if v.Status != "NOT_READY" {
		t.Fatalf("status = %q, want NOT_READY", v.Status)
	}
	if g := v.Gates["a_all_pass"]; g == nil || *g != false {
		t.Fatalf("gate a_all_pass = %v, want false", g)
	}
	if len(v.Reasons) == 0 {
		t.Fatalf("want a reason for the failing mode")
	}
}

// TestComposeVerdictUnmeasuredOnMissingMode mirrors: a nil payload (mode did not
// run) ⇒ UNMEASURED and a leading "missing evidence" reason.
func TestComposeVerdictUnmeasuredOnMissingMode(t *testing.T) {
	payloads := map[string]*ModePayload{
		"a": {Checks: []Check{{ID: "a.x", OK: true}}, AllPass: true},
		"b": nil,
	}
	v := ComposeVerdict(payloads, nil)
	if v.Status != "UNMEASURED" {
		t.Fatalf("status = %q, want UNMEASURED", v.Status)
	}
	if len(v.Reasons) == 0 || v.Reasons[0] == "" {
		t.Fatalf("want a leading missing-evidence reason")
	}
}

// TestComposeVerdictSkippedModeGatesNil mirrors verdict.py: a payload flagged
// skipped ran-but-had-nothing-to-measure ⇒ its gate is nil (not gating).
func TestComposeVerdictSkippedModeGatesNil(t *testing.T) {
	payloads := map[string]*ModePayload{
		"a": {Checks: []Check{{ID: "a.x", OK: true}}, AllPass: true},
		"b": {Skipped: true},
	}
	v := ComposeVerdict(payloads, nil)
	if v.Status != "READY" {
		t.Fatalf("status = %q, want READY", v.Status)
	}
	if g, ok := v.Gates["b_all_pass"]; !ok || g != nil {
		t.Fatalf("gate b_all_pass = %v, want present-but-nil", g)
	}
}

// TestComposeVerdictRecordsMetrics mirrors the record={} surfacing of headline
// metrics into the verdict's metrics block (mode.name keys).
func TestComposeVerdictRecordsMetrics(t *testing.T) {
	payloads := map[string]*ModePayload{
		"a": {Checks: []Check{{ID: "a.x", OK: true}}, AllPass: true,
			Metrics: map[string]any{"score": 0.9}},
	}
	v := ComposeVerdict(payloads, map[string][]string{"a": {"score"}})
	if got := v.Metrics["a.score"]; got != any(0.9) {
		t.Fatalf("metric a.score = %v, want 0.9", got)
	}
}
