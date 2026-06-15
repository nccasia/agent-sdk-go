package clients

// Test-only fake Anthropic response shape — used by the markup-recovery
// tests to simulate the duck-typed response that the real anthropic SDK
// returns. Field names are exported so the reflection helpers in reflect.go
// find them (they use FieldByName).
type fakeAnthropicResp struct {
	StopReason string
	Content    []any
	Usage      *fakeUsage
}

type fakeUsage struct {
	InputTokens              int
	OutputTokens             int
	CacheReadInputTokens     int
	CacheCreationInputTokens int
}

type fakeAnthropicContentBlock struct {
	Type string
	Text string
}

func newFakeAnthropicResp(stopReason, text string) *fakeAnthropicResp {
	return &fakeAnthropicResp{
		StopReason: stopReason,
		Content:    []any{&fakeAnthropicContentBlock{Type: "text", Text: text}},
		Usage:      nil,
	}
}

func newFakeAnthropicRespWithUsage(stopReason, text string, in, out int) *fakeAnthropicResp {
	return &fakeAnthropicResp{
		StopReason: stopReason,
		Content:    []any{&fakeAnthropicContentBlock{Type: "text", Text: text}},
		Usage:      &fakeUsage{InputTokens: in, OutputTokens: out},
	}
}
