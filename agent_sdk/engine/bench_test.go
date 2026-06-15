package engine

import "testing"

func BenchmarkBlockToDictText(b *testing.B) {
	blk := TextBlock{Text: "the real answer to the question"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = BlockToDict(blk)
	}
}

func BenchmarkBlockToDictThinking(b *testing.B) {
	blk := ThinkingBlock{Thinking: "let me reason about this for a while"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = BlockToDict(blk)
	}
}

func BenchmarkAssistantContent(b *testing.B) {
	msg := SimpleMessage{Blocks: []Block{
		ThinkingBlock{Thinking: "reasoning"},
		TextBlock{Text: "answer"},
		ToolUseBlock{Name: "search", Input: map[string]any{"q": "x"}},
	}}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = AssistantContent(msg)
	}
}

func BenchmarkStageToFlowStep(b *testing.B) {
	s := NewStage("research", StageLobes("research"), StageLoop("agentic"), StageTools("search"))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = s.ToFlowStep()
	}
}

func BenchmarkStageRegistryResolve(b *testing.B) {
	reg := NewStageRegistry(
		NewStage("plan", StageLobes("plan")),
		NewStage("research", StageLobes("research")),
		NewStage("synth", StageLobes("synthesize")),
	)
	ids := []string{"plan", "research", "synth", "missing"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = reg.Resolve(ids)
	}
}
