// Command quickstart is the README quickstart, runnable offline: a one-tool
// PreactAgent answered by the deterministic FakeClient (no network, no creds).
//
//	go run ./examples/quickstart
//
// Swap clients.NewFakeClient for clients.NewAnthropicClient("claude-opus-4-8")
// (with ANTHROPIC_API_KEY set) to drive it against a real provider.
package main

import (
	"context"
	"fmt"

	"github.com/nccasia/agent-sdk-go/agent_sdk/agent"
	"github.com/nccasia/agent-sdk-go/agent_sdk/clients"
	"github.com/nccasia/agent-sdk-go/agent_sdk/tools"
)

func main() {
	search := tools.Tool("search",
		func(_ context.Context, in map[string]any) (any, error) {
			return "v2 added streaming.", nil
		},
		tools.Desc("Search the knowledge base."),
		tools.Param("query", "string", true, nil),
	)

	a := agent.MustPreactAgent(agent.Config{
		Client:       clients.NewFakeClient([]any{"v2 added streaming."}, nil),
		Instructions: "You are a helpful research assistant.",
		Tools:        []any{search},
	})

	res, err := a.Query(context.Background(), "What changed in v2?")
	if err != nil {
		panic(err)
	}
	fmt.Printf("answer: %s\nstatus: %s\n", res.Text, res.Status)
}
