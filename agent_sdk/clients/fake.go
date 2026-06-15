package clients

import (
	"context"
	"fmt"
)

// FakeHandler is the dynamic-handler signature used when a script entry is
// a callable. Mirrors the Python `def handler(stage, system, messages, tools)` shape.
type FakeHandler func(stage, system string, messages []map[string]any, tools []map[string]any) any

// FakeHandlerFunc adapts a FakeHandler to the script-item interface.
func FakeHandlerFunc(h FakeHandler) ScriptItemFn {
	return func(stage, system string, messages []map[string]any, tools []map[string]any) any {
		return h(stage, system, messages, tools)
	}
}

// ScriptItemFn is the dynamic-handler shape that FakeClient invokes.
type ScriptItemFn func(stage, system string, messages []map[string]any, tools []map[string]any) any

// RecordedCall captures one call's arguments (for assertions on which stages
// were driven, what system was passed, etc.).
type RecordedCall struct {
	Stage       string
	System      any
	Messages    []map[string]any
	Tools       []map[string]any
	MaxTokens   int
	Temperature *float64
}

// FakeClient is a deterministic, scriptable LlmCall for tests + dev. Drive
// the engine end-to-end without a network.
//
// Script items may be:
//   - string         → a final text answer (stop_reason="end_turn")
//   - map            → {"text": …, "tools": [{"name": …, "input": …}, …]}
//     (stop_reason="tool_use" if tools present)
//   - Message        → returned unchanged
//   - ScriptItemFn   → dynamic handler
//
// When the script is exhausted, Default is returned as a plain text answer.
type FakeClient struct {
	BaseClient
	Script  []any
	idx     int
	toolSeq int
	Default string
	Calls   []RecordedCall
}

// NewFakeClient builds a FakeClient with the given script and an optional
// default answer (used when the script is exhausted). `defaultAnswer` may be
// nil for the default of "OK.".
func NewFakeClient(script []any, defaultAnswer *string) *FakeClient {
	def := "OK."
	if defaultAnswer != nil {
		def = *defaultAnswer
	}
	return &FakeClient{
		BaseClient: BaseClient{ModelName: "fake-model", Provider: "fake"},
		Script:     append([]any{}, script...),
		Default:    def,
	}
}

// Scripted returns a FakeClient driven entirely by a handler — same shape as
// the Python `scripted()` helper.
func Scripted(h FakeHandler) *FakeClient {
	c := NewFakeClient(nil, nil)
	// effectively unbounded
	c.Script = make([]any, 0, 10_000)
	for i := 0; i < 10_000; i++ {
		c.Script = append(c.Script, FakeHandlerFunc(h))
	}
	return c
}

// nextItem returns the next script item, invoking dynamic handlers in place.
// When the script is exhausted, returns Default.
func (c *FakeClient) nextItem(stage, system string, messages []map[string]any, tools []map[string]any) any {
	if c.idx < len(c.Script) {
		item := c.Script[c.idx]
		c.idx++
		if fn, ok := item.(ScriptItemFn); ok {
			return fn(stage, system, messages, tools)
		}
		// Also accept the FakeHandler type directly (callable).
		if fn, ok := item.(FakeHandler); ok {
			return fn(stage, system, messages, tools)
		}
		return item
	}
	return c.Default
}

func (c *FakeClient) toMessage(item any) Message {
	if m, ok := item.(Message); ok {
		return m
	}
	if s, ok := item.(string); ok {
		return Message{
			Content:    []any{NewTextBlock(s)},
			StopReason: "end_turn",
			Usage:      ProviderUsage{InputTokens: 4, OutputTokens: estTokens(s)},
		}
	}
	if m, ok := item.(map[string]any); ok {
		blocks := []any{}
		if t, ok := m["text"].(string); ok && t != "" {
			blocks = append(blocks, NewTextBlock(t))
		}
		var tools []map[string]any
		if t, ok := m["tools"].([]map[string]any); ok {
			tools = t
		} else if t, ok := m["tools"].([]any); ok {
			for _, x := range t {
				if mm, ok := x.(map[string]any); ok {
					tools = append(tools, mm)
				}
			}
		}
		for _, tc := range tools {
			c.toolSeq++
			id, _ := tc["id"].(string)
			if id == "" {
				id = fmt.Sprintf("call_%d", c.toolSeq)
			}
			name, _ := tc["name"].(string)
			input, _ := tc["input"].(map[string]any)
			if input == nil {
				input = map[string]any{}
			}
			blocks = append(blocks, NewToolUseBlock(id, name, input))
		}
		stop := "end_turn"
		if len(tools) > 0 {
			stop = "tool_use"
		} else if s, ok := m["stop_reason"].(string); ok {
			stop = s
		}
		textVal, _ := m["text"].(string)
		if len(blocks) == 0 {
			blocks = append(blocks, NewTextBlock(""))
		}
		return Message{
			Content:    blocks,
			StopReason: stop,
			Usage:      ProviderUsage{InputTokens: 4, OutputTokens: estTokens(textVal)},
		}
	}
	return Message{
		Content:    []any{NewTextBlock(fmt.Sprintf("%v", item))},
		StopReason: "end_turn",
		Usage:      ProviderUsage{InputTokens: 4, OutputTokens: 1},
	}
}

// Call is the LlmCall surface — records the call args, returns the next
// scripted Message, and rolls the usage into the client's total.
func (c *FakeClient) Call(ctx context.Context, req Request) (any, error) {
	c.Calls = append(c.Calls, RecordedCall{
		Stage:       req.Stage,
		System:      req.System,
		Messages:    req.Messages,
		Tools:       req.Tools,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	})
	item := c.nextItem(req.Stage, asString(req.System), req.Messages, req.Tools)
	msg := c.toMessage(item)
	// CountUsage defaults to true (matches the Python `count_usage: bool = True`).
	if req.CountUsageOrDefault() {
		c.Record(msg.Usage)
	}
	return msg, nil
}

func estTokens(s string) int {
	if len(s) == 0 {
		return 1
	}
	n := len(s) / 4
	if n < 1 {
		n = 1
	}
	return n
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}
