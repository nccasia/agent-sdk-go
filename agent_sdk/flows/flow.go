// Package flows is the progressive-state (OX / time) axis of a PreAct turn.
//
// A Flow is a complete, named pipeline: an ordered sequence of FlowSteps. The
// path picks the flow; the steps run in order, each composing its own system
// prompt from the lobes it consults and running its own loop (none / single /
// agentic / map).
//
// The two axes are independent data models — a new flow never requires touching
// a lobe, and a new lobe never requires touching a flow.
//
// Ported from agent_sdk/flows/flow.go (FlowStep, FlowStepNode, Flow,
// FlowStepResult) and the agent_sdk.flow builder.
package flows

import "fmt"

// runningModel maps a step's loop field to its friendly running-model name
// (RFC 0017): the inspectable "type" of how the step executes.
var runningModel = map[string]string{
	"agentic": "react",
	"single":  "simple",
	"map":     "map",
	"none":    "none",
}

// validLoops is the set of loop modes a FlowStep may declare.
var validLoops = map[string]struct{}{
	"none":    {},
	"single":  {},
	"agentic": {},
	"map":     {},
}

// SignalsFn computes a step's per-turn raw signals from the activation context.
type SignalsFn func(ctx map[string]any) map[string]float64

func noSignals(map[string]any) map[string]float64 { return map[string]float64{} }

// FlowStep is one progressive block of a flow's execution (the flow-axis unit).
//
// A step owns a lobe slice (the cross-axis bridge), a loop mode, a tool
// allowlist, an optional own activation signal, and per-step inference config.
// Its output is consumed by the next step; the final step's output is the
// response.
type FlowStep struct {
	Name        string
	Lobes       []string
	Loop        string // none | single | agentic | map
	Tools       []string
	Description string

	StateNodes []FlowStepNode

	// loop="map" fan-out shape.
	FanoutKey      string
	FanoutParallel bool
	FanoutMax      int
	FanoutIsolated bool

	// First-class signal (the merged FlowStepNode common case). The default
	// (no signals + MinActivation 0) is an always-on structural step.
	Signals       SignalsFn
	SignalWeights map[string]float64
	Prior         float64
	MinActivation float64 // 0 ⇒ always-on

	// Default inference config (None ⇒ engine/policy default). nil pointers
	// mean "fall back to the engine settings default".
	Model        *string
	Temperature  *float64
	MaxTokens    *int
	Hops         *int
	SystemPrompt *string
	// Subject this state instance works on (a sub-question / aspect / target).
	// nil ⇒ the step works on the whole turn.
	Subject *string
}

// NewFlowStep constructs a FlowStep with the dataclass defaults applied and the
// post-init invariants enforced (loop in the allowed set; map requires a
// fanout_key). It mirrors Python's @dataclass __post_init__.
func NewFlowStep(s FlowStep) FlowStep {
	if s.Loop == "" {
		s.Loop = "single"
	}
	if s.FanoutMax == 0 {
		s.FanoutMax = 40
	}
	if s.Signals == nil {
		s.Signals = noSignals
	}
	if s.SignalWeights == nil {
		s.SignalWeights = map[string]float64{}
	}
	if _, ok := validLoops[s.Loop]; !ok {
		panic(fmt.Sprintf("flow step %q: loop %q must be one of none/single/agentic/map", s.Name, s.Loop))
	}
	if s.Loop == "map" && s.FanoutKey == "" {
		panic(fmt.Sprintf("flow step %q: loop='map' requires a fanout_key", s.Name))
	}
	return s
}

// Type is the step's running model (how it executes), the friendly name of
// Loop: react | simple | map | none. Orthogonal to Name (the reasoning state).
func (s FlowStep) Type() string {
	if t, ok := runningModel[s.Loop]; ok {
		return t
	}
	return s.Loop
}

// FlowStepNode is one opt-in node inside a flow step's state machine. Same
// mental model as a LobeNode but execution-shaped.
type FlowStepNode struct {
	ID             string
	Flow           string
	Step           string
	Prior          float64
	Signals        SignalsFn
	SignalWeights  map[string]float64
	MinActivation  float64
	Order          int
	Description    string
	EnabledDefault bool
}

// StepFlow is the low-level flow axis unit: an ordered sequence of FlowSteps
// plus the per-bot customization surface. The façade Flow (intent pipeline of
// stage ids) is the public surface; StepFlow is the runtime form a flow's
// resolved stages compile into.
type StepFlow struct {
	Name        string
	Steps       []FlowStep
	Description string
	// Promotable: when true, this flow can be auto-promoted from a trace-mined
	// emergent shape. Informational.
	Promotable bool
}

// ID returns the flow's name (Activable surface).
func (f StepFlow) ID() string { return f.Name }

// FlowStepResult is the result envelope for a single flow step's execution.
type FlowStepResult struct {
	Flow         string
	Step         string
	Text         string
	ContextNodes []any
	ToolCalls    []any
	TokensIn     int
	TokensOut    int
	LatencyMs    float64
	Metadata     map[string]any
}

// StageName is a deprecated StageResult compatibility alias for Step.
func (r FlowStepResult) StageName() string { return r.Step }

// Path is a deprecated StageResult compatibility alias for Flow.
func (r FlowStepResult) Path() string { return r.Flow }
