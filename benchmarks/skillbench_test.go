package benchmarks

import (
	"context"
	"strings"
	"testing"
)

// TestSkillBench_Ready is the cross-language parity gate for skillbench's
// deterministic floor. skillbench is a LIVE bench: parse/mapping are pure
// (deterministic over the fixtures + rendered prompt) but the verdict-driving
// groups — activation / follow / funnel — need a real PreactAgent against a
// real provider. Python's run.py refuses to compose a verdict without a provider
// token ("skillbench is a LIVE bench — set a provider token …", exit 2). The Go
// bench reproduces that: with no model every group mode is missing evidence, so
// the verdict is UNMEASURED (no evidence is never READY).
func TestSkillBench_Ready(t *testing.T) {
	v, err := RunSkillBench(context.Background(), "")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if v.Status != "UNMEASURED" {
		t.Fatalf("status = %q, want UNMEASURED (Python parity — live bench, no provider) — reasons: %v",
			v.Status, v.Reasons)
	}
	// Every group mode is missing without a provider — it gates as nil (it did
	// not run), never as a pass — so it can never trip the free-gate.
	for _, mode := range skillBenchModeNames() {
		if g, ok := v.Gates[mode+"_all_pass"]; ok && g != nil {
			t.Fatalf("%s_all_pass gate = %v, want absent/nil without a provider", mode, *g)
		}
	}
}

// TestSkillBench_CheckIDParity asserts the Go bench emits the SAME deterministic
// check-id surface as the Python scoring.py: the lint.rejects[…] rows (one per
// adversarial fixture), the parse.* rows (description/body per production skill,
// plus the large-file toc[…], the release checklist, and the course_advisor
// search), and the mapping.* rows (in_declared_stage / absent_off_stage +
// index_and_directive|inlined per skill, plus mapping.activation_tool_exposed).
// These are pure functions of the embedded fixtures + the rendered prompt, so
// the ids are asserted on the deterministic floor independent of the provider.
func TestSkillBench_CheckIDParity(t *testing.T) {
	prod, neg, err := skillBenchLoad()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// ── lint ────────────────────────────────────────────────────────────────
	lint := idsOf(skillBenchLintChecks(neg))
	wantLint := []string{
		"lint.rejects[_bad_checklist]",
		"lint.rejects[_bad_headings]",
		"lint.rejects[_bad_vague]",
	}
	assertIDs(t, "lint", lint, wantLint)
	// every adversarial fixture must be flagged (rejected) — ok=true
	for _, c := range skillBenchLintChecks(neg) {
		if !c.OK {
			t.Errorf("lint %s ok=false, want a bad fixture to be flagged: %s", c.ID, c.Detail)
		}
	}

	// ── parse ───────────────────────────────────────────────────────────────
	var parse []Check
	for _, s := range prod {
		parse = append(parse, skillBenchParseChecks(s)...)
	}
	parse = append(parse, skillBenchSearchSelfLocates(prod,
		"bảo lưu reservation", "course_advisor", "reference/regulations.md"))
	parseIDs := idsOf(parse)
	wantParse := []string{
		"parse.billing_policy.description", "parse.billing_policy.body",
		"parse.code_review.description", "parse.code_review.body",
		"parse.course_advisor.description", "parse.course_advisor.body",
		"parse.course_advisor.toc[reference/catalog.md]",
		"parse.incident_runbook.description", "parse.incident_runbook.body",
		"parse.release_checklist.description", "parse.release_checklist.body",
		"parse.release_checklist.checklist",
		"parse.sprint_tracker.description", "parse.sprint_tracker.body",
		"parse.ticket_triage.description", "parse.ticket_triage.body",
		"parse.course_advisor.search",
	}
	assertIDs(t, "parse", parseIDs, wantParse)
	// the deterministic parse floor over the production corpus is all-green
	for _, c := range parse {
		if !c.OK {
			t.Errorf("parse %s ok=false, want a production skill to parse cleanly: %s", c.ID, c.Detail)
		}
	}

	// ── mapping ─────────────────────────────────────────────────────────────
	exposed := map[string]struct{}{"ActivateSkill": {}}
	mapping := idsOf(skillBenchMappingChecks(prod, exposed))
	wantMapping := []string{
		"mapping.billing_policy.in_declared_stage", "mapping.billing_policy.absent_off_stage",
		"mapping.billing_policy.index_and_directive",
		"mapping.code_review.in_declared_stage", "mapping.code_review.absent_off_stage",
		"mapping.code_review.index_and_directive",
		"mapping.course_advisor.in_declared_stage", "mapping.course_advisor.absent_off_stage",
		"mapping.course_advisor.index_and_directive",
		"mapping.incident_runbook.in_declared_stage", "mapping.incident_runbook.absent_off_stage",
		"mapping.incident_runbook.inlined",
		"mapping.release_checklist.in_declared_stage", "mapping.release_checklist.absent_off_stage",
		"mapping.release_checklist.index_and_directive",
		"mapping.sprint_tracker.in_declared_stage", "mapping.sprint_tracker.absent_off_stage",
		"mapping.sprint_tracker.index_and_directive",
		"mapping.ticket_triage.in_declared_stage", "mapping.ticket_triage.absent_off_stage",
		"mapping.ticket_triage.index_and_directive",
		"mapping.activation_tool_exposed",
	}
	assertIDs(t, "mapping", mapping, wantMapping)
}

func idsOf(checks []Check) []string {
	out := make([]string, 0, len(checks))
	for _, c := range checks {
		out = append(out, c.ID)
	}
	return out
}

func assertIDs(t *testing.T, group string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: %d check ids, want %d\n got=%v\nwant=%v", group, len(got), len(want), got, want)
	}
	have := map[string]bool{}
	for _, id := range got {
		have[id] = true
	}
	for _, id := range want {
		if !have[id] {
			t.Errorf("%s: missing check id %q", group, id)
		}
	}
	wantSet := map[string]bool{}
	for _, id := range want {
		wantSet[id] = true
	}
	for _, id := range got {
		if !wantSet[id] {
			t.Errorf("%s: unexpected check id %q", group, id)
		}
	}
}

// TestSkillBench_LintFlagsEachDefect asserts the adversarial fixtures are caught
// by the SPECIFIC deterministic gate each one targets (parity with the Python
// negative_defects union): _bad_vague by the description lint, _bad_headings by
// the ToC-navigability gate, _bad_checklist by the materialized-checklist gate.
func TestSkillBench_LintFlagsEachDefect(t *testing.T) {
	_, neg, err := skillBenchLoad()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	byID := map[string]string{}
	for _, s := range neg {
		byID[s.ID] = strings.Join(skillBenchNegativeDefects(s), "; ")
	}
	if d := byID["_bad_vague"]; !strings.Contains(d, "vague") && !strings.Contains(d, "WHEN") {
		t.Errorf("_bad_vague defects = %q, want a description defect", d)
	}
	if d := byID["_bad_headings"]; !strings.Contains(d, "navigable") {
		t.Errorf("_bad_headings defects = %q, want a non-navigable large-file defect", d)
	}
	if d := byID["_bad_checklist"]; !strings.Contains(d, "checklist") {
		t.Errorf("_bad_checklist defects = %q, want a degenerate-checklist defect", d)
	}
}
