// Package activable defines the uniform Activable interface and the Layer enum.
//
// Every PreAct building block — Lobe, Stage, Flow, Skill — is Activable: it
// shares one five-field interface so the framework reads uniformly, activates by
// the same free + deterministic rule, and serializes identically.
//
//	id           stable identifier
//	name         display name
//	description  WHAT it is (one line)
//	use_when     WHEN — natural-language trigger
//	signal(ctx)  the deterministic, free activation in [0, 1] (0 = dark)
//
// signal is never an LLM call. use_when doubles as documentation and the source
// for an optional semantic-activation path — kept a separate, declared term so
// the free core stays reproducible.
//
// Ported from agent_sdk/activable.py.
package activable

// Layer is the reasoning layers, brain-shaped (RFC 0015).
//
// B0/B1 (instinct, perception) are core machinery and hold no lobes; lobes live
// in B2..B5. The integer values match the activation core's LAYER_* constants so
// a Layer is interchangeable with the raw int the activation core consumes.
type Layer int

const (
	LayerInstinct   Layer = 0
	LayerPerception Layer = 1
	LayerMemory     Layer = 2
	LayerSkill      Layer = 3
	LayerCognition  Layer = 4
	LayerExpression Layer = 5
)

// Context is the deterministic activation substrate (a plain map).
type Context = map[string]any

// Activable is the shared interface for Lobes, Stages, Flows, and Skills.
type Activable interface {
	ID() string
	Name() string
	Description() string
	UseWhen() string
	// Signal is the deterministic, free activation in [0, 1]. 0 = dark.
	Signal(ctx Context) float64
}
