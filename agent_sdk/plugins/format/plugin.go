// Package format — FormatPlugin (the assemble-time contribution).
package format

import (
	"github.com/mezon/agent-sdk-go/agent_sdk/agent"
	"github.com/mezon/agent-sdk-go/agent_sdk/core/spec"
)

// FormatAgentSetup is the type alias the plugin uses when installing.
type FormatAgentSetup = agent.AgentSetup

// FormatPlugin owns the `format` style lobe.
type FormatPlugin struct{}

// NewFormatPlugin builds a FormatPlugin. Mirrors agent_sdk.plugins.format.FormatPlugin.
func NewFormatPlugin() *FormatPlugin { return &FormatPlugin{} }

// Name returns the plugin name.
func (p *FormatPlugin) Name() string { return "format" }

// Lobes returns the lobes this plugin contributes (format).
func (p *FormatPlugin) Lobes() []spec.Lobe {
	return []spec.Lobe{FormatLOBE.Spec()}
}

// Install wires the plugin into the AgentSetup: the `format` lobe.
func (p *FormatPlugin) Install(setup *FormatAgentSetup) {
	setup.AddLobe(FormatLOBE.Spec())
}
