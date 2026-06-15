package metacognition

import (
	"testing"

	"github.com/mezon/agent-sdk-go/agent_sdk/inspection"
)

// monitor → regulate is the per-step hot path the engine runs every stage.
func BenchmarkPlanNext(b *testing.B) {
	meta, _ := NewMetacognition(ModeApply, nil)
	flowAxis := &inspection.FlowAxisSnapshot{
		Flow: "research",
		Steps: []inspection.FlowStepInspection{
			{Flow: "research", Step: "s1", Lobes: []string{"x"}, StateNodes: []map[string]any{
				{"id": "context:tight", "activated": true},
			}},
			{Flow: "research", Step: "s2", Lobes: nil},
		},
	}
	step := "s1"
	in := PlanNextInput{
		MonitorInput: MonitorInput{FlowAxis: flowAxis},
		TargetStep:   &step,
		CurrentLobes: []string{"synthesize", "memory_recall", "skill_select"},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = meta.PlanNext(in)
	}
}

func BenchmarkCompileStatePlan(b *testing.B) {
	aspects := []any{"a", "b", "c", "d"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = CompileStatePlan(aspects, true)
	}
}
