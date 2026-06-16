package report

import (
	"testing"

	"github.com/nccasia/agent-sdk-go/agent_sdk/bench"
	"github.com/nccasia/agent-sdk-go/agent_sdk/probe"
	"github.com/nccasia/agent-sdk-go/agent_sdk/result"
)

// sampleProbe builds a synthetic probe record (no agent dependency) so the
// renderer's hot path can be measured in isolation.
func sampleProbe() *probe.Record {
	return &probe.Record{
		Label: "p1", Query: "compare a and b", Status: "answered", Answer: "the answer",
		Flow: "research", FlowScore: 0.82,
		Path: map[string]any{"name": "research", "score": 0.82},
		Lobes: []map[string]any{
			{"id": "synthesize", "layer": 4, "activated": true, "activation": 0.9, "reason": "qna"},
			{"id": "respond", "layer": 5, "activated": true, "activation": 0.7},
		},
		Stages: []map[string]any{
			{"flow": "research", "stage": "synthesize", "loop": "agentic", "lobes": []string{"synthesize"},
				"steps": []map[string]any{{"kind": "answer", "text": "the answer"}}},
		},
		Usage: map[string]any{"input_tokens": 12, "output_tokens": 34},
	}
}

func BenchmarkRenderHTML(b *testing.B) {
	rep := Report{Results: []bench.ScenarioResult{
		{Scenario: bench.Scenario{Input: "compare a and b", ExpectPath: "research"},
			Path: result.PathScore{Name: "research", Score: 0.82}, ActivatedLobes: []string{"synthesize"}, Passed: true},
	}}
	probes := Probes{sampleProbe()}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RenderHTML("bench", rep, probes, WithGeneratedAt("FIXED"))
	}
}
