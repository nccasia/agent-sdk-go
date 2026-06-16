// Translated from tests/test_probe_report.py — exercises the report
// package (RenderHTML / WriteHTML) end-to-end.
package report

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mezon/agent-sdk-go/agent_sdk/agent"
	"github.com/mezon/agent-sdk-go/agent_sdk/bench"
	"github.com/mezon/agent-sdk-go/agent_sdk/clients"
	"github.com/mezon/agent-sdk-go/agent_sdk/probe"
)

func scriptAgent(script []any, instructions string) *agent.PreactAgent {
	return agent.MustPreactAgent(agent.Config{
		Client:       clients.NewFakeClient(script, nil),
		Instructions: instructions,
	})
}

// TestRenderHTMLCombinesReportAndProbes mirrors test_render_html_combines_report_and_probes.
func TestRenderHTMLCombinesReportAndProbes(t *testing.T) {
	a := scriptAgent([]any{"answer"}, "helpful")
	rep, err := bench.NewHarness(a).Run(context.Background(), []bench.Scenario{
		{Input: "compare a and b in extensive detail right now", ExpectPath: "research"},
		{Input: "hi?", ExpectPath: "qna"},
	})
	if err != nil {
		t.Fatalf("harness: %v", err)
	}
	rec, err := probe.Probe(context.Background(), a, "compare a and b in extensive detail", probe.WithLabel("probe1"))
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	html := RenderHTML("coding-agent-bench", Report{Results: rep.Results}, Probes{rec},
		WithGeneratedAt("FIXED"))
	if !strings.HasPrefix(html, "<!doctype html>") {
		t.Errorf("html does not start with doctype")
	}
	if !strings.Contains(html, "coding-agent-bench") {
		t.Errorf("html missing title")
	}
	if !strings.Contains(html, "Scenarios") || !strings.Contains(html, "Probes") {
		t.Errorf("html missing Scenarios/Probes sections")
	}
	if !strings.Contains(html, "research") {
		t.Errorf("html missing routed flow 'research'")
	}
	if !strings.Contains(html, "probe1") {
		t.Errorf("html missing probe label")
	}
	if !strings.Contains(html, "signals") || !strings.Contains(html, "edges") {
		t.Errorf("html missing OY detail (signals/edges)")
	}
	if !strings.Contains(html, "raw JSON") {
		t.Errorf("html missing raw JSON drilldown")
	}
	if !strings.Contains(html, "lobe activation") {
		t.Errorf("html missing lobe activation")
	}
	if strings.Contains(html, "http://") || strings.Contains(html, "src=") {
		t.Errorf("html has external asset references (not self-contained)")
	}
}

// TestWriteHTML mirrors test_write_html.
func TestWriteHTML(t *testing.T) {
	a := scriptAgent([]any{"answer"}, "helpful")
	rec, err := probe.Probe(context.Background(), a, "hello?", probe.WithLabel("p"))
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "r.html")
	p, err := WriteHTML(out, "bench", Probes{rec})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("file not created: %v", err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), "<html>") {
		t.Errorf("html missing root <html>")
	}
}

// TestRenderHTMLCombinesOverviewAndProbes mirrors test_render_html_combines_overview_and_probes.
func TestRenderHTMLCombinesOverviewAndProbes(t *testing.T) {
	a := scriptAgent([]any{"answer"}, "helpful")
	rec, err := probe.Probe(context.Background(), a, "compare a and b", probe.WithLabel("probe1"))
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	verdict := Verdict{Status: "READY", Reasons: []string{}, Metrics: map[string]any{"activation.recall": 1.0}}
	modes := map[string]ModePayload{
		"activation": {
			Checks: []Check{
				{ID: "activation.code_review", OK: true, Detail: "P=1.0 R=1.0"},
			},
			AllPass: true,
		},
	}
	html := RenderHTML("skillbench", WithVerdict(verdict), WithModes(modes), Probes{rec}, WithGeneratedAt("FIXED"))
	if !strings.Contains(html, "Overview") || !strings.Contains(html, `class="verdict READY"`) {
		t.Errorf("overview half missing")
	}
	if !strings.Contains(html, "activation.code_review") || !strings.Contains(html, "activation.recall") {
		t.Errorf("overview half missing check id / metric")
	}
	if !strings.Contains(html, "Probes") || !strings.Contains(html, "probe1") || !strings.Contains(html, "raw JSON") {
		t.Errorf("probe half missing")
	}
	if strings.Contains(html, "http://") || strings.Contains(html, "src=") {
		t.Errorf("html has external asset references")
	}
}

// TestRenderHTMLReportOnly mirrors test_render_html_report_only: no probes
// ⇒ no "Probes" section.
func TestRenderHTMLReportOnly(t *testing.T) {
	a := scriptAgent([]any{"x"}, "helpful")
	rep, err := bench.NewHarness(a).Run(context.Background(), []bench.Scenario{
		{Input: "hi?", ExpectPath: "qna"},
	})
	if err != nil {
		t.Fatalf("harness: %v", err)
	}
	html := RenderHTML("t", Report{Results: rep.Results}, nil, WithGeneratedAt("FIXED"))
	if !strings.Contains(html, "Scenarios") {
		t.Errorf("missing Scenarios section")
	}
	if strings.Contains(html, ">Probes<") {
		t.Errorf("should not contain Probes section when none given")
	}
}
