package benchmarks

import "testing"

// TestVerdictSummaryCountsGates mirrors improve.verdict_summary: gates_pass counts
// true gates, gates_total counts non-nil gates.
func TestVerdictSummaryCountsGates(t *testing.T) {
	tru, fls := true, false
	v := Verdict{Status: "NOT_READY", Gates: map[string]*bool{
		"a_all_pass": &tru, "b_all_pass": &fls, "c_all_pass": nil}}
	s := VerdictSummary(v)
	if s.GatesPass != 1 {
		t.Fatalf("gates_pass = %d, want 1", s.GatesPass)
	}
	if s.GatesTotal != 2 {
		t.Fatalf("gates_total = %d, want 2", s.GatesTotal)
	}
}

// TestDeltaGateRatchetsUpOnStatus mirrors improve.delta_gate: a status improvement
// keeps the wave.
func TestDeltaGateRatchetsUpOnStatus(t *testing.T) {
	before := &Summary{Status: "NOT_READY", GatesPass: 1, GatesTotal: 2}
	after := Summary{Status: "READY", GatesPass: 2, GatesTotal: 2}
	d := DeltaGate(before, after)
	if !d.Kept {
		t.Fatalf("kept = false, want true (status ratchet up)")
	}
}

// TestDeltaGateHoldsOnNoImprovement mirrors: same status, same gates ⇒ not kept.
func TestDeltaGateHoldsOnNoImprovement(t *testing.T) {
	before := &Summary{Status: "READY", GatesPass: 2, GatesTotal: 2}
	after := Summary{Status: "READY", GatesPass: 2, GatesTotal: 2}
	d := DeltaGate(before, after)
	if d.Kept {
		t.Fatalf("kept = true, want false (no improvement holds baseline)")
	}
}

// TestDeltaGateRevertsOnStatusRegression mirrors: a status drop is never kept.
func TestDeltaGateRevertsOnStatusRegression(t *testing.T) {
	before := &Summary{Status: "READY", GatesPass: 2, GatesTotal: 2}
	after := Summary{Status: "NOT_READY", GatesPass: 1, GatesTotal: 2}
	d := DeltaGate(before, after)
	if d.Kept {
		t.Fatalf("kept = true, want false (status regressed)")
	}
}

// TestDeltaGateRevertsOnFewerGates mirrors: same status, fewer passing gates ⇒ revert.
func TestDeltaGateRevertsOnFewerGates(t *testing.T) {
	before := &Summary{Status: "READY", GatesPass: 3, GatesTotal: 3}
	after := Summary{Status: "READY", GatesPass: 2, GatesTotal: 3}
	d := DeltaGate(before, after)
	if d.Kept {
		t.Fatalf("kept = true, want false (fewer gates pass)")
	}
}

// TestDeltaGateNilBeforeRatchets mirrors: no prior baseline + any measured-up
// after keeps.
func TestDeltaGateNilBeforeRatchets(t *testing.T) {
	after := Summary{Status: "READY", GatesPass: 2, GatesTotal: 2}
	d := DeltaGate(nil, after)
	if !d.Kept {
		t.Fatalf("kept = false, want true (first wave ratchets from UNMEASURED)")
	}
}
