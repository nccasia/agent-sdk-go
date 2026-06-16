// Tasks — install + flow recognizer tests. Mirrors
// agent_sdk/plugins/tasks/tests/test_plugin.py + test_path.py.
package tasks

import (
	"testing"

	"github.com/nccasia/agent-sdk-go/agent_sdk/agent"
	"github.com/nccasia/agent-sdk-go/agent_sdk/flows"
)

// TestInstallContributesLobeStagesFlowAndOneTool mirrors
// test_install_contributes_lobe_stages_flow_and_one_tool.
func TestInstallContributesLobeStagesFlowAndOneTool(t *testing.T) {
	setup := agent.NewAgentSetup()
	NewTaskPlugin().Install(setup)
	// Lobe: task_rail
	if len(setup.Lobes) != 1 || setup.Lobes[0].ID != "task_rail" {
		t.Fatalf("expected lobes=[task_rail], got %+v", setup.Lobes)
	}
	// Stages: plan, execute, deliver
	gotStages := map[string]struct{}{}
	for _, s := range setup.Stages {
		switch v := s.(type) {
		case *TaskAgentSetup:
			_ = v
		default:
			if id := stageIDOf(s); id != "" {
				gotStages[id] = struct{}{}
			}
		}
	}
	for _, want := range []string{"plan", "execute", "deliver"} {
		if _, ok := gotStages[want]; !ok {
			t.Fatalf("expected stage %q in %v", want, gotStages)
		}
	}
	// Flow: task
	if len(setup.Flows) != 1 || setup.Flows[0].ID() != "task" {
		t.Fatalf("expected flows=[task], got %+v", setup.Flows)
	}
	// Tool runtimes: exactly one (the `todos` tool).
	if len(setup.ToolRuntimes) != 1 {
		t.Fatalf("expected 1 tool runtime, got %d", len(setup.ToolRuntimes))
	}
	rt, ok := setup.ToolRuntimes[0].(*TodosToolRuntime)
	if !ok {
		t.Fatalf("expected *TodosToolRuntime, got %T", setup.ToolRuntimes[0])
	}
	specs := rt.GetToolSpecs()
	if len(specs) != 1 || specs[0]["name"] != "todos" {
		t.Fatalf("expected one `todos` tool, got %v", specs)
	}
}

// TestFlowRecognizerIsTheTaskPath mirrors
// test_flow_recognizer_is_the_task_path.
func TestFlowRecognizerIsTheTaskPath(t *testing.T) {
	setup := agent.NewAgentSetup()
	NewTaskPlugin().Install(setup)
	flow := setup.Flows[0]
	if got := flow.Signal(map[string]any{"query": "compute the total"}); got < 0.5 {
		t.Fatalf("expected signal > 0.5 for analytical cue, got %v", got)
	}
	if got := flow.Signal(map[string]any{"query": "hello"}); got != 0.0 {
		t.Fatalf("expected 0.0 for chitchat, got %v", got)
	}
}

func stageIDOf(s any) string {
	switch v := s.(type) {
	case stageIDer:
		return v.StageID()
	case interface{ GetID() string }:
		return v.GetID()
	case flows.FlowStep:
		return v.Name
	case *flows.FlowStep:
		if v != nil {
			return v.Name
		}
	}
	return ""
}

type stageIDer interface {
	StageID() string
}
