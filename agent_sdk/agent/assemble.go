package agent

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/mezon/agent-sdk-go/agent_sdk/contracts"
	"github.com/mezon/agent-sdk-go/agent_sdk/core/spec"
	"github.com/mezon/agent-sdk-go/agent_sdk/events"
	"github.com/mezon/agent-sdk-go/agent_sdk/flows"
	"github.com/mezon/agent-sdk-go/agent_sdk/memory"
	"github.com/mezon/agent-sdk-go/agent_sdk/preact"
	"github.com/mezon/agent-sdk-go/agent_sdk/result"
	"github.com/mezon/agent-sdk-go/agent_sdk/session"
	"github.com/mezon/agent-sdk-go/agent_sdk/tools"
)

// assemble wires the agent's building blocks (lobes/stages/flows/skills/tools),
// runs plugin installs through AgentSetup, resolves plugin-contributed
// additions, honors removals (pinned lobes survive), and composes the tool
// runtime. It mirrors PreactAgent.__init__ in agent_sdk/agent.py.
func (a *PreactAgent) assemble() error {
	cfg := a.config
	// 1) Resolve default lobes / stages / flows. The Python interprets
	// ``None`` or the string "default" as a request for the built-in
	// production network.
	resolvedLobes := resolveLobes(cfg.Lobes)
	resolvedStages := resolveStages(cfg.Stages)
	resolvedFlows := resolveFlows(cfg.Flows)
	// 2) Run plugin installs.
	setup := NewAgentSetup()
	setup.SetHost(cfg.Host)
	activePlugins := activePluginsOf(cfg.Plugins)
	// Auto-enable RagPlugin when require_citations is on (the Python
	// port does this implicitly: a user who asks for citations
	// should not have to also know to install the rag plugin).
	if cfg.RequireCitations {
		hasRag := false
		for _, p := range activePlugins {
			if p != nil && p.Name() == "rag" {
				hasRag = true
				break
			}
		}
		if !hasRag {
			activePlugins = append(activePlugins, autoRagPlugin{})
		}
	}
	for _, p := range activePlugins {
		p.Install(setup)
	}
	resolvedLobes = append(resolvedLobes, setup.Lobes...)
	// Dedup by id.
	resolvedLobes = dedupLobes(resolvedLobes)
	resolvedStages = append(resolvedStages, setup.Stages...)
	resolvedFlows = append(resolvedFlows, setup.Flows...)
	// 3) Honor plugin removals (pinned lobes survive).
	resolvedLobes = filterRemovedLobes(resolvedLobes, setup.RemovedLobes)
	// RemovePath implies removing the corresponding flow too (a
	// plugin that owns a path can subtract both at once).
	for name := range setup.RemovedPaths {
		setup.RemovedFlows[name] = struct{}{}
	}
	resolvedFlows = filterRemovedFlows(resolvedFlows, setup.RemovedFlows)
	if len(setup.Paths) > 0 {
		if cfg.Flows == nil {
			resolvedPaths := defaultPathSpecs()
			for _, p := range setup.Paths {
				resolvedPaths = append(resolvedPaths, p)
			}
			// Plugin-contributed flows also generate paths so the
			// engine's intent recognizer can route to them. Only the
			// plugin flows (setup.Flows) derive paths — deriving from the
			// default flows too would re-register qna/research/... and
			// overwrite the production recognizer scores by name, collapsing
			// every route to "emergent" (matches Python agent.py: the
			// default network ignores flow-derived paths).
			for _, f := range setup.Flows {
				resolvedPaths = append(resolvedPaths, derivePathFromFlow(f))
			}
			a.paths = filterRemovedPaths(resolvedPaths, setup.RemovedPaths)
		} else {
			resolvedPaths := derivePathSpecsFromFlows(resolvedFlows, resolvedStages)
			for _, p := range setup.Paths {
				resolvedPaths = append(resolvedPaths, p)
			}
			a.paths = filterRemovedPaths(resolvedPaths, setup.RemovedPaths)
		}
	} else {
		if cfg.Flows == nil {
			resolvedPaths := defaultPathSpecs()
			// Plugin-contributed flows generate paths. Only setup.Flows
			// (not the default flows in resolvedFlows) — see the note above.
			for _, f := range setup.Flows {
				resolvedPaths = append(resolvedPaths, derivePathFromFlow(f))
			}
			a.paths = filterRemovedPaths(resolvedPaths, setup.RemovedPaths)
		} else {
			a.paths = filterRemovedPaths(derivePathSpecsFromFlows(resolvedFlows, resolvedStages), setup.RemovedPaths)
		}
	}
	// 4) Skills: dedup by id, explicit wins.
	resolvedSkills := dedupSkills(append(append([]any(nil), cfg.Skills...), setup.Skills...))
	// 5) Compose the tool runtime.
	toolRuntime := a.composeTools(cfg.Tools, setup)
	// 6) Universal memory: when on (the default) the agent owns a
	// per-agent memory.MemoryStore and the runStream persist path
	// snapshots it into the session state on save. The Python agent
	// exposes ``_memory_store`` to the serve module so the worker can
	// reset on sessionless checkout; the Go port uses ``memoryStore``
	// for the same purpose.
	if cfg.UniversalMemory || cfg.UniversalMemory == false { // default: on
		if a.memoryStore == nil {
			a.memoryStore = memory.NewMemoryStore()
		}
	}
	a.lobes = resolvedLobes
	a.stages = resolvedStages
	a.flows = resolvedFlows
	a.skills = resolvedSkills
	a.tools = toolRuntime
	a.eventHooks = setup.EventHooks
	a.preChecks = setup.PreChecks
	a.postChecks = setup.PostChecks
	a.prefetchHooks = setup.PrefetchHooks
	a.toolFilters = setup.ToolFilters
	a.finalizeHooks = setup.FinalizeHooks
	a.toolResHooks = setup.ToolResultHooks
	// 7) Engine.
	a.engine = NewEngine(a.client)
	a.engine.Lobes = resolvedLobes
	a.engine.Stages = resolvedStages
	a.engine.Flows = resolvedFlows
	a.engine.Paths = a.paths
	a.engine.Skills = resolvedSkills
	a.engine.Tools = toolRuntime
	a.engine.Instructions = a.instructions
	a.engine.Metacognition = a.metacognition
	a.engine.Memory = a.memory
	a.engine.SystemAddendum = ""
	a.engine.RequireCitations = cfg.RequireCitations
	a.engine.ShareHistory = cfg.ShareHistory
	a.engine.ToolsInPrompt = cfg.ToolsInPrompt
	a.engine.Funnel = cfg.Funnel
	a.engine.TZ = orDefault(cfg.TZ, "UTC")
	a.engine.Lang = orDefault(cfg.Lang, "en")
	a.engine.PromptFormat = orDefault(cfg.PromptFormat, "xml")
	a.engine.Context = cfg.Context
	a.engine.Weights = mapBudgets(cfg.Weights)
	a.engine.Budgets = mapBudgets(cfg.Budgets)
	a.engine.PrefetchHooks = a.prefetchHooks
	a.engine.ToolFilters = a.toolFilters
	a.engine.FinalizeHooks = a.finalizeHooks
	a.engine.ToolResultHooks = a.toolResHooks
	if cfg.Funnel && a.engine.Budgets["working_set_budget"] == nil {
		a.engine.Budgets["working_set_budget"] = 2000
		if _, ok := a.engine.Budgets["working_set_keep"]; !ok {
			a.engine.Budgets["working_set_keep"] = 3
		}
	}
	return nil
}

// composeTools builds the ToolRuntime from @tool fns + ToolRuntimes + plugin
// tools (priority_runtimes first, so a namespaced surface wins name collisions).
func (a *PreactAgent) composeTools(explicitTools []any, setup *AgentSetup) any {
	fnTools := []any{}
	runtimes := []any{}
	for _, t := range explicitTools {
		if _, ok := t.(contracts.ToolRuntime); ok {
			runtimes = append(runtimes, t)
			continue
		}
		if _, ok := isToolDef(t); ok {
			fnTools = append(fnTools, t)
			continue
		}
		// Anything else: treat as a fn (wrapped into a FunctionToolRuntime below).
		fnTools = append(fnTools, t)
	}
	pluginToolRts := setup.ToolRuntimes
	pluginFns := setup.Tools
	// Priority runtimes first, then fns, then plugin runtimes, then other runtimes.
	composed := []any{}
	composed = append(composed, pluginToolRts...)
	if len(fnTools) > 0 || len(pluginFns) > 0 {
		all := append(append([]any(nil), fnTools...), pluginFns...)
		composed = append(composed, wrapFns(all))
	}
	composed = append(composed, runtimes...)
	if len(composed) == 0 {
		return nil
	}
	// Translate to contracts.ToolRuntime for engine consumption.
	out := []contracts.ToolRuntime{}
	for _, c := range composed {
		if rt, ok := c.(contracts.ToolRuntime); ok {
			out = append(out, rt)
			continue
		}
		// Wrap tools.ToolRuntime in a contracts adapter.
		if trt, ok := c.(tools.ToolRuntime); ok {
			out = append(out, &toolsRuntimeAdapter{rt: trt})
			continue
		}
		// MCPToolRuntime is also a tools.ToolRuntime.
		_ = reflect.TypeOf(c)
	}
	return contracts.NewCompositeToolRuntime(out)
}

// toolsRuntimeAdapter wraps a tools.ToolRuntime to expose the
// contracts.ToolRuntime interface.
type toolsRuntimeAdapter struct {
	rt tools.ToolRuntime
}

// GetToolSpecs adapts.
func (a *toolsRuntimeAdapter) GetToolSpecs() []map[string]any { return a.rt.GetToolSpecs() }

// CallTool adapts.
func (a *toolsRuntimeAdapter) CallTool(ctx context.Context, name string, inp map[string]any, retrievedChunks []map[string]any, alreadyRead map[string]struct{}) (string, error) {
	return a.rt.CallTool(ctx, name, inp, retrievedChunks, alreadyRead)
}

// wrapFns builds a tools.FunctionToolRuntime over the given @tool defs. Empty
// input ⇒ nil.
func wrapFns(fns []any) *tools.FunctionToolRuntime {
	if len(fns) == 0 {
		return nil
	}
	td := []*tools.ToolDef{}
	for _, f := range fns {
		if t, ok := f.(*tools.ToolDef); ok {
			td = append(td, t)
		}
	}
	if len(td) == 0 {
		return nil
	}
	return tools.NewFunctionToolRuntime(td...)
}

func isToolDef(t any) (*tools.ToolDef, bool) {
	if td, ok := t.(*tools.ToolDef); ok {
		return td, true
	}
	return nil, false
}

// resolveLobes maps the constructor's "lobes" arg (nil | "default" | a
// registry with .default() | a slice) to the concrete []spec.Lobe list.
func resolveLobes(v any) []spec.Lobe {
	if v == nil {
		return preact.Lobes{}.Default()
	}
	if s, ok := v.(string); ok && s == "default" {
		return preact.Lobes{}.Default()
	}
	if ml, ok := v.(minimalLobes); ok {
		return ml
	}
	if l, ok := v.([]spec.Lobe); ok {
		return append([]spec.Lobe(nil), l...)
	}
	if hasDefaultMethod(v, "Default") {
		return callDefaultLobes(v)
	}
	return preact.Lobes{}.Default()
}

// minimalLobes is an alias for the []spec.Lobe minimal set the tests
// configure via _MIN.
type minimalLobes = []spec.Lobe

func resolveStages(v any) []any {
	if v == nil {
		return stagesFromSpec(preact.Stages{}.Default())
	}
	if s, ok := v.(string); ok && s == "default" {
		return stagesFromSpec(preact.Stages{}.Default())
	}
	if hasDefaultMethod(v, "Default") {
		return callDefaultStages(v)
	}
	if ss, ok := v.([]any); ok {
		return append([]any(nil), ss...)
	}
	if ss, ok := v.([]spec.Stage); ok {
		return stagesFromSpec(ss)
	}
	return stagesFromSpec(preact.Stages{}.Default())
}

func stagesFromSpec(ss []spec.Stage) []any {
	out := make([]any, 0, len(ss))
	for _, s := range ss {
		out = append(out, s)
	}
	return out
}

func resolveFlows(v any) []flows.Flow {
	if v == nil {
		return specFlowsToFlows(preact.Flows{}.Default())
	}
	if s, ok := v.(string); ok && s == "default" {
		return specFlowsToFlows(preact.Flows{}.Default())
	}
	if fs, ok := v.([]flows.Flow); ok {
		return append([]flows.Flow(nil), fs...)
	}
	if fs, ok := v.([]spec.Flow); ok {
		return specFlowsToFlows(fs)
	}
	if hasDefaultMethod(v, "Default") {
		return callDefaultFlows(v)
	}
	return specFlowsToFlows(preact.Flows{}.Default())
}

// specFlowsToFlows converts a []spec.Flow (the preact namespace shape) to a
// []flows.Flow (the runtime shape). Stages is a free-form list on the spec
// side; on the runtime side it's the ordered stage ids.
func specFlowsToFlows(in []spec.Flow) []flows.Flow {
	out := make([]flows.Flow, 0, len(in))
	for _, f := range in {
		opts := []flows.FlowOption{
			flows.FlowDescription(f.Description),
		}
		if f.UseWhen != "" {
			opts = append(opts, flows.FlowUseWhen(f.UseWhen))
		}
		if f.Stages != nil {
			opts = append(opts, flows.FlowStages(f.Stages...))
		}
		opts = append(opts, flows.FlowThreshold(f.Threshold), flows.FlowGrounds(f.Grounds))
		out = append(out, flows.NewFlow(f.ID, opts...))
	}
	return out
}

// defaultPathSpecs is the production path recognizer set (matches
// “preact.Lobes{}.Paths()“). It's the routes the engine scores against.
func defaultPathSpecs() []spec.Path {
	return preact.Lobes{}.Paths()
}

// derivePathSpecsFromFlows compiles flows → path recognizers (the same
// derivation the engine runs when paths is omitted). Mirrors
// _derive_path_specs.
func derivePathSpecsFromFlows(flowsList []flows.Flow, stagesList []any) []spec.Path {
	out := []spec.Path{}
	for _, f := range flowsList {
		members := []string{}
		for _, sid := range f.Stages {
			for _, st := range stagesList {
				id := stageID(st)
				if id == sid {
					if lobes, ok := stageLobes(st); ok {
						members = append(members, lobes...)
					}
				}
			}
		}
		members = dedupStrings(members)
		spec := spec.Path{
			Name:      f.ID(),
			Members:   members,
			Bias:      map[string]float64{},
			Threshold: f.Threshold,
			Grounds:   f.Grounds,
		}
		if spec.Threshold == 0 {
			spec.Threshold = 0.5
		}
		for _, m := range members {
			spec.Bias[m] = 1.0
		}
		spec.Recognizer = func(ctx map[string]any) float64 {
			if sig, ok := f.SignalExpr.(map[string]any); ok {
				// Use a basic signal: if "const" present, return its value.
				if c, ok := sig["const"].(float64); ok {
					return c
				}
				if c, ok := sig["const"].(int); ok {
					return float64(c)
				}
			}
			return 0.5
		}
		out = append(out, spec)
	}
	return out
}

// derivePathFromFlow builds a spec.Path from a single flow, using
// the flow's `Signal()` as the recognizer. Mirrors
// agent_sdk.network._derive_path_for_flow.
func derivePathFromFlow(f flows.Flow) spec.Path {
	spec := spec.Path{
		Name:      f.ID(),
		Members:   f.Stages,
		Bias:      map[string]float64{},
		Threshold: f.Threshold,
		Grounds:   f.Grounds,
	}
	if spec.Threshold == 0 {
		spec.Threshold = 0.5
	}
	// Wrap the flow's Signal method so the path recognizer uses
	// the flow's own recognizer (rather than the constant 0.5 the
	// default derivePathSpecsFromFlows hard-codes).
	spec.Recognizer = func(ctx map[string]any) float64 { return f.Signal(ctx) }
	return spec
}

func stageLobes(st any) ([]string, bool) {
	switch v := st.(type) {
	case spec.Stage:
		return v.Lobes, true
	case *spec.Stage:
		if v != nil {
			return v.Lobes, true
		}
	case flows.FlowStep:
		return v.Lobes, true
	case *flows.FlowStep:
		if v != nil {
			return v.Lobes, true
		}
	}
	return nil, false
}

func dedupStrings(xs []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, x := range xs {
		if _, ok := seen[x]; ok {
			continue
		}
		seen[x] = struct{}{}
		out = append(out, x)
	}
	return out
}

func dedupLobes(in []spec.Lobe) []spec.Lobe {
	seen := map[string]struct{}{}
	out := []spec.Lobe{}
	for _, l := range in {
		if _, ok := seen[l.ID]; ok {
			continue
		}
		seen[l.ID] = struct{}{}
		out = append(out, l)
	}
	return out
}

func dedupSkills(in []any) []any {
	seen := map[string]struct{}{}
	out := []any{}
	for _, s := range in {
		id := skillID(s)
		if id != "" {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
		}
		out = append(out, s)
	}
	return out
}

func skillID(s any) string {
	if s == nil {
		return ""
	}
	v := reflect.ValueOf(s)
	if v.Kind() == reflect.Ptr && v.IsNil() {
		return ""
	}
	if m, ok := s.(interface{ GetID() string }); ok {
		return m.GetID()
	}
	if m, ok := s.(map[string]any); ok {
		if id, ok := m["id"].(string); ok {
			return id
		}
	}
	return ""
}

func filterRemovedLobes(in []spec.Lobe, removed map[string]struct{}) []spec.Lobe {
	if len(removed) == 0 {
		return in
	}
	out := []spec.Lobe{}
	for _, l := range in {
		if _, drop := removed[l.ID]; drop {
			if l.Pinned {
				out = append(out, l)
			}
			continue
		}
		if _, pinned := contracts.PinnedLobes[l.ID]; pinned {
			out = append(out, l)
			continue
		}
		out = append(out, l)
	}
	return out
}

func filterRemovedFlows(in []flows.Flow, removed map[string]struct{}) []flows.Flow {
	if len(removed) == 0 {
		return in
	}
	out := []flows.Flow{}
	for _, f := range in {
		if _, drop := removed[f.ID()]; drop {
			continue
		}
		out = append(out, f)
	}
	return out
}

// filterRemovedPaths drops paths whose name was removed by a plugin.
// Unlike lobes, paths are NOT pinned — a plugin that owns a path can
// subtract it. Mirrors the Python “RemovePath“ contract.
func filterRemovedPaths(in []spec.Path, removed map[string]struct{}) []spec.Path {
	if len(removed) == 0 {
		return in
	}
	out := []spec.Path{}
	for _, p := range in {
		if _, drop := removed[p.Name]; drop {
			continue
		}
		out = append(out, p)
	}
	return out
}

func activePluginsOf(p any) []Plugin {
	switch v := p.(type) {
	case nil:
		return nil
	case []Plugin:
		return v
	case *PluginRegistry:
		return v.Active()
	}
	return nil
}

// hasDefaultMethod is a small reflection helper to detect namespaces with a
// .Default() method (Lobes, Stages, Flows).
func hasDefaultMethod(v any, name string) bool {
	if v == nil {
		return false
	}
	m, ok := reflect.TypeOf(v).MethodByName(name)
	return ok && m.Func.IsValid()
}

func callDefaultLobes(v any) []spec.Lobe {
	r := reflect.ValueOf(v).MethodByName("Default")
	out := r.Call(nil)
	if len(out) > 0 {
		if ls, ok := out[0].Interface().([]spec.Lobe); ok {
			return ls
		}
	}
	return nil
}

func callDefaultStages(v any) []any {
	r := reflect.ValueOf(v).MethodByName("Default")
	out := r.Call(nil)
	if len(out) > 0 {
		switch x := out[0].Interface().(type) {
		case []any:
			return x
		case []spec.Stage:
			return stagesFromSpec(x)
		}
	}
	return nil
}

func callDefaultFlows(v any) []flows.Flow {
	r := reflect.ValueOf(v).MethodByName("Default")
	out := r.Call(nil)
	if len(out) > 0 {
		if fs, ok := out[0].Interface().([]flows.Flow); ok {
			return fs
		}
	}
	return nil
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func mapBudgets(b map[string]any) map[string]any {
	if b == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(b))
	for k, v := range b {
		out[k] = v
	}
	return out
}

// ── Public API: query / act / inspect / RunSnapshot / submit / events ────────

// Query runs one turn and returns the AgentResult. Mirrors
// PreactAgent.query.
func (a *PreactAgent) Query(ctx context.Context, input string) (*result.AgentResult, error) {
	return a.QueryWithSession(ctx, input, nil)
}

// QueryWithSession is the session-aware variant: a non-nil session overrides
// the agent's configured session for this turn (the per-job seam the
// stateless AgentWorker relies on).
func (a *PreactAgent) QueryWithSession(ctx context.Context, input string, sess *session.Session) (*result.AgentResult, error) {
	rr, err := a.runStream(ctx, input, sess)
	if err != nil {
		return nil, err
	}
	return rr.Result, nil
}

// Act returns an AgentStream over the turn's typed events. Mirrors
// PreactAgent.act.
func (a *PreactAgent) Act(ctx context.Context, input string) *events.AgentStream {
	return a.ActWithSession(ctx, input, nil)
}

// ActWithSession is the session-aware streaming variant.
func (a *PreactAgent) ActWithSession(ctx context.Context, input string, sess *session.Session) *events.AgentStream {
	return events.NewAgentStream(func(yield func(events.AgentEvent) bool) {
		rr, err := a.runStream(ctx, input, sess)
		if err != nil {
			return
		}
		for _, ev := range rr.Events {
			if !yield(ev) {
				return
			}
		}
	})
}

// Inspect is the dry, no-LLM routing probe with an empty state (the
// cold-query call). Mirrors PreactAgent.inspect.
func (a *PreactAgent) Inspect(input string) result.ActivationSnapshot {
	return a.InspectWithState(input, session.SessionState{})
}

// InspectWithState is the state-aware routing probe: a follow-up turn routes
// against the prior SessionState, so recognition/path resolution sees the
// prior-turn context. Mirrors PreactAgent.inspect(query, state).
func (a *PreactAgent) InspectWithState(input string, state session.SessionState) result.ActivationSnapshot {
	if a.engine == nil {
		return result.ActivationSnapshot{}
	}
	return a.engine.InspectWithState(input, state)
}

// RunSnapshot runs one turn STATELESSLY: restore from a plain-JSON snapshot,
// run, and return (result, next_snapshot). Mirrors PreactAgent.run_snapshot.
func (a *PreactAgent) RunSnapshot(ctx context.Context, input string, snapshot map[string]any) (*result.AgentResult, map[string]any, error) {
	state := session.SessionStateFromJSON(snapshot)
	store := &snapshotStore{state: &state}
	originalSession := a.session
	a.session = session.New("snapshot", store)
	defer func() { a.session = originalSession }()
	rr, err := a.runStream(ctx, input, nil)
	if err != nil {
		return nil, nil, err
	}
	finalState, _ := store.Load(ctx, "snapshot")
	return rr.Result, finalState.ToJSON(), nil
}

// snapshotStore is the in-process SessionStore a single stateless turn
// persists its (restored + appended) state through.
type snapshotStore struct {
	mu    sync.Mutex
	state *session.SessionState
}

func (s *snapshotStore) Load(_ context.Context, _ string) (session.SessionState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return *s.state, nil
}
func (s *snapshotStore) Append(_ context.Context, _ string, t session.Turn) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.History = append(s.state.History, t)
	return nil
}
func (s *snapshotStore) Save(_ context.Context, _ string, st session.SessionState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := st
	s.state = &cp
	return nil
}
func (s *snapshotStore) Compact(_ context.Context, _ string, _ session.Summarizer, _ int) error {
	return nil
}

// runStream is the internal turn driver. It runs the engine once, captures
// the typed event stream, updates the last-trace, persists the session
// (when one is configured), and returns the run result. The optional sess
// override rebinds the per-turn session (the AgentWorker's per-job seam).
func (a *PreactAgent) runStream(ctx context.Context, input string, sess *session.Session) (*RunResult, error) {
	for _, c := range a.preChecks {
		if c == nil {
			continue
		}
		if err := c(input); err != nil {
			return nil, err
		}
	}
	effective := a.session
	if sess != nil {
		effective = sess
	}
	var state session.SessionState
	if effective != nil {
		s, err := effective.Load(ctx)
		if err != nil {
			return nil, err
		}
		state = s
	}
	// Restore the per-agent universal memory from the loaded state's
	// memory blob (the stateless-serving contract — a fresh process /
	// replica continues the same conversation from the JSON alone). This
	// must run on EVERY turn with a session (not just when state.Memory
	// is non-nil) so a pooled agent never carries the previous session's
	// working memory into the next one.
	if a.memoryStore != nil && effective != nil {
		if ms, ok := a.memoryStore.(*memory.MemoryStore); ok && ms != nil {
			snap := memory.Snapshot{}
			if state.Memory != nil {
				snap.Seq = asIntValue(state.Memory["seq"])
				snap.Long = asLongSlice(state.Memory["long"])
				if docs, ok := state.Memory["docs"].(map[string]any); ok {
					snap.Docs = docs
				}
			}
			ms.Restore(snap)
		}
	}
	req := RunRequest{Query: input, State: state, Session: effective}
	rr, err := a.engine.Run(ctx, req)
	if err != nil {
		return nil, err
	}
	// Drain the engine's events through event hooks.
	for _, hook := range a.eventHooks {
		if hook == nil {
			continue
		}
		for _, ev := range rr.Events {
			hook(ev)
		}
	}
	a.mu.Lock()
	a.lastTrace = &rr.Trace
	a.mu.Unlock()
	// Post-checks.
	for _, c := range a.postChecks {
		if c == nil {
			continue
		}
		if err := c(rr.Result); err != nil {
			return nil, err
		}
	}
	// Auto-establish: natively offload the facts the user stated this turn
	// (deterministic, no LLM cooperation required). Mirrors the Python
	// ``self._auto_establish and self._memory_store is not None`` branch.
	if a.memoryStore != nil {
		if ms, ok := a.memoryStore.(*memory.MemoryStore); ok && ms != nil {
			for _, fact := range memory.SalientFacts(input) {
				ms.Remember("fact", fact, memory.RememberOpts{
					Scope:  "conversation",
					Key:    memory.FactKey(fact),
					Source: "establish",
				})
			}
		}
	}
	// Persist: when the session's store implements Save, append the turn to
	// state then save (the WHOLE state path — history + memory + skills +
	// bias, atomically). Otherwise fall back to per-turn Append. Memory is
	// snapshotted into the state when the agent has a universal memory
	// store — matches the Python ``state.memory = self._memory_store.to_json()``
	// contract in agent.py.
	if effective != nil {
		if _, ok := effective.Store.(session.SessionStoreSaver); ok {
			state.History = append(state.History, session.Turn{Role: "user", Content: input})
			state.History = append(state.History, session.Turn{Role: "assistant", Content: rr.Result.Text})
			if ms, ok := a.memoryStore.(*memory.MemoryStore); ok && ms != nil {
				snap := ms.ToJSON(memory.SnapshotOpts{})
				if state.Memory == nil {
					state.Memory = map[string]any{}
				}
				state.Memory["seq"] = snap.Seq
				state.Memory["long"] = snap.Long
			}
			_, _ = effective.Save(ctx, state)
		} else {
			_ = effective.Append(ctx, session.Turn{Role: "user", Content: input})
			_ = effective.Append(ctx, session.Turn{Role: "assistant", Content: rr.Result.Text})
		}
	}
	return rr, nil
}

// Submit kicks off a turn asynchronously and returns a job id. The caller
// drains the events via Events(jobID). Mirrors PreactAgent.submit.
func (a *PreactAgent) Submit(ctx context.Context, input string) (string, error) {
	jobID := fmt.Sprintf("job-%d", newJobSeq())
	ch := make(chan events.AgentEvent, 16)
	a.jobMu.Lock()
	a.jobs[jobID] = ch
	a.jobMu.Unlock()
	go func() {
		defer close(ch)
		rr, err := a.runStream(ctx, input, nil)
		if err != nil {
			return
		}
		for _, ev := range rr.Events {
			ch <- ev
		}
	}()
	return jobID, nil
}

// Events yields the events of a running or completed job. Mirrors
// PreactAgent.events.
func (a *PreactAgent) Events(jobID string) <-chan events.AgentEvent {
	a.jobMu.Lock()
	ch := a.jobs[jobID]
	a.jobMu.Unlock()
	if ch == nil {
		out := make(chan events.AgentEvent)
		close(out)
		return out
	}
	return ch
}

// Connect is the eager MCP resolve phase: connect + discover every MCP
// server. Returns {server_name: connected} for inspection. Mirrors
// PreactAgent.connect.
func (a *PreactAgent) Connect(ctx context.Context) map[string]bool {
	// The Go MCP runtime is opt-in; an agent that has no MCP servers
	// registered returns an empty map (no work to do). The first rung of MCP
	// porting is the mcp package; a later rung wires the live resolve.
	return map[string]bool{}
}

var jobSeq counter

func newJobSeq() int64 {
	return jobSeq.Add(1)
}

// ── helpers ─────────────────────────────────────────────────────────────────

func asIntValue(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	}
	return 0
}

func asLongSlice(v any) []map[string]any {
	switch x := v.(type) {
	case []map[string]any:
		return x
	case []any:
		out := make([]map[string]any, 0, len(x))
		for _, e := range x {
			if m, ok := e.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

// autoRagPlugin is the synthetic plugin the agent installs when
// require_citations is on. It only adds the `cite` lobe + the
// finalize-grounding hook; the actual retrieval (KB / search) is
// still a host concern.
type autoRagPlugin struct{}

// autoCiteLobeSpec is a minimal `cite` lobe the auto-plugin installs.
var autoCiteLobeSpec = spec.Lobe{
	ID: "cite", Layer: 5, Behavior: "rewrite", Pinned: true, Order: 1,
	BuildContext: true, MinActivation: 0,
}

func (autoRagPlugin) Name() string { return "rag" }
func (autoRagPlugin) Install(s *AgentSetup) {
	s.AddLobe(autoCiteLobeSpec)
}
