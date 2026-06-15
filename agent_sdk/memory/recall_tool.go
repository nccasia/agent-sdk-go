package memory

import (
	"context"
	"strings"
)

var writeKinds = map[string]struct{}{
	"note": {}, "decision": {}, "sub_goal": {}, "hypothesis": {},
	"fact": {}, "obligation": {}, "plan": {},
}

// RecallToolRuntime exposes recall (read) + note (write-to-think) over a store.
type RecallToolRuntime struct {
	store  *MemoryStore
	Writes []map[string]any // structured {kind, scope, handle} this turn
}

// NewRecallToolRuntime builds a runtime over store.
func NewRecallToolRuntime(store *MemoryStore) *RecallToolRuntime {
	return &RecallToolRuntime{store: store}
}

// GetToolSpecs returns the recall + note tool specs.
func (r *RecallToolRuntime) GetToolSpecs() []map[string]any {
	return []map[string]any{
		{
			"name": "recall",
			"description": "Read from memory. Use query to search the digest index across everything " +
				"remembered (results, notes, decisions, facts); use handle to expand one entry " +
				"back to its full detail (full=true), or grep/section to read a slice of a large " +
				"body. This is how you read back the detail behind a digest.",
			"input_schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":   map[string]any{"type": "string", "description": "free-text search over digests"},
					"handle":  map[string]any{"type": "string", "description": "mem://… handle to expand"},
					"full":    map[string]any{"type": "boolean", "description": "return the full body"},
					"grep":    map[string]any{"type": "string", "description": "regex; matching lines of a large body"},
					"section": map[string]any{"type": "string", "description": "section id; one slice of a large body"},
					"kind":    map[string]any{"type": "string", "description": "filter the search by kind"},
				},
			},
		},
		{
			"name": "note",
			"description": "Write to think: record a decision, note, sub_goal, hypothesis, or established " +
				"fact so it persists in your working memory across steps (you won't have to " +
				"re-derive it). Use scope=conversation for a durable fact/decision; default is " +
				"this turn only.",
			"input_schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content": map[string]any{"type": "string"},
					"kind":    map[string]any{"type": "string", "enum": []string{"note", "decision", "sub_goal", "hypothesis", "fact", "obligation", "plan"}},
					"scope":   map[string]any{"type": "string", "enum": []string{FlashScope, "conversation", "user", "bot"}},
					"key":     map[string]any{"type": "string"},
				},
				"required": []string{"content"},
			},
		},
	}
}

// CallTool dispatches recall / note.
func (r *RecallToolRuntime) CallTool(_ context.Context, name string, inp map[string]any) (string, error) {
	switch name {
	case "recall":
		return r.recall(inp), nil
	case "note":
		return r.note(inp), nil
	}
	return "Error: unknown tool '" + name + "'.", nil
}

func (r *RecallToolRuntime) recall(inp map[string]any) string {
	handle := asString(inp["handle"])
	if handle != "" {
		if sec := asString(inp["section"]); sec != "" {
			s := r.store.ReadSection(handle, sec)
			if s != "" {
				return s
			}
			return "(no section '" + sec + "' in " + handle + ")"
		}
		if g := asString(inp["grep"]); g != "" {
			hits := r.store.Grep(handle, g, 50)
			var lines []string
			for _, h := range hits {
				lines = append(lines, asString(h["line"]))
			}
			if len(lines) == 0 {
				return "(no matches)"
			}
			return strings.Join(lines, "\n")
		}
		if asBool(inp["full"]) {
			if e := r.store.Get(handle); e != nil {
				return e.Body
			}
			return "(unknown handle " + handle + ")"
		}
		if e := r.store.Get(handle); e != nil {
			return e.Digest
		}
		return "(unknown handle " + handle + ")"
	}
	results := r.store.Recall(RecallOpts{Query: asString(inp["query"]), Kind: asString(inp["kind"])})
	if len(results) == 0 {
		return "(nothing remembered yet)"
	}
	var lines []string
	for _, e := range results {
		lines = append(lines, "- "+e.Handle+" — "+e.Digest)
	}
	return strings.Join(lines, "\n")
}

func (r *RecallToolRuntime) note(inp map[string]any) string {
	kind := asString(inp["kind"])
	if _, ok := writeKinds[kind]; !ok {
		kind = "note"
	}
	scope := asString(inp["scope"])
	if scope == "" {
		scope = "conversation"
	}
	pinned := kind == "decision" || kind == "plan" || kind == "obligation"
	handle := r.store.Remember(kind, asString(inp["content"]), RememberOpts{
		Scope: scope, Key: asString(inp["key"]), Pinned: pinned, Source: "note",
	})
	r.Writes = append(r.Writes, map[string]any{"kind": kind, "scope": scope, "handle": handle})
	durable := " (durable)"
	if scope == FlashScope {
		durable = ""
	}
	return "Noted " + kind + " as " + handle + durable + "."
}
