// Package rag hosts the RagPlugin — the OPT-IN retrieval-grounding
// plugin that owns the `cite` lobe + the citation contract
// (extraction / backfill / marker-strip / ground-or-refuse). With
// RagPlugin absent the engine carries zero citation logic; output
// safety is still enforced by SafetyPlugin.
package rag

import (
	"regexp"
	"strings"

	"github.com/mezon/agent-sdk-go/agent_sdk/contracts"
	"github.com/mezon/agent-sdk-go/agent_sdk/lobes"
)

// CiteLOBE is the canonical `cite` grounding lobe.
var CiteLOBE = lobes.Lobe{
	ID:           "cite",
	Name:         "cite",
	Description:  "Citation extraction + ground-or-refuse output contract.",
	UseWhen:      "The user query was grounded in the knowledge base.",
	How:          "Extracts [chunk_id] markers from the answer; backfills when paraphrased; refuses when no citations exist.",
	Layer:        5,
	Behavior:     "rewrite",
	Pinned:       true,
	Order:        1,
	BuildContext: true,
	Threshold:    0.0, // Pinned — always on while the plugin is present.
	Activation:   citeActivation,
}

func citeActivation(ctx map[string]any) float64 {
	// Cite is path-grounds-gated: live on qna/research, dark elsewhere.
	if v, ok := ctx["active_path"].(string); ok {
		switch v {
		case "qna", "research":
			return 1.0
		}
	}
	if fired, ok := ctx["fired_prompt"].(bool); ok && fired {
		return 1.0
	}
	return 0
}

// extractMarkerPattern matches inline citation markers for EXTRACTION
// — `[<chunk_id>]`, `[id1, id2]`, `[golden:<case>]`. Permissive
// (matches the Python _CITE_MARKER_RE behavior).
var extractMarkerPattern = regexp.MustCompile(`\[([a-zA-Z0-9_:-]{1,})\]`)

// stripMarkerPattern is the conservative strip regex: only hex-ish
// IDs ≥ 6 chars. Leaves short tokens like [c1] or [1] alone.
var stripMarkerPattern = regexp.MustCompile(`\[([a-zA-Z0-9_-]{6,})\]`)

// CitationsFromText extracts the citations an answer already names
// inline via [chunk_id] markers, resolving each to its source chunk
// (when present) so source_ref / score are carried.
func CitationsFromText(answer string, chunks []map[string]any) []contracts.Citation {
	if answer == "" {
		return nil
	}
	byID := map[string]map[string]any{}
	for _, c := range chunks {
		id, _ := c["chunk_id"].(string)
		if id != "" {
			byID[id] = c
		}
	}
	seen := map[string]struct{}{}
	out := []contracts.Citation{}
	for _, m := range extractMarkerPattern.FindAllStringSubmatch(answer, -1) {
		id := m[1]
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		c := byID[id]
		src, _ := c["source_ref"].(string)
		out = append(out, contracts.Citation{ChunkID: id, SourceRef: src})
	}
	return out
}

// StripCitationMarkers removes the [chunk_id] markers from an answer
// (the user-facing text shouldn't carry the raw IDs). Conservative:
// only strips hex-ish IDs ≥ 6 chars.
func StripCitationMarkers(answer string) string {
	if answer == "" {
		return answer
	}
	return stripMarkerPattern.ReplaceAllString(answer, "")
}

// BackfillCitations adds citations a paraphrased grounded answer
// omits (no [chunk_id] marker) by token overlap with the candidate
// chunks. Refusals / chitchat / already-cited chunks get NO backfill.
// Capped at 6 citations.
func BackfillCitations(answer string, chunks []map[string]any, existing []contracts.Citation) []contracts.Citation {
	if answer == "" || len(chunks) == 0 {
		return nil
	}
	// Refusal sentinel phrases — no backfill on refusals.
	low := strings.ToLower(answer)
	if strings.HasPrefix(strings.TrimSpace(low), "rất tiếc") ||
		strings.HasPrefix(strings.TrimSpace(low), "i'm sorry") ||
		strings.HasPrefix(strings.TrimSpace(low), "i apologize") ||
		strings.Contains(low, "chưa tìm thấy") {
		return nil
	}
	haveID := map[string]struct{}{}
	for _, c := range existing {
		haveID[c.ChunkID] = struct{}{}
	}
	// Tokenize the answer (lowercase, word chars only, ≥ 3 chars).
	ansTokens := tokenize(answer)
	if len(ansTokens) == 0 {
		return nil
	}
	ranked := []scored{}
	for _, c := range chunks {
		id, _ := c["chunk_id"].(string)
		if id == "" {
			continue
		}
		if _, ok := haveID[id]; ok {
			continue // already cited
		}
		text, _ := c["text"].(string)
		score := overlap(ansTokens, tokenize(text))
		// A reasonable overlap threshold: short chitchat / refusal
		// text has no real overlap; a paraphrased grounded answer
		// shares a meaningful number of content words.
		if score >= 4 {
			ranked = append(ranked, scored{c, score})
		}
	}
	if len(ranked) == 0 {
		return nil
	}
	// Sort: highest score, then highest source score, then chunk_id.
	// Stable enough for test determinism.
	for i := 0; i < len(ranked); i++ {
		for j := i + 1; j < len(ranked); j++ {
			if lessRank(ranked[j], ranked[i]) {
				ranked[i], ranked[j] = ranked[j], ranked[i]
			}
		}
	}
	if len(ranked) > 6 {
		ranked = ranked[:6]
	}
	out := []contracts.Citation{}
	for _, r := range ranked {
		id, _ := r.c["chunk_id"].(string)
		src, _ := r.c["source_ref"].(string)
		out = append(out, contracts.Citation{ChunkID: id, SourceRef: src})
	}
	return out
}

func lessRank(a, b scored) bool {
	if a.score != b.score {
		return a.score < b.score
	}
	as, _ := a.c["score"].(float64)
	bs, _ := b.c["score"].(float64)
	if as != bs {
		return as < bs
	}
	aid, _ := a.c["chunk_id"].(string)
	bid, _ := b.c["chunk_id"].(string)
	return aid > bid
}

func tokenize(s string) map[string]struct{} {
	out := map[string]struct{}{}
	word := regexp.MustCompile(`[a-zA-ZÀ-ỹ0-9_]{3,}`)
	for _, w := range word.FindAllString(strings.ToLower(s), -1) {
		out[w] = struct{}{}
	}
	return out
}

func overlap(a, b map[string]struct{}) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	n := 0
	small, big := a, b
	if len(b) < len(a) {
		small, big = b, a
	}
	for k := range small {
		if _, ok := big[k]; ok {
			n++
		}
	}
	return n
}

// scored is a (chunk, overlap-count) pair used to rank the backfill
// candidates deterministically.
type scored struct {
	c     map[string]any
	score int
}
