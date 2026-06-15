package react

import (
	"strings"
	"testing"
)

// TestRepeatedFullWriteIsSteered mirrors test_repeated_full_write_is_steered.
func TestRepeatedFullWriteIsSteered(t *testing.T) {
	g := NewDocWriteGuard(DocGuardOpts{WriteTools: []string{"write_file"}})
	if g.Check("document", "write_file", map[string]any{"path": "ARCHITECTURE.md", "content": "v1"}) != "" {
		t.Fatal("first write should pass")
	}
	out := g.Check("document", "write_file", map[string]any{"path": "ARCHITECTURE.md", "content": "v2"})
	if out == "" || !strings.Contains(out, "already written") {
		t.Fatalf("out = %q", out)
	}
	if len(g.Events) != 1 || g.Events[0].Action != "redundant_rewrite" || g.Events[0].Path != "ARCHITECTURE.md" || g.Events[0].Stage != "document" {
		t.Fatalf("events = %+v", g.Events)
	}
}

// TestDifferentPathsNotFlagged mirrors test_different_paths_not_flagged.
func TestDifferentPathsNotFlagged(t *testing.T) {
	g := NewDocWriteGuard(DocGuardOpts{WriteTools: []string{"write_file"}})
	if g.Check("document", "write_file", map[string]any{"path": "A.md", "content": "x"}) != "" {
		t.Fatal("A.md should pass")
	}
	if g.Check("document", "write_file", map[string]any{"path": "B.md", "content": "y"}) != "" {
		t.Fatal("B.md should pass")
	}
	if len(g.Events) != 0 {
		t.Fatalf("events = %+v", g.Events)
	}
}

// TestWriteToolRefusedInReadonlyStage mirrors test_write_tool_refused_in_readonly_stage.
func TestWriteToolRefusedInReadonlyStage(t *testing.T) {
	g := NewDocWriteGuard(DocGuardOpts{WriteTools: []string{"Write"}, ReadonlyStages: []string{"survey"}})
	out := g.Check("survey", "Write", map[string]any{"file_path": "ARCHITECTURE.md", "content": "x"})
	if out == "" || !strings.Contains(out, "read-only") {
		t.Fatalf("out = %q", out)
	}
	if g.Events[len(g.Events)-1].Action != "blocked_readonly_write" {
		t.Fatalf("last event = %+v", g.Events[len(g.Events)-1])
	}
	if g.Check("document", "Write", map[string]any{"file_path": "ARCHITECTURE.md", "content": "x"}) != "" {
		t.Fatal("writable stage should pass the first write")
	}
}

// TestFilePathKeyIsRecognized mirrors test_file_path_key_is_recognized.
func TestFilePathKeyIsRecognized(t *testing.T) {
	g := NewDocWriteGuard(DocGuardOpts{WriteTools: []string{"Write"}})
	if g.Check("document", "Write", map[string]any{"file_path": "A.md", "content": "v1"}) != "" {
		t.Fatal("first write should pass")
	}
	out := g.Check("document", "Write", map[string]any{"file_path": "A.md", "content": "v2"})
	if out == "" || !strings.Contains(out, "already written") {
		t.Fatalf("out = %q", out)
	}
}

// TestCrossStageRewriteIsSteered mirrors test_cross_stage_rewrite_is_steered.
func TestCrossStageRewriteIsSteered(t *testing.T) {
	g := NewDocWriteGuard(DocGuardOpts{WriteTools: []string{"write_file"}})
	if g.Check("investigate", "write_file", map[string]any{"path": "ARCHITECTURE.md", "content": "v1"}) != "" {
		t.Fatal("first write should pass")
	}
	out := g.Check("document", "write_file", map[string]any{"path": "ARCHITECTURE.md", "content": "v2"})
	if out == "" || !strings.Contains(out, "earlier step") {
		t.Fatalf("out = %q", out)
	}
	if g.Events[len(g.Events)-1].Action != "redundant_rewrite_cross_stage" {
		t.Fatalf("last event = %+v", g.Events[len(g.Events)-1])
	}
}

// TestBashWriteBlockedInReadonlyStage mirrors test_bash_write_blocked_in_readonly_stage.
func TestBashWriteBlockedInReadonlyStage(t *testing.T) {
	g := NewDocWriteGuard(DocGuardOpts{BashTool: "bash", ReadonlyStages: []string{"survey"}})
	out := g.Check("survey", "bash", map[string]any{"command": "cat > ARCHITECTURE.md << 'EOF'\n# Doc\nEOF"})
	if out == "" || !strings.Contains(out, "read-only") {
		t.Fatalf("out = %q", out)
	}
	if g.Events[0].Action != "blocked_readonly_write" {
		t.Fatalf("first event = %+v", g.Events[0])
	}
	if g.Check("survey", "bash", map[string]any{"command": "ls -la && grep foo *.py"}) != "" {
		t.Fatal("non-write bash should pass")
	}
}

// TestRecordOnlyMeasuresWithoutIntercepting mirrors test_record_only_measures_without_intercepting.
func TestRecordOnlyMeasuresWithoutIntercepting(t *testing.T) {
	g := NewDocWriteGuard(DocGuardOpts{WriteTools: []string{"write_file"}, RecordOnly: true})
	g.Check("document", "write_file", map[string]any{"path": "A.md", "content": "v1"})
	out := g.Check("document", "write_file", map[string]any{"path": "A.md", "content": "v2"})
	if out != "" {
		t.Fatalf("record-only should not intercept, got %q", out)
	}
	if len(g.Events) == 0 || g.Events[0].Action != "redundant_rewrite" {
		t.Fatalf("events = %+v", g.Events)
	}
}
