package benchmarks

import (
	"context"
	"fmt"
	"sort"

	"github.com/mezon/agent-sdk-go/agent_sdk/agent"
	"github.com/mezon/agent-sdk-go/agent_sdk/clients"
	"github.com/mezon/agent-sdk-go/agent_sdk/core/attention"
)

// attentionbench — the bench for the SDK's CONTEXT axis (OY: what the agent
// attends to). Certifies node selection (relevant context outranks flooders,
// traps drop below the floor via score_relevance) and lobe activation (recall
// always-on, grounding lobes on grounded paths, the reply lobe always) read via
// the no-LLM Inspect. FREE / deterministic. Ported from benchmarks/attentionbench/run.py.

// recall lobes fire every turn.
var attnRecall = []string{"memory_recall", "session_recall", "ctxvar_resolve"}

type attnScenario struct {
	id     string
	q      string
	want   []string
	absent []string
}

var attnSCN = []attnScenario{
	{id: "relational", q: "hello there!", want: []string{"synthesize", "respond"}, absent: []string{"cite", "filter"}},
	{id: "qna", q: "what is the capital of France?", want: []string{"classify", "synthesize", "cite", "filter", "respond"}},
	{id: "research", q: "compare React and Vue in depth and cite sources", want: []string{"synthesize", "cite", "filter", "respond"}},
}

func ck(id string, ok bool, detail string) Check { return Check{ID: id, OK: ok, Detail: detail} }

func attnAgent() *agent.PreactAgent {
	return agent.MustPreactAgent(agent.Config{
		Client:       clients.NewFakeClient([]any{"ok"}, nil),
		Instructions: "You are a research assistant.",
	})
}

func attnActive(a *agent.PreactAgent, q string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, lb := range a.Inspect(q).Lobes {
		if act, _ := lb["activated"].(bool); act {
			if id, ok := lb["id"].(string); ok {
				out[id] = struct{}{}
			}
		}
	}
	return out
}

// run_select: relevant outranks flooders; traps drop below the floor.
func attnRunSelect() *ModePayload {
	query := "refund policy for enterprise customers"
	relevant := "Enterprise refund policy: full refund within 14 days for enterprise customers."
	flooders := []string{
		"The weather in Paris is mild this week.",
		"Our office hours are nine to five.",
		"Cats are small domesticated mammals.",
		"The meeting room is on the third floor.",
		"Quarterly revenue grew in the spring.",
	}
	floor := 0.05
	rel := attention.ScoreText(query, nil, relevant, nil, attention.DefaultNodeWeights, 0).Activation
	topFlood := 0.0
	allBelow := true
	for _, f := range flooders {
		a := attention.ScoreText(query, nil, f, nil, attention.DefaultNodeWeights, 0).Activation
		if a > topFlood {
			topFlood = a
		}
		if a >= floor {
			allBelow = false
		}
	}
	checks := []Check{
		ck("select.relevant_outranks", rel > topFlood, fmt.Sprintf("relevant=%.3f > top_flood=%.3f", rel, topFlood)),
		ck("select.relevant_above_floor", rel >= floor, fmt.Sprintf("relevant=%.3f ≥ %v", rel, floor)),
		ck("select.flooders_below_floor", allBelow, fmt.Sprintf("max flooder=%.3f < %v", topFlood, floor)),
	}
	return NewPayload(checks, map[string]any{"relevant": round3(rel), "top_flood": round3(topFlood)})
}

func attnRunRecall(a *agent.PreactAgent) *ModePayload {
	checks := []Check{}
	for _, s := range attnSCN {
		act := attnActive(a, s.q)
		missing := []string{}
		for _, r := range attnRecall {
			if _, ok := act[r]; !ok {
				missing = append(missing, r)
			}
		}
		sort.Strings(missing)
		checks = append(checks, ck("recall."+s.id, len(missing) == 0, fmt.Sprintf("recall⊆active? missing=%v", missing)))
	}
	return NewPayload(checks, nil)
}

func attnRunGrounding(a *agent.PreactAgent) *ModePayload {
	checks := []Check{}
	for _, s := range attnSCN {
		act := attnActive(a, s.q)
		wantOK := subset(s.want, act)
		absentOK := disjoint(s.absent, act)
		checks = append(checks, ck("grounding."+s.id, wantOK && absentOK,
			fmt.Sprintf("want⊆active=%v absent_clear=%v active=%v", wantOK, absentOK, sortedSet(act))))
	}
	return NewPayload(checks, nil)
}

func attnRunReply(a *agent.PreactAgent) *ModePayload {
	respondAlways := true
	for _, s := range attnSCN {
		if _, ok := attnActive(a, s.q)["respond"]; !ok {
			respondAlways = false
		}
	}
	_, classifyQna := attnActive(a, "what is the capital of France?")["classify"]
	checks := []Check{
		ck("reply.respond_always", respondAlways, "the reply lobe fires every turn"),
		ck("reply.classify_on_qna", classifyQna, "classify lights on the qna router"),
	}
	return NewPayload(checks, nil)
}

func attnRunDeterminism(a *agent.PreactAgent) *ModePayload {
	checks := []Check{}
	for _, s := range attnSCN {
		same := setEqual(attnActive(a, s.q), attnActive(a, s.q))
		checks = append(checks, ck("determinism."+s.id, same, "stable activation across two inspects"))
	}
	return NewPayload(checks, nil)
}

// RunAttentionBench composes the attentionbench verdict.
func RunAttentionBench(_ context.Context, _ string) (Verdict, error) {
	a := attnAgent()
	payloads := map[string]*ModePayload{
		"select":      attnRunSelect(),
		"recall":      attnRunRecall(a),
		"grounding":   attnRunGrounding(a),
		"reply":       attnRunReply(a),
		"determinism": attnRunDeterminism(a),
	}
	return ComposeVerdict(payloads, map[string][]string{
		"grounding": {"lobe_recall"}, "select": {"relevant"},
	}), nil
}
