// Ported from the RenderContextVarsBlock assertions in test_skill_activation.py
// + test_extra_coverage.py (the "driving suppresses index and pins workspace"
// assertion is partly rung-10 work; this file covers the renderer in isolation).
package skills

import (
	"strings"
	"testing"
)

func TestRenderContextVarsBlockEmpty(t *testing.T) {
	pack := &SkillPack{ID: "x"}
	if got := RenderContextVarsBlock(pack); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestRenderContextVarsChecklist(t *testing.T) {
	pack := &SkillPack{
		ID: "code_review",
		ContextVars: []map[string]any{
			{
				"key":   "scope",
				"type":  "checklist",
				"title": "Confirm scope",
				"items": []any{
					map[string]any{"title": "Read the diff", "status": "done"},
					map[string]any{"title": "Comment on correctness", "status": "todo"},
				},
			},
		},
	}
	out := RenderContextVarsBlock(pack)
	if !strings.Contains(out, "Live workspace state") {
		t.Errorf("missing lead: %q", out)
	}
	if !strings.Contains(out, "Confirm scope") {
		t.Errorf("missing title: %q", out)
	}
	if !strings.Contains(out, "[done] Read the diff") {
		t.Errorf("missing done item: %q", out)
	}
	if !strings.Contains(out, "[todo] Comment on correctness") {
		t.Errorf("missing todo item: %q", out)
	}
	if !strings.Contains(out, "skill:code_review:scope") {
		t.Errorf("missing persist hint: %q", out)
	}
}

func TestRenderContextVarsNotesEmptyValueMentionsPersistence(t *testing.T) {
	pack := &SkillPack{
		ID: "x",
		ContextVars: []map[string]any{
			{"key": "findings", "type": "notes", "title": "Findings"},
		},
	}
	out := RenderContextVarsBlock(pack)
	if !strings.Contains(out, "Findings (empty)") {
		t.Errorf("empty notes should mention '(empty)': %q", out)
	}
	if !strings.Contains(out, "skill:x:findings") {
		t.Errorf("persist hint missing: %q", out)
	}
}

func TestRenderContextVarsNotesWithValue(t *testing.T) {
	pack := &SkillPack{
		ID: "x",
		ContextVars: []map[string]any{
			{"key": "findings", "type": "notes", "title": "Findings", "value": "all clear"},
		},
	}
	out := RenderContextVarsBlock(pack)
	if !strings.Contains(out, "Findings: all clear") {
		t.Errorf("value not rendered: %q", out)
	}
}

func TestAllContextVarsPromotesLegacyChecklist(t *testing.T) {
	pack := &SkillPack{
		ID:        "x",
		Checklist: []map[string]any{{"key": "scope"}},
	}
	cv := pack.AllContextVars()
	if len(cv) == 0 {
		t.Fatalf("expected at least one var, got 0")
	}
	first := cv[0]
	if first["type"] != "checklist" {
		t.Errorf("legacy checklist not promoted as first var: %v", first)
	}
}
