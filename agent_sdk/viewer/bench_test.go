package viewer

import (
	"testing"

	"github.com/mezon/agent-sdk-go/agent_sdk/probe"
)

func sampleProbe() *probe.Record {
	return &probe.Record{
		Label: "p1", Query: "compare a and b", Status: "answered", Answer: "ans",
		Flow: "research", FlowScore: 0.82,
		Path: map[string]any{"name": "research", "score": 0.82},
		Lobes: []map[string]any{
			{"id": "synthesize", "layer": 4, "activated": true},
		},
		Stages: []map[string]any{
			{"flow": "research", "stage": "synthesize", "loop": "agentic", "lobes": []string{"synthesize"},
				"steps":         []map[string]any{{"kind": "answer", "text": "ans"}},
				"system_prompt": "You are helpful."},
		},
		LlmCalls: []map[string]any{{"stage": "synthesize", "usage": map[string]any{"input_tokens": 5, "output_tokens": 7}}},
		Usage:    map[string]any{"input_tokens": 5, "output_tokens": 7},
	}
}

func BenchmarkToRecord(b *testing.B) {
	p := sampleProbe()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ToRecord(p)
	}
}

func BenchmarkRenderViewerHTML(b *testing.B) {
	recs := []*probe.Record{sampleProbe()}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RenderHTML(recs, WithLabel("bench"))
	}
}
