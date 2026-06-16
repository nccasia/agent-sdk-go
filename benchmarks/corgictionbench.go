package benchmarks

import (
	"context"
	"fmt"
	"sort"

	"github.com/mezon/agent-sdk-go/agent_sdk/agent"
	"github.com/mezon/agent-sdk-go/agent_sdk/clients"
	"github.com/mezon/agent-sdk-go/agent_sdk/contracts"
	"github.com/mezon/agent-sdk-go/agent_sdk/flows"
	"github.com/mezon/agent-sdk-go/agent_sdk/inspection"
	mc "github.com/mezon/agent-sdk-go/agent_sdk/metacognition"
	mcplugin "github.com/mezon/agent-sdk-go/agent_sdk/plugins/metacognition"
	"github.com/mezon/agent-sdk-go/agent_sdk/probe"
)

// corgictionbech — the deterministic gate for the SDK's METACOGNITION layer.
// Certifies monitor→regulate self-regulation (the decision table: precedence,
// thresholds), the apply/observe channel, the pinned-step guard, the shipped
// plugin surface, and the Layer-1 plan compiler. FREE / deterministic — no
// provider. Ported from benchmarks/corgictionbech/run.py (+ scoring.py).
//
// Deviation: the plugin-surface mode ports the surface.* checks (which read the
// AgentSetup directly); the enactor checks in the Python scoring.py drive the
// meta tool with an unexported turn state and a `navigate` action the Go
// metacognition package does not export, so they are not part of the Go bench.

func obs(kind, target string, sev float64) mc.MetaObservation {
	return mc.MetaObservation{ID: "t:" + target + ":" + kind, Kind: kind, Target: target,
		Severity: sev, Detail: fmt.Sprintf("%s on %s", kind, target)}
}

func strp(s string) *string { return &s }

// run_monitor: snapshots → the right observations.
func corgRunMonitor() *ModePayload {
	fa := &inspection.FlowAxisSnapshot{Flow: "research", Disabled: false, Steps: []inspection.FlowStepInspection{
		{Flow: "research", Step: "plan", Loop: "single"},
		{Flow: "research", Step: "synthesize", Loop: "single", Lobes: []string{"synthesize"}, Disabled: true},
		{Flow: "research", Step: "cite", Loop: "none", Lobes: []string{"cite"},
			StateNodes: []map[string]any{{"id": "context:tight", "activated": true}}},
	}}
	eng := &inspection.EngineSnapshot{
		Path:      map[string]any{"name": "qna", "score": 0.4, "emergent": false},
		FlowSteps: []map[string]any{{"flow": "research", "step": "research", "node_count": 0}},
	}
	la := &inspection.LobeAxisSnapshot{Lobes: []inspection.LobeInspection{
		{ID: "memory_recall", Layer: 2, Activated: false,
			StateNodes: []map[string]any{{"id": "mem", "activated": false}}},
	}}
	kinds := map[string]struct{}{}
	for _, o := range mc.Monitor(mc.MonitorInput{FlowAxis: fa, Engine: eng, LobeAxis: la}) {
		kinds[o.Kind] = struct{}{}
	}
	want := []string{"context_tight", "empty_lobe_slice", "empty_step_context",
		"inactive_lobe_group", "low_confidence_path", "step_disabled"}
	checks := []Check{}
	for _, k := range want {
		_, ok := kinds[k]
		detail := "MISSING"
		if ok {
			detail = "observed"
		}
		checks = append(checks, ck("monitor."+k, ok, detail))
	}
	return NewPayload(checks, nil)
}

// run_regulate: observations → the decision table (precedence + thresholds).
func corgRunRegulate() *ModePayload {
	type tc struct {
		id    string
		obs   []mc.MetaObservation
		step  string
		lobes []string
		want  string
	}
	cases := []tc{
		{"healthy_continue", nil, "synthesize", []string{"synthesize"}, "continue"},
		{"low_conf_review", []mc.MetaObservation{obs("low_confidence_path", "qna", 0.7)}, "synthesize", nil, "meta_review"},
		{"tight_adjust", []mc.MetaObservation{obs("context_tight", "research.synthesize", 0.75)},
			"synthesize", []string{"synthesize", "memory_recall", "skill_select"}, "adjust_lobe_slice"},
		{"empty_skip", []mc.MetaObservation{obs("empty_lobe_slice", "research.plan", 0.8)}, "plan", []string{"plan"}, "skip_step"},
		{"empty_step_retry", []mc.MetaObservation{obs("empty_step_context", "research.research", 0.65)},
			"research", []string{"research"}, "retry_step"},
		{"precedence_review", []mc.MetaObservation{obs("empty_lobe_slice", "research.plan", 0.8),
			obs("low_confidence_path", "qna", 0.7)}, "plan", []string{"plan"}, "meta_review"},
	}
	flow := "research"
	checks := []Check{}
	for _, c := range cases {
		d := mc.Regulate(c.obs, mc.RegulateInput{TargetFlow: &flow, TargetStep: strp(c.step), CurrentLobes: c.lobes})
		ok := d.Action == c.want
		if c.id == "tight_adjust" {
			ok = ok && !contains(d.TargetLobes, "memory_recall") && contains(d.TargetLobes, "synthesize")
		}
		checks = append(checks, ck("regulate."+c.id, ok, fmt.Sprintf("action=%s (want %s)", d.Action, c.want)))
	}
	return NewPayload(checks, nil)
}

// run_pinned: cite/filter empty slice → meta_review, NEVER skip_step.
func corgRunPinned() *ModePayload {
	checks := []Check{}
	for _, step := range contracts.SortedPinnedLobes() {
		d := mc.Regulate([]mc.MetaObservation{obs("empty_lobe_slice", "qna."+step, 0.8)},
			mc.RegulateInput{TargetFlow: strp("qna"), TargetStep: strp(step), CurrentLobes: nil})
		checks = append(checks, ck("pinned."+step+"_never_skipped", d.Action == "meta_review",
			fmt.Sprintf("action=%s (pinned step must escalate, not skip)", d.Action)))
	}
	return NewPayload(checks, map[string]any{"pinned_steps": len(contracts.PinnedLobes)})
}

// run_channel: apply/observe + the action allowlist.
func corgRunChannel() *ModePayload {
	apply := mc.NewMetaController(mc.ModeApply, false, nil)
	observe := mc.NewMetaController(mc.ModeObserve, false, nil)
	widened := mc.NewMetaController(mc.ModeApply, false,
		map[mc.MetaAction]struct{}{mc.ActionAdjustLobeSlice: {}, mc.ActionSkipStep: {}})
	checks := []Check{
		ck("channel.apply_default_trim", apply.ShouldApply(mc.ActionAdjustLobeSlice),
			"apply mode applies the default trim action"),
		ck("channel.apply_withholds_skip", !apply.ShouldApply(mc.ActionSkipStep),
			"skip_step needs an explicit allowlist (not default)"),
		ck("channel.observe_never_mutates", !observe.ShouldApply(mc.ActionAdjustLobeSlice),
			"observe is the floor — monitors but never mutates"),
		ck("channel.allowlist_widens", widened.ShouldApply(mc.ActionSkipStep),
			"an explicit allowlist enables skip_step"),
	}
	return NewPayload(checks, nil)
}

// run_plugin_surface: the shipped plugin assembles its surface (the surface.*
// contract). See the package deviation note.
func corgRunPluginSurface() *ModePayload {
	setup := agent.NewAgentSetup()
	mcplugin.NewMetacognitionPlugin().Install(setup)
	lset := map[string]struct{}{}
	for _, lb := range setup.Lobes {
		lset[lb.ID] = struct{}{}
	}
	sset := map[string]struct{}{}
	for _, st := range setup.Stages {
		if fs, ok := st.(flows.FlowStep); ok {
			sset[fs.Name] = struct{}{}
		}
	}
	fids := []string{}
	for _, f := range setup.Flows {
		fids = append(fids, f.ID())
	}
	toolNames := []string{}
	for _, rt := range setup.ToolRuntimes {
		if r, ok := rt.(*mcplugin.MetaControlToolRuntime); ok {
			for _, s := range r.GetToolSpecs() {
				if n, ok := s["name"].(string); ok {
					toolNames = append(toolNames, n)
				}
			}
		}
	}
	_, hasMetaContext := lset["meta_context"]
	_, hasNavBrief := lset["nav_brief"]
	_, hasReflect := sset["meta_reflect"]
	checks := []Check{
		ck("surface.lobes", hasMetaContext && hasNavBrief, fmt.Sprintf("lobes=%v", sortedSet(lset))),
		ck("surface.stage", hasReflect, fmt.Sprintf("stages=%v", sortedSet(sset))),
		ck("surface.flow", contains(fids, "meta"), fmt.Sprintf("flows=%v", fids)),
		ck("surface.tool", len(toolNames) == 1 && toolNames[0] == "meta_control", fmt.Sprintf("tools=%v", toolNames)),
	}
	return NewPayload(checks, nil)
}

// run_plan_compile: the Layer-1 plan compiler (plan → dynamic state plan).
func corgRunPlanCompile() *ModePayload {
	one := mc.CompileStatePlan([]any{map[string]any{"id": "main", "question": "what is X?"}}, false)
	three := mc.CompileStatePlan([]any{
		map[string]any{"question": "cost"}, map[string]any{"question": "scale"},
		map[string]any{"question": "ops"}}, true)
	states3 := statesOf(three)
	subjects3 := actSubjects(three)
	synthIdx := indexOf(states3, "synthesize")
	checks := []Check{
		ck("plan.single_no_fanout", equal(statesOf(one), []string{"act"}), fmt.Sprintf("one aspect → %v", statesOf(one))),
		ck("plan.expands_act_per_aspect", len(states3) >= 3 && equal(states3[:3], []string{"act", "act", "act"}),
			fmt.Sprintf("three aspects → %v", states3)),
		ck("plan.subjects_threaded", equal(subjects3, []string{"cost", "scale", "ops"}), fmt.Sprintf("act subjects = %v", subjects3)),
		ck("plan.synthesize_folds", contains(states3, "synthesize") && synthIdx == 3, fmt.Sprintf("states=%v", states3)),
		ck("plan.pinned_grounding_appended", len(states3) >= 2 && equal(states3[len(states3)-2:], []string{"cite", "filter"}),
			fmt.Sprintf("grounded tail = %v", states3[max0(len(states3)-2):])),
		ck("plan.deterministic", equalPlans(
			mc.CompileStatePlan([]any{map[string]any{"question": "a"}, map[string]any{"question": "b"}}, false),
			mc.CompileStatePlan([]any{map[string]any{"question": "a"}, map[string]any{"question": "b"}}, false)),
			"same plan → same compiled states"),
	}
	return NewPayload(checks, nil)
}

// corgProbeScenarios — a small, representative slice of the live hard-problem set
// (a reasoning trap, a logic puzzle, a decomposition prompt). Kept to 3 so
// cmd/bench stays fast. Mirrors run_live's probe loop over the scenarios.jsonl.
var corgProbeScenarios = [][2]string{
	{"trap-bat-ball", "A bat and a ball cost $1.10 in total. The bat costs $1.00 more than the ball. How much does the ball cost? Give the cost of the ball."},
	{"logic-race", "Alice, Bob and Carol finished a race in distinct positions. Alice was not last. Bob was not first. Carol finished ahead of Bob. Who finished first? Answer with the name."},
	{"decompose-budget", "Plan a 3-day trip to Tokyo on a $1500 budget: cover flights, lodging, food and activities, and show the running total."},
}

// RunCorgictionBenchProbes runs the representative hard-problem scenarios through
// probe.Probe against the EQUIPPED agent (MetacognitionPlugin + metacognition
// "apply") — the same agent run_live drives — and returns the captured records.
// Each record carries a real path/flow + the executed stages, so the viewer
// inspection renders turn/path/flow/steps + each stage's system_prompt/segments.
// Offline-deterministic via FakeClient when model=="".
func RunCorgictionBenchProbes(ctx context.Context, _ string) ([]*probe.Record, error) {
	var records []*probe.Record
	for _, sc := range corgProbeScenarios {
		a := agent.MustPreactAgent(agent.Config{
			Client:        clients.NewFakeClient([]any{"ok", "ok", "ok", "ok", "ok", "ok", "ok", "ok"}, nil),
			Instructions:  "You are a careful, methodical reasoner.",
			Plugins:       []agent.Plugin{mcplugin.NewMetacognitionPlugin()},
			Metacognition: "apply",
		})
		rec, err := probe.Probe(ctx, a, sc[1], probe.WithLabel(sc[0]))
		if err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, nil
}

// RunCorgictionBench composes the corgictionbech verdict (deterministic floor).
func RunCorgictionBench(_ context.Context, _ string) (Verdict, error) {
	payloads := map[string]*ModePayload{
		"monitor":        corgRunMonitor(),
		"regulate":       corgRunRegulate(),
		"pinned":         corgRunPinned(),
		"channel":        corgRunChannel(),
		"plugin_surface": corgRunPluginSurface(),
		"plan_compile":   corgRunPlanCompile(),
	}
	return ComposeVerdict(payloads, map[string][]string{"pinned": {"pinned_steps"}}), nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func statesOf(plan []mc.StatePlanStep) []string {
	out := make([]string, len(plan))
	for i, s := range plan {
		out[i] = s.State
	}
	return out
}

func actSubjects(plan []mc.StatePlanStep) []string {
	out := []string{}
	for _, s := range plan {
		if s.State == "act" && s.Subject != nil {
			out = append(out, *s.Subject)
		}
	}
	return out
}

func equalPlans(a, b []mc.StatePlanStep) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].State != b[i].State {
			return false
		}
		if (a[i].Subject == nil) != (b[i].Subject == nil) {
			return false
		}
		if a[i].Subject != nil && *a[i].Subject != *b[i].Subject {
			return false
		}
	}
	return true
}

func indexOf(xs []string, x string) int {
	for i, v := range xs {
		if v == x {
			return i
		}
	}
	return -1
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

var _ = sort.Strings
