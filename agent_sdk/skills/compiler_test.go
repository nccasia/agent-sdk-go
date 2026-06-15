// Ported from agent_sdk/tests/test_skill_compiler.py — content-hash, chunk
// splitting, and CompileSkill (LLM path + deterministic fallback + sidecar).
package skills

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// callRecorder is a stub LlmFunc that records every call and returns scripted
// surface text. Mirrors the role of FakeClient in the Python compiler tests.
type callRecorder struct {
	calls   []LlmRequest
	script  []string
	scriptI int
	fail    bool
}

func (c *callRecorder) call(_ context.Context, req LlmRequest) (string, error) {
	c.calls = append(c.calls, req)
	if c.fail {
		return "", errProviderDown
	}
	if c.scriptI >= len(c.script) {
		return "fallback", nil
	}
	out := c.script[c.scriptI]
	c.scriptI++
	return out, nil
}

type stubErr string

func (e stubErr) Error() string { return string(e) }

const errProviderDown = stubErr("provider down")

func bigSkill() *SkillPack {
	body := "SKILL: Advisor\n" + repeat("Follow the detailed procedure carefully. ", 120)
	return &SkillPack{
		ID:           "advisor",
		Name:         "Advisor",
		Description:  "advise",
		Stages:       []string{"synthesize"},
		Instructions: body,
		Files:        map[string]string{"reference/catalog.md": "## ML\n" + repeat("course prerequisites ", 400)},
		Injection:    "on_demand",
	}
}

func smallSkill() *SkillPack {
	return &SkillPack{
		ID:           "cr",
		Name:         "review",
		Description:  "review",
		Stages:       []string{"synthesize"},
		Instructions: "SKILL: review\nquote the bug",
		Injection:    "on_demand",
	}
}

func TestSmallSkillIsDeterministicNoLLM(t *testing.T) {
	pack := smallSkill()
	llm := &callRecorder{fail: false}
	c := CompileSkill(context.Background(), pack, llm.call, 600)
	if c.BuiltBy != "deterministic" {
		t.Errorf("built_by: got %q, want deterministic", c.BuiltBy)
	}
	if !contains(c.Surface, "quote the bug") {
		t.Errorf("surface missing 'quote the bug': %q", c.Surface)
	}
	if len(llm.calls) != 0 {
		t.Errorf("llm was called for small skill: %v", llm.calls)
	}
}

func TestLargeSkillLLMSurfaceWithinBudget(t *testing.T) {
	pack := bigSkill()
	llm := &callRecorder{script: []string{
		"CORE: advise on courses. For ML prereqs read [reference/catalog.md#ml].",
	}}
	c := CompileSkill(context.Background(), pack, llm.call, 200)
	if c.BuiltBy != "llm" {
		t.Errorf("built_by: got %q, want llm", c.BuiltBy)
	}
	if !contains(c.Surface, "[reference/catalog.md#ml]") {
		t.Errorf("surface missing chunk ref: %q", c.Surface)
	}
	if EstTokens(c.Surface) > 200 {
		t.Errorf("surface over budget: %d tokens", EstTokens(c.Surface))
	}
	foundChunk := false
	for _, ch := range c.Chunks {
		if ch.ID == "reference/catalog.md#ml" {
			foundChunk = true
		}
	}
	if !foundChunk {
		t.Errorf("expected chunk id reference/catalog.md#ml, got %v", c.Chunks)
	}
	if len(c.ContentHash) != 16 {
		t.Errorf("content_hash length: got %d, want 16 (%q)", len(c.ContentHash), c.ContentHash)
	}
}

func TestLargeSkillFallsBackOnLLMError(t *testing.T) {
	pack := bigSkill()
	llm := &callRecorder{fail: true}
	c := CompileSkill(context.Background(), pack, llm.call, 200)
	if c.BuiltBy != "deterministic" {
		t.Errorf("built_by: got %q, want deterministic", c.BuiltBy)
	}
	if !contains(c.Surface, "reference/catalog.md#ml") {
		t.Errorf("chunk index missing after fallback: %q", c.Surface)
	}
}

func TestChunkIDsMatchSections(t *testing.T) {
	pack := bigSkill()
	chunks := ChunkSkill(pack)
	ids := map[string]struct{}{}
	for _, c := range chunks {
		ids[c.ID] = struct{}{}
	}
	if _, ok := ids["reference/catalog.md#ml"]; !ok {
		t.Errorf("expected reference/catalog.md#ml in chunk ids, got %v", ids)
	}
	if _, ok := ids["SKILL.md#intro"]; !ok {
		t.Errorf("expected SKILL.md#intro in chunk ids, got %v", ids)
	}
}

func TestContentHashChangesOnEdit(t *testing.T) {
	a := smallSkill()
	b := smallSkill()
	b.Instructions = "SKILL: review\nDIFFERENT"
	if ContentHash(a) == ContentHash(b) {
		t.Errorf("content hash should differ after edit; both = %q", ContentHash(a))
	}
}

func TestSidecarRoundtripAndStale(t *testing.T) {
	dir := t.TempDir()
	pack := smallSkill()
	pack.SourceDir = dir

	cache := NewSurfaceCache(true)
	compiled := CompiledSkill{
		Slug:         pack.ID,
		ContentHash:  ContentHash(pack),
		BudgetTokens: 600,
		Surface:      "the surface",
		Chunks:       nil,
		BuiltBy:      "deterministic",
	}
	cache.Put(pack, compiled)

	// Sidecar should exist on disk.
	sidecar := filepath.Join(dir, "SKILL.compiled.json")
	if _, err := os.Stat(sidecar); err != nil {
		t.Fatalf("sidecar not written: %v", err)
	}

	// Fresh cache loads from sidecar.
	fresh := NewSurfaceCache(true)
	got, ok := fresh.Get(pack)
	if !ok {
		t.Fatalf("sidecar not loaded")
	}
	if got.Surface != "the surface" {
		t.Errorf("sidecar surface: got %q, want 'the surface'", got.Surface)
	}

	// Corrupt the content_hash → stale → ignored.
	data, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatalf("read sidecar: %v", err)
	}
	var d map[string]any
	if err := json.Unmarshal(data, &d); err != nil {
		t.Fatalf("unmarshal sidecar: %v", err)
	}
	d["content_hash"] = "deadbeef"
	corrupt, _ := json.Marshal(d)
	if err := os.WriteFile(sidecar, corrupt, 0o644); err != nil {
		t.Fatalf("rewrite sidecar: %v", err)
	}
	fresh2 := NewSurfaceCache(true)
	if _, ok := fresh2.Get(pack); ok {
		t.Errorf("expected stale sidecar to be ignored, got hit")
	}
}

func TestSidecarNotPersistedWhenPersistFalse(t *testing.T) {
	dir := t.TempDir()
	pack := smallSkill()
	pack.SourceDir = dir

	cache := NewSurfaceCache(false)
	compiled := CompiledSkill{
		Slug:         pack.ID,
		ContentHash:  ContentHash(pack),
		BudgetTokens: 600,
		Surface:      "in-memory only",
		Chunks:       nil,
		BuiltBy:      "deterministic",
	}
	cache.Put(pack, compiled)
	sidecar := filepath.Join(dir, "SKILL.compiled.json")
	if _, err := os.Stat(sidecar); err == nil {
		t.Errorf("sidecar should not exist when persist=false")
	}
}

func repeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
