package attention

import (
	"sort"
	"strings"
)

// sortStable sorts nodes by an ascending (key0, key1) tuple, stably.
func sortStable(nodes []*ContextNode, key func(*ContextNode) (int, int)) {
	sort.SliceStable(nodes, func(i, j int) bool {
		a0, a1 := key(nodes[i])
		b0, b1 := key(nodes[j])
		if a0 != b0 {
			return a0 < b0
		}
		return a1 < b1
	})
}

// RawChunkKinds are node kinds that may enter ONLY research's receptive field
// and never join the shared pool — the compression invariant made structural
// (prd.md §10). Blackboard.WriteBack rejects these kinds outright.
var RawChunkKinds = map[string]struct{}{
	"kb_chunk":  {},
	"raw_chunk": {},
}

// ContextBound is a lobe's receptive field over the blackboard — selected at the
// lobe's own execution time over the current pool via Build (one selection
// mechanism, used per-lobe instead of once per turn).
type ContextBound struct {
	Kinds         []string           // node kinds visible to this lobe (empty = all)
	Scopes        []string           // scope filter (empty = all)
	BudgetTokens  int                // default 1600
	Weights       map[string]float64 // sparse node-weight overlay
	MinActivation float64            // default 0.22
}

// DefaultContextBound returns the v1 default receptive field (all kinds/scopes,
// 1600-token budget, 0.22 floor).
func DefaultContextBound() ContextBound {
	return ContextBound{BudgetTokens: 1600, MinActivation: 0.22}
}

// WriteMeta is the per-lobe write-back trace surface.
type WriteMeta struct {
	BudgetHint  int `json:"budget_hint"`
	TokensIn    int `json:"tokens_in"`
	TokensAfter int `json:"tokens_after"`
	Trimmed     int `json:"trimmed"`
}

// Blackboard is the turn-scoped node pool replacing one-pass enrichment.
//
// It opens with the B0/B1 products and grows as lobes run. Each node carries
// provenance; only nodes written by lobes that activated AND completed are
// eligible selection sources. Raw KB chunks never join the pool (WriteBack
// rejects RawChunkKinds). Optional per-layer budgets trim contributed nodes.
type Blackboard struct {
	nodes        []*ContextNode
	provenance   map[string]string
	layerBudgets map[int]int
	writeMeta    map[string]WriteMeta
}

// NewBlackboard seeds the pool with the turn's opening nodes. layerBudgets is
// optional (nil = no per-layer trimming).
func NewBlackboard(nodes []*ContextNode, layerBudgets map[int]int) *Blackboard {
	b := &Blackboard{
		provenance:   map[string]string{},
		layerBudgets: map[int]int{},
		writeMeta:    map[string]WriteMeta{},
	}
	for k, v := range layerBudgets {
		b.layerBudgets[k] = v
	}
	for _, n := range nodes {
		b.add(n, "turn")
	}
	return b
}

// add appends a node, rejecting raw-chunk kinds (panics — a programming error,
// mirrors the Python ValueError the smoke test catches).
func (b *Blackboard) add(node *ContextNode, producedBy string) {
	if _, raw := RawChunkKinds[node.Kind]; raw {
		panic(&RawChunkError{NodeID: node.ID, Kind: node.Kind})
	}
	b.nodes = append(b.nodes, node)
	b.provenance[node.ID] = producedBy
}

// RawChunkError is raised when a raw KB chunk is written to the shared pool.
type RawChunkError struct {
	NodeID string
	Kind   string
}

func (e *RawChunkError) Error() string {
	return "node " + e.NodeID + ": kind " + e.Kind +
		" never joins the shared pool — raw chunks are confined to research's receptive field"
}

func nodeTokens(node *ContextNode) int {
	if node.Tokens > 0 {
		return node.Tokens
	}
	t := len([]rune(node.Text)) / 4
	if t < 1 {
		return 1
	}
	return t
}

// WriteBackOptions configures a per-layer-budgeted write-back.
type WriteBackOptions struct {
	Completed bool   // false drops nodes entirely (no speculative cascade)
	Layer     *int   // per-layer budget key (nil = no trimming)
	QText     string // query text for lexical-overlap trimming
	Pinned    bool   // pinned lobes skip the trim
}

// DefaultWriteBack is the simple completed write-back (no per-layer trim).
func DefaultWriteBack() WriteBackOptions { return WriteBackOptions{Completed: true} }

// WriteBack converts a completed lobe's output into new context nodes. When
// Completed is false the nodes are dropped. Returns the ids actually written.
func (b *Blackboard) WriteBack(lobeID string, nodes []*ContextNode, opts WriteBackOptions) []string {
	if !opts.Completed {
		b.writeMeta[lobeID] = WriteMeta{}
		return nil
	}
	candidates := nodes
	tokensIn := 0
	for _, n := range candidates {
		tokensIn += nodeTokens(n)
	}
	budgetHint := 0
	tokensAfter := tokensIn
	trimmed := 0

	if opts.Layer != nil && !opts.Pinned && len(b.layerBudgets) > 0 {
		if cap, ok := b.layerBudgets[*opts.Layer]; ok && cap > 0 {
			budgetHint = cap
			if tokensIn > cap {
				query := strings.ToLower(opts.QText)
				qwords := strings.Fields(query)
				score := func(n *ContextNode) (int, int) {
					text := strings.ToLower(n.Text)
					overlap := 0
					for _, tok := range qwords {
						if tok != "" && strings.Contains(text, tok) {
							overlap++
						}
					}
					return -overlap, -nodeTokens(n)
				}
				kept := make([]*ContextNode, len(candidates))
				copy(kept, candidates)
				sortStable(kept, score)
				running := 0
				cutoff := len(kept)
				for i, n := range kept {
					if running+nodeTokens(n) > cap {
						cutoff = i
						break
					}
					running += nodeTokens(n)
				}
				trimmed = len(kept) - cutoff
				candidates = kept[:cutoff]
				tokensAfter = running
			}
		}
	}

	written := []string{}
	for _, node := range candidates {
		b.add(node, lobeID)
		written = append(written, node.ID)
	}
	b.writeMeta[lobeID] = WriteMeta{
		BudgetHint: budgetHint, TokensIn: tokensIn, TokensAfter: tokensAfter, Trimmed: trimmed,
	}
	return written
}

// GetWriteMeta returns the per-lobe write-back trace (zero value if absent).
func (b *Blackboard) GetWriteMeta(lobeID string) WriteMeta { return b.writeMeta[lobeID] }

// LayerBudgets returns a snapshot of the active per-layer caps.
func (b *Blackboard) LayerBudgets() map[int]int {
	out := map[int]int{}
	for k, v := range b.layerBudgets {
		out[k] = v
	}
	return out
}

// Provenance returns the lobe that produced a node id ("" if unknown).
func (b *Blackboard) Provenance(nodeID string) string { return b.provenance[nodeID] }

// Nodes returns a copy of the current pool.
func (b *Blackboard) Nodes() []*ContextNode {
	out := make([]*ContextNode, len(b.nodes))
	copy(out, b.nodes)
	return out
}

// VisibleTo returns the slice of the current pool inside a receptive field.
func (b *Blackboard) VisibleTo(bound ContextBound) []*ContextNode {
	kinds := toSet(bound.Kinds)
	scopes := toSet(bound.Scopes)
	out := []*ContextNode{}
	for _, node := range b.nodes {
		if len(kinds) > 0 {
			if _, ok := kinds[node.Kind]; !ok {
				continue
			}
		}
		if len(scopes) > 0 && node.Scope != "" {
			if _, ok := scopes[node.Scope]; !ok {
				continue
			}
		}
		out = append(out, node)
	}
	return out
}

// SelectFor returns a lobe's bounded slice, selected at its own execution time
// over the current pool — Build reused per-lobe.
func (b *Blackboard) SelectFor(bound ContextBound, qText string, qVec []float64, nodeWeights map[string]float64, embed EmbedBatch) ([]*ContextNode, Trace) {
	visible := b.VisibleTo(bound)
	weights := MergeWeights(nodeWeights)
	for k, v := range bound.Weights {
		weights[k] = v
	}
	budget := bound.BudgetTokens
	if budget == 0 {
		budget = 1600
	}
	return Build(visible, qText, qVec, weights, budget, bound.MinActivation, embed)
}

func toSet(items []string) map[string]struct{} {
	if len(items) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(items))
	for _, i := range items {
		out[i] = struct{}{}
	}
	return out
}
