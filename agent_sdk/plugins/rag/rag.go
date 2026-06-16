// Package rag — finalize-grounding hook + RagPlugin. See citation.go
// for the citation extraction / backfill / strip helpers.
package rag

import (
	"github.com/nccasia/agent-sdk-go/agent_sdk/contracts"
	"github.com/nccasia/agent-sdk-go/agent_sdk/core/spec"
)

// FinalizeGrounding is the canonical finalize hook a RAG/grounding
// plugin installs. It runs in `_finalize` BEFORE the AgentResult is
// built: it may rewrite the answer, augment/replace the citations, and
// force a refusal (return a non-empty refusal_reason).
//
// Pure function — no LLM, no engine state. The agent passes the
// per-turn inputs (answer, citations, chunks, grounds flag, the
// require_citations policy) and the hook returns the (possibly-
// rewritten) tuple. Returning ("", existing, "") leaves the turn
// unchanged.
func FinalizeGrounding(answer string, citations []contracts.Citation, chunks []map[string]any, grounds, requireCitations bool) (string, []contracts.Citation, string) {
	// Step 1: extract [chunk_id] markers from the answer.
	extracted := CitationsFromText(answer, chunks)
	if len(extracted) > 0 {
		citations = append(citations, extracted...)
	}
	// Step 2: backfill (paraphrased) answers.
	backfilled := BackfillCitations(answer, chunks, citations)
	if len(backfilled) > 0 {
		citations = append(citations, backfilled...)
	}
	// Step 3: strip markers from the user-facing text.
	stripped := StripCitationMarkers(answer)
	// Step 4: ground-or-refuse.
	if grounds && requireCitations && len(citations) == 0 {
		return stripped, citations, "no_citations"
	}
	return stripped, citations, ""
}

// ToolResultHook extracts citations a tool emits in its output, e.g.
// a KB tool returning `{"citations": [...]}`.
func ToolResultHook(toolName, output string) []contracts.Citation {
	if output == "" {
		return nil
	}
	// Cheap heuristic: look for a "citations" array in the output.
	idx := indexOf(output, "citations")
	if idx < 0 {
		return nil
	}
	// Parse the first chunk_id occurrences (no JSON import needed for
	// the conservative scanner). The Python port uses json.loads; we
	// mirror the spirit without dragging the json dep into every tool
	// call.
	out := []contracts.Citation{}
	rest := output[idx:]
	for _, m := range extractMarkerPattern.FindAllStringSubmatch(rest, -1) {
		out = append(out, contracts.Citation{ChunkID: m[1]})
	}
	return out
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// CiteLobeSpec compiles CiteLOBE to its internal spec.Lobe form.
func CiteLobeSpec() spec.Lobe { return CiteLOBE.Spec() }
