// Package viewer — adapt SDK probe records into the reused, dependency-free
// benchmark viewer.html.
//
// The project's viewer.html is a self-contained interactive report
// (conversation/turn selector · timeline · pipeline (OX) · context lobes (OY) ·
// reasoning · tools · hotspots · raw JSON, with drag-drop fallback). It is an
// HTML asset with no code coupling, so the SDK reuses it directly: ToRecord
// maps an SDK probe.Record into the record schema the viewer reads, and
// RenderHTML injects the records at the template's <!--TRACE_DATA--> seam.
//
// Ported from agent_sdk/viewer.py.
package viewer

import (
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mezon/agent-sdk-go/agent_sdk/probe"
)

//go:embed assets/viewer.html
var viewerTemplate string

// ViewerRecord is one probe adapted to the viewer's record schema.
type ViewerRecord struct {
	Label     string           `json:"label"`
	Query     string           `json:"query"`
	Answer    string           `json:"answer"`
	Status    string           `json:"status"`
	Trace     ViewerTrace      `json:"trace"`
	ToolCalls []map[string]any `json:"tool_calls"`
	Context   map[string]any   `json:"context"`
	Hints     []map[string]any `json:"hints"`
	Error     string           `json:"error"`
}

// ViewerTrace is the trace sub-record the viewer panels read.
type ViewerTrace struct {
	Path          map[string]any   `json:"path"`
	Flow          map[string]any   `json:"flow"`
	Lobes         []map[string]any `json:"lobes"`
	FlowSteps     []map[string]any `json:"flow_steps"`
	LlmCalls      []map[string]any `json:"llm_calls"`
	UsageRollup   map[string]any   `json:"usage_rollup"`
	Meta          map[string]any   `json:"meta"`
	Attention     map[string]any   `json:"attention"`
	ContextFunnel map[string]any   `json:"context_funnel"`
	Blackboard    map[string]any   `json:"blackboard"`
	Skills        []map[string]any `json:"skills"`
	Scratchpad    map[string]any   `json:"scratchpad"`
}

// Option configures a RenderHTML / Write call.
type Option func(*opts)

type opts struct {
	label   string
	verdict any
	modes   any
	hasV    bool
	hasM    bool
}

// WithLabel sets the viewer's top-level bench label.
func WithLabel(s string) Option { return func(o *opts) { o.label = s } }

// WithVerdict attaches a verdict banner payload.
func WithVerdict(v any) Option { return func(o *opts) { o.verdict = v; o.hasV = true } }

// WithModes attaches per-group check tables.
func WithModes(m any) Option { return func(o *opts) { o.modes = m; o.hasM = true } }

// flowSteps maps the SDK's per-stage trace into the viewer's flow_steps shape.
func flowSteps(p *probe.Record) []map[string]any {
	out := make([]map[string]any, 0, len(p.Stages))
	for _, s := range p.Stages {
		stage := s["stage"]
		var calls []map[string]any
		tokIn, tokOut := 0, 0
		for _, c := range p.LlmCalls {
			if c["stage"] == stage {
				calls = append(calls, c)
				usage := mapOf(c["usage"])
				tokIn += intOf(usage["input_tokens"])
				tokOut += intOf(usage["output_tokens"])
			}
		}
		toolSet := map[string]bool{}
		for _, st := range stepsOf(s["steps"]) {
			if st["kind"] == "tool_use" {
				if name, _ := st["name"].(string); name != "" {
					toolSet[name] = true
				}
			}
		}
		toolNames := make([]string, 0, len(toolSet))
		for n := range toolSet {
			toolNames = append(toolNames, n)
		}
		sort.Strings(toolNames)

		meta := mapOf(s["metadata"])
		if meta == nil {
			meta = map[string]any{}
		} else {
			meta = cloneMap(meta)
		}
		if _, ok := meta["hops"]; !ok {
			meta["hops"] = len(calls)
		}
		skipped, _ := s["skipped"].(bool)
		meta["skipped"] = skipped

		out = append(out, map[string]any{
			"step":             stage,
			"flow":             s["flow"],
			"loop":             orDefault(s["loop"], "single"),
			"lobes":            orList(s["lobes"]),
			"tools":            toolNames,
			"tokens_in":        tokIn,
			"tokens_out":       tokOut,
			"tokens_after":     tokOut,
			"latency_ms":       0,
			"node_count":       len(orList(s["lobes"])),
			"system_prompt":    orStr(s["system_prompt"]),
			"system_segments":  orMapList(s["system_segments"]),
			"subagents":        orMapList(s["subagents"]),
			"metadata":         meta,
			"funnel_obs_chars": orList(meta["funnel_obs_chars"]),
			"attention":        orAttention(s["attention"]),
		})
	}
	return out
}

func contextFunnel(p *probe.Record) map[string]any {
	stages := orMapList(p.Attention["stages"])
	if len(stages) == 0 {
		for _, s := range p.Stages {
			md := mapOf(s["metadata"])
			att := mapOf(s["attention"])
			stages = append(stages, map[string]any{
				"stage":            s["stage"],
				"input_tokens":     intOf(md["input_tokens"]),
				"funnel_obs_chars": orList(md["funnel_obs_chars"]),
				"tier_counts":      orMap(att["tier_counts"]),
			})
		}
	}
	return map[string]any{
		"stages":      stages,
		"tier_counts": orMap(p.Attention["tier_counts"]),
	}
}

// ToRecord adapts one probe.Record to the viewer's record schema. Mirrors
// to_viewer_record.
func ToRecord(p *probe.Record) ViewerRecord {
	path := p.Path
	if len(path) == 0 {
		path = map[string]any{"name": p.Flow, "score": p.FlowScore}
	}
	meta := map[string]any{}
	if len(p.MetaActions) > 0 {
		meta = p.MetaActions[len(p.MetaActions)-1]
	}
	trace := ViewerTrace{
		Path:      path,
		Flow:      map[string]any{"name": p.Flow},
		Lobes:     orMapList(asAnyList(p.Lobes)),
		FlowSteps: flowSteps(p),
		LlmCalls:  p.LlmCalls,
		UsageRollup: map[string]any{
			"input_tokens":  intOf(p.Usage["input_tokens"]),
			"output_tokens": intOf(p.Usage["output_tokens"]),
		},
		Meta:          meta,
		Attention:     orAttention(p.Attention),
		ContextFunnel: contextFunnel(p),
		Blackboard:    orMap(p.Blackboard),
		Skills:        []map[string]any{},
		Scratchpad:    map[string]any{},
	}
	if trace.Lobes == nil {
		trace.Lobes = []map[string]any{}
	}
	if trace.LlmCalls == nil {
		trace.LlmCalls = []map[string]any{}
	}
	return ViewerRecord{
		Label:     p.Label,
		Query:     p.Query,
		Answer:    p.Answer,
		Status:    p.Status,
		Trace:     trace,
		ToolCalls: orMapList(asAnyList(p.ToolCalls)),
		Context: map[string]any{
			"task_items":     []any{},
			"task_templates": []any{},
		},
		Hints: p.Hints,
		Error: p.Error,
	}
}

// RenderHTML injects the probe records into the viewer template (one
// self-contained HTML). Mirrors render_viewer_html.
func RenderHTML(records []*probe.Record, options ...Option) string {
	o := &opts{}
	for _, fn := range options {
		fn(o)
	}
	recs := make([]ViewerRecord, 0, len(records))
	for _, r := range records {
		recs = append(recs, ToRecord(r))
	}
	data := map[string]any{"label": o.label, "records": recs}
	if o.hasV {
		data["verdict"] = o.verdict
	}
	if o.hasM {
		data["modes"] = o.modes
	}
	payload, _ := json.Marshal(data)
	// never close the embedded block early
	safe := strings.ReplaceAll(string(payload), "</", "<\\/")
	block := `<script id="trace-data" type="application/json">` + safe + `</script>`
	return strings.Replace(viewerTemplate, "<!--TRACE_DATA-->", block, 1)
}

// Write renders the rich viewer HTML to path (creating parent dirs). Returns
// it. Mirrors write_viewer.
func Write(path string, records []*probe.Record, options ...Option) (string, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(path, []byte(RenderHTML(records, options...)), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// ── small typed accessors ──

func intOf(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	}
	return 0
}

func mapOf(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func orMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok && m != nil {
		return m
	}
	return map[string]any{}
}

func orStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func orDefault(v any, def string) any {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	if v != nil {
		return v
	}
	return def
}

func orList(v any) []any {
	switch x := v.(type) {
	case []any:
		return x
	case []string:
		out := make([]any, len(x))
		for i, s := range x {
			out[i] = s
		}
		return out
	}
	return []any{}
}

func orMapList(v any) []map[string]any {
	switch x := v.(type) {
	case []map[string]any:
		return x
	case []any:
		out := make([]map[string]any, 0, len(x))
		for _, e := range x {
			if m, ok := e.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return []map[string]any{}
}

func asAnyList(xs []map[string]any) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}

func stepsOf(v any) []map[string]any { return orMapList(v) }

func orAttention(v any) map[string]any {
	if m, ok := v.(map[string]any); ok && len(m) > 0 {
		return m
	}
	return map[string]any{"nodes": []any{}, "tiers": []any{}}
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
