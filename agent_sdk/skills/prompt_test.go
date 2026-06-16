// Ported (in spirit) from the prompt-block assertions in test_skill_activation.py
// + the build_skill_prompt_block coverage in test_extra_coverage.py.
package skills

import (
	"strings"
	"testing"
)

func pack(id, name, desc, disclosure, body string, stages ...string) *SkillPack {
	if len(stages) == 0 {
		stages = []string{"synthesize"}
	}
	return &SkillPack{
		ID:           id,
		Name:         name,
		Description:  desc,
		Injection:    disclosure,
		Instructions: body,
		Stages:       stages,
	}
}

func TestBuildPromptBlockStaticInlineEagerAndIndexOnDemand(t *testing.T) {
	reg := NewSkillRegistry([]*SkillPack{
		pack("eager_a", "Eager A", "do A", "eager", "EAGER A BODY"),
		pack("ond_b", "OnD B", "lookup B", "on_demand", "OD B BODY"),
	})
	policy := map[string]any{
		"capabilities": map[string]any{"skills": []any{"eager_a", "ond_b"}},
	}
	out := BuildPromptBlock(reg, policy, "synthesize", PromptOptions{Query: "anything"})
	if !strings.Contains(out, "EAGER A BODY") {
		t.Errorf("eager body not inlined: %q", out)
	}
	if !strings.Contains(out, "Available skills:") {
		t.Errorf("on_demand index missing: %q", out)
	}
	if !strings.Contains(out, "OnD B: lookup B") {
		t.Errorf("index entry missing: %q", out)
	}
	if !strings.Contains(out, "ActivateSkill") {
		t.Errorf("directive missing: %q", out)
	}
}

func TestBuildPromptBlockSuppressesSkillsInUse(t *testing.T) {
	reg := NewSkillRegistry([]*SkillPack{
		pack("active", "Active", "an active skill", "on_demand", "OD"),
	})
	policy := map[string]any{
		"capabilities": map[string]any{"skills": []any{"active"}},
	}
	out := BuildPromptBlock(reg, policy, "synthesize", PromptOptions{
		SkillsInUse: []string{"active"},
	})
	if strings.Contains(out, "Available skills:") {
		t.Errorf("active skill should be suppressed from the index: %q", out)
	}
}

func TestBuildPromptBlockActiveSlugsScopes(t *testing.T) {
	reg := NewSkillRegistry([]*SkillPack{
		pack("a", "A", "alpha", "eager", "A BODY"),
		pack("b", "B", "beta", "eager", "B BODY"),
	})
	policy := map[string]any{
		"capabilities": map[string]any{"skills": []any{"a", "b"}},
	}
	out := BuildPromptBlock(reg, policy, "synthesize", PromptOptions{}.WithActiveSlugs([]string{"a"}))
	if !strings.Contains(out, "A BODY") {
		t.Errorf("scoped skill A missing: %q", out)
	}
	if strings.Contains(out, "B BODY") {
		t.Errorf("non-active skill B leaked: %q", out)
	}
}

func TestBuildPromptBlockAdaptiveKeepsMinimumAndReportsRanking(t *testing.T) {
	reg := NewSkillRegistry([]*SkillPack{
		pack("a", "A", "alpha", "on_demand", "A"),
		pack("b", "B", "beta", "on_demand", "B"),
		pack("c", "C", "gamma", "on_demand", "C"),
		pack("d", "D", "delta", "on_demand", "D"),
		pack("e", "E", "epsilon", "on_demand", "E"),
	})
	policy := map[string]any{
		"capabilities":   map[string]any{"skills": []any{"a", "b", "c", "d", "e"}},
		"skill_strategy": "adaptive",
		// skill_min_activation left unset ⇒ default 0.2 in floatOr;
		// rankOnDemand still keeps the top-3 always (skillMinKeep).
	}
	var ranking []RankingRow
	out := BuildPromptBlock(reg, policy, "synthesize", PromptOptions{
		Query:      "alpha",
		EmbedOne:   func(s string) []float64 { return nil },
		RankingOut: &ranking,
	})
	if !strings.Contains(out, "Available skills:") {
		t.Errorf("index missing: %q", out)
	}
	if len(ranking) != 5 {
		t.Errorf("ranking rows: got %d, want 5 (one per on_demand entry)", len(ranking))
	}
	// skillMinKeep guarantees at least 3 entries are kept.
	kept := 0
	for _, row := range ranking {
		if row.Kept {
			kept++
		}
	}
	if kept < 3 {
		t.Errorf("expected at least %d kept, got %d", 3, kept)
	}
}

func TestBuildPromptBlockEmptyRegistryProducesEmptyString(t *testing.T) {
	reg := NewSkillRegistry(nil)
	policy := map[string]any{
		"capabilities": map[string]any{"skills": []any{"absent"}},
	}
	out := BuildPromptBlock(reg, policy, "synthesize", PromptOptions{})
	if out != "" {
		t.Errorf("expected empty string, got %q", out)
	}
}

func TestResolveSkillInstructionsBuiltinOnly(t *testing.T) {
	out := ResolveSkillInstructions(map[string]any{
		"capabilities": map[string]any{"skills": []any{"kb_lookup"}},
	}, "synthesize")
	// KBLookupSkill is an eager builtin; its full instructions are inlined.
	if !strings.Contains(out, "knowledge-graph lookup with citations") {
		t.Errorf("builtin KB skill body not surfaced: %q", out)
	}
}
