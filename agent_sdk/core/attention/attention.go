// Package attention is the adaptive context builder — the attention layer over
// informational context. Ported from agent_sdk/network/context_builder.py.
//
// Context engineering as a layered node graph: every informational prompt
// fragment is a ContextNode with an ACTIVATION computed in layers:
//
//	L1  lexical/structural — token overlap with the turn query, plus
//	    admin/scope/recency boosts. Free and deterministic.
//	L2  semantic — cosine similarity between the node embedding and the turn
//	    query vector (skipped when no qVec is supplied).
//	L3  budgeted selection — pinned always; below-threshold dropped; greedy
//	    skip-not-stop fill under the budget.
//
// Pure functions throughout — deterministic given (nodes, qVec, weights,
// budget), no clock, no I/O (embeddings are injected).
package attention

import (
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/mezon/agent-sdk-go/agent_sdk/core/signal"
)

// round4 emits a trace float via banker's rounding to 4 places.
func round4(x float64) float64 { return signal.Round4(x) }

// maxNodes is the hard cap on nodes assembled per turn (embedding cost bound).
const maxNodes = 64

// DefaultNodeWeights is the node-by-node optimization surface, sparsely
// overridable per bot. Mirrors DEFAULT_NODE_WEIGHTS.
var DefaultNodeWeights = map[string]float64{
	"w_l1":     1.0,
	"w_l2":     1.2,
	"l2_floor": 0.55,

	"admin_boost":        0.40,
	"scope_conversation": 0.15,
	"scope_channel":      0.10,
	"scope_user":         0.05,
	"scope_bot":          0.0,
	"recency_boost":      0.10,

	"min_activation": 0.22,

	"topk_memory":       7,
	"topk_session_fact": 4,
	"topk_task_state":   4,

	"w_spread":          0.85,
	"spread_top_n":      3,
	"spread_source_min": 0.1,

	"self_ref_user_boost": 0.25,

	"prior_identity":        0.50,
	"prior_hints":           0.50,
	"prior_ctxvar":          0.35,
	"prior_channel_view":    0.25,
	"prior_task_state":      0.08,
	"prior_session_summary": 0.25,
	"prior_session_fact":    0.18,
	"prior_memory":          0.15,

	"utility_identity":        1.2,
	"utility_hints":           1.2,
	"utility_ctxvar":          1.3,
	"utility_channel_view":    1.0,
	"utility_task_state":      1.0,
	"utility_session_summary": 0.8,
	"utility_session_fact":    0.9,
	"utility_memory":          0.9,
	"utility_default":         1.0,
	"utility_admin_boost":     0.3,

	"cds_cost_unit":         40.0,
	"tier_inject_threshold": 0.30,
	"tier_hint_threshold":   0.12,
}

// EmbedBatch encodes a batch of texts to vectors (injected; the engine wraps
// the shared embedding model). nil disables L2.
type EmbedBatch func(texts []string) [][]float64

// ContextNode is one informational prompt fragment that competes for the budget.
type ContextNode struct {
	ID          string // stable: "{kind}:{ref}"
	Kind        string // identity|hints|session_summary|session_fact|channel_view|memory|ctxvar
	Text        string // the exact prompt fragment this node contributes
	Scope       string // memory/ctxvar: bot|user|channel|conversation ("" = none)
	Pinned      bool
	Admin       bool
	Stability   string // static|slow|turn
	EmbedText   string // L2 input (defaults to Text)
	MenuHint    string
	RecencyRank float64 // 0..1, newest=1
	Tokens      int

	// scoring outputs (filled by Build)
	L1         float64
	L2         float64
	Activation float64
	Selected   bool

	// PreAct routing outputs
	Utility float64
	CDS     float64
	Tier    int

	order   int
	overlap float64
}

// NewContextNode constructs a node with the Python field defaults
// (stability="slow").
func NewContextNode(id, kind, text, scope string) *ContextNode {
	return &ContextNode{ID: id, Kind: kind, Text: text, Scope: scope, Stability: "slow"}
}

func (n *ContextNode) embedSource() string {
	if n.EmbedText != "" {
		return n.EmbedText
	}
	return n.Text
}

var tokenRE = regexp.MustCompile(`[\p{L}\p{N}]+`)

// estTokens mirrors agent_sdk/skills/parser.py est_tokens (len//4).
func estTokens(text string) int {
	n := len([]rune(text)) / 4
	if n < 0 {
		return 0
	}
	return n
}

func tokenize(text string) map[string]struct{} {
	out := map[string]struct{}{}
	// NFC normalization is a no-op for the ASCII/precomposed text the SDK
	// handles; diacritics are intentionally KEPT (meaningful in Vietnamese).
	lowered := strings.ToLower(text)
	for _, m := range tokenRE.FindAllString(lowered, -1) {
		out[m] = struct{}{}
	}
	return out
}

func intersectionCount(a, b map[string]struct{}) int {
	// iterate the smaller set
	if len(b) < len(a) {
		a, b = b, a
	}
	n := 0
	for k := range a {
		if _, ok := b[k]; ok {
			n++
		}
	}
	return n
}

func weightOr(w map[string]float64, key string, def float64) float64 {
	if v, ok := w[key]; ok {
		return v
	}
	return def
}

// MergeWeights returns DefaultNodeWeights with a sparse per-bot override applied.
func MergeWeights(overrides map[string]float64) map[string]float64 {
	merged := make(map[string]float64, len(DefaultNodeWeights))
	for k, v := range DefaultNodeWeights {
		merged[k] = v
	}
	for k, v := range overrides {
		merged[k] = v
	}
	return merged
}

// ScoreResult is the generic L1+L2 relevance of a text to the turn query.
type ScoreResult struct {
	L1         float64
	L2         float64
	Activation float64
}

// ScoreText reproduces score_relevance: the same L1+L2 scorer the context
// attention uses, factored for tool/skill selection (no node structure, no
// spreading). L1 = lexical token overlap; L2 = floor-calibrated cosine of the
// supplied embedding vs qVec (skipped when qVec is nil). Deterministic.
func ScoreText(query string, qVec []float64, text string, textVec []float64, weights map[string]float64, prior float64) ScoreResult {
	w := weights
	if w == nil {
		w = DefaultNodeWeights
	}
	qt := tokenize(query)
	l1 := 0.0
	if len(qt) > 0 {
		l1 = float64(intersectionCount(qt, tokenize(text))) / float64(len(qt))
	}
	if l1 > 1.0 {
		l1 = 1.0
	}
	l2 := 0.0
	if qVec != nil && textVec != nil {
		cos := math.Max(0.0, dot(textVec, qVec))
		floor := weightOr(w, "l2_floor", 0.0)
		if floor < 1 {
			l2 = math.Max(0.0, (cos-floor)/(1.0-floor))
		}
	}
	act := w["w_l1"]*l1 + w["w_l2"]*l2 + prior
	return ScoreResult{L1: l1, L2: l2, Activation: act}
}

func dot(a, b []float64) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	s := 0.0
	for i := 0; i < n; i++ {
		s += a[i] * b[i]
	}
	return s
}

// Trace is the JSON-safe selection trace returned alongside the selected nodes.
type Trace struct {
	BudgetTokens  int         `json:"budget_tokens"`
	MinActivation float64     `json:"min_activation"`
	TotalTokens   int         `json:"total_tokens"`
	Nodes         []NodeTrace `json:"nodes"`
}

// NodeTrace is one node's scoring record in the trace.
type NodeTrace struct {
	ID         string  `json:"id"`
	Kind       string  `json:"kind"`
	Scope      string  `json:"scope"`
	Stability  string  `json:"stability"`
	L1         float64 `json:"l1"`
	L2         float64 `json:"l2"`
	Activation float64 `json:"activation"`
	Tokens     int     `json:"tokens"`
	Selected   bool    `json:"selected"`
	Pinned     bool    `json:"pinned"`
}

var selfRefTokens = map[string]struct{}{
	"mình": {}, "tôi": {}, "tớ": {}, "em": {}, "i": {}, "my": {}, "me": {}, "mine": {},
}

var spreadStopTokens = map[string]struct{}{
	"task": {}, "type": {}, "goal": {}, "todos": {}, "status": {}, "last": {},
	"result": {}, "active": {}, "paused": {}, "completed": {}, "cancelled": {},
	"todo": {}, "doing": {}, "blocked": {}, "done": {}, "skipped": {}, "failed": {},
	"freeform": {}, "template": {}, "scheduler": {}, "manual": {}, "cron": {},
	"once": {}, "none": {}, "null": {},
}

func rareTokens(text string) map[string]struct{} {
	out := map[string]struct{}{}
	for t := range tokenize(text) {
		if len([]rune(t)) >= 3 {
			if _, stop := spreadStopTokens[t]; !stop {
				out[t] = struct{}{}
			}
		}
	}
	return out
}

// Build scores (L1+L2) and selects (L3) nodes. Deterministic. Returns the
// selected nodes in original composition order plus a trace. qVec nil skips L2.
// embed encodes node embed-texts (one batch); pass nil for the text-only path.
func Build(nodes []*ContextNode, qText string, qVec []float64, weights map[string]float64, budgetTokens int, minActivation float64, embed EmbedBatch) ([]*ContextNode, Trace) {
	for order, node := range nodes {
		node.order = order
		node.Tokens = estTokens(node.Text)
	}

	// Assembly cap: drop lowest-prior overflow BEFORE embedding.
	if len(nodes) > maxNodes {
		ranked := make([]*ContextNode, len(nodes))
		copy(ranked, nodes)
		sort.SliceStable(ranked, func(i, j int) bool {
			a, b := ranked[i], ranked[j]
			// reverse=True on (pinned, prior_kind, -order)
			if a.Pinned != b.Pinned {
				return a.Pinned // true sorts first under reverse
			}
			pa := weightOr(weights, "prior_"+a.Kind, 0.0)
			pb := weightOr(weights, "prior_"+b.Kind, 0.0)
			if pa != pb {
				return pa > pb
			}
			return a.order > b.order // -order reversed -> larger order first... matches python -n._order reverse
		})
		nodes = ranked[:maxNodes]
		sort.SliceStable(nodes, func(i, j int) bool { return nodes[i].order < nodes[j].order })
	}

	queryTokens := tokenize(qText)
	selfReferential := intersectionCount(queryTokens, selfRefTokens) > 0

	var vectors map[int][]float64
	if qVec != nil && embed != nil {
		vectors = embedNodes(nodes, embed)
	}

	for i, node := range nodes {
		overlap := 0.0
		if len(queryTokens) > 0 {
			nodeTokens := tokenize(node.embedSource())
			overlap = float64(intersectionCount(queryTokens, nodeTokens)) / float64(len(queryTokens))
		}
		node.overlap = overlap
		l1 := overlap
		if node.Admin {
			l1 += weights["admin_boost"]
		}
		if node.Scope != "" {
			l1 += weightOr(weights, "scope_"+node.Scope, 0.0)
		}
		if selfReferential && node.Scope == "user" {
			l1 += weightOr(weights, "self_ref_user_boost", 0.0)
		}
		if node.Kind == "memory" {
			l1 += weights["recency_boost"] * math.Max(0.0, math.Min(1.0, node.RecencyRank))
		}
		node.L1 = math.Min(1.0, l1)

		node.L2 = 0.0
		if qVec != nil && vectors != nil {
			if vec, ok := vectors[i]; ok {
				cos := math.Max(0.0, dot(vec, qVec))
				floor := weightOr(weights, "l2_floor", 0.0)
				if floor < 1 {
					node.L2 = math.Max(0.0, (cos-floor)/(1.0-floor))
				}
			}
		}

		prior := weightOr(weights, "prior_"+node.Kind, 0.0)
		node.Activation = weights["w_l1"]*node.L1 + weights["w_l2"]*node.L2 + prior
	}

	// L2.5 — spreading activation.
	wSpread := weightOr(weights, "w_spread", 0.0)
	if wSpread > 0 {
		topN := int(weightOr(weights, "spread_top_n", 3))
		sourceMin := weightOr(weights, "spread_source_min", 0.1)
		candidates := []*ContextNode{}
		for _, n := range nodes {
			if !n.Pinned && n.overlap >= sourceMin {
				candidates = append(candidates, n)
			}
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			return (candidates[i].overlap + candidates[i].L2) > (candidates[j].overlap + candidates[j].L2)
		})
		if len(candidates) > topN {
			candidates = candidates[:topN]
		}
		type srcTok struct {
			tokens map[string]struct{}
			node   *ContextNode
		}
		sources := make([]srcTok, len(candidates))
		for i, s := range candidates {
			st := rareTokens(s.embedSource())
			for q := range queryTokens {
				delete(st, q)
			}
			sources[i] = srcTok{tokens: st, node: s}
		}
		inSources := map[*ContextNode]struct{}{}
		for _, s := range candidates {
			inSources[s] = struct{}{}
		}
		for _, node := range nodes {
			if node.Pinned {
				continue
			}
			if _, ok := inSources[node]; ok {
				continue
			}
			nodeTokens := rareTokens(node.embedSource())
			if len(nodeTokens) == 0 {
				continue
			}
			best := 0.0
			for _, s := range sources {
				if node.Kind == "task_state" && s.node.Kind == "task_state" {
					continue
				}
				if len(s.tokens) == 0 {
					continue
				}
				denom := len(nodeTokens)
				if denom > 8 {
					denom = 8
				}
				shared := float64(intersectionCount(nodeTokens, s.tokens)) / float64(denom)
				if shared > best {
					best = shared
				}
			}
			if best > 0 {
				node.Activation += wSpread * math.Min(1.0, best)
			}
		}
	}

	// L3 — selection.
	used := 0
	for _, node := range nodes {
		if node.Pinned {
			node.Selected = true
			used += node.Tokens
		}
	}
	scored := []*ContextNode{}
	for _, n := range nodes {
		if !n.Pinned {
			scored = append(scored, n)
		}
	}
	sort.SliceStable(scored, func(i, j int) bool {
		a, b := scored[i], scored[j]
		if a.Activation != b.Activation {
			return a.Activation > b.Activation
		}
		pa := weightOr(weights, "prior_"+a.Kind, 0.0)
		pb := weightOr(weights, "prior_"+b.Kind, 0.0)
		if pa != pb {
			return pa > pb
		}
		return a.ID < b.ID
	})
	kindCounts := map[string]int{}
	for _, node := range scored {
		if node.Activation < minActivation {
			continue
		}
		topk := int(weightOr(weights, "topk_"+node.Kind, 0))
		if topk > 0 && kindCounts[node.Kind] >= topk {
			continue
		}
		if used+node.Tokens <= budgetTokens {
			node.Selected = true
			used += node.Tokens
			kindCounts[node.Kind]++
		}
	}

	selected := []*ContextNode{}
	for _, n := range nodes {
		if n.Selected {
			selected = append(selected, n)
		}
	}
	sort.SliceStable(selected, func(i, j int) bool { return selected[i].order < selected[j].order })

	trace := Trace{
		BudgetTokens:  budgetTokens,
		MinActivation: minActivation,
		TotalTokens:   used,
		Nodes:         make([]NodeTrace, len(nodes)),
	}
	for i, n := range nodes {
		trace.Nodes[i] = NodeTrace{
			ID: n.ID, Kind: n.Kind, Scope: n.Scope, Stability: n.Stability,
			L1: round4(n.L1), L2: round4(n.L2), Activation: round4(n.Activation),
			Tokens: n.Tokens, Selected: n.Selected, Pinned: n.Pinned,
		}
	}
	return selected, trace
}

func embedNodes(nodes []*ContextNode, embed EmbedBatch) map[int][]float64 {
	texts := make([]string, len(nodes))
	for i, n := range nodes {
		texts[i] = n.embedSource()
	}
	vecs := embed(texts)
	out := map[int][]float64{}
	for i := range nodes {
		if i < len(vecs) {
			out[i] = vecs[i]
		}
	}
	return out
}
