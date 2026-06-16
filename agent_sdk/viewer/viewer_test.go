// Translated from tests/test_viewer.py — exercises the viewer package
// (ToRecord / RenderHTML / Write) end-to-end.
package viewer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mezon/agent-sdk-go/agent_sdk/agent"
	"github.com/mezon/agent-sdk-go/agent_sdk/clients"
	"github.com/mezon/agent-sdk-go/agent_sdk/probe"
	"github.com/mezon/agent-sdk-go/agent_sdk/tools"
)

func scriptAgent(script []any, instructions string, extra agent.Config) *agent.PreactAgent {
	cfg := agent.Config{
		Client:       clients.NewFakeClient(script, nil),
		Instructions: instructions,
	}
	if extra.Tools != nil {
		cfg.Tools = extra.Tools
	}
	return agent.MustPreactAgent(cfg)
}

// TestToViewerRecordSchema mirrors test_to_viewer_record_schema.
func TestToViewerRecordSchema(t *testing.T) {
	a := scriptAgent([]any{"the answer"}, "bot", agent.Config{})
	rec, err := probe.Probe(context.Background(), a, "what is up?", probe.WithLabel("t1"))
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	vr := ToRecord(rec)
	if vr.Label != "t1" {
		t.Errorf("label = %q, want t1", vr.Label)
	}
	tr := vr.Trace
	if _, ok := tr.Path["name"]; !ok {
		t.Errorf("path.name missing")
	}
	if tr.Lobes == nil {
		t.Errorf("lobes nil")
	}
	if tr.LlmCalls == nil {
		t.Errorf("llm_calls nil")
	}
	if len(tr.FlowSteps) == 0 {
		t.Errorf("flow_steps empty")
	}
	if _, ok := tr.FlowSteps[0]["step"]; !ok {
		t.Errorf("flow_steps[0].step missing")
	}
	if _, ok := tr.UsageRollup["input_tokens"]; !ok {
		t.Errorf("usage_rollup.input_tokens missing")
	}
}

// TestRenderViewerHTMLInjectsRecords mirrors test_render_viewer_html_injects_records.
func TestRenderViewerHTMLInjectsRecords(t *testing.T) {
	a := scriptAgent([]any{"x"}, "b", agent.Config{})
	rec, err := probe.Probe(context.Background(), a, "hi?", probe.WithLabel("r1"))
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	html := RenderHTML([]*probe.Record{rec}, WithLabel("bench"))
	if strings.Contains(html, "<!--TRACE_DATA-->") {
		t.Errorf("trace-data placeholder still present")
	}
	if !strings.Contains(html, `id="trace-data"`) {
		t.Errorf("trace-data injection block missing")
	}
	if !strings.Contains(html, "r1") {
		t.Errorf("record label not in html")
	}
	// Real viewer chrome is reused (the embedded template).
	if !strings.Contains(html, "trace-data") || !strings.Contains(strings.ToLower(html), "drop") {
		t.Errorf("viewer chrome missing (no 'drop' fallback)")
	}
}

// TestWriteViewer mirrors write_viewer: writes a file to disk.
func TestWriteViewer(t *testing.T) {
	a := scriptAgent([]any{"x"}, "b", agent.Config{})
	rec, err := probe.Probe(context.Background(), a, "hi?", probe.WithLabel("r1"))
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "view.html")
	p, err := Write(out, []*probe.Record{rec}, WithLabel("bench"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("file not created: %v", err)
	}
	data, _ := os.ReadFile(p)
	if !strings.Contains(string(data), "trace-data") {
		t.Errorf("written file missing trace-data")
	}
}

// TestEngineCapturesSystemPromptForPromptPanel mirrors
// test_engine_captures_system_prompt_for_prompt_panel: the viewer's
// flow_steps surface includes system_prompt.
func TestEngineCapturesSystemPromptForPromptPanel(t *testing.T) {
	a := scriptAgent([]any{"answer"}, "You are helpful.", agent.Config{})
	res, err := a.Query(context.Background(), "what?")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	fs0, _ := res.Trace.FlowStages[0]["system_prompt"].(string)
	if !strings.Contains(fs0, "You are helpful.") {
		t.Errorf("system_prompt missing instructions: %q", fs0)
	}
	rec, err := probe.Probe(context.Background(), a, "again?")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	vr := ToRecord(rec)
	sp, _ := vr.Trace.FlowSteps[0]["system_prompt"].(string)
	if sp == "" {
		t.Errorf("viewer flow_steps[0].system_prompt is empty")
	}
}

// TestEngineCapturesLLMCalls mirrors test_engine_captures_llm_calls: one hop
// with a tool_use response + a tool_result, one final answer hop.
func TestEngineCapturesLLMCalls(t *testing.T) {
	search := tools.Tool("search", func(ctx context.Context, in map[string]any) (any, error) {
		return "hit", nil
	}, tools.Desc("search"), tools.Param("q", "string", true, nil))
	a := scriptAgent(
		[]any{
			map[string]any{"tools": []map[string]any{{"name": "search", "input": map[string]any{"q": "x"}}}},
			"done",
		},
		"bot",
		agent.Config{Tools: []any{search}},
	)
	res, err := a.Query(context.Background(), "go")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(res.Trace.LlmCalls) == 0 {
		t.Errorf("no llm calls captured")
	}
}

// TestToRecordSystemSegmentsPassthrough ensures system_segments are preserved
// when the engine records them.
func TestToRecordSystemSegmentsPassthrough(t *testing.T) {
	a := scriptAgent([]any{"answer"}, "You are helpful.", agent.Config{})
	rec, err := probe.Probe(context.Background(), a, "what?")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	vr := ToRecord(rec)
	// The viewer record should always have flow_steps.
	if len(vr.Trace.FlowSteps) == 0 {
		t.Fatalf("flow_steps empty")
	}
	// The engine now composes provenance segments, so the passthrough must
	// surface them: each step carries a non-empty []map with {source,start,end,
	// stability}, and the identity (`instructions`) source leads.
	segs, ok := vr.Trace.FlowSteps[0]["system_segments"].([]map[string]any)
	if !ok || len(segs) == 0 {
		t.Fatalf("system_segments not surfaced through ToRecord: %T %v",
			vr.Trace.FlowSteps[0]["system_segments"], vr.Trace.FlowSteps[0]["system_segments"])
	}
	for _, k := range []string{"source", "start", "end", "stability"} {
		if _, has := segs[0][k]; !has {
			t.Errorf("segment missing %q key: %v", k, segs[0])
		}
	}
	if src, _ := segs[0]["source"].(string); src != "instructions" {
		t.Errorf("first segment source = %q, want instructions", src)
	}
}
