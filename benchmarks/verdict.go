// Package benchmarks ports the agent-sdk benchmark/optimize ecosystem: the
// verdict contract (READY / NOT_READY / UNMEASURED over a set of modes), the
// deterministic keep/revert ratchet, the free/live provider split, a bench
// registry, and the individual benches.
//
// A bench is a set of named Modes; a Mode runs a list of Checks (each one a
// Python run.py check id). ComposeVerdict folds the per-mode all_pass into a
// verdict whose Gates map mode → all_pass — one Python check is one Go Gate
// "mode.check" via the underlying Check rows. A missing mode (a nil payload)
// makes the verdict UNMEASURED: no evidence is never READY.
//
// Ported from benchmarks/_shared/{verdict,improve,provider}.py.
package benchmarks

// Check is one assertion: a Python run.py check id, its pass/fail, and a detail.
// Mirrors the {"id", "ok", "detail"} dicts the benches build.
type Check struct {
	ID     string `json:"id"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// ModePayload is one mode's result — its checks, the all_pass roll-up, a
// skipped flag (ran but nothing to measure), and headline metrics. Mirrors the
// _payload(...) dicts the run.py files build.
type ModePayload struct {
	Checks  []Check        `json:"checks"`
	AllPass bool           `json:"all_pass"`
	Skipped bool           `json:"skipped,omitempty"`
	Metrics map[string]any `json:"metrics,omitempty"`
}

// NewPayload builds a ModePayload from checks, computing all_pass (all OK and at
// least one check). Mirrors the shared _payload() helper.
func NewPayload(checks []Check, metrics map[string]any) *ModePayload {
	all := len(checks) > 0
	for _, c := range checks {
		if !c.OK {
			all = false
		}
	}
	if metrics == nil {
		metrics = map[string]any{}
	}
	return &ModePayload{Checks: checks, AllPass: all, Metrics: metrics}
}

// Pass counts the passing checks.
func (p *ModePayload) Pass() int {
	n := 0
	for _, c := range p.Checks {
		if c.OK {
			n++
		}
	}
	return n
}

// Verdict is the composed READY / NOT_READY / UNMEASURED outcome. Gates maps
// "<mode>_all_pass" → true/false/nil (nil = a skipped mode, not gating).
// Mirrors compose_verdict()'s {status, reasons, gates, metrics}.
type Verdict struct {
	Status  string           `json:"status"`
	Reasons []string         `json:"reasons"`
	Gates   map[string]*bool `json:"gates"`
	Metrics map[string]any   `json:"metrics"`
}

// ComposeVerdict folds {mode: payload} into a verdict (a nil payload = the mode
// did not run ⇒ UNMEASURED). record optionally maps mode → metric names to
// surface (without gating). Mirrors benchmarks/_shared/verdict.py:compose_verdict.
//
// Iteration order over the modes is deterministic (sorted mode names) so the
// gates/reasons/metrics are reproducible.
func ComposeVerdict(payloads map[string]*ModePayload, record map[string][]string) Verdict {
	reasons := []string{}
	missing := []string{}
	gates := map[string]*bool{}
	metrics := map[string]any{}

	for _, mode := range sortedKeys(payloads) {
		payload := payloads[mode]
		if payload == nil {
			missing = append(missing, mode)
			continue
		}
		if payload.Skipped {
			gates[mode+"_all_pass"] = nil
			continue
		}
		ok := payload.AllPass
		okCopy := ok
		gates[mode+"_all_pass"] = &okCopy
		if !ok {
			failed := []string{}
			for _, c := range payload.Checks {
				if !c.OK {
					failed = append(failed, c.ID)
				}
			}
			shown := failed
			if len(shown) > 5 {
				shown = shown[:5]
			}
			reasons = append(reasons, modeFailReason(mode, len(failed), shown))
		}
		for _, name := range record[mode] {
			if val, ok := payload.Metrics[name]; ok && val != nil {
				metrics[mode+"."+name] = val
			}
		}
	}

	status := "READY"
	if len(missing) > 0 {
		status = "UNMEASURED"
	} else if len(reasons) > 0 {
		status = "NOT_READY"
	}
	if len(missing) > 0 {
		reasons = append([]string{"missing evidence: " + joinComma(missing)}, reasons...)
	}
	return Verdict{Status: status, Reasons: reasons, Gates: gates, Metrics: metrics}
}
