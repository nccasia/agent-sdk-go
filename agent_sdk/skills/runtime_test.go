// Ported from agent_sdk/tests/test_skill_activation.py — the on-demand
// ActivateSkill / skill.read / skill.search triple.
//
// The Python suite also exercises the full PreactAgent path (agent query →
// skill activation → session load). Those are rung-10 work and stay skipped
// here; the runtime contract is what this rung locks.
package skills

import (
	"context"
	"strings"
	"testing"
)

func TestActivateReturnsBodyAndFiles(t *testing.T) {
	sk := NewSkill("code_review",
		When("review code"),
		Instructions("SKILL: Code review\nQuote the bug and fix it."),
		WithFiles(map[string]string{"GUIDE.md": "## Deep checklist\nCheck the edges."}),
		WithStages("synthesize"),
	)
	reg := NewSkillRegistry([]*SkillPack{sk.ToPack()})
	rt := NewSkillToolRuntime(reg, []string{"code_review"})

	out, _ := rt.CallTool(context.Background(), ACTIVATE, map[string]any{"slug": "code_review"}, nil, nil)
	if !strings.Contains(out, "Quote the bug") {
		t.Errorf("body missing: %q", out)
	}
	if !strings.Contains(out, "GUIDE.md") {
		t.Errorf("reference files not advertised: %q", out)
	}
	if len(rt.Activated) != 1 || rt.Activated[0] != "code_review" {
		t.Errorf("Activated: got %v, want [code_review]", rt.Activated)
	}
}

func TestActivateUnknownSlugErrors(t *testing.T) {
	sk := NewSkill("a", When("x"), Instructions("do a"))
	reg := NewSkillRegistry([]*SkillPack{sk.ToPack()})
	rt := NewSkillToolRuntime(reg, []string{"a"})

	out, _ := rt.CallTool(context.Background(), ACTIVATE, map[string]any{"slug": "nope"}, nil, nil)
	if !strings.HasPrefix(out, "Error") {
		t.Errorf("expected Error prefix, got %q", out)
	}
}

func TestReadSectionAndToc(t *testing.T) {
	big := "## One\n" + strings.Repeat("alpha ", 50) + "\n## Two\n" + strings.Repeat("beta ", 50)
	sk := NewSkill("doc",
		When("docs"),
		Instructions(big),
		WithFiles(map[string]string{"BIG.md": strings.Repeat("x ", 4000)}),
	)
	reg := NewSkillRegistry([]*SkillPack{sk.ToPack()})
	rt := NewSkillToolRuntime(reg, []string{"doc"})

	// Bare read of a large file returns its ToC.
	toc, _ := rt.CallTool(context.Background(), READ,
		map[string]any{"slug": "doc", "file": "BIG.md"}, nil, nil)
	if !strings.Contains(strings.ToLower(toc), "section") {
		t.Errorf("ToC missing: %q", toc)
	}

	// Section read returns just that section.
	sec, _ := rt.CallTool(context.Background(), READ,
		map[string]any{"slug": "doc", "section": "one"}, nil, nil)
	if !strings.Contains(sec, "alpha") || strings.Contains(sec, "beta") {
		t.Errorf("section content wrong: %q", sec)
	}

	// Unknown file → clean error.
	errOut, _ := rt.CallTool(context.Background(), READ,
		map[string]any{"slug": "doc", "file": "missing.md"}, nil, nil)
	if !strings.HasPrefix(errOut, "Error") {
		t.Errorf("expected Error for missing file, got %q", errOut)
	}
}

func TestSearchLocatesSection(t *testing.T) {
	sk := NewSkill("advisor",
		When("advise"),
		Instructions("SKILL: Advisor"),
		WithFiles(map[string]string{"rules.md": "## Reservation\nReserve up to two semesters."}),
	)
	reg := NewSkillRegistry([]*SkillPack{sk.ToPack()})
	rt := NewSkillToolRuntime(reg, []string{"advisor"})

	out, _ := rt.CallTool(context.Background(), SEARCH,
		map[string]any{"query": "reservation semesters"}, nil, nil)
	if !strings.Contains(out, "rules.md") || !strings.Contains(out, "Reservation") {
		t.Errorf("search result wrong: %q", out)
	}
}

func TestReadResolvesChunkID(t *testing.T) {
	sk := NewSkill("adv",
		When("advise"),
		Instructions("SKILL"),
		WithFiles(map[string]string{"ref.md": "## Reservation\nReserve up to two semesters."}),
		WithStages("synthesize"),
	)
	reg := NewSkillRegistry([]*SkillPack{sk.ToPack()})
	rt := NewSkillToolRuntime(reg, []string{"adv"})

	out, _ := rt.CallTool(context.Background(), READ,
		map[string]any{"slug": "adv", "chunk": "ref.md#reservation"}, nil, nil)
	if !strings.Contains(out, "two semesters") {
		t.Errorf("chunk read returned wrong content: %q", out)
	}
}

func TestGetToolSpecsListsThreeTools(t *testing.T) {
	sk := NewSkill("x", When("x"), Instructions("body"))
	reg := NewSkillRegistry([]*SkillPack{sk.ToPack()})
	rt := NewSkillToolRuntime(reg, []string{"x"})
	specs := rt.GetToolSpecs()
	if len(specs) != 3 {
		t.Errorf("spec count: got %d, want 3 (activate/read/search)", len(specs))
	}
	names := map[string]bool{}
	for _, s := range specs {
		names[s["name"].(string)] = true
	}
	if !names[ACTIVATE] || !names[READ] || !names[SEARCH] {
		t.Errorf("missing one of ACTIVATE/READ/SEARCH: %v", names)
	}
}

func TestGetToolSpecsEmptyWhenNoSlugs(t *testing.T) {
	reg := NewSkillRegistry(nil)
	rt := NewSkillToolRuntime(reg, nil)
	if specs := rt.GetToolSpecs(); specs != nil {
		t.Errorf("expected nil specs when no slugs, got %v", specs)
	}
}

func TestActivateCompilesLazilyThenCaches(t *testing.T) {
	bigA := NewSkill("adv_a",
		When("advise A"),
		Instructions("SKILL A\n"+strings.Repeat("step detail ", 120)),
		WithStages("synthesize"),
	)
	bigB := NewSkill("adv_b",
		When("advise B"),
		Instructions("SKILL B\n"+strings.Repeat("step detail ", 120)),
		WithStages("synthesize"),
	)
	reg := NewSkillRegistry([]*SkillPack{bigA.ToPack(), bigB.ToPack()})
	llm := &callRecorder{script: []string{
		"CORE surface. read [SKILL.md#intro] for detail.",
	}}
	rt := NewSkillToolRuntime(reg, []string{"adv_a", "adv_b"},
		WithLlm(llm.call),
		WithBudgetTokens(150),
		WithSurfaceMode("llm"),
	)

	out1, _ := rt.CallTool(context.Background(), ACTIVATE, map[string]any{"slug": "adv_a"}, nil, nil)
	if !strings.Contains(out1, "CORE surface") {
		t.Errorf("first activation: %q", out1)
	}
	if len(llm.calls) != 1 {
		t.Errorf("llm calls after first activate: got %d, want 1", len(llm.calls))
	}
	_, _ = rt.CallTool(context.Background(), ACTIVATE, map[string]any{"slug": "adv_a"}, nil, nil)
	if len(llm.calls) != 1 {
		t.Errorf("cache hit failed: llm called %d times, want 1", len(llm.calls))
	}
	// adv_b never activated → never compiled.
	if llm.scriptI != 1 {
		t.Errorf("script advance: %d, want 1 (only first call consumed)", llm.scriptI)
	}
}
