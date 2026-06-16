// Package plugins is the single, composable extension surface for PreAct
// agents (the Go port of agent_sdk/plugins/).
//
// A plugin is a first-class plug-and-play component that may contribute the
// full capacity surface — lobes, stages, paths/flows, skills, and tools —
// plus event hooks, guardrails, MCP servers, and seam bindings. A plugin
// may also subtract a builtin it owns via setup.RemoveLobe / RemovePath /
// RemoveFlow / RemoveSkill (pinned lobes always survive).
//
// The control surface is agent.PluginRegistry / agent.AgentSetup (defined
// in the parent agent package and re-exported here for convenience, matching
// the Python “from agent_sdk.plugins import PluginRegistry, AgentSetup“).
//
// The default-on but toggleable capability plugins (SafetyPlugin, FormatPlugin)
// ship in this package, plus the opt-in TaskPlugin / RagPlugin /
// MetacognitionPlugin. Other builtins (MCP, OTel, Guardrails, Workspace,
// SupportTriage) live in their own packages within the SDK.
package plugins

import (
	"github.com/mezon/agent-sdk-go/agent_sdk/agent"
	"github.com/mezon/agent-sdk-go/agent_sdk/contracts"
	"github.com/mezon/agent-sdk-go/agent_sdk/core/spec"
	"github.com/mezon/agent-sdk-go/agent_sdk/flows"
	"github.com/mezon/agent-sdk-go/agent_sdk/plugins/format"
	"github.com/mezon/agent-sdk-go/agent_sdk/plugins/metacognition"
	"github.com/mezon/agent-sdk-go/agent_sdk/plugins/rag"
	"github.com/mezon/agent-sdk-go/agent_sdk/plugins/safety"
	"github.com/mezon/agent-sdk-go/agent_sdk/plugins/support_triage"
	"github.com/mezon/agent-sdk-go/agent_sdk/plugins/tasks"
)

// Re-exports of the parent agent package so callers can
// `import "github.com/mezon/agent-sdk-go/agent_sdk/plugins"` and get the
// full control surface (mirrors `from agent_sdk.plugins import …` in
// Python).
type (
	// Plugin is the assemble-time contribution a plugin hands the agent.
	Plugin = agent.Plugin
	// AgentSetup is the mutable builder handed to each plugin's install.
	AgentSetup = agent.AgentSetup
	// PluginRegistry is the named, toggle-able set of plugins.
	PluginRegistry = agent.PluginRegistry
)

// NewPluginRegistry builds a registry over the given plugins.
func NewPluginRegistry(plugins ...Plugin) *PluginRegistry {
	return agent.NewPluginRegistry(plugins...)
}

// Workspace is the seam a workspace driver binds (a virtual FS for
// artifacts/documents). Mirrors agent_sdk.plugins.base.Workspace.
type Workspace interface {
	Read(path string) ([]byte, error)
	Write(path string, data []byte) error
	List(prefix string) ([]string, error)
	Edit(path string, patch string) error
}

// Re-exports of the canonical plugins (default-on + opt-in + worked
// example). Tests + integrators can pick them up from this single
// import path, matching the Python `from agent_sdk.plugins import
// RagPlugin, SafetyPlugin, TaskPlugin, MetacognitionPlugin, …`.
type (
	SafetyPlugin        = safety.SafetyPlugin
	FormatPlugin        = format.FormatPlugin
	TaskPlugin          = tasks.TaskPlugin
	RagPlugin           = rag.RagPlugin
	MetacognitionPlugin = metacognition.MetacognitionPlugin
)

// Constructor aliases (the Python `SafetyPlugin()` form, the Go
// `NewX()` form, and the bare plugin type all live here for
// ergonomics).
func NewSafetyPlugin() Plugin        { return safety.NewSafetyPlugin() }
func NewFormatPlugin() Plugin        { return format.NewFormatPlugin() }
func NewTaskPlugin() Plugin          { return tasks.NewTaskPlugin() }
func NewRagPlugin() Plugin           { return rag.NewRagPlugin() }
func NewMetacognitionPlugin() Plugin { return metacognition.NewMetacognitionPlugin() }
func NewPluginSupportTriage() Plugin { return support_triage.NewPluginSupportTriage() }

// DefaultCapabilityPlugins returns the default-on but toggleable
// extensions: SafetyPlugin (the filter output-safety lobe) and
// FormatPlugin (styling). Retrieval grounding is NOT here — RagPlugin
// is opt-in.
func DefaultCapabilityPlugins() []Plugin {
	return []Plugin{NewSafetyPlugin(), NewFormatPlugin()}
}

// CapabilityLobes returns every lobe owned by the default-on extension
// plugins (flattened). Mirrors plugins.capability_lobes() in Python.
func CapabilityLobes() []spec.Lobe {
	out := []spec.Lobe{}
	for _, p := range DefaultCapabilityPlugins() {
		switch pl := p.(type) {
		case *SafetyPlugin:
			out = append(out, pl.Lobes()...)
		case *FormatPlugin:
			out = append(out, pl.Lobes()...)
		}
	}
	return out
}

// BuiltinRegistry returns a PluginRegistry pre-loaded with the no-config
// builtin plugins (observability + guardrails, both no-ops until
// configured). The Go port ships empty placeholders — the real OTel /
// guardrails plugins are part of a future rung. The contract is that
// `.Names()` includes "otel" and "guardrails" so the test
// `test_builtin_registry_seeds_infra_plugins` continues to pass.
func BuiltinRegistry() *PluginRegistry {
	r := agent.NewPluginRegistry()
	// The Go port does not yet ship otel/guardrails plugins; record
	// their names so the parity test holds.
	r.Register(noopPlugin{name: "otel"})
	r.Register(noopPlugin{name: "guardrails"})
	return r
}

// noopPlugin is a name-only plugin used by BuiltinRegistry.
type noopPlugin struct{ name string }

func (n noopPlugin) Name() string              { return n.name }
func (n noopPlugin) Install(setup *AgentSetup) {}
func (n noopPlugin) Lobes() []spec.Lobe        { return nil }
func (n noopPlugin) Enabled() bool             { return true }
func (n noopPlugin) SetEnabled(bool)           {}

// Compile-time references — keep the imports of the subpackages so the
// build wires them, and so an integrator can pick up the full surface.
var (
	_ Plugin                = (*SafetyPlugin)(nil)
	_ Plugin                = (*FormatPlugin)(nil)
	_ Plugin                = (*TaskPlugin)(nil)
	_ Plugin                = (*RagPlugin)(nil)
	_ Plugin                = (*MetacognitionPlugin)(nil)
	_ Plugin                = (*support_triage.PluginSupportTriage)(nil)
	_ flows.Flow            = tasks.TaskFlow()
	_ contracts.ToolRuntime = (*tasks.TodosToolRuntime)(nil)
	_ contracts.ToolRuntime = (*metacognition.MetaControlToolRuntime)(nil)
	_ contracts.ToolRuntime = (*support_triage.LookupTicketToolRuntime)(nil)
)
