package benchmarks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mezon/agent-sdk-go/agent_sdk/agent"
	"github.com/mezon/agent-sdk-go/agent_sdk/clients"
	"github.com/mezon/agent-sdk-go/agent_sdk/probe"
)

// TestWriteReportsOnePerBenchPlusIndex asserts WriteReports writes one HTML per
// registered bench plus an index.html, each carrying the bench's verdict.
func TestWriteReportsOnePerBenchPlusIndex(t *testing.T) {
	r := DefaultRegistry()
	dir := t.TempDir()
	paths, err := r.WriteReports(context.Background(), "", dir)
	if err != nil {
		t.Fatalf("WriteReports: %v", err)
	}
	benches := r.All()
	// one report per bench + an index
	if want := len(benches) + 1; len(paths) != want {
		t.Fatalf("wrote %d paths, want %d", len(paths), want)
	}
	for _, b := range benches {
		out := filepath.Join(dir, b.Name+".html")
		if _, err := os.Stat(out); err != nil {
			t.Errorf("missing report for %s: %v", b.Name, err)
		}
		data, _ := os.ReadFile(out)
		if !strings.Contains(string(data), "trace-data") {
			t.Errorf("%s report missing viewer chrome", b.Name)
		}
		if !strings.Contains(string(data), b.Name) {
			t.Errorf("%s report missing bench label", b.Name)
		}
	}
	index := filepath.Join(dir, "index.html")
	if _, err := os.Stat(index); err != nil {
		t.Fatalf("missing index.html: %v", err)
	}
	idx, _ := os.ReadFile(index)
	for _, b := range benches {
		if !strings.Contains(string(idx), b.Name+".html") {
			t.Errorf("index missing link to %s.html", b.Name)
		}
	}
	// the index is the last returned path
	if paths[len(paths)-1] != index {
		t.Errorf("last path = %q, want index %q", paths[len(paths)-1], index)
	}
}

// TestWriteReportsRendersProbeTrace asserts a bench whose Probe returns a real
// probe.Record yields HTML containing that record's trace markers (path / flow /
// stage). The Go engine records messages/system_prompt/system_segments on each
// flow stage, so a captured record renders a populated inspection.
func TestWriteReportsRendersProbeTrace(t *testing.T) {
	r := NewRegistry()
	r.Register(Bench{
		Name:         "tracebench",
		Tier:         Free,
		ExpectStatus: "READY",
		Run: func(ctx context.Context, model string) (Verdict, error) {
			ok := true
			return Verdict{
				Status:  "READY",
				Reasons: []string{},
				Gates:   map[string]*bool{"core_all_pass": &ok},
				Metrics: map[string]any{},
			}, nil
		},
		Probe: func(ctx context.Context, model string) ([]*probe.Record, error) {
			a := agent.MustPreactAgent(agent.Config{
				Client:       clients.NewFakeClient([]any{"the answer"}, nil),
				Instructions: "You are helpful.",
			})
			rec, err := probe.Probe(ctx, a, "what is up?", probe.WithLabel("trace-1"))
			if err != nil {
				return nil, err
			}
			return []*probe.Record{rec}, nil
		},
	})

	dir := t.TempDir()
	paths, err := r.WriteReports(context.Background(), "", dir)
	if err != nil {
		t.Fatalf("WriteReports: %v", err)
	}
	if len(paths) != 2 { // tracebench.html + index.html
		t.Fatalf("wrote %d paths, want 2", len(paths))
	}
	data, _ := os.ReadFile(filepath.Join(dir, "tracebench.html"))
	html := string(data)
	if !strings.Contains(html, "trace-1") {
		t.Errorf("report missing probe label trace-1")
	}
	// trace markers the viewer reads: path / flow / per-stage flow_steps.
	for _, marker := range []string{"\"path\"", "\"flow\"", "\"flow_steps\"", "\"trace\""} {
		if !strings.Contains(html, marker) {
			t.Errorf("report missing trace marker %s", marker)
		}
	}
	// the captured turn's answer is rendered into the inspection.
	if !strings.Contains(html, "the answer") {
		t.Errorf("report missing captured answer")
	}
}
