// Package engine is the generic PreAct turn kernel and its data model: Stages
// (one execution unit each), the StageRegistry (the per-turn stage table), and
// per-stage overrides. A turn = recognize flow → activate lobes → resolve
// stages → build per-stage prompt, wrapped around the I/O seams.
//
// Ported from agent_sdk/{stages,stage_overrides,engine}.py.
package engine

import "github.com/mezon/agent-sdk-go/agent_sdk/flows"

// Stage is one execution unit, Activable like a Lobe or Flow: a slice of lobes
// it consults, a loop mode, and its tools. Its Signal gates whether the step
// runs this turn (0 = skip; default always-on 1.0).
//
// loop ∈ none (pure prompt) · single (one LLM call) · agentic (a ReAct tool
// loop) · map (fan-out over a scratchpad key). A Stage compiles to the internal
// FlowStep the runtime consumes via ToFlowStep.
type Stage struct {
	// Activable surface.
	IDField     string
	NameField   string
	Description string
	UseWhenStr  string

	// Execution shape.
	Lobes     []string
	Loop      string // none | single | agentic | map; default single
	Tools     []string
	FanoutKey string
	Threshold float64 // min activation to run; 0 = always-on (default)

	// Fan-out shape (loop="map" only).
	FanoutParallel bool
	FanoutMax      int // concurrency cap / item ceiling
	FanoutIsolated bool

	// Per-stage inference overrides (nil ⇒ engine/policy default).
	Model        *string
	Temperature  *float64
	MaxTokens    *int
	Hops         *int
	SystemPrompt *string
	Subject      *string

	signalFn func(ctx map[string]any) float64
}

// StageOption configures a Stage built via NewStage.
type StageOption func(*Stage)

// StageName sets the display name (defaults to the id).
func StageName(name string) StageOption { return func(s *Stage) { s.NameField = name } }

// StageDescription sets the one-line description.
func StageDescription(d string) StageOption { return func(s *Stage) { s.Description = d } }

// StageUseWhen sets the natural-language trigger.
func StageUseWhen(u string) StageOption { return func(s *Stage) { s.UseWhenStr = u } }

// StageLobes sets the slice of lobes the stage consults.
func StageLobes(lobes ...string) StageOption {
	return func(s *Stage) { s.Lobes = append([]string(nil), lobes...) }
}

// StageLoop sets the loop mode (none | single | agentic | map).
func StageLoop(loop string) StageOption { return func(s *Stage) { s.Loop = loop } }

// StageTools sets the tool allowlist.
func StageTools(tools ...string) StageOption {
	return func(s *Stage) { s.Tools = append([]string(nil), tools...) }
}

// StageFanoutKey sets the scratchpad list key a map stage fans out over.
func StageFanoutKey(k string) StageOption { return func(s *Stage) { s.FanoutKey = k } }

// StageFanoutParallel sets concurrent fan-out (loop="map" only).
func StageFanoutParallel(b bool) StageOption { return func(s *Stage) { s.FanoutParallel = b } }

// StageFanoutMax sets the concurrency cap / item ceiling.
func StageFanoutMax(n int) StageOption { return func(s *Stage) { s.FanoutMax = n } }

// StageFanoutIsolated sets per-worker evidence isolation.
func StageFanoutIsolated(b bool) StageOption { return func(s *Stage) { s.FanoutIsolated = b } }

// StageThreshold sets the min activation to run (0 = always-on).
func StageThreshold(t float64) StageOption { return func(s *Stage) { s.Threshold = t } }

// StageSignal sets the gating signal callable (default always-on 1.0).
func StageSignal(fn func(ctx map[string]any) float64) StageOption {
	return func(s *Stage) { s.signalFn = fn }
}

// StageModel sets the per-stage model override.
func StageModel(m string) StageOption { return func(s *Stage) { s.Model = &m } }

// StageTemperature sets the per-stage temperature override.
func StageTemperature(t float64) StageOption { return func(s *Stage) { s.Temperature = &t } }

// StageMaxTokens sets the per-stage max_tokens override.
func StageMaxTokens(n int) StageOption { return func(s *Stage) { s.MaxTokens = &n } }

// StageHops sets the per-stage hop budget override.
func StageHops(n int) StageOption { return func(s *Stage) { s.Hops = &n } }

// StageSystemPrompt sets the per-stage system prompt override.
func StageSystemPrompt(p string) StageOption { return func(s *Stage) { s.SystemPrompt = &p } }

// StageSubject sets the sub-question/aspect this stage instance works on.
func StageSubject(s string) StageOption { return func(st *Stage) { st.Subject = &s } }

// NewStage is the concise builder for a stage (signal defaults to always-on).
// Defaults: loop "single", fanout_max 40, threshold 0. Mirrors the Python
// stage() builder.
func NewStage(id string, opts ...StageOption) *Stage {
	s := &Stage{IDField: id, Loop: "single", FanoutMax: 40}
	for _, opt := range opts {
		opt(s)
	}
	if s.NameField == "" {
		s.NameField = s.IDField
	}
	return s
}

// ID returns the stage id (Activable surface).
func (s *Stage) ID() string { return s.IDField }

// Name returns the display name (Activable surface).
func (s *Stage) Name() string { return s.NameField }

// Signal is the deterministic, free activation in [0, 1] — gates the step
// (0 = skip). Defaults to always-on (1.0).
func (s *Stage) Signal(ctx map[string]any) float64 {
	if s.signalFn != nil {
		return s.signalFn(ctx)
	}
	return 1.0
}

// ToFlowStep compiles the stage to the internal FlowStep the runtime consumes.
func (s *Stage) ToFlowStep() flows.FlowStep {
	if s.IDField == "" {
		panic("Stage requires a non-empty id")
	}
	sid := s.IDField
	return flows.NewFlowStep(flows.FlowStep{
		Name:           sid,
		Lobes:          append([]string(nil), s.Lobes...),
		Loop:           s.Loop,
		Tools:          append([]string(nil), s.Tools...),
		Description:    s.Description,
		FanoutKey:      s.FanoutKey,
		FanoutParallel: s.FanoutParallel,
		FanoutMax:      s.FanoutMax,
		FanoutIsolated: s.FanoutIsolated,
		Signals: func(ctx map[string]any) map[string]float64 {
			return map[string]float64{sid: s.Signal(ctx)}
		},
		SignalWeights: map[string]float64{sid: 1.0},
		MinActivation: s.Threshold,
		Model:         s.Model,
		Temperature:   s.Temperature,
		MaxTokens:     s.MaxTokens,
		Hops:          s.Hops,
		SystemPrompt:  s.SystemPrompt,
		Subject:       s.Subject,
	})
}

// StageRegistry is the per-turn view of the stage table — id → Stage; it
// resolves a flow's stage-id references against the table so the same stage is
// freely combined into many flows.
type StageRegistry struct {
	order  []string
	stages map[string]*Stage
}

// NewStageRegistry builds a registry from a slice of stages (registration order
// preserved).
func NewStageRegistry(stages ...*Stage) *StageRegistry {
	r := &StageRegistry{stages: map[string]*Stage{}}
	for _, s := range stages {
		r.Register(s)
	}
	return r
}

// Register adds a stage, replacing any prior stage with the same id (order kept
// at first insertion, matching Python dict semantics).
func (r *StageRegistry) Register(s *Stage) {
	if s.IDField == "" {
		panic("cannot register a Stage with an empty id")
	}
	if _, exists := r.stages[s.IDField]; !exists {
		r.order = append(r.order, s.IDField)
	}
	r.stages[s.IDField] = s
}

// Get returns the stage with the given id, or nil.
func (r *StageRegistry) Get(stageID string) *Stage { return r.stages[stageID] }

// Stages returns the registered stages in insertion order.
func (r *StageRegistry) Stages() []*Stage {
	out := make([]*Stage, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.stages[id])
	}
	return out
}

// IDs returns the registered stage ids in insertion order.
func (r *StageRegistry) IDs() []string {
	return append([]string(nil), r.order...)
}

// Resolve expands a flow's stage-id references against this table; unknown ids
// are skipped.
func (r *StageRegistry) Resolve(stageIDs []string) []*Stage {
	out := make([]*Stage, 0, len(stageIDs))
	for _, sid := range stageIDs {
		if s, ok := r.stages[sid]; ok {
			out = append(out, s)
		}
	}
	return out
}
