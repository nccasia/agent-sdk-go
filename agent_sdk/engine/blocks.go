// Provider content-block normalization — the engine's defense against a thinking
// block leaking as a language repr into the answer or corrupting replayed
// history.
//
// Ported from agent_sdk/engine.py (_block_to_dict / _assistant_content /
// _text_of).
package engine

// Block is the minimal provider content-block shape the engine normalizes: a
// type discriminator plus the per-type payload accessors. A concrete provider
// block (e.g. a TextBlock or an Anthropic thinking block) implements it; only
// the fields relevant to its Type need return meaningful values.
type Block interface {
	// Type is the block discriminator: "text", "tool_use", "thinking",
	// "redacted_thinking", or an unknown future type.
	BlockType() string
}

// TextBlock is a plain text content block.
type TextBlock struct{ Text string }

// BlockType identifies a text block.
func (TextBlock) BlockType() string { return "text" }

// ToolUseBlock is a tool-call content block.
type ToolUseBlock struct {
	ID    string
	Name  string
	Input map[string]any
}

// BlockType identifies a tool_use block.
func (ToolUseBlock) BlockType() string { return "tool_use" }

// ThinkingBlock is a provider reasoning block (carrying its reasoning text).
type ThinkingBlock struct {
	Thinking string
	Redacted bool
}

// BlockType identifies a thinking block.
func (b ThinkingBlock) BlockType() string {
	if b.Redacted {
		return "redacted_thinking"
	}
	return "thinking"
}

// Message is the minimal provider message shape: a content block list.
type Message interface{ Content() []Block }

// SimpleMessage is a concrete Message wrapping a content block list.
type SimpleMessage struct{ Blocks []Block }

// Content returns the message's content blocks.
func (m SimpleMessage) Content() []Block { return m.Blocks }

// blockText reads a genuine .Text from a block, if it carries one.
func blockText(b Block) (string, bool) {
	if tb, ok := b.(TextBlock); ok {
		return tb.Text, true
	}
	return "", false
}

// BlockToDict normalizes a provider content block to a plain map.
//
// A thinking block is surfaced as type:"thinking" carrying its reasoning text —
// never stringified to a language repr (which both leaked into the answer and
// corrupted replayed history when a provider parroted the repr back as text). An
// unknown block keeps only a genuine string .Text; never a repr.
func BlockToDict(block Block) map[string]any {
	switch block.BlockType() {
	case "text":
		txt, _ := blockText(block)
		return map[string]any{"type": "text", "text": txt}
	case "tool_use":
		tb, _ := block.(ToolUseBlock)
		input := tb.Input
		if input == nil {
			input = map[string]any{}
		}
		return map[string]any{"type": "tool_use", "id": tb.ID, "name": tb.Name, "input": input}
	case "thinking", "redacted_thinking":
		th := ""
		if tb, ok := block.(ThinkingBlock); ok {
			th = tb.Thinking
		}
		return map[string]any{"type": "thinking", "text": th}
	default:
		// Unknown block: keep only a genuine string .Text; never a repr.
		txt, _ := blockText(block)
		return map[string]any{"type": "text", "text": txt}
	}
}

// AssistantContent is the assistant content for REPLAY into the running history.
// Thinking blocks are dropped — the provider does not need its own prior
// reasoning replayed, and serializing it corrupts the next hop.
func AssistantContent(msg Message) []map[string]any {
	out := []map[string]any{}
	if msg == nil {
		return out
	}
	for _, b := range msg.Content() {
		d := BlockToDict(b)
		if d["type"] == "thinking" {
			continue
		}
		out = append(out, d)
	}
	return out
}

// TextOf is the answer source: the joined text of a message's text blocks only
// (thinking and other blocks ignored), trimmed.
func TextOf(msg Message) string {
	if msg == nil {
		return ""
	}
	var parts []string
	for _, b := range msg.Content() {
		if b.BlockType() == "text" {
			if tb, ok := b.(TextBlock); ok {
				parts = append(parts, tb.Text)
			}
		}
	}
	return trimJoin(parts)
}

func trimJoin(parts []string) string {
	joined := ""
	for i, p := range parts {
		if i > 0 {
			joined += "\n"
		}
		joined += p
	}
	return trimSpace(joined)
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && isSpace(s[start]) {
		start++
	}
	for end > start && isSpace(s[end-1]) {
		end--
	}
	return s[start:end]
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}
