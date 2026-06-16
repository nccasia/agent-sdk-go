package benchmarks

import (
	"context"
	"testing"
)

// TestPromptBenchReady is the cross-language parity gate for promptbench's
// deterministic free floor (structure + quality). The Python source of truth
// (benchmarks/promptbench/verdicts/promptbench.md) ships READY — 20/20 checks,
// quality_avg 1.0. The Go port reproduces that verdict: every probed stage is
// well-layered and every authored prompt constant is clean.
func TestPromptBenchReady(t *testing.T) {
	v, err := RunPromptBench(context.Background(), "")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if v.Status != "READY" {
		t.Fatalf("status = %q, want READY (Python parity) — reasons: %v", v.Status, v.Reasons)
	}
	if got, ok := v.Metrics["quality.quality_avg"]; !ok || got != 1.0 {
		t.Errorf("quality.quality_avg = %v, want 1.0 (every authored prompt clean)", got)
	}
}

// TestPromptBenchCheckIDParity asserts the structure tier emits the exact eight
// check-ids the Python run.py run_structure produces, and that the quality tier
// emits a quality.<name> check per collected authored prompt. The structure
// ids are a fixed, byte-for-byte contract; the quality ids are data-driven over
// the shipped prompt constants (the Go surface has not ported cite.SYSTEM /
// plan_prompt, so those two rows are absent — the others match the baseline).
func TestPromptBenchCheckIDParity(t *testing.T) {
	stages, err := pbProbeStages(context.Background())
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	structure := pbRunStructure(stages)
	wantStructure := []string{
		"identity.once", "identity.single_persona", "dedup.source_unique",
		"ordering.identity_first", "ordering.env_last", "ordering.volatile_tail",
		"convo.not_duplicated", "coverage.valid_offsets",
	}
	gotStructure := checkIDs(structure)
	if !equal(gotStructure, wantStructure) {
		t.Errorf("structure check-ids = %v, want %v", gotStructure, wantStructure)
	}
	if !structure.AllPass {
		t.Errorf("structure: not all_pass — checks: %v", structure.Checks)
	}

	quality := pbRunQuality(pbAuthoredPrompts())
	wantQuality := []string{
		"quality.synthesize.SYSTEM", "quality.synthesize.SIMPLE", "quality.respond.SYSTEM",
		"quality.filter.SYSTEM", "quality.format.SYSTEM", "quality.classify.SYSTEM",
		"quality.condense.SYSTEM", "quality.plan.SYSTEM", "quality.research.SYSTEM",
		"quality.memory_directive",
	}
	gotQuality := checkIDs(quality)
	if !equal(gotQuality, wantQuality) {
		t.Errorf("quality check-ids = %v, want %v", gotQuality, wantQuality)
	}
	if !quality.AllPass {
		t.Errorf("quality: not all_pass — checks: %v", quality.Checks)
	}
}

func checkIDs(p *ModePayload) []string {
	out := make([]string, len(p.Checks))
	for i, c := range p.Checks {
		out[i] = c.ID
	}
	return out
}
