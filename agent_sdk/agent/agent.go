// Package agent holds the PreactAgent façade — the public surface every
// example + live bench drives. It wires the building blocks (lobes / stages /
// flows / skills / tools), the I/O seams (client, session, memory), and the
// extensions (plugins, metacognition) into a ready-to-run agent. Building
// blocks default to the built-in PreAct network when omitted; persistence
// defaults to in-memory.
//
// Ported from agent_sdk/agent.py:PreactAgent.
package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/mezon/agent-sdk-go/agent_sdk/clients"
	"github.com/mezon/agent-sdk-go/agent_sdk/contracts"
	"github.com/mezon/agent-sdk-go/agent_sdk/core/spec"
	"github.com/mezon/agent-sdk-go/agent_sdk/events"
	"github.com/mezon/agent-sdk-go/agent_sdk/flows"
	"github.com/mezon/agent-sdk-go/agent_sdk/metacognition"
	"github.com/mezon/agent-sdk-go/agent_sdk/result"
	"github.com/mezon/agent-sdk-go/agent_sdk/session"
)

// MemoryDirective is the system-prompt block injected when universal memory
// is on (the default) so the agent NATIVELY memorizes and recalls — it doesn't
// depend on the user's instructions mentioning memory. Ported from
// agent_sdk/agent.py:MEMORY_DIRECTIVE.
const MemoryDirective = "You have a persistent memory across this conversation and future ones. A `## Memory` section in " +
	"your context lists what you have stored.\n" +
	"- MEMORIZE: when you learn durable facts — deadlines, owners, schedules, decisions, values, " +
	"preferences, postmortems, agreements — call `note` for EACH distinct fact, one note per fact, " +
	"the moment you read it (scope=conversation for things that outlive this turn). Do not skip facts.\n" +
	"- RECALL: before answering any question about earlier information, check the `## Memory` list and " +
	"use `recall` to pull the detail. Always recall first, then answer.\n" +
	"- When a fact changes, note the new value; recall returns the latest."

// FinalizeHookFn is the seam a RAG/grounding plugin uses to own the
// grounding/citation contract: it can rewrite the answer, augment/replace
// citations, and force a refusal. Returning (answer, citations, "") leaves
// the turn unchanged; returning a non-empty refusal reason ends the turn in
// a structured refusal. Mirrors AgentSetup.add_finalize_hook in
// agent_sdk/plugins/base.py.
type FinalizeHookFn func(answer string, citations []contracts.Citation, chunks []map[string]any, grounds, requireCitations bool) (string, []contracts.Citation, string)

// ToolResultHookFn is the seam that extracts citations a tool emits in its
// output. The returned list is appended to the turn's citations.
type ToolResultHookFn func(toolName, output string) []contracts.Citation

// PreCheckFn is a guardrail run on the user input before the turn (raise to
// block).
type PreCheckFn func(input string) error

// PostCheckFn is a guardrail run on the AgentResult after the turn (raise to
// block).
type PostCheckFn func(res *result.AgentResult) error

// EventHookFn is a side-effect hook fired on every streamed event.
type EventHookFn func(ev events.AgentEvent)

// PrefetchHookFn is a per-turn “(query, state) -> map“ whose result is
// merged into the turn context (e.g. “{"memory_items": [...]}“) before the
// lobes assemble context.
type PrefetchHookFn func(query string, state session.SessionState) map[string]any

// ToolFilterFn is a guard “(stage_id, tool_name, input) -> string | nil“:
// non-nil short-circuits the call with that result.
type ToolFilterFn func(stageID, toolName string, input map[string]any) string

// Plugin is the assemble-time contribution a plugin hands the agent.
// Ported from agent_sdk/plugins/base.py:Plugin.
type Plugin interface {
	Name() string
	Install(setup *AgentSetup)
}

// AgentSetup is the mutable builder handed to each plugin's install. A
// plugin may add lobes, stages, flows, paths, skills, tools, MCP servers,
// event hooks, and finalize/tool-result/tool-filter hooks; or remove a
// builtin lobe/path/flow/skill it owns. Pinned lobes (cite, filter) survive
// any removal.
type AgentSetup struct {
	Tools           []any
	ToolRuntimes    []any
	Lobes           []spec.Lobe
	Stages          []any
	Flows           []flows.Flow
	Paths           []spec.Path
	Skills          []any
	MCPServers      []any
	EventHooks      []EventHookFn
	PreChecks       []PreCheckFn
	PostChecks      []PostCheckFn
	PrefetchHooks   []PrefetchHookFn
	ToolFilters     []ToolFilterFn
	FinalizeHooks   []FinalizeHookFn
	ToolResultHooks []ToolResultHookFn
	RemovedLobes    map[string]struct{}
	RemovedPaths    map[string]struct{}
	RemovedFlows    map[string]struct{}
	RemovedSkills   map[string]struct{}
	Workspace       any
	Host            any
}

// NewAgentSetup builds an empty AgentSetup (mirrors AgentSetup.__init__).
func NewAgentSetup() *AgentSetup {
	return &AgentSetup{
		RemovedLobes:  map[string]struct{}{},
		RemovedPaths:  map[string]struct{}{},
		RemovedFlows:  map[string]struct{}{},
		RemovedSkills: map[string]struct{}{},
	}
}

// AddTool appends a tool (@tool function or ToolDef).
func (s *AgentSetup) AddTool(t any) { s.Tools = append(s.Tools, t) }

// AddToolRuntime appends a whole, stateful ToolRuntime (get_tool_specs +
// call_tool).
func (s *AgentSetup) AddToolRuntime(rt any) { s.ToolRuntimes = append(s.ToolRuntimes, rt) }

// AddLobe appends a spec.Lobe.
func (s *AgentSetup) AddLobe(l spec.Lobe) { s.Lobes = append(s.Lobes, l) }

// AddStage appends a stage object.
func (s *AgentSetup) AddStage(st any) { s.Stages = append(s.Stages, st) }

// AddFlow appends a flows.Flow.
func (s *AgentSetup) AddFlow(f flows.Flow) { s.Flows = append(s.Flows, f) }

// AddPath appends a path recognizer.
func (s *AgentSetup) AddPath(p spec.Path) { s.Paths = append(s.Paths, p) }

// AddSkill appends a skill.
func (s *AgentSetup) AddSkill(sk any) { s.Skills = append(s.Skills, sk) }

// AddMCPServer appends an MCP server spec or runtime.
func (s *AgentSetup) AddMCPServer(srv any) { s.MCPServers = append(s.MCPServers, srv) }

// RemoveLobe subtracts a builtin lobe this plugin owns/overrides.
func (s *AgentSetup) RemoveLobe(id string) { s.RemovedLobes[id] = struct{}{} }

// RemovePath subtracts a path this plugin owns/overrides.
func (s *AgentSetup) RemovePath(name string) { s.RemovedPaths[name] = struct{}{} }

// RemoveFlow subtracts a flow this plugin owns/overrides.
func (s *AgentSetup) RemoveFlow(name string) { s.RemovedFlows[name] = struct{}{} }

// RemoveSkill subtracts a skill this plugin owns/overrides.
func (s *AgentSetup) RemoveSkill(slug string) { s.RemovedSkills[slug] = struct{}{} }

// OnEvent registers a per-event hook.
func (s *AgentSetup) OnEvent(hook EventHookFn) { s.EventHooks = append(s.EventHooks, hook) }

// AddPreCheck registers a pre-turn guardrail.
func (s *AgentSetup) AddPreCheck(check PreCheckFn) { s.PreChecks = append(s.PreChecks, check) }

// AddPostCheck registers a post-turn guardrail.
func (s *AgentSetup) AddPostCheck(check PostCheckFn) { s.PostChecks = append(s.PostChecks, check) }

// AddPrefetchHook registers a per-turn async/sync hook whose result is merged
// into the turn context.
func (s *AgentSetup) AddPrefetchHook(hook PrefetchHookFn) {
	s.PrefetchHooks = append(s.PrefetchHooks, hook)
}

// AddToolFilter registers a tool-call guard.
func (s *AgentSetup) AddToolFilter(f ToolFilterFn) { s.ToolFilters = append(s.ToolFilters, f) }

// AddFinalizeHook registers a post-answer finalize hook.
func (s *AgentSetup) AddFinalizeHook(hook FinalizeHookFn) {
	s.FinalizeHooks = append(s.FinalizeHooks, hook)
}

// AddToolResultHook registers a per-tool-result citation extractor.
func (s *AgentSetup) AddToolResultHook(hook ToolResultHookFn) {
	s.ToolResultHooks = append(s.ToolResultHooks, hook)
}

// BindWorkspace attaches a virtual filesystem driver.
func (s *AgentSetup) BindWorkspace(ws any) { s.Workspace = ws }

// SetHost stores the host object a plugin may read to build a stateful
// runtime bound to the application.
func (s *AgentSetup) SetHost(h any) { s.Host = h }

// PluginRegistry is a pluggable plugin collection with enable/disable
// semantics. An active() view yields the plugins currently enabled.
type PluginRegistry struct {
	plugins  []Plugin
	byName   map[string]Plugin
	disabled map[string]struct{}
}

// NewPluginRegistry builds a registry over the given plugins.
func NewPluginRegistry(plugins ...Plugin) *PluginRegistry {
	r := &PluginRegistry{
		plugins:  append([]Plugin(nil), plugins...),
		byName:   map[string]Plugin{},
		disabled: map[string]struct{}{},
	}
	for _, p := range plugins {
		if p != nil {
			r.byName[p.Name()] = p
		}
	}
	return r
}

// Active returns the enabled plugin list, preserving insertion order.
func (r *PluginRegistry) Active() []Plugin {
	out := []Plugin{}
	for _, p := range r.plugins {
		if p == nil {
			continue
		}
		if _, off := r.disabled[p.Name()]; off {
			continue
		}
		out = append(out, p)
	}
	return out
}

// IsDisabled reports whether the named plugin is disabled.
func (r *PluginRegistry) IsDisabled(name string) bool {
	_, ok := r.disabled[name]
	return ok
}

// Disable marks a plugin as disabled (a no-op on the engine side).
func (r *PluginRegistry) Disable(name string) { r.disabled[name] = struct{}{} }

// Enable un-disables a plugin.
func (r *PluginRegistry) Enable(name string) { delete(r.disabled, name) }

// ── PreactAgent ─────────────────────────────────────────────────────────────

// PreactAgent is the public façade over the PreAct engine. Wires the
// building blocks (lobes / stages / flows / skills / tools), the I/O seams
// (client, session, memory), and the extensions (plugins, metacognition)
// into a ready-to-run agent.
type PreactAgent struct {
	mu sync.Mutex

	config Config

	// Resolved wiring.
	client        clients.LlmCall
	instructions  string
	session       *session.Session
	memory        Memory
	metacognition *metacognition.Metacognition
	lobes         []spec.Lobe
	stages        []any
	flows         []flows.Flow
	paths         []spec.Path
	skills        []any
	tools         any
	eventHooks    []EventHookFn
	preChecks     []PreCheckFn
	postChecks    []PostCheckFn
	prefetchHooks []PrefetchHookFn
	toolFilters   []ToolFilterFn
	finalizeHooks []FinalizeHookFn
	toolResHooks  []ToolResultHookFn

	// Engine surface (a Runner the engine package exposes).
	engine *Engine

	// Universal memory store (nil when off).
	memoryStore any

	// Last trace from the most recent query (or nil).
	lastTrace *result.Trace

	// In-process serving jobs.
	jobMu sync.Mutex
	jobs  map[string]chan events.AgentEvent
}

// Config is the immutable per-agent configuration a PreactAgent holds.
// “With“ builds a fresh agent with overrides applied.
type Config struct {
	Client           any
	Instructions     string
	Lobes            any
	Stages           any
	Flows            any
	Skills           []any
	Tools            []any
	ToolFilters      []ToolFilterFn
	MCPServers       []any
	Session          *session.Session
	Memory           Memory
	Plugins          any
	Metacognition    any
	Weights          map[string]any
	Budgets          map[string]any
	RequireCitations bool
	ShareHistory     bool
	ToolsInPrompt    bool
	Funnel           bool
	UniversalMemory  bool
	AutoEstablish    bool
	Embed            any
	TZ               string
	Lang             string
	PromptFormat     string
	Context          any
	Host             any
	PreTurnGate      any
}

// Memory is the narrow per-agent memory seam (the agent owns universal
// memory by default; pass a Memory to override). Mirrors agent_sdk/memory.
type Memory interface {
	ToolRuntime() any
	Read(ctx context.Context, scope, key string) (any, error)
	Write(ctx context.Context, scope, key string, value any) error
}

// NewPreactAgent builds the PreactAgent. Defaults: client must be supplied
// (string shorthand or clients.LlmCall), the built-in network is used when
// lobes/stages/flows are nil, an in-memory Session is created when none is
// passed, and universal memory is on. Ported from
// agent_sdk/agent.py:PreactAgent.__init__.
func NewPreactAgent(cfg Config) (*PreactAgent, error) {
	if cfg.Client == nil {
		return nil, fmt.Errorf("agent: a client is required")
	}
	// Defensive copy of the config (the original Config is mutated by the
	// caller-supplied Tools slice; this clone is what with_() copies off of).
	cfg = cloneConfig(cfg)
	c, err := clients.MakeClientErr(cfg.Client)
	if err != nil {
		return nil, err
	}
	metacog, err := metacognition.CoerceMetacognition(cfg.Metacognition)
	if err != nil {
		return nil, err
	}

	a := &PreactAgent{
		config:        cfg,
		client:        c,
		instructions:  cfg.Instructions,
		session:       cfg.Session,
		memory:        cfg.Memory,
		metacognition: metacog,
		jobs:          map[string]chan events.AgentEvent{},
	}

	if err := a.assemble(); err != nil {
		return nil, err
	}
	return a, nil
}

// MustPreactAgent is the panic-on-error form of NewPreactAgent. Useful in
// tests + main.
func MustPreactAgent(cfg Config) *PreactAgent {
	a, err := NewPreactAgent(cfg)
	if err != nil {
		panic(err)
	}
	return a
}

// ── Public API ──────────────────────────────────────────────────────────────

// Client returns the resolved LlmCall (after string shorthand).
func (a *PreactAgent) Client() clients.LlmCall { return a.client }

// Instructions returns the user-supplied system instructions.
func (a *PreactAgent) Instructions() string { return a.instructions }

// Engine returns the underlying Engine (a Runner the engine package owns).
// Tests + plugins may inspect / patch engine fields (e.g. finalize hooks).
func (a *PreactAgent) Engine() *Engine { return a.engine }

// LastTrace returns the trace from the most recent query/act, or nil.
func (a *PreactAgent) LastTrace() *result.Trace { return a.lastTrace }

// SuggestOptimizations returns a list of weight-patch proposals derived from
// the most recent trace. Mirrors PreactAgent.suggest_optimizations.
func (a *PreactAgent) SuggestOptimizations() []result.Optimization {
	tr := a.lastTrace
	if tr == nil {
		return nil
	}
	out := []result.Optimization{}
	for _, stage := range tr.FlowStages {
		flow, _ := stage["flow"].(string)
		name, _ := stage["stage"].(string)
		produced := false
		if steps, ok := stage["steps"].([]map[string]any); ok {
			for _, s := range steps {
				if k, _ := s["kind"].(string); k == "answer" {
					if txt, _ := s["text"].(string); txt != "" {
						produced = true
					}
				}
			}
		}
		skipped, _ := stage["skipped"].(bool)
		if flow != "" && name != "" && !produced && !skipped {
			out = append(out, result.Optimization{
				Axis:        "flow",
				Target:      flow + "." + name,
				Reason:      "stage produced no answer text",
				WeightPatch: map[string]float64{"flow_" + flow + "__step_" + name + "__disable": 1.0},
			})
		}
	}
	return out
}

// Spec renders the agent's serializable Spec (the Python build_spec shape).
// Ported from agent_sdk/agent.py:PreactAgent.spec.
func (a *PreactAgent) Spec() *spec.Spec {
	s := spec.NewSpec()
	s.Instructions = a.instructions
	s.TZ = a.config.TZ
	if s.TZ == "" {
		s.TZ = "UTC"
	}
	s.Lang = a.config.Lang
	if s.Lang == "" {
		s.Lang = "en"
	}
	s.RequireCitations = a.config.RequireCitations
	for _, l := range a.lobes {
		row := map[string]any{
			"id": l.ID, "behavior": l.Behavior, "layer": l.Layer,
			"order": l.Order, "prior": l.Prior, "pinned": l.Pinned,
			"min_activation": l.MinActivation,
		}
		if l.Writes != nil {
			row["writes"] = l.Writes
		}
		s.Lobes = append(s.Lobes, row)
	}
	for _, st := range a.stages {
		row := stageRow(st)
		if row != nil {
			s.Stages = append(s.Stages, row)
		}
	}
	for _, f := range a.flows {
		row := map[string]any{
			"id": f.ID(), "name": f.Name(), "description": f.Description,
			"use_when": f.UseWhen, "stages": f.Stages, "threshold": f.Threshold,
			"grounds": f.Grounds,
		}
		s.Flows = append(s.Flows, row)
	}
	return s
}

// With returns an immutable copy of this agent with the given overrides
// applied to the constructor config. The original agent is unchanged.
func (a *PreactAgent) With(overrides ...Overrides) *PreactAgent {
	cfg := a.config
	for _, o := range overrides {
		o(&cfg)
	}
	other, err := NewPreactAgent(cfg)
	if err != nil {
		panic(err)
	}
	return other
}

// Overrides is a function that mutates a Config in place. Used by With.
type Overrides func(*Config)

// OverrideInstructions replaces the agent's instructions.
func OverrideInstructions(s string) Overrides { return func(c *Config) { c.Instructions = s } }

// OverrideClient replaces the client.
func OverrideClient(c any) Overrides { return func(cfg *Config) { cfg.Client = c } }

// OverrideTools replaces the tool list.
func OverrideTools(tools ...any) Overrides { return func(cfg *Config) { cfg.Tools = tools } }

// OverrideFlows replaces the flow list.
func OverrideFlows(flows ...flows.Flow) Overrides { return func(cfg *Config) { cfg.Flows = flows } }

// OverrideStages replaces the stage list.
func OverrideStages(stages ...any) Overrides { return func(cfg *Config) { cfg.Stages = stages } }

// OverrideLobes replaces the lobe list.
func OverrideLobes(lobes ...spec.Lobe) Overrides { return func(cfg *Config) { cfg.Lobes = lobes } }

// OverridePlugins replaces the plugin list.
func OverridePlugins(plugins any) Overrides { return func(cfg *Config) { cfg.Plugins = plugins } }

// OverrideRequireCitations toggles the require_citations flag.
func OverrideRequireCitations(b bool) Overrides {
	return func(cfg *Config) { cfg.RequireCitations = b }
}

// OverrideMetacognition replaces metacognition mode.
func OverrideMetacognition(m any) Overrides { return func(cfg *Config) { cfg.Metacognition = m } }

// OverrideShareHistory toggles the share_history flag.
func OverrideShareHistory(b bool) Overrides { return func(cfg *Config) { cfg.ShareHistory = b } }

// OverrideFunnel toggles the funnel budget.
func OverrideFunnel(b bool) Overrides { return func(cfg *Config) { cfg.Funnel = b } }

// OverrideUniversalMemory toggles the universal_memory flag.
func OverrideUniversalMemory(b bool) Overrides { return func(cfg *Config) { cfg.UniversalMemory = b } }

// OverrideSession replaces the session.
func OverrideSession(s *session.Session) Overrides { return func(cfg *Config) { cfg.Session = s } }

// OverrideMemory replaces the memory.
func OverrideMemory(m Memory) Overrides { return func(cfg *Config) { cfg.Memory = m } }

// OverrideMCPServers replaces the MCP server list.
func OverrideMCPServers(servers ...any) Overrides {
	return func(cfg *Config) { cfg.MCPServers = servers }
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func cloneConfig(c Config) Config {
	out := c
	out.Tools = append([]any(nil), c.Tools...)
	out.Skills = append([]any(nil), c.Skills...)
	out.ToolFilters = append([]ToolFilterFn(nil), c.ToolFilters...)
	out.MCPServers = append([]any(nil), c.MCPServers...)
	if c.Weights != nil {
		out.Weights = make(map[string]any, len(c.Weights))
		for k, v := range c.Weights {
			out.Weights[k] = v
		}
	}
	if c.Budgets != nil {
		out.Budgets = make(map[string]any, len(c.Budgets))
		for k, v := range c.Budgets {
			out.Budgets[k] = v
		}
	}
	return out
}

// stageRow converts a stage (any of the supported types) to a Spec row.
func stageRow(st any) map[string]any {
	switch s := st.(type) {
	case *flows.FlowStep:
		return map[string]any{
			"name": s.Name, "lobes": s.Lobes, "loop": s.Loop, "tools": s.Tools,
			"description": s.Description, "fanout_key": s.FanoutKey,
		}
	case map[string]any:
		return s
	}
	return nil
}
