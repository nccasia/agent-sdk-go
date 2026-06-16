// Micro-benchmarks for the agent façade — measure the hot paths the agent
// runs on every turn (path resolution, stage dispatch, event emission,
// tool-call dispatch).
package agent

import (
	"context"
	"testing"

	"github.com/nccasia/agent-sdk-go/agent_sdk/clients"
)

// BenchmarkQueryOneShot measures a single PreactAgent.Query against a
// FakeClient that emits a fixed text answer.
func BenchmarkQueryOneShot(b *testing.B) {
	a := MustPreactAgent(Config{
		Client:       clients.NewFakeClient([]any{"v2 added streaming and a new spec."}, nil),
		Instructions: "bench",
	})
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = a.Query(ctx, "What changed in v2?")
	}
}

// BenchmarkInspect measures a no-LLM Inspect.
func BenchmarkInspect(b *testing.B) {
	a := MustPreactAgent(Config{
		Client:       clients.NewFakeClient([]any{"x"}, nil),
		Instructions: "bench",
	})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.Inspect("what changed?")
	}
}

// BenchmarkActEmit measures a full Act stream and Final drain.
func BenchmarkActEmit(b *testing.B) {
	a := MustPreactAgent(Config{
		Client:       clients.NewFakeClient([]any{"answer text"}, nil),
		Instructions: "bench",
	})
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stream := a.Act(ctx, "q?")
		for ev := range stream.Iter() {
			_ = ev
		}
	}
}
