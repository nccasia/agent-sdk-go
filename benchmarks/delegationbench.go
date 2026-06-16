package benchmarks

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/mezon/agent-sdk-go/agent_sdk/agent"
	"github.com/mezon/agent-sdk-go/agent_sdk/plugins/tasks"
	"github.com/mezon/agent-sdk-go/agent_sdk/probe"
)

// delegationbench — the LIVE benchmark for plan-driven fan-out (docs 08/12). On
// rich, realistic, multi-faceted queries that a single-shot answer handles worse
// than a planned one, it runs the REAL agent (plan → supervise → execute →
// fanin) and measures the whole loop:
//
//   - planning precision/recall — did the agent write a plan (TodoWrite) when —
//     and only when — the task warranted it? (the simple / near-neighbor
//     scenarios guard against over-planning.)
//   - execution coverage — on should-plan cases that planned ≥2 steps, was every
//     planned piece SOLVED? fanout/sequential structures run a subagent per todo
//     (checked against blackboard["todos_results"]); inline has the main agent
//     work the list in its own stage (checked by a completed answer).
//   - fan-in fidelity — did every facet land in the combined final answer?
//
// delegationbench is LIVE only: every check needs a real agent + provider, so
// the bench is composed as a SINGLE mode "live". Without a provider (the
// deterministic floor) the mode is MISSING → the verdict is UNMEASURED (no
// evidence is never READY) — mirroring run.py's refusal to run without a
// provider token (payloads["live"] = None). Ported from
// benchmarks/delegationbench/run.py.

//go:embed delegationbench_dataset/scenarios.jsonl
var delegationBenchData embed.FS

// Gating thresholds (see METHOD.md).
const (
	delegationBenchPrecision = 0.80
	delegationBenchRecall    = 0.70
	delegationBenchFidelity  = 0.70
	delegationBenchExec      = 0.70
)

// delegationBenchScenario is one line of dataset/scenarios.jsonl.
type delegationBenchScenario struct {
	ID             string
	Category       string
	Query          string
	Want           bool     // expect.delegate — "this query warrants a plan"
	AnswerContains []string // expect.answer_contains — the facet contract
}

// delegationBenchScenarios loads the committed scenario dataset. Mirrors
// run.py:_scenarios().
func delegationBenchScenarios() ([]delegationBenchScenario, error) {
	raw, err := delegationBenchData.ReadFile("delegationbench_dataset/scenarios.jsonl")
	if err != nil {
		return nil, err
	}
	out := []delegationBenchScenario{}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var row struct {
			ID       string `json:"id"`
			Category string `json:"category"`
			Query    string `json:"query"`
			Expect   struct {
				Delegate       bool     `json:"delegate"`
				AnswerContains []string `json:"answer_contains"`
			} `json:"expect"`
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, err
		}
		out = append(out, delegationBenchScenario{
			ID: row.ID, Category: row.Category, Query: row.Query,
			Want: row.Expect.Delegate, AnswerContains: row.Expect.AnswerContains,
		})
	}
	return out, nil
}

// delegationBenchStaticCheckIDs is the static, provider-independent check-id
// surface: one per scenario (live.fanin.{id} for should-plan-with-facets,
// live.decision.{id} otherwise) plus the four aggregate ids. The per-scenario
// live.exec.{id} ids are runtime-conditional on the plan width (≥2 steps), so
// they are NOT part of the static floor. Asserted for cross-language parity
// independent of the provider.
func delegationBenchStaticCheckIDs() []string {
	scns, err := delegationBenchScenarios()
	if err != nil {
		return nil
	}
	out := []string{}
	for _, s := range scns {
		if s.Want && len(s.AnswerContains) > 0 {
			out = append(out, "live.fanin."+s.ID)
		} else {
			out = append(out, "live.decision."+s.ID)
		}
	}
	out = append(out,
		"live.planning.precision", "live.planning.recall",
		"live.exec.coverage", "live.fanin.fidelity")
	return out
}

// RunDelegationBench composes the delegationbench verdict. With no model (the
// deterministic floor) the single "live" mode is missing → UNMEASURED. With a
// model the real PreactAgent is driven over each scenario and the plan/exec/
// fanin loop scored.
func RunDelegationBench(ctx context.Context, model string) (Verdict, error) {
	var payload *ModePayload
	if model != "" {
		p, err := delegationBenchLive(ctx, model)
		if err != nil {
			return Verdict{}, err
		}
		payload = p
	}
	payloads := map[string]*ModePayload{"live": payload}
	record := map[string][]string{"live": {
		"planning_precision", "planning_recall", "execution_coverage",
		"fanin_fidelity", "avg_plan_steps", "avg_subagents",
	}}
	return ComposeVerdict(payloads, record), nil
}

// RunDelegationBenchProbes captures inspectable traces for the viewer. With a
// real model it drives the actual planning agent; offline (model=="") it builds
// the SAME representative agent (TaskPlugin mounted, the research flow dropped so
// a complex query routes to the plan flow) against a FakeClient and runs ONE
// representative scenario through probe.Probe, so the inspection renders a real
// path/flow + the executed stages. Adds traces only — the live verdict (Run)
// stays UNMEASURED without a provider. Mirrors run.py's scenario probe feeding
// write_viewer.
func RunDelegationBenchProbes(ctx context.Context, model string) ([]*probe.Record, error) {
	scns, err := delegationBenchScenarios()
	if err != nil {
		return nil, err
	}
	if len(scns) == 0 {
		return nil, nil
	}
	ag := agent.MustPreactAgent(agent.Config{
		Client:       benchProbeClient(model),
		Instructions: delegationBenchInstr,
		Plugins:      []any{tasks.NewTaskPlugin(), delegationBenchNoResearch{}},
	})
	rec, err := probe.Probe(ctx, ag, scns[0].Query, probe.WithLabel(scns[0].ID))
	if err != nil {
		return nil, err
	}
	return []*probe.Record{rec}, nil
}

// delegationBenchInstr is the planning system prompt. Mirrors the run.py agent
// instructions.
const delegationBenchInstr = "Answer fully and accurately. When a task has several distinct parts, " +
	"plan it with the TodoWrite tool — one todo per part, each with its own prompt and tools — then " +
	"let each part run as its own subagent and combine their results into one answer."

// delegationBenchLive drives the real agent over every scenario and scores the
// plan decision (precision/recall), execution coverage, and fan-in fidelity.
// Mirrors run.py:run_live().
func delegationBenchLive(ctx context.Context, model string) (*ModePayload, error) {
	scns, err := delegationBenchScenarios()
	if err != nil {
		return nil, err
	}
	// Mount the planning surface; drop the research flow so a no-KB agent routes
	// a complex query to the plan flow (per METHOD.md / run.py _NoResearch).
	ag := agent.MustPreactAgent(agent.Config{
		Client:       model,
		Instructions: delegationBenchInstr,
		Plugins:      []any{tasks.NewTaskPlugin(), delegationBenchNoResearch{}},
	})

	checks := []Check{}
	var should, did []bool
	var widths, subagentCounts []int
	fidelityHits, fidelityTotal := 0, 0
	execHits, execTotal := 0, 0

	for _, s := range scns {
		rec, perr := probe.Probe(ctx, ag, s.Query, probe.WithLabel(s.ID))
		if perr != nil {
			return nil, perr
		}
		width := delegationBenchPlanned(rec)
		planned := width > 0
		should = append(should, s.Want)
		did = append(did, planned)
		subs := delegationBenchSubagents(rec)
		if planned {
			widths = append(widths, width)
			subagentCounts = append(subagentCounts, len(subs))
		}

		// Execution coverage: when the agent plans (≥2 steps) on a should-plan
		// case, every planned piece must be SOLVED — by a subagent
		// (fanout/sequential) or by the main agent itself (inline).
		if s.Want && width >= 2 {
			execTotal++
			structure := delegationBenchStructure(rec, len(subs))
			var solved bool
			if structure == "fanout" || structure == "sequential" {
				solved = len(subs) >= delegationBenchMax2(width-1)
			} else { // inline — the main agent worked the list in its own stage
				solved = rec.Status == "answered"
			}
			if solved {
				execHits++
			}
			checks = append(checks, ck("live.exec."+s.ID, solved,
				fmt.Sprintf("structure=%s todos=%d subagents=%d", structure, width, len(subs))))
		}

		if s.Want && len(s.AnswerContains) > 0 { // fan-in fidelity on should-plan cases with a facet contract
			fidelityTotal++
			covered := delegationBenchCovers(rec.Answer, s.AnswerContains)
			if covered {
				fidelityHits++
			}
			facetState := "MISS"
			if covered {
				facetState = "all"
			}
			checks = append(checks, ck("live.fanin."+s.ID, covered && rec.Status == "answered",
				fmt.Sprintf("planned=%v facets=%s", planned, facetState)))
		} else {
			checks = append(checks, ck("live.decision."+s.ID, planned == s.Want,
				fmt.Sprintf("want=%v planned=%v steps=%d", s.Want, planned, width)))
		}
	}

	precision, recall := delegationBenchPR(should, did)
	fidelity := 1.0
	if fidelityTotal > 0 {
		fidelity = float64(fidelityHits) / float64(fidelityTotal)
	}
	execution := 1.0
	if execTotal > 0 {
		execution = float64(execHits) / float64(execTotal)
	}
	checks = append(checks,
		ck("live.planning.precision", precision >= delegationBenchPrecision,
			fmt.Sprintf("%.2f >= %.2f", precision, delegationBenchPrecision)),
		ck("live.planning.recall", recall >= delegationBenchRecall,
			fmt.Sprintf("%.2f >= %.2f", recall, delegationBenchRecall)),
		ck("live.exec.coverage", execution >= delegationBenchExec,
			fmt.Sprintf("%.2f >= %.2f (every planned piece solved: subagent or inline)", execution, delegationBenchExec)),
		ck("live.fanin.fidelity", fidelity >= delegationBenchFidelity,
			fmt.Sprintf("%.2f >= %.2f", fidelity, delegationBenchFidelity)),
	)

	metrics := map[string]any{
		"planning_precision": round3(precision),
		"planning_recall":    round3(recall),
		"execution_coverage": round3(execution),
		"fanin_fidelity":     round3(fidelity),
		"avg_plan_steps":     delegationBenchAvg(widths),
		"avg_subagents":      delegationBenchAvg(subagentCounts),
	}
	return NewPayload(checks, metrics), nil
}

// delegationBenchNoResearch drops the RAG research flow so a no-KB delegation
// agent routes a complex query to the plan flow, not the general research flow
// (per METHOD.md). Mirrors run.py's _NoResearch plugin.
type delegationBenchNoResearch struct{}

func (delegationBenchNoResearch) Name() string { return "no_research" }

func (delegationBenchNoResearch) Install(setup *agent.AgentSetup) { setup.RemoveFlow("research") }

// ── scoring helpers (namespaced by the delegationBench prefix) ───────────────

// delegationBenchPlanned returns the plan steps the agent wrote — the largest
// TodoWrite list it sent (0 ⇒ no plan). Mirrors run.py:_planned().
func delegationBenchPlanned(rec *probe.Record) int {
	best := 0
	for _, tc := range rec.ToolCalls {
		if n, _ := tc["name"].(string); n != "TodoWrite" {
			continue
		}
		in, _ := tc["input"].(map[string]any)
		if in == nil {
			continue
		}
		todos, _ := in["todos"].([]any)
		if len(todos) > best {
			best = len(todos)
		}
	}
	return best
}

// delegationBenchSubagents returns the per-todo subagent result rows the engine
// fanned out (blackboard["todos_results"]), skipping scratchpad cap markers
// (rows without a "status"). Mirrors run.py:_subagents().
func delegationBenchSubagents(rec *probe.Record) []map[string]any {
	out := []map[string]any{}
	raw, _ := rec.Blackboard["todos_results"].([]any)
	for _, r := range raw {
		m, ok := r.(map[string]any)
		if !ok {
			continue
		}
		if status, _ := m["status"].(string); status != "" {
			out = append(out, m)
		}
	}
	return out
}

// delegationBenchStructure reads blackboard["plan_structure"], defaulting to
// "inline" when absent and no subagents ran. Mirrors run.py's
// `rec.blackboard.get("plan_structure") or ("inline" if not subs else "-")`.
func delegationBenchStructure(rec *probe.Record, nSubs int) string {
	if s, _ := rec.Blackboard["plan_structure"].(string); s != "" {
		return s
	}
	if nSubs == 0 {
		return "inline"
	}
	return "-"
}

// delegationBenchPR is the precision/recall of the plan decision against the
// labels. Mirrors run.py:_pr().
func delegationBenchPR(should, did []bool) (float64, float64) {
	tp, fp, fn := 0, 0, 0
	for i := range should {
		s, d := should[i], did[i]
		switch {
		case s && d:
			tp++
		case d && !s:
			fp++
		case s && !d:
			fn++
		}
	}
	precision := 1.0
	if tp+fp > 0 {
		precision = float64(tp) / float64(tp+fp)
	}
	recall := 1.0
	if tp+fn > 0 {
		recall = float64(tp) / float64(tp+fn)
	}
	return precision, recall
}

// delegationBenchCovers reports whether every facet (case-insensitive) appears
// in the answer. Mirrors run.py's `all(f.lower() in answer.lower() …)`.
func delegationBenchCovers(answer string, facets []string) bool {
	a := strings.ToLower(answer)
	for _, f := range facets {
		if !strings.Contains(a, strings.ToLower(f)) {
			return false
		}
	}
	return true
}

func delegationBenchMax2(n int) int {
	if n < 2 {
		return 2
	}
	return n
}

func delegationBenchAvg(xs []int) float64 {
	if len(xs) == 0 {
		return 0
	}
	sum := 0
	for _, x := range xs {
		sum += x
	}
	return math.Round(float64(sum)/float64(len(xs))*100) / 100
}
