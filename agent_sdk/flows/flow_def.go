// Façade Flow — an intent pipeline: a list of Stage ids + its own recognition
// signal. A Flow is Activable: its signal / use_when recognize the turn's intent
// (replacing the separate path recognizer; the highest-scoring flow over
// threshold wins, else emergent). Its stages are resolved against the
// StageRegistry at run time.
//
// Ported from agent_sdk/flow_def.py (Flow, flow).
package flows

import "github.com/nccasia/agent-sdk-go/agent_sdk/core/signal"

// SignalFn is a deterministic, free recognition score in [0, 1].
type SignalFn func(ctx map[string]any) float64

// Flow is an intent pipeline, Activable. Use NewFlow or construct directly.
type Flow struct {
	IDField     string
	NameField   string
	Description string
	UseWhen     string
	Stages      []string
	Threshold   float64
	Grounds     bool // produces a grounded (citable) reply

	// SignalExpr retains the declarative form (when given) so a spec can
	// round-trip recognition faithfully. nil when a callable or no signal.
	SignalExpr any
	signalFn   SignalFn
}

// FlowOption configures a Flow built via NewFlow.
type FlowOption func(*Flow)

// FlowName sets the display name (defaults to the id).
func FlowName(name string) FlowOption { return func(f *Flow) { f.NameField = name } }

// FlowDescription sets the one-line description.
func FlowDescription(d string) FlowOption { return func(f *Flow) { f.Description = d } }

// FlowUseWhen sets the natural-language trigger.
func FlowUseWhen(u string) FlowOption { return func(f *Flow) { f.UseWhen = u } }

// FlowStages sets the ordered list of stage ids.
func FlowStages(stages ...string) FlowOption {
	return func(f *Flow) { f.Stages = append([]string(nil), stages...) }
}

// FlowThreshold sets the recognition threshold (default 0.5).
func FlowThreshold(t float64) FlowOption { return func(f *Flow) { f.Threshold = t } }

// FlowGrounds sets whether the flow produces a grounded (citable) reply.
func FlowGrounds(g bool) FlowOption { return func(f *Flow) { f.Grounds = g } }

// FlowSignalFn sets a callable recognition signal.
func FlowSignalFn(fn SignalFn) FlowOption { return func(f *Flow) { f.signalFn = fn } }

// FlowSignalExpr sets a declarative recognition signal (dict / float), compiled
// via the signal package. The expression is retained for round-tripping.
func FlowSignalExpr(expr any) FlowOption {
	return func(f *Flow) {
		f.SignalExpr = expr
		fn, err := signal.Compile(expr)
		if err != nil {
			panic(err)
		}
		f.signalFn = func(ctx map[string]any) float64 { return fn(ctx) }
	}
}

// NewFlow is the concise builder for a flow. defaults: threshold 0.5, grounds
// true, name defaults to id, signal default score 0.0.
func NewFlow(id string, opts ...FlowOption) Flow {
	f := Flow{IDField: id, Threshold: 0.5, Grounds: true}
	for _, opt := range opts {
		opt(&f)
	}
	if f.NameField == "" {
		f.NameField = f.IDField
	}
	return f
}

// ID returns the flow id (Activable surface).
func (f Flow) ID() string { return f.IDField }

// Name returns the display name (Activable surface).
func (f Flow) Name() string { return f.NameField }

// Signal is the recognition score in [0, 1] (deterministic, free). Default 0.0.
func (f Flow) Signal(ctx map[string]any) float64 {
	if f.signalFn != nil {
		return f.signalFn(ctx)
	}
	return 0.0
}
