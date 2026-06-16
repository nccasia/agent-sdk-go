// Package tasks hosts the TaskPlugin — the OPT-IN, self-contained
// plugin for todo-driven task execution. Lives outside the core
// network; mount `plugins=[TaskPlugin()]` to add it, drop it to
// remove every task factor, or replace it wholesale.
package tasks

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Status values for a Todo. Mirrors agent_sdk.plugins.tasks.todos.
const (
	StatusOpen    = "open"
	StatusDone    = "done"
	StatusBlocked = "blocked"
)

// Todo is one row in the rail.
type Todo struct {
	ID     string         `json:"id"`
	Title  string         `json:"title"`
	Status string         `json:"status"`
	Deps   []string       `json:"deps"`
	Result string         `json:"result,omitempty"`
	Spec   map[string]any `json:"spec,omitempty"`
}

// TodoRail is the in-memory checklist. IDs are auto-assigned (t0, t1, …).
type TodoRail struct {
	todos    []*Todo
	byID     map[string]*Todo
	humanAsk []string
}

// NewTodoRail builds an empty rail.
func NewTodoRail() *TodoRail {
	return &TodoRail{byID: map[string]*Todo{}}
}

// Add appends a new todo. `title` is required; `deps` is an optional
// list of dependency todo IDs. Mirrors TodoRail.add.
func (r *TodoRail) Add(title string, deps ...string) *Todo {
	id := fmt.Sprintf("t%d", len(r.todos))
	t := &Todo{ID: id, Title: title, Status: StatusOpen, Deps: append([]string(nil), deps...)}
	r.todos = append(r.todos, t)
	r.byID[id] = t
	return t
}

// AddWithSpec appends a new todo with per-todo spec overrides
// (e.g. `tools`, `system_prompt`).
func (r *TodoRail) AddWithSpec(title string, spec map[string]any, deps ...string) *Todo {
	t := r.Add(title, deps...)
	t.Spec = spec
	return t
}

// All returns a defensive copy of the todos in insertion order.
func (r *TodoRail) All() []*Todo {
	out := make([]*Todo, len(r.todos))
	copy(out, r.todos)
	return out
}

// ByID looks up a todo by id (or nil if not present).
func (r *TodoRail) ByID(id string) *Todo { return r.byID[id] }

// Ready returns the open, deps-satisfied todos. The list is sorted
// by id for determinism.
func (r *TodoRail) Ready() []*Todo {
	out := []*Todo{}
	for _, t := range r.todos {
		if t.Status != StatusOpen {
			continue
		}
		if allDepsDone(r, t.Deps) {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func allDepsDone(r *TodoRail, deps []string) bool {
	for _, d := range deps {
		t, ok := r.byID[d]
		if !ok || t.Status != StatusDone {
			return false
		}
	}
	return true
}

// TopoOrder returns the todos in dependency order (a todo follows
// its deps). Cyclic / dangling deps go last. Mirrors the Python
// Kahn's-algorithm implementation.
func (r *TodoRail) TopoOrder() []*Todo {
	ordered := []*Todo{}
	placed := map[string]struct{}{}
	remaining := append([]*Todo{}, r.todos...)
	progressed := true
	for progressed && len(remaining) > 0 {
		progressed = false
		for i := 0; i < len(remaining); i++ {
			t := remaining[i]
			ok := true
			for _, d := range t.Deps {
				if _, has := placed[d]; !has {
					ok = false
					break
				}
			}
			if ok {
				ordered = append(ordered, t)
				placed[t.ID] = struct{}{}
				remaining = append(remaining[:i], remaining[i+1:]...)
				progressed = true
				break
			}
		}
	}
	return append(ordered, remaining...)
}

// IsComplete reports whether every todo is done.
func (r *TodoRail) IsComplete() bool {
	for _, t := range r.todos {
		if t.Status != StatusDone {
			return false
		}
	}
	return len(r.todos) > 0
}

// AsItems returns the todos as the engine-consumable items (used by
// the `execute` stage's fan-out). Per-todo spec overrides (tools,
// system_prompt) are carried over.
func (r *TodoRail) AsItems() []map[string]any {
	out := []map[string]any{}
	for _, t := range r.TopoOrder() {
		item := map[string]any{
			"id":     t.ID,
			"input":  t.Title,
			"status": t.Status,
		}
		if len(t.Deps) > 0 {
			item["deps"] = t.Deps
		}
		if t.Result != "" {
			item["result"] = t.Result
		}
		if t.Spec != nil {
			for k, v := range t.Spec {
				item[k] = v
			}
		}
		out = append(out, item)
	}
	return out
}

// MarkDone marks the named todo as done with a result. Returns the
// todo and a friendly message for the model.
func (r *TodoRail) MarkDone(id string, result string) string {
	t := r.byID[id]
	if t == nil {
		return fmt.Sprintf("Error: no such todo %q.", id)
	}
	if t.Status == StatusDone {
		return fmt.Sprintf("%s already done.", id)
	}
	t.Status = StatusDone
	t.Result = result
	return fmt.Sprintf("%s marked done.", id)
}

// MarkBlocked marks the todo as blocked and records the human question.
func (r *TodoRail) MarkBlocked(id, question string) string {
	t := r.byID[id]
	if t == nil {
		return fmt.Sprintf("Error: no such todo %q.", id)
	}
	t.Status = StatusBlocked
	r.humanAsk = append(r.humanAsk, question)
	return fmt.Sprintf("Escalated to human: %s — %s", id, question)
}

// HumanAsks returns the recorded human questions in insertion order.
func (r *TodoRail) HumanAsks() []string {
	return append([]string(nil), r.humanAsk...)
}

// AsScratchpad renders the todos into the engine's scratchpad data
// (so the task_rail lobe can render them into a stage's prompt).
func (r *TodoRail) AsScratchpad() (todos, results []map[string]any) {
	todos = []map[string]any{}
	for _, t := range r.TopoOrder() {
		row := map[string]any{
			"id":    t.ID,
			"input": t.Title,
		}
		if len(t.Deps) > 0 {
			row["deps"] = t.Deps
		}
		todos = append(todos, row)
	}
	results = []map[string]any{}
	for _, t := range r.TopoOrder() {
		if t.Status == StatusDone {
			results = append(results, map[string]any{"label": t.ID, "result": t.Result})
		}
	}
	return todos, results
}

// TodosToolRuntime is the ONE `todos` tool the task plugin mounts.
// It owns the rail and exposes the manage surface.
type TodosToolRuntime struct {
	rail *TodoRail
}

// NewTodosToolRuntime builds a runtime over a fresh rail.
func NewTodosToolRuntime() *TodosToolRuntime { return &TodosToolRuntime{rail: NewTodoRail()} }

// NewTodosToolRuntimeWithRail builds a runtime over an existing rail.
func NewTodosToolRuntimeWithRail(rail *TodoRail) *TodosToolRuntime {
	return &TodosToolRuntime{rail: rail}
}

// Rail returns the underlying rail (the engine + the task_rail lobe
// both need read access).
func (rt *TodosToolRuntime) Rail() *TodoRail { return rt.rail }

// GetToolSpecs returns the single `todos` tool spec.
func (rt *TodosToolRuntime) GetToolSpecs() []map[string]any {
	return []map[string]any{
		{
			"name": "todos",
			"input_schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{
						"type": "string",
						"enum": []string{"add", "add_many", "list", "done", "block", "request_human"},
					},
					"title":    map[string]any{"type": "string"},
					"steps":    map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
					"id":       map[string]any{"type": "string"},
					"deps":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"result":   map[string]any{"type": "string"},
					"question": map[string]any{"type": "string"},
					"spec":     map[string]any{"type": "object"},
				},
				"required": []string{"action"},
			},
			"description": "Manage the task rail (add / add_many / list / done / block / request_human).",
		},
	}
}

// CallTool dispatches the action. Matches the Python `TodosToolRuntime.call_tool`.
func (rt *TodosToolRuntime) CallTool(ctx context.Context, name string, inp map[string]any, _ []map[string]any, _ map[string]struct{}) (string, error) {
	if name != "todos" {
		return fmt.Sprintf("Error: unknown tool %q.", name), nil
	}
	action, _ := inp["action"].(string)
	switch action {
	case "add":
		title, _ := inp["title"].(string)
		if title == "" {
			return "Error: `title` required.", nil
		}
		deps := toStringSlice(inp["deps"])
		spec, _ := inp["spec"].(map[string]any)
		if spec != nil {
			rt.rail.AddWithSpec(title, spec, deps...)
		} else {
			rt.rail.Add(title, deps...)
		}
		return fmt.Sprintf("added (%d open)", len(rt.rail.Ready())), nil
	case "add_many":
		steps, _ := inp["steps"].([]any)
		for _, s := range steps {
			row, _ := s.(map[string]any)
			if row == nil {
				continue
			}
			title, _ := row["title"].(string)
			deps := toStringSlice(row["deps"])
			spec, _ := row["spec"].(map[string]any)
			if spec != nil {
				rt.rail.AddWithSpec(title, spec, deps...)
			} else {
				rt.rail.Add(title, deps...)
			}
		}
		return fmt.Sprintf("added_many (now %d todos)", len(rt.rail.All())), nil
	case "list":
		items := []string{}
		for _, t := range rt.rail.TopoOrder() {
			items = append(items, fmt.Sprintf("%s %s: %s", statusMark(t.Status), t.ID, t.Title))
		}
		return strings.Join(items, "\n"), nil
	case "done":
		id, _ := inp["id"].(string)
		if id == "" {
			// no id ⇒ next open todo.
			ready := rt.rail.Ready()
			if len(ready) == 0 {
				return "Error: no open todo to mark done.", nil
			}
			id = ready[0].ID
		}
		result, _ := inp["result"].(string)
		// If the next todo is not ready (deps not done), report it.
		t := rt.rail.ByID(id)
		if t == nil {
			return fmt.Sprintf("Error: no such todo %q.", id), nil
		}
		if t.Status != StatusOpen {
			return fmt.Sprintf("Error: %s is %s.", id, t.Status), nil
		}
		if !allDepsDone(rt.rail, t.Deps) {
			return fmt.Sprintf("Error: %s needs %s.", id, strings.Join(t.Deps, ",")), nil
		}
		rt.rail.MarkDone(id, result)
		if rt.rail.IsComplete() {
			return "rail complete", nil
		}
		return fmt.Sprintf("%s done", id), nil
	case "block", "request_human":
		id, _ := inp["id"].(string)
		if id == "" {
			ready := rt.rail.Ready()
			if len(ready) == 0 {
				return "Error: no open todo to block.", nil
			}
			id = ready[0].ID
		}
		q, _ := inp["question"].(string)
		rt.rail.MarkBlocked(id, q)
		return fmt.Sprintf("Escalated to human: %s", id), nil
	default:
		return fmt.Sprintf("Error: unknown action %q.", action), nil
	}
}

func statusMark(s string) string {
	switch s {
	case StatusDone:
		return "[x]"
	case StatusBlocked:
		return "[!]"
	default:
		return "[ ]"
	}
}

func toStringSlice(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := []string{}
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
