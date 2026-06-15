// Package events holds the typed streaming events + the AgentStream wrapper.
// agent.Act(...) returns an AgentStream — an iterable of typed events that is
// also drainable to the final result (à la the Vercel AI SDK / Pydantic AI
// run_stream). Events serialize 1:1 to JSON (ev.ToJSON()) for SSE / pub-sub
// transport — the same wire shape the Mezon worker publishes. Ported from
// agent_sdk/events.py.
package events

import (
	"context"
	"time"

	"github.com/mezon/agent-sdk-go/agent_sdk/contracts"
)

// AgentEvent is a typed, pattern-matchable streaming event. Every event tags
// itself with a stable type string and serializes 1:1 to JSON.
type AgentEvent interface {
	// Type returns the wire-stable type tag (e.g. "run_start", "text_delta").
	Type() string
	// ToJSON renders the event as a wire-stable map (always includes "type").
	ToJSON() map[string]any
	// setStamp fills trace_id + ts (the emitter's single touch-point).
	setStamp(traceID string, ts float64)
}

// jsonify mirrors the Python _jsonify helper: prefers a ToJSON() rendering,
// recurses into maps/slices, and passes scalars through.
func jsonify(v any) any {
	switch x := v.(type) {
	case nil, string, int, int64, float64, bool:
		return x
	case interface{ ToJSON() map[string]any }:
		return x.ToJSON()
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, vv := range x {
			out[k] = jsonify(vv)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, vv := range x {
			out[i] = jsonify(vv)
		}
		return out
	default:
		return x
	}
}

// RunStart marks the start of a run.
type RunStart struct {
	TraceID string  `json:"trace_id"`
	TS      float64 `json:"ts"`
}

// Type returns "run_start".
func (e *RunStart) Type() string { return "run_start" }

// ToJSON renders the event.
func (e *RunStart) ToJSON() map[string]any {
	return map[string]any{"type": e.Type(), "trace_id": e.TraceID, "ts": e.TS}
}
func (e *RunStart) setStamp(t string, ts float64) { e.TraceID, e.TS = t, ts }

// PathResolved reports the resolved routing path + its score.
type PathResolved struct {
	Path    string  `json:"path"`
	Score   float64 `json:"score"`
	TraceID string  `json:"trace_id"`
	TS      float64 `json:"ts"`
}

// Type returns "path_resolved".
func (e *PathResolved) Type() string { return "path_resolved" }

// ToJSON renders the event.
func (e *PathResolved) ToJSON() map[string]any {
	return map[string]any{"type": e.Type(), "path": e.Path, "score": e.Score, "trace_id": e.TraceID, "ts": e.TS}
}
func (e *PathResolved) setStamp(t string, ts float64) { e.TraceID, e.TS = t, ts }

// StageStart marks the start of a flow stage.
type StageStart struct {
	Flow    string  `json:"flow"`
	Stage   string  `json:"stage"`
	TraceID string  `json:"trace_id"`
	TS      float64 `json:"ts"`
}

// Type returns "stage_start".
func (e *StageStart) Type() string { return "stage_start" }

// ToJSON renders the event.
func (e *StageStart) ToJSON() map[string]any {
	return map[string]any{"type": e.Type(), "flow": e.Flow, "stage": e.Stage, "trace_id": e.TraceID, "ts": e.TS}
}
func (e *StageStart) setStamp(t string, ts float64) { e.TraceID, e.TS = t, ts }

// TextDelta is a streamed chunk of answer text.
type TextDelta struct {
	Text    string  `json:"text"`
	TraceID string  `json:"trace_id"`
	TS      float64 `json:"ts"`
}

// Type returns "text_delta".
func (e *TextDelta) Type() string { return "text_delta" }

// ToJSON renders the event.
func (e *TextDelta) ToJSON() map[string]any {
	return map[string]any{"type": e.Type(), "text": e.Text, "trace_id": e.TraceID, "ts": e.TS}
}
func (e *TextDelta) setStamp(t string, ts float64) { e.TraceID, e.TS = t, ts }

// ToolCall reports a tool invocation.
type ToolCall struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Input   map[string]any `json:"input"`
	TraceID string         `json:"trace_id"`
	TS      float64        `json:"ts"`
}

// Type returns "tool_call".
func (e *ToolCall) Type() string { return "tool_call" }

// ToJSON renders the event.
func (e *ToolCall) ToJSON() map[string]any {
	return map[string]any{"type": e.Type(), "id": e.ID, "name": e.Name, "input": jsonify(toAnyMap(e.Input)), "trace_id": e.TraceID, "ts": e.TS}
}
func (e *ToolCall) setStamp(t string, ts float64) { e.TraceID, e.TS = t, ts }

// ToolResult reports a tool result.
type ToolResult struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Output  string  `json:"output"`
	TraceID string  `json:"trace_id"`
	TS      float64 `json:"ts"`
}

// Type returns "tool_result".
func (e *ToolResult) Type() string { return "tool_result" }

// ToJSON renders the event.
func (e *ToolResult) ToJSON() map[string]any {
	return map[string]any{"type": e.Type(), "id": e.ID, "name": e.Name, "output": e.Output, "trace_id": e.TraceID, "ts": e.TS}
}
func (e *ToolResult) setStamp(t string, ts float64) { e.TraceID, e.TS = t, ts }

// CitationFound reports a citation discovered during the run.
type CitationFound struct {
	Citation any     `json:"citation"`
	TraceID  string  `json:"trace_id"`
	TS       float64 `json:"ts"`
}

// Type returns "citation".
func (e *CitationFound) Type() string { return "citation" }

// ToJSON renders the event.
func (e *CitationFound) ToJSON() map[string]any {
	return map[string]any{"type": e.Type(), "citation": jsonify(citationJSON(e.Citation)), "trace_id": e.TraceID, "ts": e.TS}
}
func (e *CitationFound) setStamp(t string, ts float64) { e.TraceID, e.TS = t, ts }

// MetaAction reports a metacognition decision.
type MetaAction struct {
	Action  string  `json:"action"`
	Reason  string  `json:"reason"`
	TraceID string  `json:"trace_id"`
	TS      float64 `json:"ts"`
}

// Type returns "meta_action".
func (e *MetaAction) Type() string { return "meta_action" }

// ToJSON renders the event.
func (e *MetaAction) ToJSON() map[string]any {
	return map[string]any{"type": e.Type(), "action": e.Action, "reason": e.Reason, "trace_id": e.TraceID, "ts": e.TS}
}
func (e *MetaAction) setStamp(t string, ts float64) { e.TraceID, e.TS = t, ts }

// StageEnd marks the end of a flow stage, carrying its usage roll-up.
type StageEnd struct {
	Flow    string  `json:"flow"`
	Stage   string  `json:"stage"`
	Usage   any     `json:"usage"`
	TraceID string  `json:"trace_id"`
	TS      float64 `json:"ts"`
}

// Type returns "stage_end".
func (e *StageEnd) Type() string { return "stage_end" }

// ToJSON renders the event.
func (e *StageEnd) ToJSON() map[string]any {
	return map[string]any{"type": e.Type(), "flow": e.Flow, "stage": e.Stage, "usage": jsonify(e.Usage), "trace_id": e.TraceID, "ts": e.TS}
}
func (e *StageEnd) setStamp(t string, ts float64) { e.TraceID, e.TS = t, ts }

// Final carries the terminal AgentResult of a run.
type Final struct {
	Result  any     `json:"result"`
	TraceID string  `json:"trace_id"`
	TS      float64 `json:"ts"`
}

// Type returns "final".
func (e *Final) Type() string { return "final" }

// ToJSON renders the event.
func (e *Final) ToJSON() map[string]any {
	return map[string]any{"type": e.Type(), "result": jsonify(e.Result), "trace_id": e.TraceID, "ts": e.TS}
}
func (e *Final) setStamp(t string, ts float64) { e.TraceID, e.TS = t, ts }

// Stamp fills trace_id + ts on an event (the emitter's single touch-point) and
// returns it for chaining.
func Stamp(event AgentEvent, traceID string) AgentEvent {
	event.setStamp(traceID, float64(time.Now().UnixNano())/1e9)
	return event
}

// toAnyMap normalises a typed input map to map[string]any for jsonify.
func toAnyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	return m
}

// citationJSON renders an arbitrary citation payload (ToJSON-able value or raw
// map) to a wire map; nil stays nil.
func citationJSON(v any) any {
	if v == nil {
		return nil
	}
	if j, ok := v.(interface{ ToJSON() map[string]any }); ok {
		return j.ToJSON()
	}
	switch c := v.(type) {
	case contracts.Citation:
		return citationMap(c)
	case *contracts.Citation:
		return citationMap(*c)
	}
	return v
}

func citationMap(c contracts.Citation) map[string]any {
	return map[string]any{
		"chunk_id":        c.ChunkID,
		"source_ref":      c.SourceRef,
		"supporting_span": []int{c.SupportingSpan[0], c.SupportingSpan[1]},
	}
}

// AgentStream is an iterable of events that is also drainable to the final
// result. The source is a Go iterator (func(yield func(AgentEvent) bool)).
type AgentStream struct {
	source func(yield func(AgentEvent) bool)
	result any
	done   bool
}

// NewAgentStream wraps an event-producing iterator.
func NewAgentStream(source func(yield func(AgentEvent) bool)) *AgentStream {
	return &AgentStream{source: source}
}

// Iter returns a channel of events, capturing the Final result as it passes.
func (s *AgentStream) Iter() <-chan AgentEvent {
	ch := make(chan AgentEvent)
	go func() {
		defer close(ch)
		s.source(func(ev AgentEvent) bool {
			if f, ok := ev.(*Final); ok {
				s.result = f.Result
			}
			ch <- ev
			return true
		})
		s.done = true
	}()
	return ch
}

// TextStream returns a channel of just the text-delta chunks.
func (s *AgentStream) TextStream() <-chan string {
	ch := make(chan string)
	go func() {
		defer close(ch)
		for ev := range s.Iter() {
			if td, ok := ev.(*TextDelta); ok {
				ch <- td.Text
			}
		}
	}()
	return ch
}

// Result drains the stream (if not already drained) and returns the captured
// Final result.
func (s *AgentStream) Result(ctx context.Context) (any, error) {
	if !s.done && s.result == nil {
		for range s.Iter() {
			select {
			case <-ctx.Done():
				return s.result, ctx.Err()
			default:
			}
		}
	}
	return s.result, nil
}
