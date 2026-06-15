package result

import (
	"testing"

	"github.com/mezon/agent-sdk-go/agent_sdk/contracts"
)

func TestUsageFromProviderAndCost(t *testing.T) {
	u := UsageFromProvider(ProviderUsage{InputTokens: 1_000_000, OutputTokens: 1_000_000}, DefaultCostPerMTok)
	if u.InputTokens != 1_000_000 {
		t.Errorf("InputTokens = %d", u.InputTokens)
	}
	if u.EstimatedCost <= 0 {
		t.Errorf("EstimatedCost = %v", u.EstimatedCost)
	}
}

func TestAgentResultStrAndJSON(t *testing.T) {
	r := AgentResult{
		Text:      "hi",
		Citations: []contracts.Citation{{ChunkID: "c", SourceRef: "s", SupportingSpan: [2]int{0, 1}}},
	}
	if r.String() != "hi" {
		t.Errorf("String = %q", r.String())
	}
	j := r.ToJSON()
	if j["status"] != "answered" {
		t.Errorf("status = %v", j["status"])
	}
	cits, _ := j["citations"].([]map[string]any)
	if len(cits) == 0 || cits[0]["chunk_id"] != "c" {
		t.Errorf("citations = %v", j["citations"])
	}
}

func TestRefusedResult(t *testing.T) {
	r := AgentResult{
		Text:    "",
		Status:  "refused",
		Refusal: &Refusal{Reason: "no_citations", Message: "cannot confirm"},
	}
	j := r.ToJSON()
	ref, _ := j["refusal"].(map[string]any)
	if ref["reason"] != "no_citations" {
		t.Errorf("refusal.reason = %v", ref["reason"])
	}
}

func TestTraceTimeline(t *testing.T) {
	tr := Trace{
		FlowStages: []map[string]any{
			{"stage": "synthesize", "steps": []map[string]any{{"kind": "answer", "text": "x"}}},
		},
	}
	tl := tr.Timeline()
	if tl[0]["kind"] != "stage_start" {
		t.Errorf("tl[0] = %v", tl[0])
	}
	if tl[1]["kind"] != "answer" {
		t.Errorf("tl[1] = %v", tl[1])
	}
	if tl[len(tl)-1]["kind"] != "stage_end" {
		t.Errorf("tl[-1] = %v", tl[len(tl)-1])
	}
}
