// Micro-benchmarks for the serve hot paths — job enqueue, single-job drain
// through the worker pool, and per-event sink publish/subscribe fan-out.
// These measure the per-turn overhead the stateless serving layer adds on
// top of the agent façade.
package serve

import (
	"context"
	"testing"

	"github.com/mezon/agent-sdk-go/agent_sdk/agent"
	"github.com/mezon/agent-sdk-go/agent_sdk/clients"
)

// BenchmarkInProcessQueueEnqueue measures the cost of pushing a Job onto
// the in-process queue.
func BenchmarkInProcessQueueEnqueue(b *testing.B) {
	q := NewInProcessQueue()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = q.Enqueue(Job{Input: "hi", SessionID: "s"})
	}
}

// BenchmarkInProcessEventSinkPublish measures the cost of fanning an event
// out to one subscriber.
func BenchmarkInProcessEventSinkPublish(b *testing.B) {
	sink := NewInProcessEventSink()
	_ = sink.Subscribe("tr")
	ev := map[string]any{"type": "text_delta", "text": "hi"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sink.Publish("tr", ev)
	}
}

// BenchmarkAgentWorkerServeOneJob measures a single-job drain through the
// worker (one turn on one pooled agent).
func BenchmarkAgentWorkerServeOneJob(b *testing.B) {
	a := agent.MustPreactAgent(agent.Config{
		Client:       clients.NewFakeClient([]any{"v2 added streaming and a new spec."}, nil),
		Instructions: "bench",
	})
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q := NewInProcessQueue()
		sink := NewInProcessEventSink()
		w := NewAgentWorker(WorkerConfig{Agent: a, Queue: q, Sink: sink, Concurrency: 1})
		_, _ = q.Enqueue(Job{Input: "What changed in v2?", SessionID: "s"})
		_ = w.Serve(ctx, 1)
	}
}
