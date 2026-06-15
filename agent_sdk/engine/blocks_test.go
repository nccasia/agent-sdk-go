package engine

import (
	"reflect"
	"strings"
	"testing"
)

// Thinking blocks never leak as a language repr. Translated from the A4 block
// of tests/test_engine_robustness.py.

// fakeUnknown is a future/unknown block with no usable text.
type fakeUnknown struct{}

func (fakeUnknown) BlockType() string { return "weird" }

func TestThinkingBlockNormalizedNotReprd(t *testing.T) {
	d := BlockToDict(ThinkingBlock{Thinking: "let me reason about this"})
	want := map[string]any{"type": "thinking", "text": "let me reason about this"}
	if !reflect.DeepEqual(d, want) {
		t.Fatalf("BlockToDict(thinking) = %v, want %v", d, want)
	}
	du := BlockToDict(fakeUnknown{})
	wantU := map[string]any{"type": "text", "text": ""}
	if !reflect.DeepEqual(du, wantU) {
		t.Fatalf("BlockToDict(unknown) = %v, want %v", du, wantU)
	}
}

func TestThinkingDroppedFromReplayedHistory(t *testing.T) {
	msg := SimpleMessage{Blocks: []Block{
		ThinkingBlock{Thinking: "let me reason about this"},
		TextBlock{Text: "the real answer"},
	}}
	hist := AssistantContent(msg)
	found := false
	for _, b := range hist {
		if b["type"] == "text" && b["text"] == "the real answer" {
			found = true
		}
		if b["type"] == "thinking" {
			t.Fatalf("thinking block leaked into replayed history: %v", b)
		}
		if s, ok := b["text"].(string); ok && strings.Contains(s, "Thinking") {
			t.Fatalf("Thinking repr leaked into replayed content: %v", b)
		}
	}
	if !found {
		t.Fatal("expected {type:text, text:the real answer} in history")
	}
}

func TestTextOfIgnoresThinking(t *testing.T) {
	got := TextOf(SimpleMessage{Blocks: []Block{
		ThinkingBlock{Thinking: "let me reason about this"},
		TextBlock{Text: "hello"},
	}})
	if got != "hello" {
		t.Fatalf("TextOf = %q, want hello", got)
	}
	got = TextOf(SimpleMessage{Blocks: []Block{ThinkingBlock{Thinking: "x"}}})
	if got != "" {
		t.Fatalf("TextOf(thinking only) = %q, want empty", got)
	}
}
