// Package report — one self-contained HTML report combining a benchmark
// scorecard + probe internals.
//
// RenderHTML produces a single, dependency-free HTML page combining a
// bench.Report (scenario pass/fail + scores) and any probe.Record turns (the
// recognized flow, per-lobe activation, and the stage-by-stage ReAct timeline).
// No JS, no external assets — open the file in any browser. WriteHTML writes it
// to disk.
//
// Ported from agent_sdk/report.py.
package report

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nccasia/agent-sdk-go/agent_sdk/bench"
	"github.com/nccasia/agent-sdk-go/agent_sdk/probe"
)

// Report is the bench scorecard half of the render. It mirrors
// bench.Report's surface so the report package can render it without a hard
// coupling. Either embed a bench.Report's Results or pass one straight through.
type Report struct {
	Results []bench.ScenarioResult
}

func (r Report) summary() bench.Summary { return (&bench.Report{Results: r.Results}).Summary() }

// Probes is a named slice of probe records (the variadic-arg sugar the API
// accepts alongside a bench Report).
type Probes []*probe.Record

// Verdict is the readiness banner payload (status + reasons + metrics).
type Verdict struct {
	Status  string         `json:"status"`
	Reasons []string       `json:"reasons"`
	Metrics map[string]any `json:"metrics"`
}

// Check is one per-group check row.
type Check struct {
	ID     string `json:"id"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
	Diag   bool   `json:"diag"`
}

// ModePayload is the per-group check table payload.
type ModePayload struct {
	Checks  []Check `json:"checks"`
	N       int     `json:"n"`
	Pass    int     `json:"pass"`
	AllPass bool    `json:"all_pass"`
}

// Option configures a RenderHTML / WriteHTML call.
type Option func(*opts)

type opts struct {
	report      *Report
	probes      Probes
	verdict     *Verdict
	modes       map[string]ModePayload
	generatedAt string
}

// WithVerdict attaches the readiness verdict banner.
func WithVerdict(v Verdict) Option { return func(o *opts) { o.verdict = &v } }

// WithModes attaches the per-group check tables.
func WithModes(m map[string]ModePayload) Option { return func(o *opts) { o.modes = m } }

// WithGeneratedAt pins the report timestamp (for deterministic output).
func WithGeneratedAt(s string) Option { return func(o *opts) { o.generatedAt = s } }

// applyArg folds a positional argument (Report / Probes / []*probe.Record /
// Option) into the option struct.
func (o *opts) applyArg(arg any) {
	switch v := arg.(type) {
	case nil:
		// a nil probes slot — ignore
	case Report:
		o.report = &v
	case *Report:
		o.report = v
	case Probes:
		o.probes = v
	case []*probe.Record:
		o.probes = Probes(v)
	case Option:
		v(o)
	}
}

const css = `
:root{--paper:#FAFAF7;--ink:#0E0E0C;--muted:#6b6b63;--line:#e4e3db;
--emerald:#1F6B4A;--amber:#B8845B;--red:#b3261e;--card:#fff}
*{box-sizing:border-box}
body{background:var(--paper);color:var(--ink);margin:0;
font:14px/1.5 -apple-system,Roboto,Segoe UI,sans-serif}
.wrap{max-width:1040px;margin:0 auto;padding:28px 20px 80px}
h1{font-size:22px;margin:0 0 2px}.sub{color:var(--muted);font-size:13px;margin-bottom:18px}
h2{font-size:15px;margin:30px 0 10px;letter-spacing:.02em;text-transform:uppercase;color:var(--muted)}
.cards{display:flex;gap:10px;flex-wrap:wrap;margin:14px 0}
.pill{background:var(--card);border:1px solid var(--line);border-radius:10px;padding:10px 14px;min-width:120px}
.pill .k{font-size:11px;color:var(--muted);text-transform:uppercase;letter-spacing:.04em}
.pill .v{font-size:20px;font-weight:600;margin-top:2px}
table{width:100%;border-collapse:collapse;background:var(--card);
border:1px solid var(--line);border-radius:10px;overflow:hidden;font-size:13px}
th,td{text-align:left;padding:8px 10px;border-bottom:1px solid var(--line);vertical-align:top}
th{background:#f3f2ec;font-weight:600;font-size:12px;color:var(--muted)}
tr:last-child td{border-bottom:0}
code,.mono{font-family:"Geist Mono",ui-monospace,Menlo,monospace;font-size:12px}
.ok{color:var(--emerald);font-weight:600}.bad{color:var(--red);font-weight:600}
.badge{display:inline-block;padding:1px 8px;border-radius:99px;font-size:12px;
border:1px solid var(--line);background:#f3f2ec;margin:0 4px 4px 0}
.badge.flow{background:#e7f0ea;border-color:#cfe2d6;color:var(--emerald)}
.badge.tool{background:#f5ece2;border-color:#e7d6c2;color:var(--amber)}
.badge.on{background:#e7f0ea;color:var(--emerald);border-color:#cfe2d6}
.badge.off{color:var(--muted)}
details{background:var(--card);border:1px solid var(--line);border-radius:10px;margin:10px 0;padding:4px 14px}
summary{cursor:pointer;padding:8px 0;font-weight:600;list-style:none}
summary::-webkit-details-marker{display:none}
summary .meta{font-weight:400;color:var(--muted);margin-left:8px}
.stage{border-left:2px solid var(--line);margin:10px 0 10px 4px;padding:2px 0 2px 14px}
.stage>.h{font-weight:600;margin-bottom:4px}
.step{margin:3px 0;font-size:12.5px}
.step .lbl{display:inline-block;width:64px;color:var(--muted);font-size:11px;text-transform:uppercase}
.step.think .t{color:var(--muted)}
.step.tool .t{color:var(--amber)}
.step.result .t{color:var(--ink)}
.step.answer .t{color:var(--emerald)}
.ans{background:#f3f2ec;border-radius:8px;padding:10px 12px;margin-top:8px;white-space:pre-wrap}
.fail{color:var(--red);font-size:12px}
.empty{color:var(--muted);font-style:italic}
.verdict{display:inline-block;padding:6px 16px;border-radius:99px;font-weight:700;
font-size:15px;letter-spacing:.04em;margin:8px 0 2px}
.verdict.READY{background:#e7f0ea;color:var(--emerald);border:1px solid #cfe2d6}
.verdict.NOT_READY{background:#fbe9e7;color:var(--red);border:1px solid #f0cdc8}
.verdict.UNMEASURED{background:#f3f2ec;color:var(--muted);border:1px solid var(--line)}
.reasons{color:var(--red);font-size:12.5px;margin:4px 0 8px}
td.diag{color:var(--muted);font-style:italic}
.tabs{position:relative}
input.tabradio{position:absolute;opacity:0;width:0;height:0}
.tabnav{display:flex;gap:4px;border-bottom:2px solid var(--line);margin:18px 0 16px;flex-wrap:wrap}
.tablabel{cursor:pointer;padding:8px 16px;font-weight:600;color:var(--muted);
border:1px solid transparent;border-bottom:none;border-radius:8px 8px 0 0;margin-bottom:-2px}
.tablabel:hover{color:var(--ink)}
.tabpanel{display:none}
`

func e(x any) string { return html.EscapeString(fmt.Sprintf("%v", x)) }

func trunc(x any, n int) string {
	s := fmt.Sprintf("%v", x)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func scorecard(rep *Report, probes Probes) string {
	type pill struct{ k, v, cls string }
	var pills []pill
	if rep != nil {
		s := rep.summary()
		cls := "bad"
		if s.Passed == s.Scenarios {
			cls = "ok"
		}
		pills = append(pills, pill{"scenarios", fmt.Sprintf("%d/%d", s.Passed, s.Scenarios), cls})
		pills = append(pills, pill{"path acc", fmt.Sprintf("%.0f%%", s.PathAccuracy*100), ""})
		if s.LobeRecall != nil {
			pills = append(pills, pill{"lobe recall", fmt.Sprintf("%.0f%%", *s.LobeRecall*100), ""})
		}
		pills = append(pills, pill{"p95", fmt.Sprintf("%.0fms", s.P95LatencyMs), ""})
	}
	if len(probes) > 0 {
		toks := 0
		for _, p := range probes {
			toks += intOf(p.Usage["output_tokens"])
		}
		pills = append(pills, pill{"probes", fmt.Sprintf("%d", len(probes)), ""})
		pills = append(pills, pill{"out tokens", fmt.Sprintf("%d", toks), ""})
	}
	var b strings.Builder
	for _, p := range pills {
		fmt.Fprintf(&b, `<div class="pill"><div class="k">%s</div><div class="v %s">%s</div></div>`, e(p.k), p.cls, e(p.v))
	}
	return `<div class="cards">` + b.String() + `</div>`
}

func scenarios(rep *Report) string {
	var rows strings.Builder
	for _, r := range rep.Results {
		mark := `<span class="ok">PASS</span>`
		if !r.Passed {
			mark = `<span class="bad">FAIL</span>`
		}
		fail := ""
		if len(r.Failures) > 0 {
			fail = `<div class="fail">` + e(strings.Join(r.Failures, "; ")) + `</div>`
		}
		var lobes strings.Builder
		for i, x := range r.ActivatedLobes {
			if i >= 8 {
				break
			}
			fmt.Fprintf(&lobes, `<span class="badge on">%s</span>`, e(x))
		}
		expect := r.Scenario.ExpectPath
		if expect == "" {
			expect = "—"
		}
		fmt.Fprintf(&rows,
			"<tr><td class=mono>%s</td><td>%s</td>"+
				`<td><span class="badge flow">%s</span> <span class="mono">%.2f</span></td>`+
				"<td>%s</td><td>%s%s</td></tr>",
			e(trunc(r.Scenario.Input, 70)), e(expect), e(r.Path.Name), r.Path.Score, lobes.String(), mark, fail)
	}
	return "<table><tr><th>input</th><th>expect</th><th>routed flow</th>" +
		"<th>activated lobes</th><th>result</th></tr>" + rows.String() + "</table>"
}

func kv(d map[string]any) string {
	var items []string
	for k, v := range d {
		if v != nil && fmt.Sprintf("%v", v) != "" && v != false && v != 0 {
			items = append(items, fmt.Sprintf("%v=%v", k, v))
		}
	}
	sort.Strings(items)
	if len(items) == 0 {
		return "—"
	}
	return strings.Join(items, ", ")
}

func edges(d map[string]any) string {
	var items []string
	for k, v := range d {
		items = append(items, fmt.Sprintf("%v→%v", k, v))
	}
	sort.Strings(items)
	if len(items) == 0 {
		return "—"
	}
	return strings.Join(items, ", ")
}

func lobeTable(lobes []map[string]any) string {
	var rows strings.Builder
	for _, lb := range lobes {
		on, _ := lb["activated"].(bool)
		badge := "off"
		yn := "no"
		if on {
			badge = "on"
			yn = "yes"
		}
		fmt.Fprintf(&rows,
			"<tr><td class=mono>%s</td><td>%s</td>"+
				`<td><span class="badge %s">%s</span></td>`+
				"<td class=mono>%.2f</td><td class=mono>%s</td>"+
				"<td class=mono>%s</td><td class=mono>%s</td></tr>",
			e(lb["id"]), e(lb["layer"]), badge, yn, floatOf(lb["activation"]),
			e(lb["reason"]), e(kv(mapOf(lb["signals"]))), e(edges(mapOf(lb["in_edges"]))))
	}
	return "<table><tr><th>lobe</th><th>layer</th><th>activated</th><th>activation</th>" +
		"<th>reason</th><th>signals</th><th>edges</th></tr>" + rows.String() + "</table>"
}

func hotspots(hints []map[string]any) string {
	if len(hints) == 0 {
		return ""
	}
	var rows strings.Builder
	for _, h := range hints {
		var patch []string
		for k, v := range mapOf(h["weight_patch"]) {
			patch = append(patch, fmt.Sprintf("%v=%v", k, v))
		}
		sort.Strings(patch)
		fmt.Fprintf(&rows, "<tr><td>%s</td><td class=mono>%s</td><td>%s</td><td class=mono>%s</td></tr>",
			e(h["axis"]), e(h["target"]), e(h["reason"]), e(strings.Join(patch, ", ")))
	}
	return "<details><summary>optimization hotspots " +
		fmt.Sprintf(`<span class="meta">(%d)</span></summary>`, len(hints)) +
		"<table><tr><th>axis</th><th>target</th><th>reason</th><th>weight patch</th></tr>" +
		rows.String() + "</table></details>"
}

func stageBlock(stage map[string]any) string {
	name := e(stage["stage"])
	loop := e(stage["loop"])
	var lobes strings.Builder
	for _, x := range listOf(stage["lobes"]) {
		fmt.Fprintf(&lobes, `<span class="badge">%s</span>`, e(x))
	}
	if skipped, _ := stage["skipped"].(bool); skipped {
		return fmt.Sprintf(`<div class="stage"><div class="h">%s <span class=empty>(skipped)</span></div></div>`, name)
	}
	var steps strings.Builder
	for _, st := range stepsOf(stage["steps"]) {
		switch st["kind"] {
		case "thinking":
			fmt.Fprintf(&steps, `<div class="step think"><span class=lbl>think</span><span class="t">%s</span></div>`, e(trunc(st["text"], 240)))
		case "tool_use":
			fmt.Fprintf(&steps, `<div class="step tool"><span class=lbl>&rarr; tool</span><span class="t mono">%s(%s)</span></div>`, e(st["name"]), e(trunc(st["input"], 110)))
		case "tool_result":
			fmt.Fprintf(&steps, `<div class="step result"><span class=lbl>&larr; result</span><span class="t mono">%s</span></div>`, e(trunc(st["output"], 160)))
		case "answer":
			if txt, _ := st["text"].(string); txt != "" {
				fmt.Fprintf(&steps, `<div class="step answer"><span class=lbl>answer</span><span class="t">%s</span></div>`, e(trunc(txt, 300)))
			}
		}
	}
	body := steps.String()
	if body == "" {
		body = `<div class="step"><span class=empty>(no LLM step)</span></div>`
	}
	return fmt.Sprintf(`<div class="stage"><div class="h">%s <span class="badge">%s</span> %s</div>%s</div>`, name, loop, lobes.String(), body)
}

func runnerUp(path map[string]any) string {
	ru := mapOf(path["runner_up"])
	if name, _ := ru["name"].(string); name != "" {
		return fmt.Sprintf(` <span class="meta mono">(runner-up: %s %.2f)</span>`, e(name), floatOf(ru["score"]))
	}
	return ""
}

func rawJSON(p *probe.Record) string {
	blob, _ := json.MarshalIndent(p.ToJSON(), "", "  ")
	return "<details><summary>raw JSON</summary><pre class=mono>" + e(string(blob)) + "</pre></details>"
}

func probeBlock(p *probe.Record) string {
	statusCls := "bad"
	if p.Status == "answered" {
		statusCls = "ok"
	}
	toks := intOf(p.Usage["output_tokens"])
	var seqParts []string
	for _, s := range p.Stages {
		seqParts = append(seqParts, e(s["stage"]))
	}
	seq := strings.Join(seqParts, " → ")
	if seq == "" {
		seq = "—"
	}
	head := fmt.Sprintf(`<summary>%s<span class="meta"><span class="badge flow">%s %.2f</span>%s <span class="%s">%s</span> · %d out-tok · %d tool calls</span></summary>`,
		e(p.Label), e(p.Flow), p.FlowScore, runnerUp(p.Path), statusCls, e(p.Status), toks, len(p.ToolCalls))
	var body string
	if p.Error != "" {
		body = `<div class="fail">` + e(p.Error) + `</div>`
	} else {
		var timeline strings.Builder
		for _, s := range p.Stages {
			timeline.WriteString(stageBlock(s))
		}
		lobes := "<details><summary>lobe activation (OY)</summary>" + lobeTable(p.Lobes) + "</details>"
		ans := ""
		if p.Answer != "" {
			ans = `<div class="ans">` + e(trunc(p.Answer, 1200)) + `</div>`
		}
		body = fmt.Sprintf(`<div class="mono" style="color:var(--muted);margin:6px 0">flow (OX): %s</div>%s%s%s%s%s`,
			seq, timeline.String(), lobes, hotspots(p.Hints), ans, rawJSON(p))
	}
	return "<details open>" + head + body + "</details>"
}

func overview(verdict *Verdict, modes map[string]ModePayload) string {
	if verdict == nil {
		return ""
	}
	status := verdict.Status
	if status == "" {
		status = "?"
	}
	var parts strings.Builder
	fmt.Fprintf(&parts, `<div class="verdict %s">%s</div>`, status, e(status))
	var reasons []string
	for _, r := range verdict.Reasons {
		if r != "" {
			reasons = append(reasons, e(r))
		}
	}
	if len(reasons) > 0 {
		parts.WriteString(`<div class="reasons">` + strings.Join(reasons, "<br>") + `</div>`)
	}
	if len(verdict.Metrics) > 0 {
		keys := sortedKeys(verdict.Metrics)
		var pills strings.Builder
		for _, k := range keys {
			fmt.Fprintf(&pills, `<div class="pill"><div class="k">%s</div><div class="v">%s</div></div>`, e(k), e(verdict.Metrics[k]))
		}
		parts.WriteString(`<div class="cards">` + pills.String() + `</div>`)
	}
	for _, mode := range sortedModeKeys(modes) {
		payload := modes[mode]
		checks := payload.Checks
		n := payload.N
		if n == 0 {
			n = len(checks)
		}
		npass := payload.Pass
		var rows strings.Builder
		for _, c := range checks {
			tick := "✗"
			if c.OK {
				tick = "✓"
			}
			diag := ""
			if c.Diag {
				diag = "diag"
			}
			fmt.Fprintf(&rows, "<tr><td>%s</td><td class=mono>%s</td><td>%s</td><td class=diag>%s</td></tr>",
				tick, e(c.ID), e(trunc(c.Detail, 110)), diag)
		}
		cls := "bad"
		open := " open"
		if payload.AllPass {
			cls = "ok"
			open = ""
		}
		fmt.Fprintf(&parts, `<details%s><summary class="%s">%s <span class="meta">%d/%d</span></summary><table><tr><th></th><th>check</th><th>detail</th><th></th></tr>%s</table></details>`,
			open, cls, e(mode), npass, n, rows.String())
	}
	return parts.String()
}

func tabbed(tabs [][2]string) string {
	var rules, inputs, nav, panels strings.Builder
	for i := range tabs {
		fmt.Fprintf(&rules, "#tab%d:checked~.tabnav label[for=tab%d]{color:var(--ink);background:var(--card);border-color:var(--line)}#tab%d:checked~#panel%d{display:block}", i, i, i, i)
		checked := ""
		if i == 0 {
			checked = " checked"
		}
		fmt.Fprintf(&inputs, "<input class=tabradio type=radio name=rtabs id=tab%d%s>", i, checked)
	}
	nav.WriteString("<nav class=tabnav>")
	for i, t := range tabs {
		fmt.Fprintf(&nav, "<label class=tablabel for=tab%d>%s</label>", i, e(t[0]))
	}
	nav.WriteString("</nav>")
	for i, t := range tabs {
		fmt.Fprintf(&panels, "<div class=tabpanel id=panel%d>%s</div>", i, t[1])
	}
	return "<style>" + rules.String() + "</style><div class=tabs>" + inputs.String() + nav.String() + panels.String() + "</div>"
}

// RenderHTML renders the single self-contained HTML report string. Positional
// args may be a Report, Probes (or []*probe.Record), or any Option
// (WithVerdict / WithModes / WithGeneratedAt). Mirrors render_html.
func RenderHTML(title string, args ...any) string {
	o := &opts{}
	for _, a := range args {
		o.applyArg(a)
	}
	ts := o.generatedAt
	if ts == "" {
		ts = time.Now().UTC().Format("2006-01-02 15:04 UTC")
	}
	var ov []string
	if o.verdict != nil {
		ov = append(ov, overview(o.verdict, o.modes))
	}
	ov = append(ov, scorecard(o.report, o.probes))
	if o.report != nil {
		ov = append(ov, "<h2>Scenarios (routing &amp; behavior)</h2>")
		ov = append(ov, scenarios(o.report))
	}
	var overviewHTML strings.Builder
	for _, p := range ov {
		if p != "" {
			overviewHTML.WriteString(p)
		}
	}

	var parts strings.Builder
	parts.WriteString("<!doctype html><html><head><meta charset='utf-8'>")
	fmt.Fprintf(&parts, "<title>%s</title><style>%s</style></head><body><div class='wrap'>", e(title), css)
	fmt.Fprintf(&parts, "<h1>%s</h1><div class='sub'>agent_sdk benchmark · %s</div>", e(title), e(ts))
	if len(o.probes) > 0 {
		var traces strings.Builder
		traces.WriteString("<h2>Probes (real turn internals)</h2>")
		for _, p := range o.probes {
			traces.WriteString(probeBlock(p))
		}
		parts.WriteString(tabbed([][2]string{
			{"Overview", overviewHTML.String()},
			{fmt.Sprintf("Traces (%d)", len(o.probes)), traces.String()},
		}))
	} else {
		parts.WriteString(overviewHTML.String())
	}
	parts.WriteString("</div></body></html>")
	return parts.String()
}

// WriteHTML writes the report to path (creating parent dirs). Returns the path.
// Mirrors write_html.
func WriteHTML(path, title string, args ...any) (string, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(path, []byte(RenderHTML(title, args...)), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// ── small typed accessors over the wire-shaped map[string]any payloads ──

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

func floatOf(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	}
	return 0
}

func mapOf(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func listOf(v any) []any {
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
	return nil
}

func stepsOf(v any) []map[string]any {
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
	return nil
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedModeKeys(m map[string]ModePayload) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
