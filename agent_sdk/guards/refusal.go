package guards

import (
	"math"
	"regexp"
	"strings"

	"github.com/nccasia/agent-sdk-go/agent_sdk/contracts"
	"github.com/nccasia/agent-sdk-go/agent_sdk/result"
)

// Vec is a dense embedding vector (float32, as numpy float32 in Python).
type Vec []float32

// RefusalRule is one opaque refusal row: {rule_type, pattern, reason, ...}.
// Mirrors the dicts the Python gate consumes (no host type enters the leaf).
type RefusalRule struct {
	RuleType      string
	Pattern       string
	Reason        string
	QueryExamples []string
	Tags          []string
}

// GoldenItem pairs a canonical question with its approved answer.
// Ported from agent_sdk/memory/golden_head.py (the leaf the gate needs).
type GoldenItem struct {
	CaseID           string
	Query            string
	ExpectedBehavior string
	Criteria         []string
	Tags             []string
}

// GoldenHead is an in-memory cosine index over golden cases. Embeddings are the
// L2-normalized per-row question vectors.
type GoldenHead struct {
	Items            []GoldenItem
	Embeddings       []Vec // each L2-normalized, parallel to Items
	EmbeddingModelID string
}

// Size returns the number of golden items.
func (h *GoldenHead) Size() int {
	if h == nil {
		return 0
	}
	return len(h.Items)
}

// NewGoldenHead builds a head and L2-normalizes each row embedding.
func NewGoldenHead(items []GoldenItem, embeddings []Vec, modelID string) *GoldenHead {
	norm := make([]Vec, len(embeddings))
	for i, v := range embeddings {
		norm[i] = l2norm(v)
	}
	return &GoldenHead{Items: items, Embeddings: norm, EmbeddingModelID: modelID}
}

// ToCacheBundle renders a JSON-safe bundle (rows + precomputed embeddings).
func (h *GoldenHead) ToCacheBundle() map[string]any {
	entries := make([]map[string]any, 0, len(h.Items))
	for i, item := range h.Items {
		var vec []float64
		if i < len(h.Embeddings) {
			vec = make([]float64, len(h.Embeddings[i]))
			for j, x := range h.Embeddings[i] {
				vec[j] = float64(x)
			}
		}
		entries = append(entries, map[string]any{
			"case_id":           item.CaseID,
			"query":             item.Query,
			"expected_behavior": item.ExpectedBehavior,
			"criteria":          item.Criteria,
			"tags":              item.Tags,
			"embedding":         vec,
		})
	}
	dim := 0
	if len(h.Embeddings) > 0 {
		dim = len(h.Embeddings[0])
	}
	return map[string]any{"model_id": h.EmbeddingModelID, "dim": dim, "entries": entries}
}

// GoldenHeadFromCached rebuilds a head from a cached bundle's entries without
// re-embedding. Returns nil when no entry carries a query+embedding.
func GoldenHeadFromCached(entries []map[string]any, modelID string) *GoldenHead {
	var items []GoldenItem
	var vecs []Vec
	for _, e := range entries {
		query := strings.TrimSpace(asString(e["query"]))
		vec := asVec(e["embedding"])
		if query == "" || len(vec) == 0 {
			continue
		}
		items = append(items, GoldenItem{
			CaseID:           asString(e["case_id"]),
			Query:            query,
			ExpectedBehavior: asString(e["expected_behavior"]),
			Criteria:         asStrings(e["criteria"]),
			Tags:             asStrings(e["tags"]),
		})
		vecs = append(vecs, vec)
	}
	if len(items) == 0 {
		return nil
	}
	// Cached entries are already normalized; keep them verbatim.
	return &GoldenHead{Items: items, Embeddings: vecs, EmbeddingModelID: modelID}
}

func l2norm(v Vec) Vec {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	n := math.Sqrt(sum)
	if n == 0 {
		n = 1.0
	}
	out := make(Vec, len(v))
	for i, x := range v {
		out[i] = float32(float64(x) / n)
	}
	return out
}

func dot(a, b Vec) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var s float64
	for i := 0; i < n; i++ {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}

// MatchRefusal returns the first matching refusal rule's reason, else "".
// keyword supports pipe-separated alternatives; topic is a substring; regex is
// case-insensitive.
func MatchRefusal(query string, rules []RefusalRule) string {
	low := strings.ToLower(query)
	for _, rule := range rules {
		if rule.Pattern == "" {
			continue
		}
		reason := rule.Reason
		if reason == "" {
			reason = "This topic is restricted."
		}
		rtype := rule.RuleType
		if rtype == "" {
			rtype = "keyword"
		}
		switch rtype {
		case "keyword":
			for _, kw := range strings.Split(strings.ToLower(rule.Pattern), "|") {
				kw = strings.TrimSpace(kw)
				if kw != "" && strings.Contains(low, kw) {
					return reason
				}
			}
		case "topic":
			if strings.Contains(low, strings.ToLower(rule.Pattern)) {
				return reason
			}
		default: // regex
			re, err := regexp.Compile("(?i)" + rule.Pattern)
			if err != nil {
				continue
			}
			if re.MatchString(query) {
				return reason
			}
		}
	}
	return ""
}

// GoldenHit returns the approved answer (cited golden://<case_id>) when a query
// vector is a near-duplicate of an approved golden question (cosine ≥ threshold),
// else nil.
func GoldenHit(qVec Vec, head *GoldenHead, threshold float64) *result.AgentResult {
	if head == nil || len(head.Embeddings) == 0 || len(head.Items) == 0 {
		return nil
	}
	bestIdx, bestScore := -1, math.Inf(-1)
	for i, emb := range head.Embeddings {
		if s := dot(emb, qVec); s > bestScore {
			bestScore, bestIdx = s, i
		}
	}
	if bestIdx < 0 || bestScore < threshold {
		return nil
	}
	item := head.Items[bestIdx]
	answer := strings.TrimSpace(item.ExpectedBehavior)
	if answer == "" {
		return nil
	}
	return &result.AgentResult{
		Text:   answer,
		Status: "answered",
		Citations: []contracts.Citation{{
			ChunkID:        "golden:" + item.CaseID,
			SourceRef:      "golden://" + item.CaseID,
			SupportingSpan: [2]int{0, len(answer)},
		}},
	}
}

// SemanticRefusal scores a query vector against pre-embedded refusal examples
// and returns the nearest example's reason when its cosine ≥ threshold, plus the
// top cosine, the matched examples (every phrasing of the matched rule), and the
// matched tags. Reason is "" on a miss.
type SemanticRefusal func(qVec Vec) (reason string, topCosine float64, examples, tags []string)

// MakeSemanticRefusal builds a SemanticRefusal from rule_type="semantic" rules.
// embed embeds each query example once. excludeTags drops topic-relevant-but-
// absent rules (default {"not-in-docs"}). Returns nil when there are no usable
// semantic rules. Pass excludeTags=nil for the default.
func MakeSemanticRefusal(rules []RefusalRule, embed func(string) Vec, threshold float64, excludeTags []string) SemanticRefusal {
	if threshold == 0 {
		threshold = 0.72
	}
	if excludeTags == nil {
		excludeTags = []string{"not-in-docs"}
	}
	excluded := map[string]struct{}{}
	for _, t := range excludeTags {
		excluded[t] = struct{}{}
	}

	type row struct {
		example string
		reason  string
		tags    []string
		vec     Vec
	}
	var rows []row
	examplesByReason := map[string][]string{}
	for _, rule := range rules {
		if rule.RuleType != "semantic" {
			continue
		}
		if intersects(rule.Tags, excluded) {
			continue
		}
		reason := rule.Reason
		if reason == "" {
			reason = "This question is outside what I can help with."
		}
		for _, ex := range rule.QueryExamples {
			ex = strings.TrimSpace(ex)
			if ex == "" {
				continue
			}
			rows = append(rows, row{example: ex, reason: reason, tags: rule.Tags})
			examplesByReason[reason] = append(examplesByReason[reason], ex)
		}
	}
	if len(rows) == 0 || embed == nil {
		return nil
	}
	for i := range rows {
		rows[i].vec = l2norm(embed(rows[i].example))
	}

	return func(qVec Vec) (string, float64, []string, []string) {
		qn := l2norm(qVec)
		bestIdx, bestScore := -1, math.Inf(-1)
		for i, r := range rows {
			if s := dot(r.vec, qn); s > bestScore {
				bestScore, bestIdx = s, i
			}
		}
		if bestIdx < 0 || bestScore < threshold {
			score := 0.0
			if bestIdx >= 0 {
				score = bestScore
			}
			return "", score, nil, nil
		}
		r := rows[bestIdx]
		return r.reason, bestScore, examplesByReason[r.reason], r.tags
	}
}

// PreTurnGate is the (query, state) -> *AgentResult seam run before reasoning; a
// non-nil result ends the turn.
type PreTurnGate func(query string, state any) *result.AgentResult

// GateOptions configures MakePreTurnGate. Zero values are inert (no rules, no
// embedder, etc.); GoldenThreshold defaults to 0.86 when 0.
type GateOptions struct {
	RefusalRules       []RefusalRule
	GoldenHead         *GoldenHead
	Embed              func(string) Vec
	GoldenThreshold    float64
	SemanticRefusal    SemanticRefusal
	RefusalEnforcement string // "disabled" makes the gate a no-op
}

// MakePreTurnGate builds the pre-turn gate. Order: keyword/topic/regex refusal →
// embed once → golden BEFORE semantic refusal (an approved answer beats a fuzzy
// guess) → semantic refusal. RefusalEnforcement="disabled" → no-op.
func MakePreTurnGate(opts GateOptions) PreTurnGate {
	threshold := opts.GoldenThreshold
	if threshold == 0 {
		threshold = 0.86
	}
	return func(query string, _ any) *result.AgentResult {
		if opts.RefusalEnforcement == "disabled" {
			return nil
		}
		if reason := MatchRefusal(query, opts.RefusalRules); reason != "" {
			return refusalResult(reason)
		}
		if opts.Embed == nil {
			return nil
		}
		qVec := opts.Embed(query)
		if hit := GoldenHit(qVec, opts.GoldenHead, threshold); hit != nil {
			return hit
		}
		if opts.SemanticRefusal != nil {
			if reason, _, _, _ := opts.SemanticRefusal(qVec); reason != "" {
				return refusalResult(reason)
			}
		}
		return nil
	}
}

func refusalResult(reason string) *result.AgentResult {
	return &result.AgentResult{
		Text:    reason,
		Status:  "refused",
		Refusal: &result.Refusal{Reason: "policy_violation", Message: reason},
	}
}

func intersects(tags []string, set map[string]struct{}) bool {
	for _, t := range tags {
		if _, ok := set[t]; ok {
			return true
		}
	}
	return false
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asStrings(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			out = append(out, asString(e))
		}
		return out
	}
	return nil
}

func asVec(v any) Vec {
	switch x := v.(type) {
	case Vec:
		return x
	case []float32:
		return Vec(x)
	case []float64:
		out := make(Vec, len(x))
		for i, f := range x {
			out[i] = float32(f)
		}
		return out
	case []any:
		out := make(Vec, 0, len(x))
		for _, e := range x {
			switch f := e.(type) {
			case float64:
				out = append(out, float32(f))
			case float32:
				out = append(out, f)
			}
		}
		return out
	}
	return nil
}
