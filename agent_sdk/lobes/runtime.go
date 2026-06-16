package lobes

import "github.com/nccasia/agent-sdk-go/agent_sdk/core/spec"

// Lobe is the skill-style authoring API for a lobe — one self-describing value
// instead of a spec.Lobe + a separate signals function + weights + a magic
// threshold. Declare the metadata and ONE programmatic activation; Spec()
// compiles to the internal spec.Lobe the network consumes (byte-identical at
// default weights, so parity holds). Ported from agent_sdk/lobes/runtime.py:Lobe.
//
// Mirrors a skill: ID/Name/Description + UseWhen (the natural-language trigger).
// The deterministic free signal is Activation; cross-lobe wiring is Excites.
// Lobes that need multi-signal activation set Signals directly.
type Lobe struct {
	ID          string
	Name        string
	Description string // WHAT it does (one line)
	UseWhen     string // WHEN — NL trigger
	How         string // HOW it works when active

	SystemPrompt string
	UserTemplate string

	Layer     int
	Behavior  string // recall|decompose|compose|select|rewrite|…
	Writes    []string
	Excites   map[string]float64 // outgoing edges: lobe_id -> weight
	Pinned    bool
	Prior     float64
	Threshold float64 // min_activation; default 0.5
	Order     int

	BuildContext  bool
	SignalWeights map[string]float64
	AttendsKinds  []string

	// Activation is the ONE deterministic, free activation signal (0 = dark).
	// Reads ctx flags / lexical cues / session state — never an LLM call. nil
	// defaults to 0.0 (signal-gated dark) unless Pinned. The default signal is
	// emitted under a single name equal to ID.
	Activation func(ctx map[string]any) float64

	// Signals overrides the single-signal default for multi-signal lobes. When
	// set, it is used verbatim and Activation is ignored.
	Signals spec.SignalFn
}

// ID accessor mirrors BaseLobe.id.
func (l Lobe) GetID() string { return l.ID }

// Spec compiles the authoring lobe to the internal spec.Lobe. The network
// defaults an absent signal weight to 1.0, so an empty SignalWeights matches a
// hand-written spec.Lobe that omits them.
func (l Lobe) Spec() spec.Lobe {
	threshold := l.Threshold
	signals := l.Signals
	if signals == nil {
		act := l.Activation
		id := l.ID
		signals = func(ctx map[string]any) map[string]float64 {
			v := 0.0
			if act != nil {
				v = act(ctx)
			}
			return map[string]float64{id: v}
		}
	}
	weights := map[string]float64{}
	for k, v := range l.SignalWeights {
		weights[k] = v
	}
	edges := map[string]float64{}
	for k, v := range l.Excites {
		edges[k] = v
	}
	var attends any
	if len(l.AttendsKinds) > 0 {
		attends = ContextBound{Kinds: append([]string(nil), l.AttendsKinds...), BudgetTokens: 1600, MinActivation: 0.22}
	}
	return spec.Lobe{
		ID:            l.ID,
		Behavior:      l.Behavior,
		Layer:         l.Layer,
		Prior:         l.Prior,
		Pinned:        l.Pinned,
		Attends:       attends,
		Signals:       signals,
		SignalWeights: weights,
		Edges:         edges,
		Writes:        append([]string(nil), l.Writes...),
		MinActivation: threshold,
		Order:         l.Order,
		BuildContext:  l.BuildContext,
	}
}

// ContextBound is the receptive-field descriptor a row/lobe attends over
// (kinds/scopes/budget). Held opaquely by spec.Lobe.Attends. Ported from
// agent_sdk/network/activation.py:ContextBound.
type ContextBound struct {
	Kinds         []string
	Scopes        []string
	BudgetTokens  int
	MinActivation float64
}
