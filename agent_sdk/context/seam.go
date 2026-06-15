package context

// The shared-context seam — one binding every tool/skill reaches.
//
// Python uses a contextvar (per async-task). Go has no contextvar primitive;
// this is a package-level binding with restore-on-exit semantics. The intended
// usage is one bound context per synchronous reasoning scope (mirroring the
// engine's current_turn binding), restored via the closure Bind returns.
var current *AgentContext

// Current returns the active AgentContext (or nil) — the seam a tool or skill
// uses to reach shared state without an explicit argument.
func Current() *AgentContext { return current }

// Bind binds ctx as the active shared context, returning a restore func that
// reinstates the previous binding (re-entrant / nesting-safe).
func Bind(ctx *AgentContext) func() {
	prev := current
	current = ctx
	return func() { current = prev }
}
