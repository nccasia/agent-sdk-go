// Ported from agent_sdk/tests/test_skill_loader.py — SKILL.md folder → SkillPack.
package skills

import (
	"os"
	"path/filepath"
	"testing"
)

const skillMDFixture = `---
name: Code review
description: Review a diff for correctness and style.
slug: code_review
injection: on_demand
stages:
  - synthesize
  - research
required_tools:
  - kb.retrieve
checklist:
  - key: scope
    title: Confirm scope
context_vars:
  - key: notes
    type: notes
    title: Review notes
---
# Code review

Read the diff, then comment on correctness first.
`

func writeSkill(t *testing.T, dir, md, ref string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	if ref != "" {
		if err := os.WriteFile(filepath.Join(dir, "reference.md"), []byte(ref), 0o644); err != nil {
			t.Fatalf("write reference.md: %v", err)
		}
	}
}

func TestParseSkillMDSplitsFrontmatterAndBody(t *testing.T) {
	front, body, err := ParseSkillMD(skillMDFixture)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if front["name"] != "Code review" {
		t.Errorf("name: got %v", front["name"])
	}
	if front["slug"] != "code_review" {
		t.Errorf("slug: got %v", front["slug"])
	}
	if len(body) == 0 || !contains(body, "# Code review") {
		t.Errorf("body missing heading: %q", body)
	}
}

func TestParseSkillMDRequiresFrontmatter(t *testing.T) {
	_, _, err := ParseSkillMD("no frontmatter here")
	if !IsSkillLoadError(err) {
		t.Fatalf("expected SkillLoadError, got %v", err)
	}
}

func TestLoadSkillPackReadsFolder(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, skillMDFixture, "# Reference\nDetail here.")

	pack, err := LoadSkillPack(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if pack.ID != "code_review" {
		t.Errorf("id: got %q, want code_review", pack.ID)
	}
	if pack.Name != "Code review" {
		t.Errorf("name: got %q", pack.Name)
	}
	if pack.Injection != "on_demand" {
		t.Errorf("injection: got %q", pack.Injection)
	}
	if !equalStringSlices(pack.Stages, []string{"synthesize", "research"}) {
		t.Errorf("stages: got %v", pack.Stages)
	}
	if !equalStringSlices(pack.RequiredTools, []string{"kb.retrieve"}) {
		t.Errorf("required_tools: got %v", pack.RequiredTools)
	}
	wantChecklist := []map[string]any{{"key": "scope", "title": "Confirm scope"}}
	if len(pack.Checklist) != 1 || pack.Checklist[0]["key"] != "scope" {
		t.Errorf("checklist: got %v", pack.Checklist)
	}
	if len(pack.ContextVars) != 1 || pack.ContextVars[0]["key"] != "notes" {
		t.Errorf("context_vars: got %v", pack.ContextVars)
	}
	if pack.Files["reference.md"] != "# Reference\nDetail here." {
		t.Errorf("reference file: got %q", pack.Files["reference.md"])
	}
	if pack.SourceDir != dir {
		t.Errorf("source_dir: got %q, want %q", pack.SourceDir, dir)
	}
	// AllContextVars should expose the legacy checklist uniformly.
	cv := pack.AllContextVars()
	found := false
	for _, v := range cv {
		if v["type"] == "checklist" {
			found = true
		}
	}
	if !found {
		t.Errorf("AllContextVars did not surface legacy checklist: %v", cv)
	}
	_ = wantChecklist
}

func TestLoadSkillPackRequiresNameAndDescription(t *testing.T) {
	dir := t.TempDir()
	md := "---\nslug: broken\n---\nbody"
	writeSkill(t, dir, md, "")
	_, err := LoadSkillPack(dir)
	if !IsSkillLoadError(err) {
		t.Fatalf("expected SkillLoadError, got %v", err)
	}
}

func TestLoadSkillPackMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadSkillPack(filepath.Join(dir, "nope"))
	if !IsSkillLoadError(err) {
		t.Fatalf("expected SkillLoadError, got %v", err)
	}
}

func TestLoadSkillPacksScansImmediateSubdirs(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a")
	writeSkill(t, a, skillMDFixture, "")
	b := filepath.Join(root, "b")
	if err := os.MkdirAll(b, 0o755); err != nil {
		t.Fatalf("mkdir b: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "loose.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatalf("write loose: %v", err)
	}

	packs, err := LoadSkillPacks(root)
	if err != nil {
		t.Fatalf("load_skill_packs: %v", err)
	}
	if len(packs) != 1 || packs[0].ID != "code_review" {
		t.Errorf("packs: got %v, want [code_review]", packs)
	}
}

func TestLoadSkillPacksMissingRootIsEmpty(t *testing.T) {
	dir := t.TempDir()
	packs, err := LoadSkillPacks(filepath.Join(dir, "nope"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if packs != nil {
		t.Errorf("expected nil packs, got %v", packs)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
