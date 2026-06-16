package engine

import (
	"reflect"
	"testing"
)

// loopMsg is a test provider message implementing both LoopMessage and the
// engine Message surface (so extractText can fall back to its text blocks).
type loopMsg struct {
	stop   string
	blocks []Block
}

func (m loopMsg) StopReason() string { return m.stop }
func (m loopMsg) Content() []Block   { return m.blocks }

func textMsg(stop, text string) loopMsg {
	return loopMsg{stop: stop, blocks: []Block{TextBlock{Text: text}}}
}

// A tool_use hop appends the assistant turn + the tool_result turn, then loops;
// the strict end_turn hop yields the answer.
func TestToolLoopExecutesToolsThenAnswers(t *testing.T) {
	scripted := []loopMsg{
		{stop: "tool_use", blocks: []Block{ToolUseBlock{ID: "t1", Name: "search"}}},
		textMsg("end_turn", "final answer"),
	}
	var hop int
	var sawTools [][]map[string]any
	res := ToolLoop(ToolLoopParams{
		Messages: []map[string]any{{"role": "user", "content": "q"}},
		Tools:    []map[string]any{{"name": "search"}},
		MaxLoops: 5,
		Call: func(messages, tools []map[string]any) (LoopMessage, string) {
			sawTools = append(sawTools, tools)
			m := scripted[hop]
			hop++
			return m, ""
		},
		ExecuteTools: func(msg LoopMessage) []map[string]any {
			return []map[string]any{{"type": "tool_result", "tool_use_id": "t1", "content": "obs"}}
		},
		AssistantContent: func(msg LoopMessage) []map[string]any {
			return []map[string]any{{"type": "tool_use", "id": "t1"}}
		},
		StrictEndTurn: true,
	})
	if res.AnswerText != "final answer" {
		t.Fatalf("AnswerText = %q, want %q", res.AnswerText, "final answer")
	}
	if hop != 2 {
		t.Fatalf("hops = %d, want 2", hop)
	}
	// user, assistant(tool_use), user(tool_result).
	if len(res.Messages) != 3 {
		t.Fatalf("len(messages) = %d, want 3: %v", len(res.Messages), res.Messages)
	}
	if res.Messages[1]["role"] != "assistant" || res.Messages[2]["role"] != "user" {
		t.Fatalf("appended turns wrong: %v", res.Messages)
	}
}

// An empty textHint on a strict end_turn exit falls back to the message's text.
func TestToolLoopAnswerFallsBackToMessageText(t *testing.T) {
	res := ToolLoop(ToolLoopParams{
		Messages: []map[string]any{},
		MaxLoops: 1,
		Call: func(messages, tools []map[string]any) (LoopMessage, string) {
			return textMsg("end_turn", "from blocks"), ""
		},
		StrictEndTurn: true,
	})
	if res.AnswerText != "from blocks" {
		t.Fatalf("AnswerText = %q, want %q", res.AnswerText, "from blocks")
	}
}

// A non-empty textHint wins over the message text on a strict end_turn exit.
func TestToolLoopAnswerPrefersTextHint(t *testing.T) {
	res := ToolLoop(ToolLoopParams{
		MaxLoops: 1,
		Call: func(messages, tools []map[string]any) (LoopMessage, string) {
			return textMsg("end_turn", "blocks"), "streamed hint"
		},
		StrictEndTurn: true,
	})
	if res.AnswerText != "streamed hint" {
		t.Fatalf("AnswerText = %q, want %q", res.AnswerText, "streamed hint")
	}
}

// DropToolsOnFinalHop runs the last allowed hop without tools.
func TestToolLoopDropsToolsOnFinalHop(t *testing.T) {
	var toolsByHop [][]map[string]any
	tools := []map[string]any{{"name": "search"}}
	ToolLoop(ToolLoopParams{
		Tools:               tools,
		MaxLoops:            2,
		DropToolsOnFinalHop: true,
		Call: func(messages, tools []map[string]any) (LoopMessage, string) {
			toolsByHop = append(toolsByHop, tools)
			return loopMsg{stop: "tool_use", blocks: []Block{ToolUseBlock{ID: "t"}}}, ""
		},
		ExecuteTools:     func(msg LoopMessage) []map[string]any { return nil },
		AssistantContent: func(msg LoopMessage) []map[string]any { return nil },
		StrictEndTurn:    true,
	})
	if len(toolsByHop) != 2 {
		t.Fatalf("hops = %d, want 2", len(toolsByHop))
	}
	if len(toolsByHop[0]) != 1 {
		t.Fatalf("first hop tools = %v, want the tool list", toolsByHop[0])
	}
	if toolsByHop[1] != nil {
		t.Fatalf("final hop tools = %v, want nil (dropped)", toolsByHop[1])
	}
}

// StrictEndTurn=false (research path): any non-tool_use stop reason breaks and
// yields no answer text — the caller extracts from the returned msg.
func TestToolLoopResearchPathBreaksOnNonToolUse(t *testing.T) {
	res := ToolLoop(ToolLoopParams{
		MaxLoops: 5,
		Call: func(messages, tools []map[string]any) (LoopMessage, string) {
			return textMsg("max_tokens", "partial"), "partial"
		},
		StrictEndTurn: false,
	})
	if res.AnswerText != "" {
		t.Fatalf("AnswerText = %q, want empty (research path)", res.AnswerText)
	}
	if res.Msg.StopReason() != "max_tokens" {
		t.Fatalf("returned msg stop = %q, want max_tokens", res.Msg.StopReason())
	}
}

// StrictEndTurn=true: a non-end_turn, non-tool_use stop reason loops again
// rather than ending the turn (never accept a truncated turn as final).
func TestToolLoopStrictRetriesNonEndTurn(t *testing.T) {
	scripted := []loopMsg{
		textMsg("max_tokens", "truncated"),
		textMsg("end_turn", "done"),
	}
	var hop int
	res := ToolLoop(ToolLoopParams{
		MaxLoops: 5,
		Call: func(messages, tools []map[string]any) (LoopMessage, string) {
			m := scripted[hop]
			hop++
			return m, ""
		},
		StrictEndTurn: true,
	})
	if hop != 2 {
		t.Fatalf("hops = %d, want 2 (looped past the truncated turn)", hop)
	}
	if res.AnswerText != "done" {
		t.Fatalf("AnswerText = %q, want %q", res.AnswerText, "done")
	}
}

// Retier rewrites the message tail after each observation, before the next call.
func TestToolLoopRetierFunnelsTail(t *testing.T) {
	scripted := []loopMsg{
		{stop: "tool_use", blocks: []Block{ToolUseBlock{ID: "t"}}},
		textMsg("end_turn", "ans"),
	}
	var hop int
	var retierCalls []int
	res := ToolLoop(ToolLoopParams{
		Messages:         []map[string]any{{"role": "user", "content": "q"}},
		MaxLoops:         5,
		Call:             func(m, t []map[string]any) (LoopMessage, string) { x := scripted[hop]; hop++; return x, "" },
		ExecuteTools:     func(msg LoopMessage) []map[string]any { return []map[string]any{{"type": "tool_result"}} },
		AssistantContent: func(msg LoopMessage) []map[string]any { return []map[string]any{{"type": "tool_use"}} },
		StrictEndTurn:    true,
		Retier: func(messages []map[string]any, hop int) []map[string]any {
			retierCalls = append(retierCalls, hop)
			return []map[string]any{{"role": "system", "content": "funnelled"}}
		},
	})
	if !reflect.DeepEqual(retierCalls, []int{0}) {
		t.Fatalf("retier called with hops %v, want [0]", retierCalls)
	}
	// The funnel replaced the tail in place before the final hop.
	if len(res.Messages) != 1 || res.Messages[0]["content"] != "funnelled" {
		t.Fatalf("messages after retier = %v, want the funnelled tail", res.Messages)
	}
}
