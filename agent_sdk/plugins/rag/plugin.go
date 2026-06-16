// Package rag — RagPlugin (the assemble-time contribution).
package rag

import (
	"github.com/nccasia/agent-sdk-go/agent_sdk/agent"
	"github.com/nccasia/agent-sdk-go/agent_sdk/contracts"
	"github.com/nccasia/agent-sdk-go/agent_sdk/core/spec"
)

// RagAgentSetup is the type alias the plugin uses when installing.
type RagAgentSetup = agent.AgentSetup

// RagPlugin is the opt-in retrieval-grounding plugin. It contributes
// the `cite` lobe (pinned when present) + the finalize-grounding
// hook + the tool-result-citation-extractor hook. It does NOT mount
// any tools (retrieval is a host concern).
type RagPlugin struct {
	enabled bool
}

// NewRagPlugin builds a RagPlugin. Mirrors agent_sdk.plugins.rag.RagPlugin.
func NewRagPlugin() *RagPlugin { return &RagPlugin{enabled: true} }

// Name returns the plugin name (used by the registry).
func (p *RagPlugin) Name() string { return "rag" }

// SetEnabled toggles the plugin's contribution at install time.
func (p *RagPlugin) SetEnabled(v bool) { p.enabled = v }

// Enabled reports the current state.
func (p *RagPlugin) Enabled() bool { return p.enabled }

// Lobes returns the lobes this plugin contributes (cite, pinned when
// present). Mirrors the Python `RagPlugin.lobes()`.
func (p *RagPlugin) Lobes() []spec.Lobe {
	return []spec.Lobe{CiteLOBE.Spec()}
}

// Install wires the plugin into the AgentSetup: the `cite` lobe, the
// finalize-grounding hook, and the tool-result-citation-extractor.
func (p *RagPlugin) Install(setup *RagAgentSetup) {
	if !p.Enabled() {
		return
	}
	setup.AddLobe(CiteLOBE.Spec())
	setup.AddFinalizeHook(finalizeToHook(FinalizeGrounding))
	setup.AddToolResultHook(toolResultToHook(ToolResultHook))
}

// finalizeToHook adapts FinalizeGrounding to the agent.FinalizeHookFn
// signature.
func finalizeToHook(fn func(answer string, citations []contracts.Citation, chunks []map[string]any, grounds, requireCitations bool) (string, []contracts.Citation, string)) agent.FinalizeHookFn {
	return func(answer string, citations []contracts.Citation, chunks []map[string]any, grounds, requireCitations bool) (string, []contracts.Citation, string) {
		return fn(answer, citations, chunks, grounds, requireCitations)
	}
}

func toolResultToHook(fn func(toolName, output string) []contracts.Citation) agent.ToolResultHookFn {
	return func(toolName, output string) []contracts.Citation {
		return fn(toolName, output)
	}
}
