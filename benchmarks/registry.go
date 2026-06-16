package benchmarks

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"

	"github.com/nccasia/agent-sdk-go/agent_sdk/probe"
	"github.com/nccasia/agent-sdk-go/agent_sdk/viewer"
)

// Tier classifies a bench: Free benches are fully deterministic (no provider)
// and must be READY for the free-gate; Live benches additionally register
// provider-driven tiers when a model is configured.
type Tier int

const (
	// Free is a deterministic bench (the free-gate measures these).
	Free Tier = iota
	// Live is a provider-driven bench (deterministic tiers still run).
	Live
)

// RunFn runs a bench's modes and composes its verdict. model is "" when no
// provider is configured (the live tiers self-skip to UNMEASURED/READY).
type RunFn func(ctx context.Context, model string) (Verdict, error)

// Bench is one registered benchmark.
type Bench struct {
	Name string
	Tier Tier
	Run  RunFn
	// ExpectStatus is the bench's Python source-of-truth verdict for the
	// deterministic (no-provider) run — the cross-language parity target. The
	// free-gate passes iff each free bench's Go status equals ExpectStatus. Most
	// free benches are READY; attentionbench ships NOT_READY in Python (the qna/
	// research grounding scenarios reference a cite lobe that does not activate
	// without RAG), so its parity target is NOT_READY, matching ci-free-gates.sh
	// (which gates the unit suite + statelessbench, not attentionbench).
	ExpectStatus string
	// Probe optionally captures real probe traces for the inspection viewer. nil
	// ⇒ no traces are captured (the report still renders the verdict). Mirrors how
	// the Python run.py files pass their probe records into write_viewer.
	Probe func(ctx context.Context, model string) ([]*probe.Record, error)
}

// Registry is an ordered set of benches.
type Registry struct {
	benches map[string]Bench
}

// NewRegistry builds an empty registry.
func NewRegistry() *Registry { return &Registry{benches: map[string]Bench{}} }

// Register adds a bench (last registration of a name wins).
func (r *Registry) Register(b Bench) { r.benches[b.Name] = b }

// Names returns the registered bench names, sorted.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.benches))
	for n := range r.benches {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Get returns a bench by name.
func (r *Registry) Get(name string) (Bench, bool) {
	b, ok := r.benches[name]
	return b, ok
}

// Free returns the free-tier benches, sorted by name.
func (r *Registry) Free() []Bench {
	out := []Bench{}
	for _, n := range r.Names() {
		if b := r.benches[n]; b.Tier == Free {
			out = append(out, b)
		}
	}
	return out
}

// All returns every registered bench, sorted by name.
func (r *Registry) All() []Bench {
	out := []Bench{}
	for _, n := range r.Names() {
		out = append(out, r.benches[n])
	}
	return out
}

// GateRow is one bench's gate outcome.
type GateRow struct {
	Name   string
	Status string
	Expect string
	OK     bool
}

// FreeGate runs every bench's deterministic floor (model="") and reports, per
// bench, whether its Go verdict matches its Python ExpectStatus. The overall
// gate passes iff every row is OK. Mirrors ci-free-gates.sh's no-provider run:
// each bench reproduces its source-of-truth verdict.
func (r *Registry) FreeGate(ctx context.Context) ([]GateRow, bool, error) {
	rows := []GateRow{}
	allOK := true
	for _, b := range r.All() {
		v, err := b.Run(ctx, "")
		if err != nil {
			return rows, false, err
		}
		ok := v.Status == b.ExpectStatus
		if !ok {
			allOK = false
		}
		rows = append(rows, GateRow{Name: b.Name, Status: v.Status, Expect: b.ExpectStatus, OK: ok})
	}
	return rows, allOK, nil
}

// verdictMap converts a composed Verdict to the map[string]any shape the viewer
// (and the Python contract) consume: {status, reasons, gates, metrics}. The
// gates carry *bool (nil = skipped); JSON round-tripping yields the same shape
// the Python compose_verdict() emits.
func verdictMap(v Verdict) map[string]any {
	b, _ := json.Marshal(v)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m == nil {
		m = map[string]any{}
	}
	return m
}

// modesFromVerdict derives a per-mode rubric from a verdict's gates so the
// viewer's Overview shows each gate's pass/fail even when a bench exposes only a
// composed Verdict (RunFn returns no per-mode payloads). Each gate "<mode>_all_pass"
// becomes a single-check mode {checks, all_pass, n, pass}; a nil gate (skipped)
// is rendered as a skipped mode. Mirrors the {group: {checks,...}} modes dict.
func modesFromVerdict(v Verdict) map[string]any {
	modes := map[string]any{}
	for gate, ok := range v.Gates {
		mode := gate
		if len(gate) > len("_all_pass") && gate[len(gate)-len("_all_pass"):] == "_all_pass" {
			mode = gate[:len(gate)-len("_all_pass")]
		}
		if ok == nil {
			modes[mode] = map[string]any{
				"checks":      []any{},
				"all_pass":    false,
				"skipped":     true,
				"skip_reason": "skipped",
				"n":           0,
				"pass":        0,
			}
			continue
		}
		pass := 0
		if *ok {
			pass = 1
		}
		modes[mode] = map[string]any{
			"checks":   []any{map[string]any{"id": gate, "ok": *ok, "detail": ""}},
			"all_pass": *ok,
			"n":        1,
			"pass":     pass,
		}
	}
	return modes
}

// WriteReports writes one inspectable viewer HTML per bench under dir, plus an
// index.html that links every per-bench report with its verdict badge. For each
// bench it computes the Verdict via Run(ctx, model) and, when Probe != nil,
// captures the bench's probe records so the viewer renders a populated
// inspection. Returns the written paths (per-bench reports followed by the
// index). Mirrors the Python run.py write_viewer(label, verdict, modes) shape.
func (r *Registry) WriteReports(ctx context.Context, model, dir string) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	type row struct {
		name, status, file string
	}
	var rows []row
	var written []string
	for _, b := range r.All() {
		v, err := b.Run(ctx, model)
		if err != nil {
			return written, fmt.Errorf("%s: run: %w", b.Name, err)
		}
		var records []*probe.Record
		if b.Probe != nil {
			records, err = b.Probe(ctx, model)
			if err != nil {
				return written, fmt.Errorf("%s: probe: %w", b.Name, err)
			}
		}
		out := filepath.Join(dir, b.Name+".html")
		_, err = viewer.Write(out, records,
			viewer.WithLabel(b.Name+" · "+v.Status),
			viewer.WithVerdict(verdictMap(v)),
			viewer.WithModes(modesFromVerdict(v)),
		)
		if err != nil {
			return written, fmt.Errorf("%s: write: %w", b.Name, err)
		}
		written = append(written, out)
		rows = append(rows, row{name: b.Name, status: v.Status, file: b.Name + ".html"})
	}

	// index.html — links every per-bench report with its verdict badge.
	var body []string
	body = append(body,
		"<!doctype html><html><head><meta charset='utf-8'><title>benchmark reports</title>",
		"<style>body{background:#FAFAF7;color:#0E0E0C;margin:0;font:14px/1.55 -apple-system,Roboto,Segoe UI,sans-serif}",
		".wrap{max-width:760px;margin:0 auto;padding:28px 20px 80px}h1{font-size:22px;margin:0 0 18px}",
		"ul{list-style:none;padding:0;margin:0}li{margin:0 0 8px}a{color:#0E0E0C;text-decoration:none}",
		".badge{display:inline-block;padding:1px 10px;border-radius:99px;font-size:12px;font-weight:600;margin-right:10px;min-width:84px;text-align:center}",
		".READY{background:#e7f3ec;color:#1F6B4A}.NOT_READY{background:#fde9e9;color:#b3261e}.UNMEASURED{background:#f0f0ea;color:#6b6b63}",
		"</style></head><body><div class='wrap'><h1>benchmark reports</h1><ul>",
	)
	for _, ro := range rows {
		body = append(body, fmt.Sprintf(
			"<li><span class='badge %s'>%s</span><a href='%s'>%s</a></li>",
			html.EscapeString(ro.status), html.EscapeString(ro.status),
			html.EscapeString(ro.file), html.EscapeString(ro.name),
		))
	}
	body = append(body, "</ul></div></body></html>")
	index := filepath.Join(dir, "index.html")
	if err := os.WriteFile(index, []byte(joinStr(body)), 0o644); err != nil {
		return written, err
	}
	written = append(written, index)
	return written, nil
}

func joinStr(xs []string) string {
	out := ""
	for _, x := range xs {
		out += x
	}
	return out
}

// DefaultRegistry builds the registry of all benches the ladder ships. model is
// passed through to the live tiers ("" ⇒ free tiers only).
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(Bench{Name: "agentbench", Tier: Live, Run: RunAgentBench, ExpectStatus: "UNMEASURED", Probe: RunAgentBenchProbes})
	r.Register(Bench{Name: "attentionbench", Tier: Free, Run: RunAttentionBench, ExpectStatus: "NOT_READY", Probe: RunAttentionBenchProbes})
	r.Register(Bench{Name: "codingagentbench", Tier: Live, Run: RunCodingAgentBench, ExpectStatus: "UNMEASURED", Probe: RunCodingAgentBenchProbes})
	r.Register(Bench{Name: "corgictionbech", Tier: Free, Run: RunCorgictionBench, ExpectStatus: "READY", Probe: RunCorgictionBenchProbes})
	r.Register(Bench{Name: "delegationbench", Tier: Live, Run: RunDelegationBench, ExpectStatus: "UNMEASURED", Probe: RunDelegationBenchProbes})
	r.Register(Bench{Name: "extensionbench", Tier: Live, Run: RunExtensionBench, ExpectStatus: "UNMEASURED", Probe: RunExtensionBenchProbes})
	r.Register(Bench{Name: "flowbench", Tier: Free, Run: RunFlowBench, ExpectStatus: "READY", Probe: RunFlowBenchProbes})
	r.Register(Bench{Name: "promptbench", Tier: Free, Run: RunPromptBench, ExpectStatus: "READY", Probe: RunPromptBenchProbes})
	r.Register(Bench{Name: "skillbench", Tier: Live, Run: RunSkillBench, ExpectStatus: "UNMEASURED", Probe: RunSkillBenchProbes})
	r.Register(Bench{Name: "statelessbench", Tier: Free, Run: RunStatelessBench, ExpectStatus: "READY", Probe: RunStatelessBenchProbes})
	r.Register(Bench{Name: "taskbench", Tier: Live, Run: RunTaskBench, ExpectStatus: "UNMEASURED", Probe: RunTaskBenchProbes})
	r.Register(Bench{Name: "toolbench", Tier: Live, Run: RunToolBench, ExpectStatus: "READY", Probe: RunToolBenchProbes})
	return r
}
