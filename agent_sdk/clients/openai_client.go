package clients

import (
	"context"
	"encoding/json"
	"fmt"
)

// OpenAIClientOption is a functional option for OpenAIClient construction.
type OpenAIClientOption func(*OpenAIClient)

// WithOpenAITimeout overrides the per-request timeout.
func WithOpenAITimeout(t float64) OpenAIClientOption { return func(c *OpenAIClient) { c.timeout = t } }

// WithOpenAIAPIKey sets the API key.
func WithOpenAIAPIKey(k string) OpenAIClientOption { return func(c *OpenAIClient) { c.APIKey = k } }

// WithOpenAIBaseURL sets the OpenAI base URL.
func WithOpenAIBaseURL(u string) OpenAIClientOption { return func(c *OpenAIClient) { c.BaseURL = u } }

// WithOpenAIMaxRetries sets the SDK max_retries.
func WithOpenAIMaxRetries(n int) OpenAIClientOption {
	return func(c *OpenAIClient) { c.maxRetries = n }
}

// OpenAIClient is the OpenAI Chat Completions API adapter. It maps the
// Anthropic-style request/response shape onto OpenAI's function-calling shape.
type OpenAIClient struct {
	BaseClient
	timeout    float64
	maxRetries int
	client     any // *openai.AsyncOpenAI, lazily initialized
}

// NewOpenAIClient builds a client with the given model + options.
func NewOpenAIClient(model string, opts ...OpenAIClientOption) *OpenAIClient {
	c := &OpenAIClient{
		BaseClient: BaseClient{ModelName: model, Provider: "openai"},
		timeout:    300.0,
		maxRetries: 2,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// SetClient injects a real *openai.AsyncOpenAI (or compatible surface).
func (c *OpenAIClient) SetClient(cl any) { c.client = cl }

// Timeout returns the per-request timeout in seconds.
func (c *OpenAIClient) Timeout() float64 { return c.timeout }

// Call performs one Chat Completions call and adapts the response into a
// provider Message.
func (c *OpenAIClient) Call(ctx context.Context, req Request) (any, error) {
	if c.client == nil {
		return nil, errNoOpenAIClient
	}
	tools := ToOpenAITools(req.Tools)
	messages := ToOpenAIMessages(req.System, req.Messages)
	resp, err := callOpenAI(ctx, c.client, c.ModelFor(req.Stage), messages, req.MaxTokens, temperatureOrZero(req.Temperature), tools)
	if err != nil {
		return nil, err
	}
	msg, err := AdaptOpenAIResponse(resp)
	if err != nil {
		return nil, err
	}
	if req.CountUsageOrDefault() {
		c.Record(msg.Usage)
	}
	return msg, nil
}

// ToOpenAITools converts Anthropic-style tool specs into OpenAI's
// function-calling shape.
func ToOpenAITools(tools []map[string]any) []map[string]any {
	if len(tools) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		name, _ := t["name"].(string)
		desc, _ := t["description"].(string)
		schema, ok := t["input_schema"].(map[string]any)
		if !ok {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        name,
				"description": desc,
				"parameters":  schema,
			},
		})
	}
	return out
}

// ToOpenAIMessages converts a (system, messages) tuple into the OpenAI
// messages array. Tool results are flattened to text — sufficient for the
// single/agentic loops the engine drives.
func ToOpenAIMessages(system any, messages []map[string]any) []map[string]any {
	out := []map[string]any{{"role": "system", "content": flattenSystem(system)}}
	for _, m := range messages {
		out = append(out, anthropicMsgToOpenAI(m))
	}
	return out
}

func flattenSystem(system any) string {
	switch s := system.(type) {
	case string:
		return s
	case []any:
		parts := []string{}
		for _, b := range s {
			if m, ok := b.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					parts = append(parts, t)
					continue
				}
			}
			if tb, ok := b.(TextBlock); ok {
				parts = append(parts, tb.Text)
				continue
			}
			parts = append(parts, fmt.Sprintf("%v", b))
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
	return fmt.Sprintf("%v", system)
}

func anthropicMsgToOpenAI(m map[string]any) map[string]any {
	role, _ := m["role"].(string)
	if role == "" {
		role = "user"
	}
	content := m["content"]
	if s, ok := content.(string); ok {
		return map[string]any{"role": role, "content": s}
	}
	arr, ok := content.([]any)
	if !ok {
		return map[string]any{"role": role, "content": fmt.Sprintf("%v", content)}
	}
	var texts []string
	for _, b := range arr {
		mb, ok := b.(map[string]any)
		if !ok {
			texts = append(texts, fmt.Sprintf("%v", b))
			continue
		}
		switch mb["type"] {
		case "text":
			texts = append(texts, fmt.Sprintf("%v", mb["text"]))
		case "tool_result":
			texts = append(texts, fmt.Sprintf("%v", mb["content"]))
		case "tool_use":
			arg, _ := json.Marshal(mb["input"])
			texts = append(texts, fmt.Sprintf("[called %v(%s)]", mb["name"], arg))
		default:
			texts = append(texts, fmt.Sprintf("%v", b))
		}
	}
	joined := ""
	for i, t := range texts {
		if i > 0 {
			joined += "\n"
		}
		joined += t
	}
	return map[string]any{"role": role, "content": joined}
}

// AdaptOpenAIResponse converts an OpenAI Chat Completions response into a
// provider Message.
func AdaptOpenAIResponse(resp any) (Message, error) {
	choices, ok := getField(resp, "Choices")
	if !ok {
		return Message{}, fmt.Errorf("openai: missing choices in response")
	}
	cv := reflectValue(choices)
	if !reflectSliceKind(cv) || cv.Len() == 0 {
		return Message{}, fmt.Errorf("openai: no choices in response")
	}
	choice := cv.Index(0).Interface()
	m, ok := getField(choice, "Message")
	if !ok {
		return Message{}, fmt.Errorf("openai: missing message in choice")
	}
	blocks := []any{}
	if txt, ok := getString(m, "Content"); ok && txt != "" {
		blocks = append(blocks, NewTextBlock(txt))
	}
	if tcs, ok := getField(m, "ToolCalls"); ok {
		tcv := reflectValue(tcs)
		if tcv.IsValid() && reflectSliceKind(tcv) {
			for i := 0; i < tcv.Len(); i++ {
				tc := tcv.Index(i).Interface()
				id, _ := getString(tc, "ID")
				fn, _ := getField(tc, "Function")
				name, _ := getString(fn, "Name")
				var args map[string]any
				if raw, ok := getString(fn, "Arguments"); ok && raw != "" {
					if jerr := json.Unmarshal([]byte(raw), &args); jerr != nil {
						args = map[string]any{}
					}
				} else {
					args = map[string]any{}
				}
				blocks = append(blocks, NewToolUseBlock(id, name, args))
			}
		}
	}
	if len(blocks) == 0 {
		blocks = append(blocks, NewTextBlock(""))
	}
	stop := "end_turn"
	if fr, ok := getString(choice, "FinishReason"); ok {
		if mapped, ok := openAIFinishToStop[fr]; ok {
			stop = mapped
		}
	}
	usage := ProviderUsage{}
	if u, ok := getField(resp, "Usage"); ok && u != nil {
		if v, ok := getInt(u, "PromptTokens"); ok {
			usage.InputTokens = v
		}
		if v, ok := getInt(u, "CompletionTokens"); ok {
			usage.OutputTokens = v
		}
	}
	return Message{Content: blocks, StopReason: stop, Usage: usage}, nil
}

var openAIFinishToStop = map[string]string{
	"stop":          "end_turn",
	"tool_calls":    "tool_use",
	"function_call": "tool_use",
	"length":        "max_tokens",
}
