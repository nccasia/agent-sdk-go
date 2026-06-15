package clients

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// MiniMaxClient is a MiniMax adapter that talks the Anthropic-compatible
// endpoint, with markup-tool-call recovery. When MiniMax emits a tool call as
// <invoke name="…">…</invoke> markup inside a text block (instead of a real
// tool_use), we recover it into a synthetic tool_use so the engine can route
// the call. Recovery is truncation-tolerant: max_tokens may cut off the
// closing tags, so we match to the end of the text (\Z).
type MiniMaxClient struct {
	AnthropicClient
	markupSeq int // monotonic id seed (unique per conversation)
}

// NewMiniMaxClient builds a MiniMax client with the given model + options.
// All AnthropicClientOption values are honored.
func NewMiniMaxClient(model string, opts ...AnthropicClientOption) *MiniMaxClient {
	if model == "" {
		model = "MiniMax-M2.7"
	}
	c := &MiniMaxClient{
		AnthropicClient: *NewAnthropicClient(model, opts...),
	}
	// Override the provider tag — MiniMax is not "anthropic" even though it
	// shares the AnthropicClient code path.
	c.BaseClient.Provider = "minimax"
	return c
}

// MiniMaxClientFromMap is a helper for callers that build a client from a
// config map.
func MiniMaxClientFromMap(model string, cfg map[string]any) *MiniMaxClient {
	return NewMiniMaxClient(model, WithTimeout(extractTimeout(cfg)), WithAPIKey(extractString(cfg, "api_key")), WithBaseURL(extractString(cfg, "base_url")))
}

func extractString(m map[string]any, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}

func extractTimeout(m map[string]any) float64 {
	if v, ok := m["timeout"].(float64); ok {
		return v
	}
	return 300.0
}

// Postprocess recovers markup-emitted tool calls. Native tool_use responses
// pass through unchanged. When a text block contains <invoke name="…"> markup,
// we parse it into ToolUseBlocks, advance the markup id counter, strip the
// markup, and return a reconstructed Message with stop_reason=tool_use.
func (c *MiniMaxClient) postprocess(resp any) any {
	if stop, _ := getString(resp, "StopReason"); stop == "tool_use" {
		return resp
	}
	text := joinTextBlocks(resp)
	if !strings.Contains(text, "<invoke name=") {
		return resp
	}
	uses := parseMarkupToolCalls(text, c.markupSeq)
	if len(uses) == 0 {
		return resp
	}
	c.markupSeq += len(uses)
	cleaned := markupStripRe.ReplaceAllString(text, "")
	cleaned = strings.TrimSpace(cleaned)
	blocks := []any{}
	if cleaned != "" {
		blocks = append(blocks, NewTextBlock(cleaned))
	}
	for _, u := range uses {
		blocks = append(blocks, u)
	}
	return Message{Content: blocks, StopReason: "tool_use", Usage: usageFromAnthropic(resp)}
}

// Postprocess is the public form.
func (c *MiniMaxClient) Postprocess(resp any) any { return c.postprocess(resp) }

func joinTextBlocks(resp any) string {
	cs, ok := getField(resp, "Content")
	if !ok {
		return ""
	}
	cv := reflectValue(cs)
	if !cv.IsValid() || !reflectSliceKind(cv) {
		return ""
	}
	parts := []string{}
	for i := 0; i < cv.Len(); i++ {
		b := cv.Index(i).Interface()
		if t, ok := getString(b, "Type"); ok && t == "text" {
			if txt, ok := getString(b, "Text"); ok {
				parts = append(parts, txt)
			}
		}
	}
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "\n"
		}
		out += p
	}
	return out
}

// Tolerant of TRUNCATION: when the closing tag is absent (call cut off by
// max_tokens), match to the end of the text. Go's regexp (RE2) has no \Z
// anchor and no lookahead, so we use the closed-form regex first and fall
// back to a hand-rolled scan that respects sibling <invoke / </minimax:
// boundaries.
var (
	invokeClosedRe = regexp.MustCompile(`<invoke\s+name="([^"]+)">([\s\S]*?)</invoke>`)
	paramClosedRe  = regexp.MustCompile(`<parameter\s+name="([^"]+)">([\s\S]*?)</parameter>`)
	markupStripRe  = regexp.MustCompile(`(?s)<minimax:tool_call>.*?(?:</minimax:tool_call>|$)|<invoke\s+name=.*?(?:</invoke>|$)`)
	invokeStartRe  = regexp.MustCompile(`<invoke\s+name="([^"]+)">`)
	paramStartRe   = regexp.MustCompile(`<parameter\s+name="([^"]+)">`)
)

const (
	invokeEndTag       = "</invoke>"
	parameterEndTag    = "</parameter>"
	toolCallEndTag     = "</minimax:tool_call>"
	nextInvokeBoundary = "<invoke "
)

// parseMarkupToolCalls extracts ToolUseBlocks from <invoke name="…"> markup.
// The `start` arg seeds the id counter so ids are unique across the whole
// conversation (duplicate `markup_0` ids across hops make the Anthropic-
// compatible API reject the round-tripped tool_result).
func parseMarkupToolCalls(text string, start int) []ToolUseBlock {
	closed := invokeClosedRe.FindAllStringSubmatchIndex(text, -1)
	open := scanOpenInvokes(text)
	if len(open) == 0 && len(closed) == 0 {
		return nil
	}
	out := make([]ToolUseBlock, 0, len(open)+len(closed))
	seen := map[string]bool{}
	// First emit the closed-form matches (more accurate when both are
	// present).
	idx := 0
	for _, c := range closed {
		if len(c) < 6 {
			continue
		}
		name := text[c[2]:c[3]]
		body := text[c[4]:c[5]]
		if seen[name+body] {
			continue
		}
		seen[name+body] = true
		out = append(out, NewToolUseBlock(fmt.Sprintf("markup_%d", start+idx), name, parseMarkupParams(body)))
		idx++
	}
	for _, o := range open {
		if seen[o.name+o.body] {
			continue
		}
		seen[o.name+o.body] = true
		out = append(out, NewToolUseBlock(fmt.Sprintf("markup_%d", start+idx), o.name, parseMarkupParams(o.body)))
		idx++
	}
	return out
}

type openInvoke struct{ name, body string }

// scanOpenInvokes finds <invoke name="…"> blocks whose </invoke> tag may be
// missing (truncation). The body extends to the next sibling <invoke or to
// the </minimax:tool_call> / end-of-text.
func scanOpenInvokes(text string) []openInvoke {
	locs := invokeStartRe.FindAllStringSubmatchIndex(text, -1)
	out := make([]openInvoke, 0, len(locs))
	for i, loc := range locs {
		if len(loc) < 4 {
			continue
		}
		name := text[loc[2]:loc[3]]
		// Find the body start (after the opening tag) and end:
		bodyStart := loc[1]
		bodyEnd := len(text)
		if closeIdx := strings.Index(text[bodyStart:], invokeEndTag); closeIdx >= 0 {
			bodyEnd = bodyStart + closeIdx
		} else {
			// Sibling-aware truncation: stop at the next <invoke or the
			// </minimax:tool_call> tag.
			for j := i + 1; j < len(locs); j++ {
				bodyEnd = locs[j][0]
				break
			}
			if tcEnd := strings.Index(text[bodyStart:], toolCallEndTag); tcEnd >= 0 && bodyStart+tcEnd < bodyEnd {
				bodyEnd = bodyStart + tcEnd
			}
		}
		// Empty body would mean the </invoke> sits immediately at the start
		// (e.g. "<invoke name=…"></invoke>"). The closed regex handles those;
		// here we'd capture "" and skip to avoid duplicates.
		body := text[bodyStart:bodyEnd]
		out = append(out, openInvoke{name: name, body: body})
	}
	return out
}

func parseMarkupParams(body string) map[string]any {
	closed := paramClosedRe.FindAllStringSubmatchIndex(body, -1)
	open := scanOpenParams(body)
	params := map[string]any{}
	seen := map[string]bool{}
	for _, c := range closed {
		if len(c) < 6 {
			continue
		}
		key := body[c[2]:c[3]]
		raw := body[c[4]:c[5]]
		if seen[key] {
			continue
		}
		seen[key] = true
		var v any
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			v = raw
		}
		params[key] = v
	}
	for _, o := range open {
		if seen[o.name] {
			continue
		}
		seen[o.name] = true
		var v any
		if err := json.Unmarshal([]byte(o.body), &v); err != nil {
			v = o.body
		}
		params[o.name] = v
	}
	return params
}

func scanOpenParams(body string) []openInvoke {
	locs := paramStartRe.FindAllStringSubmatchIndex(body, -1)
	out := make([]openInvoke, 0, len(locs))
	for i, loc := range locs {
		if len(loc) < 4 {
			continue
		}
		name := body[loc[2]:loc[3]]
		bodyStart := loc[1]
		bodyEnd := len(body)
		if closeIdx := strings.Index(body[bodyStart:], parameterEndTag); closeIdx >= 0 {
			bodyEnd = bodyStart + closeIdx
		} else {
			for j := i + 1; j < len(locs); j++ {
				bodyEnd = locs[j][0]
				break
			}
		}
		out = append(out, openInvoke{name: name, body: body[bodyStart:bodyEnd]})
	}
	return out
}
