package benchmarks

import (
	"context"
	"sort"
)

// Tier classifies a bench: Free benches are fully deterministic (no provider)
// and must be READY for the free-gate; Live benches additionally register
// provider-driven tiers when a model is configured.
type Tier int

const (
	// Free is a deterministic bench (the free-gate measures these).
	Free Tier = iota
	// Live is a provider-driven bench (deterministic tiers still run).
	Live
)

// RunFn runs a bench's modes and composes its verdict. model is "" when no
// provider is configured (the live tiers self-skip to UNMEASURED/READY).
type RunFn func(ctx context.Context, model string) (Verdict, error)

// Bench is one registered benchmark.
type Bench struct {
	Name string
	Tier Tier
	Run  RunFn
	// ExpectStatus is the bench's Python source-of-truth verdict for the
	// deterministic (no-provider) run — the cross-language parity target. The
	// free-gate passes iff each free bench's Go status equals ExpectStatus. Most
	// free benches are READY; attentionbench ships NOT_READY in Python (the qna/
	// research grounding scenarios reference a cite lobe that does not activate
	// without RAG), so its parity target is NOT_READY, matching ci-free-gates.sh
	// (which gates the unit suite + statelessbench, not attentionbench).
	ExpectStatus string
}

// Registry is an ordered set of benches.
type Registry struct {
	benches map[string]Bench
}

// NewRegistry builds an empty registry.
func NewRegistry() *Registry { return &Registry{benches: map[string]Bench{}} }

// Register adds a bench (last registration of a name wins).
func (r *Registry) Register(b Bench) { r.benches[b.Name] = b }

// Names returns the registered bench names, sorted.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.benches))
	for n := range r.benches {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Get returns a bench by name.
func (r *Registry) Get(name string) (Bench, bool) {
	b, ok := r.benches[name]
	return b, ok
}

// Free returns the free-tier benches, sorted by name.
func (r *Registry) Free() []Bench {
	out := []Bench{}
	for _, n := range r.Names() {
		if b := r.benches[n]; b.Tier == Free {
			out = append(out, b)
		}
	}
	return out
}

// All returns every registered bench, sorted by name.
func (r *Registry) All() []Bench {
	out := []Bench{}
	for _, n := range r.Names() {
		out = append(out, r.benches[n])
	}
	return out
}

// GateRow is one bench's gate outcome.
type GateRow struct {
	Name   string
	Status string
	Expect string
	OK     bool
}

// FreeGate runs every bench's deterministic floor (model="") and reports, per
// bench, whether its Go verdict matches its Python ExpectStatus. The overall
// gate passes iff every row is OK. Mirrors ci-free-gates.sh's no-provider run:
// each bench reproduces its source-of-truth verdict.
func (r *Registry) FreeGate(ctx context.Context) ([]GateRow, bool, error) {
	rows := []GateRow{}
	allOK := true
	for _, b := range r.All() {
		v, err := b.Run(ctx, "")
		if err != nil {
			return rows, false, err
		}
		ok := v.Status == b.ExpectStatus
		if !ok {
			allOK = false
		}
		rows = append(rows, GateRow{Name: b.Name, Status: v.Status, Expect: b.ExpectStatus, OK: ok})
	}
	return rows, allOK, nil
}

// DefaultRegistry builds the registry of all benches the ladder ships. model is
// passed through to the live tiers ("" ⇒ free tiers only).
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(Bench{Name: "agentbench", Tier: Live, Run: RunAgentBench, ExpectStatus: "UNMEASURED"})
	r.Register(Bench{Name: "attentionbench", Tier: Free, Run: RunAttentionBench, ExpectStatus: "NOT_READY"})
	r.Register(Bench{Name: "corgictionbech", Tier: Free, Run: RunCorgictionBench, ExpectStatus: "READY"})
	r.Register(Bench{Name: "flowbench", Tier: Free, Run: RunFlowBench, ExpectStatus: "READY"})
	r.Register(Bench{Name: "promptbench", Tier: Free, Run: RunPromptBench, ExpectStatus: "READY"})
	r.Register(Bench{Name: "statelessbench", Tier: Free, Run: RunStatelessBench, ExpectStatus: "READY"})
	r.Register(Bench{Name: "toolbench", Tier: Live, Run: RunToolBench, ExpectStatus: "READY"})
	return r
}
