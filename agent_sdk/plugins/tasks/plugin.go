package tasks

import (
	"github.com/nccasia/agent-sdk-go/agent_sdk/agent"
	"github.com/nccasia/agent-sdk-go/agent_sdk/flows"
)

// TaskAgentSetup is the type alias the plugin uses when installing.
// It is the parent agent package's *AgentSetup (so callers can pass
// the same builder the engine consumes).
type TaskAgentSetup = agent.AgentSetup

// TaskPlugin is the opt-in todo-driven task execution plugin:
// plan → per-todo execute → deliver, with one `todos` tool. Mirrors
// agent_sdk.plugins.tasks.TaskPlugin.
type TaskPlugin struct{}

// NewTaskPlugin builds a TaskPlugin.
func NewTaskPlugin() *TaskPlugin { return &TaskPlugin{} }

// Name returns the plugin name.
func (p *TaskPlugin) Name() string { return "task" }

// Enabled reports whether the plugin is active. Tests can flip it
// off to suppress the contribution at install time.
func (p *TaskPlugin) Enabled() bool { return true }

// Lobes returns the lobes the plugin contributes (just task_rail).
func (p *TaskPlugin) Lobes() []taskRailLobe { return []taskRailLobe{{id: "task_rail"}} }

// Install wires the plugin into the AgentSetup: the task_rail lobe,
// the plan/execute/deliver stages, the task flow, and the `todos` tool
// runtime.
func (p *TaskPlugin) Install(setup *TaskAgentSetup) {
	if !p.Enabled() {
		return
	}
	setup.AddLobe(TaskRailLOBE.Spec())
	for _, st := range TaskStages() {
		setup.AddStage(st)
	}
	setup.AddFlow(TaskFlow())
	setup.AddToolRuntime(NewTodosToolRuntime())
}

// TaskFlow returns the canonical `task` flow (an intent pipeline of
// the plan → execute → deliver stage ids, recognized by the task
// path's recognizer).
func TaskFlow() flows.Flow {
	return flows.NewFlow("task",
		flows.FlowUseWhen("accomplish a multi-step task / run a checklist to completion"),
		flows.FlowStages("plan", "execute", "deliver"),
		flows.FlowGrounds(false),
		flows.FlowThreshold(0.5),
		flows.FlowSignalFn(Recognize),
	)
}

// taskRailLobe is the minimal projection the public API returns
// (id-only); tests can iterate lobes() to discover the id.
type taskRailLobe struct{ id string }

func (t taskRailLobe) ID() string { return t.id }
