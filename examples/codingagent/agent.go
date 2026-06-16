package codingagent

import (
	"context"
	"os"
	"path/filepath"

	"github.com/nccasia/agent-sdk-go/agent_sdk/agent"
	"github.com/nccasia/agent-sdk-go/agent_sdk/core/spec"
	"github.com/nccasia/agent-sdk-go/agent_sdk/flows"
	"github.com/nccasia/agent-sdk-go/agent_sdk/memory"
	"github.com/nccasia/agent-sdk-go/agent_sdk/react"
)

// Instructions states the workflow, not the tool mechanics (the tools are
// Claude Code's canonical Read/Write/Edit/Bash/Glob/Grep/LS). Mirrors
// coding_agent.agent.INSTRUCTIONS.
const Instructions = "You are an interactive coding agent working in the user's real repository " +
	"(which may be large). Follow standard practice:\n" +
	"- Orient before acting: use Glob and Grep to locate the few relevant files, then " +
	"Read the exact files (offset/limit for large ones). Never guess file contents.\n" +
	"- Edit with Edit (exact-string match — Read first) or Write for new files. Match " +
	"the surrounding style, change as little as possible, and keep the tree green — run " +
	"the tests with Bash.\n" +
	"- Track multi-step work in memory (action=remember, scope=conversation, key=plan).\n" +
	"- Report concisely what you found or changed, citing concrete files.\n" +
	"\n" +
	"To understand a whole system and document it: survey the structure (Glob/Grep/LS), " +
	"plan the subsystems, investigate each by Reading the real code, saving each finding " +
	"to memory (key=finding:<area>); then recall all findings and Write a single " +
	"ARCHITECTURE.md."

// CodingPlugin packages the whole coding capability as ONE first-class plugin:
// lobes + stages + flows + paths + tools + the read-only/grounding write guards.
// Mirrors coding_agent.agent.CodingPlugin.
type CodingPlugin struct {
	Root string
}

// Name returns the plugin name.
func (p *CodingPlugin) Name() string { return "coding" }

// Install wires the whole coding network into the AgentSetup.
func (p *CodingPlugin) Install(setup *agent.AgentSetup) {
	for _, lobe := range CodingLobeSpecs() {
		setup.AddLobe(lobe)
	}
	for _, st := range CodingStagesAny() {
		setup.AddStage(st)
	}
	for _, fl := range CodingFlows() {
		setup.AddFlow(fl)
	}
	for _, pa := range codingPaths() {
		setup.AddPath(pa)
	}
	for _, t := range CodingTools(p.Root) {
		setup.AddTool(t)
	}

	// Read-only stages must not write files.
	writeGuard := react.NewDocWriteGuard(react.DocGuardOpts{
		WriteTools:     []string{"Write"},
		BashTool:       "Bash",
		ReadonlyStages: ReadonlyStages,
	})
	setup.AddToolFilter(func(stageID, name string, inp map[string]any) string {
		return writeGuard.Check(stageID, name, inp)
	})

	// A written doc must not cite paths that don't exist.
	root, _ := filepath.Abs(p.Root)
	groundGuard := react.NewDocGroundingGuard(
		react.WithExists(func(rel string) bool {
			_, err := os.Stat(filepath.Join(root, rel))
			return err == nil
		}),
		react.WithReadTools("Read"),
		react.WithWriteTools("Write"),
		react.WithDocSuffixes(".md"),
	)
	setup.AddToolFilter(func(stageID, name string, inp map[string]any) string {
		return groundGuard.Check(stageID, name, inp)
	})
}

// BuildCodingAgent builds a Claude-Code-grade coding agent bound to the real
// directory root, by mounting CodingPlugin on a bare base network. Mirrors
// coding_agent.agent.build_coding_agent.
func BuildCodingAgent(root string, client any, opts ...Option) *agent.PreactAgent {
	o := options{shareHistory: true, repoMap: true}
	for _, opt := range opts {
		opt(&o)
	}
	budgets := map[string]any{
		"working_set_budget":     6000,
		"working_set_keep":       4,
		"working_set_max_spent":  8,
		"stall_patience":         3,
		"enforce_tool_allowlist": true,
	}
	for k, v := range o.budgets {
		budgets[k] = v
	}

	instructions := Instructions
	if o.repoMap {
		instructions = Instructions + "\n\n" + BuildRepoMap(root)
	}

	mem := o.memory
	if mem == nil {
		mem = memory.NewMemory(nil, nil)
	}

	plugins := []agent.Plugin{&CodingPlugin{Root: root}}
	plugins = append(plugins, o.plugins...)

	cfg := agent.Config{
		Client:        client,
		Instructions:  instructions,
		Plugins:       plugins,
		Lobes:         []spec.Lobe{},
		Stages:        []any{},
		Flows:         []flows.Flow{},
		Memory:        memoryAdapter{mem},
		Tools:         []any{mem.ToolRuntime()},
		ShareHistory:  o.shareHistory,
		Funnel:        true,
		ToolsInPrompt: true,
		Budgets:       budgets,
	}
	return agent.MustPreactAgent(cfg)
}

// memoryAdapter adapts *memory.Memory to the agent.Memory interface (whose
// ToolRuntime returns any).
type memoryAdapter struct{ m *memory.Memory }

func (a memoryAdapter) ToolRuntime() any { return a.m.ToolRuntime() }
func (a memoryAdapter) Read(ctx context.Context, scope, key string) (any, error) {
	return a.m.Read(ctx, scope, key)
}
func (a memoryAdapter) Write(ctx context.Context, scope, key string, value any) error {
	return a.m.Write(ctx, scope, key, value)
}

// options + Option configure BuildCodingAgent.
type options struct {
	shareHistory bool
	repoMap      bool
	budgets      map[string]any
	plugins      []agent.Plugin
	memory       *memory.Memory
}

// Option configures BuildCodingAgent.
type Option func(*options)

// WithShareHistory toggles cross-stage history sharing (default on).
func WithShareHistory(b bool) Option { return func(o *options) { o.shareHistory = b } }

// WithRepoMap toggles injecting the deterministic repo map (default on).
func WithRepoMap(b bool) Option { return func(o *options) { o.repoMap = b } }

// WithBudgets overrides individual budget keys.
func WithBudgets(b map[string]any) Option { return func(o *options) { o.budgets = b } }

// WithPlugins augments the agent with extra plugins.
func WithPlugins(p ...agent.Plugin) Option {
	return func(o *options) { o.plugins = append(o.plugins, p...) }
}

// WithMemory overrides the durable memory.
func WithMemory(m *memory.Memory) Option { return func(o *options) { o.memory = m } }
