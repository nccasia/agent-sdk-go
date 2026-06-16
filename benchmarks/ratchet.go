package benchmarks

// The deterministic keep/revert ratchet — ported from benchmarks/_shared/improve.py.
// The decision is a pure function of (status rank, count of passing gates); never
// model-judged, so the workflow driver and skills reproduce it exactly.

var statusRank = map[string]int{"UNMEASURED": 0, "NOT_READY": 1, "READY": 2}

// Summary is the ratchet-shaped normalization of a verdict: status + the count
// of passing / measured gates. Mirrors improve.verdict_summary's output dict.
type Summary struct {
	Status     string `json:"status"`
	GatesPass  int    `json:"gates_pass"`
	GatesTotal int    `json:"gates_total"`
}

// VerdictSummary normalizes a Verdict to a Summary: gates_pass counts true gates,
// gates_total counts non-nil gates. Mirrors improve.verdict_summary.
func VerdictSummary(v Verdict) Summary {
	pass, total := 0, 0
	for _, g := range v.Gates {
		if g == nil {
			continue
		}
		total++
		if *g {
			pass++
		}
	}
	return Summary{Status: orStatus(v.Status), GatesPass: pass, GatesTotal: total}
}

func orStatus(s string) string {
	if s == "" {
		return "UNMEASURED"
	}
	return s
}

// Delta is the ratchet decision: kept + a human reason + the before/after
// summaries it compared. Mirrors improve.delta_gate's return dict.
type Delta struct {
	Kept   bool    `json:"kept"`
	Reason string  `json:"reason"`
	Before Summary `json:"before"`
	After  Summary `json:"after"`
}

// DeltaGate keeps the wave iff it ratchets UP and nothing regressed: a status
// improvement OR more passing gates, never a status drop or fewer gates at the
// same status. A nil before is treated as UNMEASURED/0. Mirrors improve.delta_gate.
func DeltaGate(before *Summary, after Summary) Delta {
	b := Summary{Status: "UNMEASURED"}
	if before != nil {
		b = *before
	}
	b.Status = orStatus(b.Status)
	after.Status = orStatus(after.Status)
	br, ar := statusRank[b.Status], statusRank[after.Status]
	if ar < br {
		return Delta{Kept: false, Reason: "status regressed " + b.Status + "→" + after.Status, Before: b, After: after}
	}
	if ar == br && after.GatesPass < b.GatesPass {
		return Delta{Kept: false, Reason: "fewer gates pass", Before: b, After: after}
	}
	improved := ar > br || after.GatesPass > b.GatesPass
	reason := "no-improvement (hold baseline)"
	if improved {
		reason = "ratchet up"
	}
	return Delta{Kept: improved, Reason: reason, Before: b, After: after}
}
