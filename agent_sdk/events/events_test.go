package events

import (
	"context"
	"testing"

	"github.com/mezon/agent-sdk-go/agent_sdk/contracts"
	"github.com/mezon/agent-sdk-go/agent_sdk/result"
)

func TestEventPositionalMatch(t *testing.T) {
	ev := &TextDelta{Text: "hello"}
	if ev.Text != "hello" {
		t.Fatalf("Text = %q", ev.Text)
	}
	if ev.Type() != "text_delta" {
		t.Fatalf("Type = %q", ev.Type())
	}
}

func TestEventToJSON(t *testing.T) {
	ev := &ToolCall{ID: "1", Name: "search", Input: map[string]any{"q": "x"}}
	j := ev.ToJSON()
	if j["type"] != "tool_call" {
		t.Errorf("type = %v", j["type"])
	}
	if j["name"] != "search" {
		t.Errorf("name = %v", j["name"])
	}
	inp, _ := j["input"].(map[string]any)
	if inp["q"] != "x" {
		t.Errorf("input = %v", j["input"])
	}
}

func TestCitationEventSerializesPydantic(t *testing.T) {
	cit := contracts.Citation{ChunkID: "c1", SourceRef: "doc#1", SupportingSpan: [2]int{0, 5}}
	ev := &CitationFound{Citation: cit}
	j := ev.ToJSON()
	c, _ := j["citation"].(map[string]any)
	if c["chunk_id"] != "c1" {
		t.Errorf("citation.chunk_id = %v", c["chunk_id"])
	}
}

func TestStampFillsTraceIDAndTS(t *testing.T) {
	ev := Stamp(&RunStart{}, "trace-9").(*RunStart)
	if ev.TraceID != "trace-9" {
		t.Errorf("TraceID = %q", ev.TraceID)
	}
	if ev.TS <= 0 {
		t.Errorf("TS = %v", ev.TS)
	}
}

func TestAgentStreamIteratesAndAwaits(t *testing.T) {
	res := &result.AgentResult{Text: "done"}
	source := func(yield func(AgentEvent) bool) {
		if !yield(&RunStart{TraceID: "t"}) {
			return
		}
		if !yield(&TextDelta{Text: "partial"}) {
			return
		}
		yield(&Final{Result: res, TraceID: "t"})
	}
	stream := NewAgentStream(source)
	var seen []string
	for ev := range stream.Iter() {
		seen = append(seen, typeName(ev))
	}
	want := []string{"RunStart", "TextDelta", "Final"}
	if len(seen) != len(want) {
		t.Fatalf("seen = %v", seen)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("seen = %v", seen)
		}
	}
	got, _ := stream.Result(context.Background())
	if r, _ := got.(*result.AgentResult); r == nil || r.Text != "done" {
		t.Errorf("result = %v", got)
	}
}

func TestAgentStreamAwaitDrains(t *testing.T) {
	res := &result.AgentResult{Text: "answer"}
	source := func(yield func(AgentEvent) bool) {
		if !yield(&TextDelta{Text: "a"}) {
			return
		}
		yield(&Final{Result: res})
	}
	stream := NewAgentStream(source)
	got, _ := stream.Result(context.Background())
	if r, _ := got.(*result.AgentResult); r == nil || r.Text != "answer" {
		t.Errorf("result = %v", got)
	}
}

func TestAgentStreamTextStream(t *testing.T) {
	source := func(yield func(AgentEvent) bool) {
		for _, ev := range []AgentEvent{
			&TextDelta{Text: "foo"},
			&ToolCall{Name: "t"},
			&TextDelta{Text: "bar"},
			&Final{Result: &result.AgentResult{Text: "foobar"}},
		} {
			if !yield(ev) {
				return
			}
		}
	}
	stream := NewAgentStream(source)
	var chunks []string
	for c := range stream.TextStream() {
		chunks = append(chunks, c)
	}
	if len(chunks) != 2 || chunks[0] != "foo" || chunks[1] != "bar" {
		t.Errorf("chunks = %v", chunks)
	}
}

func typeName(ev AgentEvent) string {
	switch ev.(type) {
	case *RunStart:
		return "RunStart"
	case *PathResolved:
		return "PathResolved"
	case *StageStart:
		return "StageStart"
	case *TextDelta:
		return "TextDelta"
	case *ToolCall:
		return "ToolCall"
	case *ToolResult:
		return "ToolResult"
	case *CitationFound:
		return "CitationFound"
	case *MetaAction:
		return "MetaAction"
	case *StageEnd:
		return "StageEnd"
	case *Final:
		return "Final"
	}
	return "?"
}
