// The shared agentic tool loop: model call → execute tool_use blocks → repeat,
// with a forced tool-free final hop. Ported from agent_sdk/lobes/runtime.py
// (tool_loop), the shared run_tool_loop() of RFC 0015 Phase 4.
package engine

// LoopMessage is the minimal provider-message surface ToolLoop reads: only the
// stop_reason discriminator drives the loop's control flow. Mirrors the Python
// loop's reliance on “msg.stop_reason“ (the host owns everything else via the
// callbacks).
type LoopMessage interface {
	// StopReason is the provider stop_reason: "tool_use", "end_turn", or
	// another non-tool reason (e.g. "max_tokens").
	StopReason() string
}

// ToolLoopParams configures one ToolLoop run. Every field mirrors a Python
// tool_loop keyword argument.
type ToolLoopParams struct {
	// Messages is the running conversation; ToolLoop appends the assistant's
	// tool_use turn and the tool_result turn in place, and Retier (when set)
	// rewrites the tail. The caller holds this same slice via the returned
	// Messages so pipeline/evidence state stays coherent.
	Messages []map[string]any

	// Tools is the tool spec list offered on each non-final hop.
	Tools []map[string]any

	// Call performs ONE model call: call(messages, tools) -> (msg, textHint).
	// The caller owns streaming, partial emission, and usage policy; textHint
	// is the live-streamed text ("" for non-streaming callers).
	Call func(messages, tools []map[string]any) (LoopMessage, string)

	// ExecuteTools runs the msg's tool_use blocks and returns the tool_result
	// content blocks.
	ExecuteTools func(msg LoopMessage) []map[string]any

	// AssistantContent rebuilds the assistant message's content blocks.
	AssistantContent func(msg LoopMessage) []map[string]any

	// MaxLoops caps the number of model calls.
	MaxLoops int

	// DropToolsOnFinalHop runs the final allowed hop WITHOUT tools so the model
	// must answer from what it gathered (the forced tool-free final hop).
	DropToolsOnFinalHop bool

	// StrictEndTurn selects the simple-path break semantics: only end_turn ends
	// the loop and yields the answer; any other non-tool stop reason loops again
	// (never accept a truncated turn as final). When false (research path), any
	// non-tool_use stop reason breaks and the caller extracts from the msg.
	StrictEndTurn bool

	// Retier — when non-nil — re-tiers the message tail AFTER each hop's
	// observation is appended and BEFORE the next model call (PreAct funnel,
	// react-context-management.md). nil ⇒ vanilla-ReAct accumulation.
	Retier func(messages []map[string]any, hop int) []map[string]any
}

// ToolLoopResult is the (last_msg, answer_text, messages) return of ToolLoop.
// AnswerText is non-empty only for a strict end_turn exit. Messages is the
// (possibly retiered) conversation slice after the loop.
type ToolLoopResult struct {
	Msg        LoopMessage
	AnswerText string
	Messages   []map[string]any
}

// ToolLoop runs the shared agentic loop: model call → execute tool_use blocks →
// repeat. Break semantics (both legacy loops preserved exactly):
//
//   - StrictEndTurn=true (simple path): only "end_turn" ends the loop and yields
//     the answer (textHint falling back to extractText); any other non-tool stop
//     reason loops again.
//   - StrictEndTurn=false (research): any non-"tool_use" stop reason breaks; the
//     caller extracts what it needs from the returned msg.
//   - DropToolsOnFinalHop: the final allowed hop runs WITHOUT tools so the model
//     must answer from what it gathered.
//
// Returns (lastMsg, answerText, messages) — answerText is non-empty only for a
// strict end_turn exit.
func ToolLoop(p ToolLoopParams) ToolLoopResult {
	messages := p.Messages
	var msg LoopMessage
	answerText := ""
	for loop := 0; loop < p.MaxLoops; loop++ {
		loopTools := p.Tools
		if p.DropToolsOnFinalHop && loop >= p.MaxLoops-1 {
			loopTools = nil
		}
		var textHint string
		msg, textHint = p.Call(messages, loopTools)
		switch {
		case msg.StopReason() == "tool_use":
			messages = append(messages,
				map[string]any{"role": "assistant", "content": p.AssistantContent(msg)},
				map[string]any{"role": "user", "content": p.ExecuteTools(msg)},
			)
			if p.Retier != nil {
				// Funnel the tail in place — the caller holds this same list
				// reference (pipeline/evidence state stays coherent).
				messages = p.Retier(messages, loop)
			}
		case p.StrictEndTurn:
			if msg.StopReason() == "end_turn" {
				answerText = textHint
				if answerText == "" {
					answerText = extractText(msg)
				}
				goto done
			}
			// Other stop reasons (e.g. max_tokens) loop again — legacy
			// simple-path semantics: never accept a truncated turn as final.
		default:
			goto done
		}
	}
done:
	return ToolLoopResult{Msg: msg, AnswerText: answerText, Messages: messages}
}

// extractText falls back to the message's joined text when the live textHint is
// empty. Mirrors runtime.extract_text — all text blocks newline-joined — for a
// msg that also implements the engine Message surface; otherwise "".
func extractText(msg LoopMessage) string {
	if m, ok := msg.(Message); ok {
		return TextOf(m)
	}
	return ""
}
