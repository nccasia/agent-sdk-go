package metacognition

import (
	"github.com/mezon/agent-sdk-go/agent_sdk/agent"
	"github.com/mezon/agent-sdk-go/agent_sdk/core/spec"
	"github.com/mezon/agent-sdk-go/agent_sdk/flows"
)

// MetaAgentSetup is the type alias the plugin uses when installing.
type MetaAgentSetup = agent.AgentSetup

// MetacognitionPlugin is the opt-in metacognition plugin: meta_context
// lobe + nav_brief lobe + the meta_reflect stage + the meta flow
// (recognized by a conservative rethink cue + recorded next-turn
// bias) + the meta_control tool runtime.
type MetacognitionPlugin struct {
	withFlow bool
}

// NewMetacognitionPlugin builds a MetacognitionPlugin. The flow is
// included by default; pass `WithFlow(false)` to omit it.
func NewMetacognitionPlugin(opts ...MetaOption) *MetacognitionPlugin {
	p := &MetacognitionPlugin{withFlow: true}
	for _, o := range opts {
		o(p)
	}
	return p
}

// MetaOption configures a MetacognitionPlugin.
type MetaOption func(*MetacognitionPlugin)

// WithFlow toggles whether the meta flow is contributed.
func WithFlow(b bool) MetaOption { return func(p *MetacognitionPlugin) { p.withFlow = b } }

// Name returns the plugin name.
func (p *MetacognitionPlugin) Name() string { return "metacognition" }

// Lobes returns the lobes the plugin contributes (meta_context,
// nav_brief).
func (p *MetacognitionPlugin) Lobes() []spec.Lobe {
	return []spec.Lobe{MetaContextLOBE.Spec(), NavBriefLOBE.Spec()}
}

// Install wires the plugin into the AgentSetup.
func (p *MetacognitionPlugin) Install(setup *MetaAgentSetup) {
	setup.AddLobe(MetaContextLOBE.Spec())
	setup.AddLobe(NavBriefLOBE.Spec())
	// The single meta_reflect stage (a single-shot reflect step).
	setup.AddStage(metaReflectStage())
	if p.withFlow {
		setup.AddFlow(metaFlow())
	}
	setup.AddToolRuntime(NewMetaControlToolRuntime())
}

func metaReflectStage() flows.FlowStep {
	return flows.NewFlowStep(flows.FlowStep{
		Name:        "meta_reflect",
		Description: "Reflect on the active path; record any next-turn bias.",
		Loop:        "single",
		Lobes:       []string{"meta_context"},
	})
}

func metaFlow() flows.Flow {
	return flows.NewFlow("meta",
		flows.FlowUseWhen("rethink the approach / reflect on what to do next"),
		flows.FlowStages("meta_reflect"),
		flows.FlowGrounds(false),
		flows.FlowThreshold(0.5),
		flows.FlowSignalFn(RecognizerWithBias()),
	)
}
