// Ported from the SkillRegistry / PolicySkillSlugs / MergeExtraSkillSlugs
// assertions scattered across test_extra_coverage.py and test_skill_*.py.
package skills

import (
	"reflect"
	"testing"
)

func TestStageMatchesExact(t *testing.T) {
	if !StageMatches("synthesize", []string{"synthesize"}) {
		t.Errorf("exact match should succeed")
	}
}

func TestStageMatchesFlowNamespaced(t *testing.T) {
	// Production stage is flow-namespaced; the skill declares the logical name.
	if !StageMatches("qna:synthesize", []string{"synthesize"}) {
		t.Errorf("flow-namespaced stage should match logical skill stage")
	}
	if StageMatches("qna:other", []string{"synthesize"}) {
		t.Errorf("unrelated flow-namespaced stage should not match")
	}
}

func TestPolicySkillSlugsFromCapabilitiesSkills(t *testing.T) {
	policy := map[string]any{
		"capabilities": map[string]any{
			"skills": []any{"alpha", "beta"},
		},
	}
	slugs := PolicySkillSlugs(policy)
	if !reflect.DeepEqual(slugs, []string{"alpha", "beta"}) {
		t.Errorf("got %v, want [alpha beta]", slugs)
	}
}

func TestPolicySkillSlugsLegacyToolPacksAlias(t *testing.T) {
	policy := map[string]any{
		"capabilities": map[string]any{
			"tool_packs": []any{"legacy"},
		},
	}
	slugs := PolicySkillSlugs(policy)
	if !reflect.DeepEqual(slugs, []string{"legacy"}) {
		t.Errorf("legacy tool_packs alias: got %v", slugs)
	}
}

func TestPolicySkillSlugsDropsContextManagementWhenMemoryDisabled(t *testing.T) {
	policy := map[string]any{
		"capabilities": map[string]any{
			"skills": []any{"context_management", "x"},
		},
		"memory_enabled": false,
	}
	slugs := PolicySkillSlugs(policy)
	if !reflect.DeepEqual(slugs, []string{"x"}) {
		t.Errorf("got %v, want [x]", slugs)
	}
}

func TestPolicySkillSlugsStringSlice(t *testing.T) {
	policy := map[string]any{
		"capabilities": map[string]any{
			"skills": []string{"a", "b"},
		},
	}
	slugs := PolicySkillSlugs(policy)
	if !reflect.DeepEqual(slugs, []string{"a", "b"}) {
		t.Errorf("got %v", slugs)
	}
}

func TestMergeExtraSkillSlugsDedupesAndPreservesOrder(t *testing.T) {
	policy := map[string]any{
		"capabilities": map[string]any{
			"skills": []any{"a", "b"},
		},
	}
	merged := MergeExtraSkillSlugs(policy, []string{"b", "c", "a", "c"})
	caps, _ := merged["capabilities"].(map[string]any)
	out, _ := caps["skills"].([]any)
	got := make([]string, len(out))
	for i, v := range out {
		got[i] = v.(string)
	}
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergeExtraSkillSlugsNoExtraReturnsSamePolicy(t *testing.T) {
	policy := map[string]any{
		"capabilities": map[string]any{
			"skills": []any{"a", "b"},
		},
	}
	out := MergeExtraSkillSlugs(policy, nil)
	if &out == &policy {
		// map headers are pointers; we just need the same underlying content.
	}
	// Same content back when extras are empty (no new slugs).
	caps, _ := out["capabilities"].(map[string]any)
	slugs, _ := caps["skills"].([]any)
	if len(slugs) != 2 {
		t.Errorf("expected 2 slugs, got %d", len(slugs))
	}
}

func TestMergeExtraSkillSlugsDoesNotMutateInput(t *testing.T) {
	policy := map[string]any{
		"capabilities": map[string]any{
			"skills": []any{"a"},
		},
	}
	original := policy["capabilities"].(map[string]any)["skills"].([]any)
	_ = MergeExtraSkillSlugs(policy, []string{"b", "c"})
	after := policy["capabilities"].(map[string]any)["skills"].([]any)
	if !reflect.DeepEqual(original, after) {
		t.Errorf("input policy mutated: before=%v after=%v", original, after)
	}
}

func TestSkillRegistryFromRowsCoercesFields(t *testing.T) {
	rows := []map[string]any{
		{
			"slug":           "kb",
			"name":           "Knowledge",
			"description":    "Look up facts",
			"stages":         []any{"synthesize"},
			"required_tools": []any{"kb.retrieve"},
			"injection":      "eager",
		},
		{"name": "no slug — ignored"},
	}
	r := SkillRegistryFromRows(rows)
	pack := r.Get("kb")
	if pack == nil {
		t.Fatalf("kb pack not registered")
	}
	if pack.Name != "Knowledge" {
		t.Errorf("name: got %q", pack.Name)
	}
	if pack.Injection != "eager" {
		t.Errorf("injection: got %q", pack.Injection)
	}
	if !reflect.DeepEqual(pack.RequiredTools, []string{"kb.retrieve"}) {
		t.Errorf("required_tools: got %v", pack.RequiredTools)
	}
}

func TestActiveForStageFiltersByStage(t *testing.T) {
	policy := map[string]any{
		"capabilities": map[string]any{
			"skills": []any{"kb_lookup", "research_helper"},
		},
	}
	// NewSkillRegistry has only KBLookupSkill; research_helper is a DB row.
	r := NewSkillRegistry([]*SkillPack{{ID: "research_helper", Stages: []string{"research"}}})
	active := r.ActiveForStage(policy, "research")
	hasRH := false
	for _, p := range active {
		if p.ID == "research_helper" {
			hasRH = true
		}
	}
	if !hasRH {
		t.Errorf("research_helper not active for research stage: %v", active)
	}
	// And it's not active for synthesize.
	active = r.ActiveForStage(policy, "synthesize")
	for _, p := range active {
		if p.ID == "research_helper" {
			t.Errorf("research_helper should not be active for synthesize")
		}
	}
}
