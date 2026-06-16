// Plugins integration tests — the end-to-end flow the plugins
// drive. Mirrors tests/test_task_integration.py and
// tests/test_integration.py (metacognition).
package plugins

import (
	"context"
	"strings"
	"testing"

	"github.com/mezon/agent-sdk-go/agent_sdk/agent"
	"github.com/mezon/agent-sdk-go/agent_sdk/clients"
	"github.com/mezon/agent-sdk-go/agent_sdk/network"
)

// TestTaskLivesOnlyInThePlugin mirrors test_task_lives_only_in_the_plugin.
func TestTaskLivesOnlyInThePlugin(t *testing.T) {
	// default_lobes has no task lobe
	for _, lb := range network.ProductionLobes() {
		if strings.Contains(lb.ID, "task") {
			t.Fatalf("default lobe leaks task: %q", lb.ID)
		}
	}
	// bare agent has no todos tool
	bare, err := agent.NewPreactAgent(agent.Config{
		Client:          clients.Scripted(func(stage, system string, messages, tools []map[string]any) any { return "x" }),
		UniversalMemory: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if toolNames(bare).has("todos") {
		t.Fatalf("bare agent should not have `todos` tool, got %v", toolNames(bare))
	}
	// bare inspect routes to something other than `task`
	if bare.Inspect("compute the total revenue").Path.Name == "task" {
		t.Fatalf("bare agent should not route to `task`")
	}
}

// TestPluginDrivesPlanThenPerTodoThenDeliver mirrors
// test_plugin_drives_plan_then_per_todo_then_deliver.
func TestPluginDrivesPlanThenPerTodoThenDeliver(t *testing.T) {
	added := 0
	model := func(stage, system string, messages, tools []map[string]any) any {
		switch stage {
		case "plan":
			if added < 2 {
				added++
				return map[string]any{
					"text": "planned",
					"tools": []map[string]any{
						{"name": "todos", "input": map[string]any{"action": "add", "title": "step " + intToStr(added)}},
					},
				}
			}
			return "planned"
		case "deliver":
			return "FINAL ANSWER: 42"
		default:
			return "sub-result"
		}
	}
	ag, err := agent.NewPreactAgent(agent.Config{
		Client:          clients.Scripted(model),
		Plugins:         []Plugin{NewTaskPlugin()},
		UniversalMemory: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	q := "compute the total and list the top items"
	if ag.Inspect(q).Path.Name != "task" {
		t.Fatalf("expected path=task, got %q", ag.Inspect(q).Path.Name)
	}
	res, err := ag.Query(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "FINAL ANSWER") {
		t.Fatalf("expected FINAL ANSWER in response, got %q", res.Text)
	}
}

// TestMetacognitionLivesOnlyInThePlugin mirrors
// test_metacognition_lives_only_in_the_plugin.
func TestMetacognitionLivesOnlyInThePlugin(t *testing.T) {
	for _, lb := range network.ProductionLobes() {
		if lb.ID == "meta_context" {
			t.Fatalf("default lobe leaks meta_context")
		}
	}
	bare, _ := agent.NewPreactAgent(agent.Config{
		Client:          clients.Scripted(func(stage, system string, messages, tools []map[string]any) any { return "x" }),
		UniversalMemory: false,
	})
	if toolNames(bare).has("meta_control") {
		t.Fatalf("bare agent should not have meta_control tool")
	}
	if bare.Inspect("rethink your approach to this").Path.Name == "meta" {
		t.Fatalf("bare agent should not route to `meta`")
	}
}

// TestPluginAddsTheMetaLobes mirrors test_plugin_adds_the_meta_lobes.
func TestPluginAddsTheMetaLobes(t *testing.T) {
	ag, _ := agent.NewPreactAgent(agent.Config{
		Client:          clients.Scripted(func(stage, system string, messages, tools []map[string]any) any { return "x" }),
		Plugins:         []Plugin{NewMetacognitionPlugin()},
		UniversalMemory: false,
	})
	ids := map[string]struct{}{}
	for _, lb := range ag.Engine().Lobes {
		ids[lb.ID] = struct{}{}
	}
	for _, want := range []string{"meta_context", "nav_brief"} {
		if _, ok := ids[want]; !ok {
			t.Fatalf("expected lobe %q in agent", want)
		}
	}
}

type set struct{ m map[string]struct{} }

func (s set) has(x string) bool { _, ok := s.m[x]; return ok }
func (s set) String() string {
	out := []string{}
	for k := range s.m {
		out = append(out, k)
	}
	return "[" + strings.Join(out, ",") + "]"
}

func toolNames(ag *agent.PreactAgent) set {
	out := map[string]struct{}{}
	rt, ok := ag.Engine().Tools.(interface {
		GetToolSpecs() []map[string]any
	})
	if !ok {
		return set{m: out}
	}
	for _, s := range rt.GetToolSpecs() {
		if n, ok := s["name"].(string); ok {
			out[n] = struct{}{}
		}
	}
	return set{m: out}
}

func intToStr(i int) string {
	if i == 0 {
		return "0"
	}
	out := ""
	for i > 0 {
		out = string(rune('0'+i%10)) + out
		i /= 10
	}
	return out
}
