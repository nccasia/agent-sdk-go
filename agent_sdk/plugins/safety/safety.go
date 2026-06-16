// Package safety hosts the SafetyPlugin — the default-on but toggleable
// output-safety filter lobe (the `filter` output-contract lobe). Every
// agent wants it; disable via a PluginRegistry only if an integrator
// owns output safety elsewhere.
package safety

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/mezon/agent-sdk-go/agent_sdk/clients"
	"github.com/mezon/agent-sdk-go/agent_sdk/contracts"
	"github.com/mezon/agent-sdk-go/agent_sdk/core/spec"
	"github.com/mezon/agent-sdk-go/agent_sdk/lobes"
)

// FilterLOBE is the canonical `filter` output-safety lobe. It is pinned
// (PINNED_LOBES includes "filter") and lives in the B5 expression layer.
var FilterLOBE = lobes.Lobe{
	ID:           "filter",
	Name:         "filter",
	Description:  "Output-safety filter: refuse ungrounded / redact PII / strip speculation.",
	UseWhen:      "Final answer is being shaped for the user.",
	How:          "Refuses if no citations, redacts PII, removes unsupported claims.",
	Layer:        5,
	Behavior:     "rewrite",
	Pinned:       true,
	Order:        3,
	BuildContext: true,
	Threshold:    0.0, // Pinned — activation is path-gated, weight-immune.
	// Activation: pinned lobes are always on, but for the recognizer
	// wiring we use a path-grounds gate: the filter only fires on
	// grounded paths (qna, research). For the no-grounds paths it's
	// dark, so the system doesn't pay the rewrite cost.
	Activation: filterActivation,
}

// filterActivation gates the filter lobe on grounded paths (matches the
// Python `path-grounds-gated` semantics). Dark on chitchat paths.
func filterActivation(ctx map[string]any) float64 {
	if v, ok := ctx["active_path"].(string); ok {
		switch v {
		case "qna", "research":
			return 1.0
		}
	}
	// A `fired_prompt` flag is treated as grounds-on.
	if fired, ok := ctx["fired_prompt"].(bool); ok && fired {
		return 1.0
	}
	// No path known — stay dark, the engine's deterministic
	// ground-or-refuse enforces the safety contract anyway.
	return 0.0
}

// FilterLobeSpec compiles FilterLOBE to its internal spec.Lobe form.
func FilterLobeSpec() spec.Lobe { return FilterLOBE.Spec() }

// RunFilter is the LLM filter pass the filter lobe calls: parse a JSON
// envelope and either return the (possibly-redacted) answer with a
// refusal, or pass the original answer through on unparseable output.
// Legacy-exact behavior: max_tokens=1024, temperature=0, pass-through
// on JSON parse failure.
func RunFilter(ctx context.Context, llm contracts.LlmCall, answer string) (string, bool, string) {
	resp, err := llm.Call(ctx, contracts.LlmRequest{
		Stage:  "filter",
		System: FilterSystemPrompt,
		Messages: []map[string]any{
			{"role": "user", "content": strings.Replace(FilterUserTemplate, "{answer}", answer, 1)},
		},
		MaxTokens:   1024,
		Temperature: floatPtr(0.0),
		CountUsage:  false,
	})
	if err != nil {
		return answer, false, ""
	}
	text := extractText(resp)
	var parsed struct {
		Output        string `json:"output"`
		Refuse        bool   `json:"refuse"`
		RefusalReason string `json:"refusal_reason"`
	}
	if jerr := json.Unmarshal([]byte(text), &parsed); jerr != nil {
		// Pass-through on unparseable output (legacy-exact).
		return answer, false, ""
	}
	if parsed.Refuse {
		return text, true, firstNonEmpty(parsed.RefusalReason, "no_citations")
	}
	if parsed.Output != "" {
		return parsed.Output, false, ""
	}
	return answer, false, ""
}

// FilterSystemPrompt is the legacy-exact system prompt the filter LLM is
// called with.
const FilterSystemPrompt = "Apply the following output-filter rules:\n" +
	"1. REFUSE (return refusal_reason=\"no_citations\") if no verified citations are present\n" +
	"2. Redact any PII (emails, phone numbers, SSNs)\n" +
	"3. Remove speculation, opinions, or information not supported by citations\n\n" +
	"Respond with:\n" +
	"- \"output\": the filtered markdown answer\n" +
	"- \"refuse\": true/false\n" +
	"- \"refusal_reason\": reason if refused"

// FilterUserTemplate is the legacy-exact user template the filter LLM
// is called with.
const FilterUserTemplate = "Answer to filter:\n{answer}"

// FilterFlowGatePrompt is the text the filter flow-step emits when it is
// the final flow step (the pipeline's final response — never an analysis
// or a JSON verdict).
const FilterFlowGatePrompt = "Apply the ground-or-refuse gate of a research pipeline.\n" +
	"The system context carries the grounded answer (under \"## Step output — cite\", falling\n" +
	"back to \"## Step output — synthesize\") and the evidence index of chunks actually read.\n\n" +
	"Output the FINAL user-facing message, in the user's language:\n" +
	"- If the answer's claims are supported by the evidence index, output the answer\n" +
	"  UNCHANGED (keep its inline [chunk_id] citations). Redact any PII.\n" +
	"- If the evidence index is missing or supports none of the claims, output a short\n" +
	"  refusal explaining you could not ground the answer in the knowledge base.\n" +
	"Output ONLY the final message — no analysis, no verdict, no JSON."

// extractText pulls the human-visible text out of an LLM response.
func extractText(resp any) string {
	if resp == nil {
		return ""
	}
	if s, ok := resp.(string); ok {
		return s
	}
	if m, ok := resp.(clients.Message); ok {
		var parts []string
		for _, b := range m.Content {
			if tb, ok := b.(clients.TextBlock); ok {
				parts = append(parts, tb.Text)
			}
		}
		return strings.Join(parts, "")
	}
	return ""
}

func floatPtr(f float64) *float64 { return &f }
func boolPtr(b bool) *bool        { return &b }
func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
