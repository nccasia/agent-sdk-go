package tasks

import (
	"github.com/nccasia/agent-sdk-go/agent_sdk/flows"
)

// TaskStages returns the canonical plan → execute → deliver pipeline
// stages for the task plugin. Mirrors agent_sdk.plugins.tasks.stages.task_stages.
//
//	plan     — agentic loop with the `todos` tool (the model builds the rail)
//	execute  — generic per-todo map over the rail (fanout_key="todos")
//	deliver  — single-shot final answer
func TaskStages() []flows.FlowStep {
	taskRail := []string{"task_rail"}
	todosTool := []string{"todos"}
	return []flows.FlowStep{
		flows.NewFlowStep(flows.FlowStep{
			Name:        "plan",
			Description: "Build the todo rail via the `todos` tool.",
			Loop:        "agentic",
			Tools:       todosTool,
			Lobes:       taskRail,
		}),
		flows.NewFlowStep(flows.FlowStep{
			Name:        "execute",
			Description: "Run one sub-execution per open todo (map fan-out).",
			Loop:        "map",
			Tools:       []string{},
			FanoutKey:   "todos",
			Lobes:       taskRail,
		}),
		flows.NewFlowStep(flows.FlowStep{
			Name:        "deliver",
			Description: "State the final answer with the rail rendered.",
			Loop:        "single",
			Tools:       []string{},
			Lobes:       taskRail,
		}),
	}
}
