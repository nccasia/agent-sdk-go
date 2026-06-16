// Translated from tests/test_finalize_hook.py — the finalize + tool-result
// hook seams a plugin's grounding/citation contract.
//
// The engine carries no citation logic of its own; a plugin (RagPlugin) owns
// it via “AddFinalizeHook“ (rewrite the answer / replace citations / force
// a refusal) and “AddToolResultHook“ (extract citations a tool emits).
// These lock the seam in: hooks run, can mutate the result, and the default
// (no-hook) path is unchanged.
package agent

import (
	"context"
	"testing"

	"github.com/mezon/agent-sdk-go/agent_sdk/clients"
	"github.com/mezon/agent-sdk-go/agent_sdk/contracts"
)

// finalizeProbe is a test plugin that records what the engine hands its
// finalize hook, then optionally rewrites / adds / refuses.
type finalizeProbe struct {
	name        string
	rewrite     string
	addCitation *contracts.Citation
	refuse      bool
	seen        []finalizeCall
}

type finalizeCall struct {
	answer           string
	citations        []contracts.Citation
	grounds          bool
	requireCitations bool
}

func (p *finalizeProbe) Name() string { return p.name }
func (p *finalizeProbe) Install(setup *AgentSetup) {
	setup.AddFinalizeHook(func(answer string, citations []contracts.Citation, chunks []map[string]any, grounds, requireCitations bool) (string, []contracts.Citation, string) {
		p.seen = append(p.seen, finalizeCall{
			answer: answer, citations: append([]contracts.Citation(nil), citations...),
			grounds: grounds, requireCitations: requireCitations,
		})
		cites := append([]contracts.Citation(nil), citations...)
		if p.addCitation != nil {
			cites = append(cites, *p.addCitation)
		}
		refusalReason := ""
		if p.refuse {
			refusalReason = "policy_violation"
		}
		if p.rewrite != "" {
			return p.rewrite, cites, refusalReason
		}
		return answer, cites, refusalReason
	})
}

func newTestAgent(plugins ...Plugin) *PreactAgent {
	return MustPreactAgent(Config{
		Client:       clients.NewFakeClient(nil, ptrString("hello world")),
		Instructions: "test",
		Plugins:      plugins,
	})
}

func ptrString(s string) *string { return &s }

// TestFinalizeHookRunsAndSeesTheAnswer mirrors
// test_finalize_hook_runs_and_sees_the_answer.
func TestFinalizeHookRunsAndSeesTheAnswer(t *testing.T) {
	p := &finalizeProbe{name: "finalize_probe"}
	a := newTestAgent(p)
	res, err := a.Query(context.Background(), "hi")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(p.seen) == 0 {
		t.Fatal("finalize hook was never called")
	}
	if p.seen[0].answer != "hello world" {
		t.Errorf("seen[0].answer = %q, want %q", p.seen[0].answer, "hello world")
	}
	if res == nil {
		t.Fatal("nil result")
	}
}

// TestFinalizeHookRewritesTheAnswer mirrors
// test_finalize_hook_rewrites_the_answer.
func TestFinalizeHookRewritesTheAnswer(t *testing.T) {
	a := newTestAgent(&finalizeProbe{name: "fp", rewrite: "REWRITTEN"})
	res, err := a.Query(context.Background(), "hi")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if res.Text != "REWRITTEN" {
		t.Errorf("text = %q, want %q", res.Text, "REWRITTEN")
	}
	if res.Status != "answered" {
		t.Errorf("status = %q, want %q", res.Status, "answered")
	}
}

// TestFinalizeHookCanAddACitation mirrors
// test_finalize_hook_can_add_a_citation.
func TestFinalizeHookCanAddACitation(t *testing.T) {
	cit := contracts.Citation{ChunkID: "c1", SourceRef: "doc://x", SupportingSpan: [2]int{0, 5}}
	a := newTestAgent(&finalizeProbe{name: "fp", addCitation: &cit})
	res, err := a.Query(context.Background(), "hi")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	found := false
	for _, c := range res.Citations {
		if c.ChunkID == "c1" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("citation c1 not found in %+v", res.Citations)
	}
}

// TestFinalizeHookCanForceARefusal mirrors
// test_finalize_hook_can_force_a_refusal.
func TestFinalizeHookCanForceARefusal(t *testing.T) {
	a := newTestAgent(&finalizeProbe{name: "fp", refuse: true})
	res, err := a.Query(context.Background(), "hi")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if res.Status != "refused" {
		t.Errorf("status = %q, want %q", res.Status, "refused")
	}
	if res.Refusal == nil {
		t.Fatal("nil refusal")
	}
	if res.Refusal.Reason != "policy_violation" {
		t.Errorf("refusal.reason = %q, want %q", res.Refusal.Reason, "policy_violation")
	}
}

// TestNoFinalizeHookLeavesTheTurnUnchanged mirrors
// test_no_finalize_hook_leaves_the_turn_unchanged.
func TestNoFinalizeHookLeavesTheTurnUnchanged(t *testing.T) {
	a := newTestAgent() // no plugins
	res, err := a.Query(context.Background(), "hi")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if res.Status != "answered" {
		t.Errorf("status = %q, want %q", res.Status, "answered")
	}
	if res.Text != "hello world" {
		t.Errorf("text = %q, want %q", res.Text, "hello world")
	}
}

// toolResultProbe is a plugin that registers a tool-result citation hook
// (mirrors the Python “_ToolResultPlugin“).
type toolResultProbe struct {
	name string
}

func (p *toolResultProbe) Name() string { return p.name }
func (p *toolResultProbe) Install(setup *AgentSetup) {
	setup.AddToolResultHook(func(toolName, output string) []contracts.Citation {
		return []contracts.Citation{{ChunkID: "from-tool", SourceRef: "t://1", SupportingSpan: [2]int{0, 1}}}
	})
}

// TestToolResultHookSeamRegisters mirrors
// test_tool_result_hook_seam_registers — the engine exposes the seam even
// when no tools run this turn.
func TestToolResultHookSeamRegisters(t *testing.T) {
	a := newTestAgent(&toolResultProbe{name: "trp"})
	if len(a.Engine().ToolResultHooks) != 1 {
		t.Errorf("expected 1 tool_result_hook, got %d", len(a.Engine().ToolResultHooks))
	}
}
