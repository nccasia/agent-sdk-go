package benchmarks

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mezon/agent-sdk-go/agent_sdk/agent"
	"github.com/mezon/agent-sdk-go/agent_sdk/clients"
	"github.com/mezon/agent-sdk-go/agent_sdk/flows"
	"github.com/mezon/agent-sdk-go/agent_sdk/probe"
	"github.com/mezon/agent-sdk-go/agent_sdk/session"
	"github.com/mezon/agent-sdk-go/agent_sdk/stores/memory"
)

// flowbench — the deterministic gate for the SDK's FLOW axis (OX): are ALL the
// default flows wired + working? Proves every flow end-to-end with no provider:
// each query ROUTES to the right path + flow-qualified state sequence (read via
// the no-LLM Inspect/InspectWithState), maps to a complexity TIER whose
// grounding contract holds, runs only CANONICAL reasoning STATES in canonical
// order, GROUNDS iff deep, COVERS every default flow, routes DETERMINISTICALLY,
// threads a SUBJECT into the prompt, and actually EXECUTES (a FakeClient probe
// runs the declared stages and answers). FREE / deterministic. Ported from
// benchmarks/flowbench/run.py (verdict: READY, 63/63).

// flowCanonical is the canonical reasoning-state vocabulary + order: every
// default stage id must be one of these, and a flow's states must appear in
// non-decreasing canonical order. Mirrors run.py's _CANONICAL.
var flowCanonical = []string{"understand", "explore", "plan", "act", "synthesize", "cite", "filter", "respond"}

func flowOrder() map[string]int {
	m := map[string]int{}
	for i, s := range flowCanonical {
		m[s] = i
	}
	return m
}

// flowTiers is the complexity spectrum (mirrors run.py's _TIERS).
var flowTiers = map[string]struct{}{"direct": {}, "standard": {}, "deep": {}, "steward": {}}

// flowScenario is one default flow's routing/tier/sequence contract + how to
// trigger it. Mirrors a row of run.py's SCN.
type flowScenario struct {
	id          string
	flow        string
	tier        string
	q           string
	path        string
	seq         []string
	grounded    bool
	warmup      string // seeds clarify's anaphora (a prior info turn)
	configMode  bool   // flags onboarding
	adversarial bool
}

// flowSCN — one entry per default flow + two adversarial near-neighbours.
// Mirrors run.py's SCN list verbatim.
var flowSCN = []flowScenario{
	{id: "relational-hi", flow: "relational", tier: "direct", q: "hello there!",
		path: "relational", seq: []string{"relational:synthesize"}},
	{id: "relational-thanks", flow: "relational", tier: "direct", q: "thanks, that's great",
		path: "relational", seq: []string{"relational:synthesize"}},
	{id: "qna-fact", flow: "qna", tier: "standard", q: "what is the capital of France?",
		path: "qna", seq: []string{"qna:act"}},
	{id: "qna-howto", flow: "qna", tier: "standard", q: "how does TCP congestion control work?",
		path: "qna", seq: []string{"qna:act"}},
	{id: "clarify-followup", flow: "clarify", tier: "standard", q: "what about that one?",
		path: "clarify", seq: []string{"clarify:synthesize"}, warmup: "what is the PTO policy?"},
	{id: "research-compare", flow: "research", tier: "deep",
		q: "compare React and Vue in depth and cite sources", path: "research",
		seq: []string{"research:act", "research:cite", "research:filter"}, grounded: true},
	{id: "research-tradeoffs", flow: "research", tier: "deep",
		q:    "research the tradeoffs of microservices vs monolith across cost, scale and ops",
		path: "research", seq: []string{"research:act", "research:cite", "research:filter"}, grounded: true},
	{id: "fallback-nonsense", flow: "fallback", tier: "standard",
		q: "xyzzy plugh frobnicate the borogoves", path: "emergent", seq: []string{"fallback:act"}},
	{id: "onboarding-steward", flow: "onboarding", tier: "steward",
		q: "set up the knowledge base for the team", path: "onboarding",
		seq: []string{"onboarding:synthesize"}, configMode: true},
	// adversarial near-neighbours — routing must not be fooled.
	{id: "adv-greeting-question", flow: "qna", tier: "standard",
		q: "hi! quick one — what is the capital of Japan?", path: "qna",
		seq: []string{"qna:act"}, adversarial: true},
	{id: "adv-imperative", flow: "fallback", tier: "standard",
		q: "summarize the theory of general relativity for me", path: "emergent",
		seq: []string{"fallback:act"}, adversarial: true},
}

// flowAgent builds the no-provider agent for a scenario (config_mode seeds
// onboarding routing). Mirrors run.py's _agent.
func flowAgent(s flowScenario) *agent.PreactAgent {
	cfg := agent.Config{
		Client:       clients.NewFakeClient([]any{"ok", "ok", "ok", "ok", "ok", "ok", "ok", "ok"}, nil),
		Instructions: "You are a helpful assistant.",
	}
	if s.configMode {
		cfg.Context = map[string]any{"config_mode": true}
	}
	return agent.MustPreactAgent(cfg)
}

// flowState seeds the history a flow needs to route — clarify's anaphora needs
// a prior info turn. Mirrors run.py's _state.
func flowState(s flowScenario) session.SessionState {
	if s.warmup != "" {
		return session.SessionState{History: []session.Turn{
			{Role: "user", Content: s.warmup},
			{Role: "assistant", Content: "It is 20 days."},
		}}
	}
	return session.SessionState{}
}

// flowStatesOf strips the flow qualifier from a sequence (research:act → act).
func flowStatesOf(seq []string) []string {
	out := make([]string, len(seq))
	for i, q := range seq {
		if idx := strings.LastIndex(q, ":"); idx >= 0 {
			out[i] = q[idx+1:]
		} else {
			out[i] = q
		}
	}
	return out
}

// flowRunRouting: query → right path + flow-qualified state sequence.
func flowRunRouting() *ModePayload {
	checks := []Check{}
	for _, s := range flowSCN {
		snap := flowAgent(s).InspectWithState(s.q, flowState(s))
		ok := snap.Path.Name == s.path && equal(snap.Flow, s.seq)
		checks = append(checks, ck("routing."+s.id, ok,
			fmt.Sprintf("path=%s flow=%v", snap.Path.Name, snap.Flow)))
	}
	return NewPayload(checks, nil)
}

// flowRunTiers: each flow maps to a tier, and the tier fixes the grounding
// contract (only deep grounds); the whole spectrum is represented.
func flowRunTiers() *ModePayload {
	checks := []Check{}
	covered := map[string]struct{}{}
	for _, s := range flowSCN {
		covered[s.tier] = struct{}{}
		states := flowStatesOf(s.seq)
		grounded := tailEq(states, []string{"cite", "filter"})
		_, inTiers := flowTiers[s.tier]
		ok := inTiers && (grounded == (s.tier == "deep"))
		checks = append(checks, ck("tiers."+s.id, ok, fmt.Sprintf("tier=%s grounded=%t", s.tier, grounded)))
	}
	checks = append(checks, ck("tiers.spectrum_covered", subsetSet(flowTiers, covered),
		fmt.Sprintf("covered=%v", sortedSet(covered))))
	return NewPayload(checks, map[string]any{"tiers": sortedSet(covered)})
}

// flowRunStates: canonical vocabulary + canonical order.
func flowRunStates() *ModePayload {
	order := flowOrder()
	checks := []Check{}
	for _, s := range flowSCN {
		states := flowStatesOf(s.seq)
		inVocab := true
		for _, st := range states {
			if _, ok := order[st]; !ok {
				inVocab = false
			}
		}
		ordered := true
		for i := 0; i+1 < len(states); i++ {
			if order[states[i]] > order[states[i+1]] {
				ordered = false
			}
		}
		checks = append(checks, ck("states."+s.id, inVocab && ordered,
			fmt.Sprintf("states=%v vocab=%t ordered=%t", states, inVocab, ordered)))
	}
	return NewPayload(checks, map[string]any{"canonical": flowCanonical})
}

// flowRunGrounding: deep grounds (cite→filter); social/standard do not.
func flowRunGrounding() *ModePayload {
	checks := []Check{}
	for _, s := range flowSCN {
		states := flowStatesOf(s.seq)
		hasGround := tailEq(states, []string{"cite", "filter"})
		ok := hasGround
		if !s.grounded {
			ok = !contains(states, "cite") && !contains(states, "filter")
		}
		checks = append(checks, ck("grounding."+s.id, ok,
			fmt.Sprintf("grounded=%t tail=%v", s.grounded, tail2(states))))
	}
	return NewPayload(checks, nil)
}

// flowRunCoverage: every default flow is exercised by at least one scenario.
func flowRunCoverage() *ModePayload {
	a := flowAgent(flowScenario{})
	defined := map[string]struct{}{}
	for _, f := range a.Spec().Flows {
		if id, ok := f["id"].(string); ok {
			defined[id] = struct{}{}
		}
	}
	covered := map[string]struct{}{}
	for _, s := range flowSCN {
		covered[s.flow] = struct{}{}
	}
	missing := []string{}
	for d := range defined {
		if _, ok := covered[d]; !ok {
			missing = append(missing, d)
		}
	}
	sort.Strings(missing)
	nCovered := 0
	for d := range defined {
		if _, ok := covered[d]; ok {
			nCovered++
		}
	}
	checks := []Check{ck("coverage.all_flows_tested", len(missing) == 0,
		fmt.Sprintf("defined=%v untested=%v", sortedSet(defined), missing))}
	return NewPayload(checks, map[string]any{
		"flows_defined": len(defined), "flows_covered": nCovered})
}

// flowRunDeterminism: routing is a pure function of (spec, context).
func flowRunDeterminism() *ModePayload {
	checks := []Check{}
	for _, s := range flowSCN[:4] {
		a := flowAgent(s)
		x := a.InspectWithState(s.q, flowState(s))
		y := a.InspectWithState(s.q, flowState(s))
		ok := x.Path.Name == y.Path.Name && equal(x.Flow, y.Flow)
		checks = append(checks, ck("determinism."+s.id, ok, "identical across two inspects"))
	}
	return NewPayload(checks, nil)
}

// flowRunSubject: a state instantiated against a subject threads it into the
// composed prompt (rendered as its own <subject> section).
func flowRunSubject() *ModePayload {
	a := agent.MustPreactAgent(agent.Config{
		Client: clients.NewFakeClient([]any{"ok"}, nil), Instructions: "x"})
	subj := "aspect-3: licensing differences"
	st := flows.FlowStep{Name: "act", Lobes: []string{"synthesize"}, Loop: "agentic", Subject: &subj}
	sysp := a.Engine().ComposeSystem(st, nil)
	checks := []Check{
		ck("subject.threaded", strings.Contains(sysp, subj), "subject text in prompt"),
		ck("subject.tagged", strings.Contains(sysp, "<subject>"), "subject rendered as its own <subject> section"),
	}
	return NewPayload(checks, nil)
}

// flowRunExecution: each flow actually RUNS its declared stages and answers.
func flowRunExecution(ctx context.Context) *ModePayload {
	checks := []Check{}
	for _, s := range flowSCN {
		store := memory.NewSessionStoreInMemory()
		cfg := agent.Config{
			Client:       clients.NewFakeClient([]any{"ok", "ok", "ok", "ok", "ok", "ok", "ok", "ok"}, nil),
			Instructions: "You are a helpful assistant.",
			Session:      session.New(s.id, store),
		}
		if s.configMode {
			cfg.Context = map[string]any{"config_mode": true}
		}
		a := agent.MustPreactAgent(cfg)
		if s.warmup != "" {
			if _, err := probe.Probe(ctx, a, s.warmup, probe.WithLabel(s.id+"·warmup")); err != nil {
				checks = append(checks, ck("execution."+s.id, false, "warmup: "+err.Error()))
				continue
			}
		}
		rec, err := probe.Probe(ctx, a, s.q, probe.WithLabel(s.id))
		if err != nil {
			checks = append(checks, ck("execution."+s.id, false, err.Error()))
			continue
		}
		ran := []string{}
		for _, st := range rec.Stages {
			if name, ok := st["stage"].(string); ok {
				ran = append(ran, name)
			}
		}
		ok := equal(ran, s.seq) && (rec.Status == "answered" || rec.Status == "refused")
		checks = append(checks, ck("execution."+s.id, ok, fmt.Sprintf("ran=%v status=%s", ran, rec.Status)))
	}
	return NewPayload(checks, nil)
}

// flowPayloads runs every flowbench mode (the deterministic floor). Shared by
// RunFlowBench and the check-id parity test.
func flowPayloads(ctx context.Context) map[string]*ModePayload {
	return map[string]*ModePayload{
		"routing":     flowRunRouting(),
		"tiers":       flowRunTiers(),
		"states":      flowRunStates(),
		"grounding":   flowRunGrounding(),
		"coverage":    flowRunCoverage(),
		"determinism": flowRunDeterminism(),
		"subject":     flowRunSubject(),
		"execution":   flowRunExecution(ctx),
	}
}

// RunFlowBench composes the flowbench verdict (deterministic floor). Routing is
// a pure function of (spec, context) ⇒ READY. Mirrors run.py's compose_verdict.
func RunFlowBench(ctx context.Context, _ string) (Verdict, error) {
	return ComposeVerdict(flowPayloads(ctx), map[string][]string{
		"coverage": {"flows_defined", "flows_covered"},
		"tiers":    {"tiers"},
	}), nil
}

// ── helpers (namespaced by the flow prefix where they would collide) ─────────

// tailEq reports whether the last len(want) elements of states equal want.
func tailEq(states, want []string) bool {
	if len(states) < len(want) {
		return false
	}
	return equal(states[len(states)-len(want):], want)
}

// tail2 returns the last (≤2) elements of states (for the grounding detail).
func tail2(states []string) []string {
	if len(states) <= 2 {
		return states
	}
	return states[len(states)-2:]
}

// subsetSet reports whether every key of want is present in have.
func subsetSet(want, have map[string]struct{}) bool {
	for k := range want {
		if _, ok := have[k]; !ok {
			return false
		}
	}
	return true
}
