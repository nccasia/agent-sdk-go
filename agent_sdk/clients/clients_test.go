// Tests ported from agent-sdk/tests/test_clients.py — LLM clients:
// FakeClient scripting, MixedClient routing, make_client shorthand, MiniMax
// markup recovery, message helpers, finite per-request timeout.
package clients

import (
	"context"
	"testing"
)

// asMessage is a tiny test helper: a Call() result that came back as `any` is
// unwrapped to a `Message` (the engine + these tests never inspect the
// concrete Anthropic / OpenAI response shape — they read .text / .tool_uses
// / .stop_reason / .usage off the engine's Message contract).
func asMessage(t *testing.T, v any) Message {
	t.Helper()
	m, ok := v.(Message)
	if !ok {
		t.Fatalf("expected Message, got %T: %+v", v, v)
	}
	return m
}

// fakeHandler mirrors the Python `def handler(stage, system, messages, tools)` callable
// contract used by FakeClient when a script entry is a function.
func fakeHandler(stage, system string, messages []map[string]any, tools []map[string]any) any {
	return "stage=" + stage
}

// TestFakeTextAnswer — c = FakeClient(["Hello there."]); first call returns end_turn
// with the text and accumulates output tokens > 0.
func TestFakeTextAnswer(t *testing.T) {
	c := NewFakeClient([]any{"Hello there."}, nil)
	v, err := c.Call(context.Background(), Request{
		Stage:     "synthesize",
		System:    "sys",
		Messages:  []map[string]any{},
		MaxTokens: 100,
	})
	if err != nil {
		t.Fatalf("FakeClient.Call: %v", err)
	}
	msg := asMessage(t, v)
	if msg.StopReason != "end_turn" {
		t.Fatalf("stop_reason: got %q want %q", msg.StopReason, "end_turn")
	}
	if msg.Text() != "Hello there." {
		t.Fatalf("text: got %q want %q", msg.Text(), "Hello there.")
	}
	if c.TotalUsage().OutputTokens <= 0 {
		t.Fatalf("expected output_tokens > 0, got %d", c.TotalUsage().OutputTokens)
	}
}

// TestFakeToolCallThenAnswer — first scripted item is a tool dict, second is a
// final text. The first call returns stop_reason=tool_use with one tool_use
// block; the second returns end_turn with the final text.
func TestFakeToolCallThenAnswer(t *testing.T) {
	c := NewFakeClient([]any{
		map[string]any{"tools": []map[string]any{{"name": "search", "input": map[string]any{"query": "x"}}}},
		"Final answer.",
	}, nil)
	v1, err := c.Call(context.Background(), Request{
		Stage:     "research",
		System:    "s",
		Messages:  []map[string]any{},
		MaxTokens: 100,
		Tools:     []map[string]any{{"name": "search"}},
	})
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	m1 := asMessage(t, v1)
	if m1.StopReason != "tool_use" {
		t.Fatalf("m1.stop_reason: got %q want %q", m1.StopReason, "tool_use")
	}
	uses := m1.ToolUses()
	if len(uses) != 1 || uses[0].Name != "search" {
		t.Fatalf("m1.tool_uses: %+v", uses)
	}
	if v, _ := uses[0].Input["query"].(string); v != "x" {
		t.Fatalf("m1.tool_uses[0].input.query: %v", uses[0].Input["query"])
	}
	v2, err := c.Call(context.Background(), Request{
		Stage:     "research",
		System:    "s",
		Messages:  []map[string]any{},
		MaxTokens: 100,
	})
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	m2 := asMessage(t, v2)
	if m2.StopReason != "end_turn" {
		t.Fatalf("m2.stop_reason: got %q want %q", m2.StopReason, "end_turn")
	}
	if m2.Text() != "Final answer." {
		t.Fatalf("m2.text: got %q want %q", m2.Text(), "Final answer.")
	}
}

// TestFakeDefaultWhenExhausted — empty script + default="fallback" yields the
// fallback text once the script is consumed.
func TestFakeDefaultWhenExhausted(t *testing.T) {
	c := NewFakeClient(nil, stringPtr("fallback"))
	v, err := c.Call(context.Background(), Request{
		Stage:     "x",
		System:    "",
		Messages:  []map[string]any{},
		MaxTokens: 10,
	})
	if err != nil {
		t.Fatalf("FakeClient.Call: %v", err)
	}
	msg := asMessage(t, v)
	if msg.Text() != "fallback" {
		t.Fatalf("text: got %q want %q", msg.Text(), "fallback")
	}
}

// TestFakeRecordsCalls — the first recorded call carries the stage + system
// passed in by the caller (used by orchestrators / funnel logic to introspect).
func TestFakeRecordsCalls(t *testing.T) {
	c := NewFakeClient([]any{"ok"}, nil)
	_, err := c.Call(context.Background(), Request{
		Stage:     "synthesize",
		System:    "SYS",
		Messages:  []map[string]any{{"role": "user", "content": "hi"}},
		MaxTokens: 50,
	})
	if err != nil {
		t.Fatalf("FakeClient.Call: %v", err)
	}
	if len(c.Calls) != 1 {
		t.Fatalf("expected 1 recorded call, got %d", len(c.Calls))
	}
	if c.Calls[0].Stage != "synthesize" {
		t.Fatalf("recorded stage: got %q", c.Calls[0].Stage)
	}
	if c.Calls[0].System != "SYS" {
		t.Fatalf("recorded system: got %q", c.Calls[0].System)
	}
}

// TestFakeHandlerCallable — a callable script item is invoked with
// (stage, system, messages, tools) and its return value is rendered as the
// final message.
func TestFakeHandlerCallable(t *testing.T) {
	c := NewFakeClient([]any{FakeHandlerFunc(fakeHandler)}, nil)
	v, err := c.Call(context.Background(), Request{
		Stage:     "classify",
		System:    "",
		Messages:  []map[string]any{},
		MaxTokens: 10,
	})
	if err != nil {
		t.Fatalf("FakeClient.Call: %v", err)
	}
	msg := asMessage(t, v)
	if msg.Text() != "stage=classify" {
		t.Fatalf("text: got %q want %q", msg.Text(), "stage=classify")
	}
}

// TestMixedRoutesPerStage — MixedClient dispatches per stage and aggregates
// total_usage from its sub-clients.
func TestMixedRoutesPerStage(t *testing.T) {
	classify := NewFakeClient([]any{"SIMPLE"}, nil)
	synth := NewFakeClient([]any{"The answer."}, nil)
	deflt := NewFakeClient([]any{"default"}, nil)
	mixed := NewMixedClient(map[string]any{"default": deflt, "classify": classify, "synthesize": synth})

	v1, err := mixed.Call(context.Background(), Request{Stage: "classify", MaxTokens: 10})
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if asMessage(t, v1).Text() != "SIMPLE" {
		t.Fatalf("classify: %q", asMessage(t, v1).Text())
	}
	v2, err := mixed.Call(context.Background(), Request{Stage: "synthesize", MaxTokens: 10})
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if asMessage(t, v2).Text() != "The answer." {
		t.Fatalf("synthesize: %q", asMessage(t, v2).Text())
	}
	v3, err := mixed.Call(context.Background(), Request{Stage: "other", MaxTokens: 10})
	if err != nil {
		t.Fatalf("other: %v", err)
	}
	if asMessage(t, v3).Text() != "default" {
		t.Fatalf("other: %q", asMessage(t, v3).Text())
	}
	if mixed.TotalUsage().OutputTokens <= 0 {
		t.Fatalf("expected aggregate output_tokens > 0, got %d", mixed.TotalUsage().OutputTokens)
	}
}

// TestMakeClientShorthand — make_client("claude-opus-4-6") returns
// AnthropicClient; "gpt-4.1" → OpenAIClient; a client instance passes through
// unchanged.
func TestMakeClientShorthand(t *testing.T) {
	if _, ok := MakeClient("claude-opus-4-6").(*AnthropicClient); !ok {
		t.Fatalf("expected AnthropicClient for claude-opus-4-6")
	}
	if _, ok := MakeClient("gpt-4.1").(*OpenAIClient); !ok {
		t.Fatalf("expected OpenAIClient for gpt-4.1")
	}
	fc := NewFakeClient([]any{"x"}, nil)
	if MakeClient(fc) != LlmCall(fc) {
		t.Fatalf("expected FakeClient instance to pass through unchanged")
	}
}

// TestMessageHelpers — Message{TextBlock, ToolUseBlock} exposes .text (joined
// text blocks) and .tool_uses (filtered tool_use blocks).
func TestMessageHelpers(t *testing.T) {
	msg := NewMessage([]any{TextBlock{Text: "hi"}, ToolUseBlock{ID: "1", Name: "t", Input: map[string]any{}}})
	if msg.Text() != "hi" {
		t.Fatalf("msg.text: %q", msg.Text())
	}
	uses := msg.ToolUses()
	if len(uses) != 1 || uses[0].Name != "t" {
		t.Fatalf("msg.tool_uses: %+v", uses)
	}
}

// TestMiniMaxRoutingAndDefaults — make_client("MiniMax-M2.7") / "abab-6.5"
// return MiniMaxClient; MiniMaxClient() defaults to "MiniMax-M2.7" and reports
// provider="minimax".
func TestMiniMaxRoutingAndDefaults(t *testing.T) {
	if _, ok := MakeClient("MiniMax-M2.7").(*MiniMaxClient); !ok {
		t.Fatalf("expected MiniMaxClient for MiniMax-M2.7")
	}
	if _, ok := MakeClient("abab-6.5").(*MiniMaxClient); !ok {
		t.Fatalf("expected MiniMaxClient for abab-6.5")
	}
	def := NewMiniMaxClient("")
	if def.Model() != "MiniMax-M2.7" {
		t.Fatalf("MiniMaxClient default model: %q", def.Model())
	}
	if def.ProviderName() != "minimax" {
		t.Fatalf("MiniMaxClient provider: %q", def.ProviderName())
	}
}

// TestAnthropicIsFaithfulPassthrough — the base Anthropic client is a
// passthrough: _postprocess returns the same response, markup untouched.
func TestAnthropicIsFaithfulPassthrough(t *testing.T) {
	markup := `<minimax:tool_call>
<invoke name="x"><parameter name="a">1</parameter></invoke>`
	resp := newFakeAnthropicResp("end_turn", markup)
	out := (&AnthropicClient{}).postprocess(resp)
	if out != resp {
		t.Fatalf("expected passthrough, got different object")
	}
}

// TestMiniMaxRecoversMarkupToolCalls — MiniMax._postprocess reconstructs a
// Message with stop_reason=tool_use and one ToolUseBlock per <invoke>, the
// markup is stripped from the text.
func TestMiniMaxRecoversMarkupToolCalls(t *testing.T) {
	markup := "Sure, writing the doc.\n" +
		`<minimax:tool_call>
<invoke name="write_file">
<parameter name="path">ARCHITECTURE.md</parameter>
<parameter name="content"># Title

Body line.</parameter>
</invoke>
</minimax:tool_call>`
	resp := newFakeAnthropicRespWithUsage("end_turn", markup, 10, 20)
	v := (&MiniMaxClient{}).postprocess(resp)
	out, ok := v.(Message)
	if !ok {
		t.Fatalf("expected Message, got %T: %+v", v, v)
	}
	if out.StopReason != "tool_use" {
		t.Fatalf("stop_reason: %q", out.StopReason)
	}
	uses := out.ToolUses()
	if len(uses) != 1 {
		t.Fatalf("tool_uses: %+v", uses)
	}
	if uses[0].Name != "write_file" {
		t.Fatalf("tool_use name: %q", uses[0].Name)
	}
	if uses[0].Input["path"] != "ARCHITECTURE.md" {
		t.Fatalf("tool_use input.path: %v", uses[0].Input["path"])
	}
	if uses[0].Input["content"] != "# Title\n\nBody line." {
		t.Fatalf("tool_use input.content: %v", uses[0].Input["content"])
	}
	if out.Text() != "Sure, writing the doc." {
		t.Fatalf("text after strip: %q", out.Text())
	}
}

// TestMiniMaxRecoveredIDsAreUniqueAcrossHops — recovered tool-call ids must
// not collide across messages (the Anthropic-compatible API rejects duplicate
// ids on round-tripped tool_results).
func TestMiniMaxRecoveredIDsAreUniqueAcrossHops(t *testing.T) {
	client := &MiniMaxClient{}
	markup := func(name string) string {
		return `<invoke name="` + name + `"><parameter name="x">1</parameter></invoke>`
	}
	var ids []string
	for i := 0; i < 4; i++ {
		resp := newFakeAnthropicRespWithUsage("end_turn", markup("Read"), 1, 1)
		v := client.postprocess(resp)
		out, ok := v.(Message)
		if !ok {
			t.Fatalf("expected Message, got %T", v)
		}
		uses := out.ToolUses()
		if len(uses) < 1 {
			t.Fatalf("expected at least one tool_use, got %+v", uses)
		}
		ids = append(ids, uses[0].ID)
	}
	want := []string{"markup_0", "markup_1", "markup_2", "markup_3"}
	for i, w := range want {
		if ids[i] != w {
			t.Fatalf("id[%d]: got %q want %q", i, ids[i], w)
		}
	}
	seen := map[string]struct{}{}
	for _, id := range ids {
		seen[id] = struct{}{}
	}
	if len(seen) != 4 {
		t.Fatalf("ids not unique: %v", ids)
	}
}

// TestClientsHaveFiniteRequestTimeout — finite per-request timeout so a stalled
// provider call fails fast instead of hanging the turn.
func TestClientsHaveFiniteRequestTimeout(t *testing.T) {
	if NewAnthropicClient("").Timeout() != 300.0 {
		t.Fatalf("AnthropicClient default timeout: %v", NewAnthropicClient("").Timeout())
	}
	if NewMiniMaxClient("").Timeout() != 300.0 {
		t.Fatalf("MiniMaxClient default timeout: %v", NewMiniMaxClient("").Timeout())
	}
	ac := NewAnthropicClient("m", WithTimeout(45.0))
	if ac.Timeout() != 45.0 {
		t.Fatalf("AnthropicClient custom timeout: %v", ac.Timeout())
	}
}

// TestMiniMaxPassthroughOnNativeToolUse — native tool_use responses pass
// through unchanged (no markup to recover).
func TestMiniMaxPassthroughOnNativeToolUse(t *testing.T) {
	resp := &fakeAnthropicResp{StopReason: "tool_use", Content: []any{}}
	if out := (&MiniMaxClient{}).postprocess(resp); out != resp {
		t.Fatalf("expected passthrough on native tool_use, got different object")
	}
}

// TestMiniMaxRecoversTruncatedMarkup — when max_tokens cuts the call off
// mid-content, the closing tags are absent; recovery must still match to the
// end of the text (\Z tolerance).
func TestMiniMaxRecoversTruncatedMarkup(t *testing.T) {
	markup := `writing… <minimax:tool_call>
<invoke name="bash">
<parameter name="command">cat > F.md << EOF
# Title
lots of content that got cut`
	resp := newFakeAnthropicResp("max_tokens", markup)
	v := (&MiniMaxClient{}).postprocess(resp)
	out, ok := v.(Message)
	if !ok {
		t.Fatalf("expected Message, got %T", v)
	}
	if out.StopReason != "tool_use" {
		t.Fatalf("stop_reason: %q", out.StopReason)
	}
	uses := out.ToolUses()
	if len(uses) != 1 || uses[0].Name != "bash" {
		t.Fatalf("tool_uses: %+v", uses)
	}
	cmd, _ := uses[0].Input["command"].(string)
	if !contains(cmd, "cat > F.md") {
		t.Fatalf("recovered command missing prefix: %q", cmd)
	}
}

// BenchmarkMixedAggregate measures the cost of aggregating usage across
// sub-clients on every read. Hot path when the engine polls accounting.
func BenchmarkMixedAggregate(b *testing.B) {
	c1 := NewFakeClient([]any{"a", "b", "c"}, nil)
	c2 := NewFakeClient([]any{"d", "e"}, nil)
	m := NewMixedClient(map[string]any{"default": c1, "classify": c2})
	for i := 0; i < 100; i++ {
		_, _ = m.Call(context.Background(), Request{Stage: "classify"})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.TotalUsage()
	}
}

// BenchmarkFakeScript walks a long script to measure FakeClient throughput —
// the offline engine path used by every offline test.
func BenchmarkFakeScript(b *testing.B) {
	script := make([]any, 0, b.N)
	for i := 0; i < b.N; i++ {
		script = append(script, "x")
	}
	c := NewFakeClient(script, nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Call(context.Background(), Request{Stage: "s"})
	}
}

// stringPtr is a tiny helper so the test table reads as Pythonic kwargs.
func stringPtr(s string) *string { return &s }

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(s) > 0 && indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
