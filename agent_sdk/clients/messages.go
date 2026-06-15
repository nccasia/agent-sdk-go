// Package clients provides concrete LLMCall implementations (anthropic, openai,
// MiniMax) plus a deterministic FakeClient for offline tests and a
// stage-dispatching MixedClient. Ported from agent_sdk/clients/.
package clients

// TextBlock is one text content block in a provider Message.
type TextBlock struct {
	Text string
	Type string // always "text"
}

// NewTextBlock builds a text block with type="text".
func NewTextBlock(text string) TextBlock {
	return TextBlock{Text: text, Type: "text"}
}

// ToolUseBlock is one tool-use content block in a provider Message.
type ToolUseBlock struct {
	ID    string
	Name  string
	Input map[string]any
	Type  string // always "tool_use"
}

// NewToolUseBlock builds a tool-use block with type="tool_use".
func NewToolUseBlock(id, name string, input map[string]any) ToolUseBlock {
	return ToolUseBlock{ID: id, Name: name, Input: input, Type: "tool_use"}
}

// ProviderUsage is the per-call token accounting shape the engine reads.
type ProviderUsage struct {
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
}

// Add returns the sum of two ProviderUsage records.
func (u ProviderUsage) Add(other ProviderUsage) ProviderUsage {
	return ProviderUsage{
		InputTokens:      u.InputTokens + other.InputTokens,
		OutputTokens:     u.OutputTokens + other.OutputTokens,
		CacheReadTokens:  u.CacheReadTokens + other.CacheReadTokens,
		CacheWriteTokens: u.CacheWriteTokens + other.CacheWriteTokens,
	}
}

// Message is the provider-agnostic response shape: a list of content blocks
// (text + tool_use), a stop_reason, and usage.
type Message struct {
	Content    []any
	StopReason string
	Usage      ProviderUsage
}

// NewMessage constructs a Message with a sensible default stop_reason.
func NewMessage(content []any) Message {
	return Message{Content: content, StopReason: "end_turn", Usage: ProviderUsage{}}
}

// Text returns the joined text of all text-type content blocks, separated by
// newlines (matches the Python `Message.text` property).
func (m Message) Text() string {
	out := ""
	first := true
	for _, b := range m.Content {
		tb, ok := b.(TextBlock)
		if !ok {
			continue
		}
		if !first {
			out += "\n"
		}
		out += tb.Text
		first = false
	}
	return out
}

// ToolUses returns the tool_use content blocks (matches Python Message.tool_uses).
func (m Message) ToolUses() []ToolUseBlock {
	out := []ToolUseBlock{}
	for _, b := range m.Content {
		if tu, ok := b.(ToolUseBlock); ok {
			out = append(out, tu)
		}
	}
	return out
}
