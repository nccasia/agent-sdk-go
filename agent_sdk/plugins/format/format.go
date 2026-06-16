// Package format hosts the FormatPlugin — the default-on but toggleable
// channel/language/tone styling lobe (the `format` B5 expression lobe).
// Disable it (via a PluginRegistry) to drop channel/language/tone
// shaping; the core reply flow and grounding stay intact.
package format

import (
	"context"
	"strings"

	"github.com/mezon/agent-sdk-go/agent_sdk/clients"
	"github.com/mezon/agent-sdk-go/agent_sdk/contracts"
	"github.com/mezon/agent-sdk-go/agent_sdk/core/spec"
	"github.com/mezon/agent-sdk-go/agent_sdk/lobes"
)

// FormatLOBE is the canonical `format` style lobe.
var FormatLOBE = lobes.Lobe{
	ID:           "format",
	Name:         "format",
	Description:  "Restyles the final answer for fixed-format surfaces (channel, language, tone).",
	UseWhen:      "The policy declares a fixed response_format (e.g. Mezon chat).",
	How:          "Rewrites the answer in the target language/tone/format without changing facts or citations.",
	Layer:        5,
	Behavior:     "rewrite",
	Order:        2,
	BuildContext: true,
	Threshold:    0.5,
	Activation:   formatActivation,
}

// formatActivation gates the format lobe on a fixed response_format
// (matches the Python `_should_format_response` policy check). Returns
// 0 when the policy declares nothing.
func formatActivation(ctx map[string]any) float64 {
	policy, _ := ctx["policy"].(map[string]any)
	if policy == nil {
		return 0
	}
	if fmt, _ := policy["response_format"].(string); fmt != "" && fmt != "markdown" {
		return 1.0
	}
	// Bias from the task_execute path.
	if v, ok := ctx["path_bias_format"].(float64); ok && v > 0 {
		return v
	}
	return 0
}

// FormatLobeSpec compiles FormatLOBE to its internal spec.Lobe form.
func FormatLobeSpec() spec.Lobe { return FormatLOBE.Spec() }

// languageNames is the display-name table for the language requirement
// (matches the Python LANGUAGE_NAMES dict).
var languageNames = map[string]string{
	"en": "English",
	"vi": "Vietnamese (tiếng Việt)",
	"ja": "Japanese (日本語)",
	"zh": "Chinese (中文)",
	"ko": "Korean (한국어)",
	"fr": "French (français)",
	"de": "German (Deutsch)",
	"es": "Spanish (español)",
}

// MezonRequirement is the chat-markdown constraint appended when the
// deployment is Mezon.
const MezonRequirement = "MEZON FORMAT: The final answer will be sent to Mezon chat, which supports only limited markdown.\n" +
	"- Use plain text with light markdown only.\n" +
	"- Use `**bold**` for headings or important labels instead of `#` heading lines.\n" +
	"- Do NOT use markdown tables or pipe columns. Convert every table/comparison into short `- ` bullet items.\n" +
	"- Do NOT use horizontal-rule dividers such as `---`, `***`, or `___`.\n" +
	"- Preserve citations exactly as [chunk_id](source_ref)."

// BuildRequirements assembles the language / tone / voice / format /
// Mezon requirement lines from a policy + optional deployment. Pure
// function: no LLM call.
func BuildRequirements(policy map[string]any, deploymentID string) string {
	if policy == nil {
		policy = map[string]any{}
	}
	reqs := []string{}
	lang, _ := policy["language"].(string)
	if lang == "" {
		lang = "en"
	}
	if lang != "en" {
		full := languageNames[lang]
		if full == "" {
			full = lang
		}
		reqs = append(reqs, "LANGUAGE: Write the ENTIRE response in "+full+". "+
			"Every word, heading, bullet point, and explanation must be in "+full+". "+
			"Only keep proper nouns, code snippets, and file paths in English.")
	}
	if tone, _ := policy["tone"].(string); tone != "" && tone != "professional" {
		reqs = append(reqs, "TONE: Use a "+tone+" tone throughout.")
	}
	if voice, _ := policy["voice"].(string); voice != "" {
		reqs = append(reqs, "VOICE: "+voice)
	}
	if format, _ := policy["response_format"].(string); format != "" && format != "markdown" {
		reqs = append(reqs, "RESPONSE FORMAT: "+format+".")
	}
	if deploymentID == "mezon" {
		reqs = append(reqs, MezonRequirement)
	}
	return strings.Join(reqs, "\n")
}

// FormatSystemPrompt is the legacy-exact system prompt the format LLM
// is called with.
const FormatSystemPrompt = "You are a response formatter. Rewrite the provided answer according to the requirements below.\n" +
	"Preserve all factual content, citations [chunk_id](source_ref), and logical structure. Do NOT add, remove, or change any facts.\n" +
	"Only change the language, tone, and formatting as specified.\n\n" +
	"{requirements}\n\n" +
	"Rewrite the answer below to meet ALL requirements. Output ONLY the rewritten answer, nothing else."

// FormatUserTemplate is the legacy-exact user template.
const FormatUserTemplate = "Answer to rewrite:\n\n{answer}"

// RunFormat is the format rewrite pass. An empty output ships the
// original answer; a raised call is handled by the orchestration
// (degrade to the unformatted answer, never lose the turn).
func RunFormat(ctx context.Context, llm contracts.LlmCall, answer, requirements string, maxTokens int) string {
	if maxTokens == 0 {
		maxTokens = 1024
	}
	sys := strings.Replace(FormatSystemPrompt, "{requirements}", requirements, 1)
	usr := strings.Replace(FormatUserTemplate, "{answer}", answer, 1)
	resp, err := llm.Call(ctx, contracts.LlmRequest{
		Stage:      "filter", // legacy repurposing
		System:     sys,
		Messages:   []map[string]any{{"role": "user", "content": usr}},
		MaxTokens:  maxTokens,
		CountUsage: false,
	})
	if err != nil {
		return "" // caller falls back to the original answer
	}
	text := extractText(resp)
	if text == "" {
		return ""
	}
	return text
}

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
