// Package safety — SafetyPlugin (the assemble-time contribution).
package safety

import (
	"github.com/nccasia/agent-sdk-go/agent_sdk/agent"
	"github.com/nccasia/agent-sdk-go/agent_sdk/core/spec"
)

// SafetyAgentSetup is the type alias the plugin uses when installing.
type SafetyAgentSetup = agent.AgentSetup

// SafetyPlugin owns the `filter` output-safety lobe.
type SafetyPlugin struct{}

// NewSafetyPlugin builds a SafetyPlugin. Mirrors agent_sdk.plugins.safety.SafetyPlugin.
func NewSafetyPlugin() *SafetyPlugin { return &SafetyPlugin{} }

// Name returns the plugin name.
func (p *SafetyPlugin) Name() string { return "safety" }

// Lobes returns the lobes this plugin contributes (filter).
func (p *SafetyPlugin) Lobes() []spec.Lobe {
	return []spec.Lobe{FilterLOBE.Spec()}
}

// Install wires the plugin into the AgentSetup: the `filter` lobe.
func (p *SafetyPlugin) Install(setup *SafetyAgentSetup) {
	setup.AddLobe(FilterLOBE.Spec())
}
