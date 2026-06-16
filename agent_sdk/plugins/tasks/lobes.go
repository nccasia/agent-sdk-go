package tasks

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nccasia/agent-sdk-go/agent_sdk/core/spec"
	"github.com/nccasia/agent-sdk-go/agent_sdk/lobes"
)

// TaskRailLOBE is the canonical `task_rail` render lobe. It reads the
// rail (todos + their results) from the turn's scratchpad and renders
// a checklist into the stage's prompt. Mounts into execute + deliver
// stages.
var TaskRailLOBE = lobes.Lobe{
	ID:           "task_rail",
	Name:         "task_rail",
	Description:  "Renders the current todo rail (with done/open status and deps) into the prompt.",
	UseWhen:      "The todos tool has been used this turn.",
	How:          "Reads the scratchpad's todos / todos_results; emits a checklist block. Empty rail ⇒ no contribution.",
	Layer:        4,
	Behavior:     "recall",
	Order:        0,
	BuildContext: true,
	Threshold:    0.0,
	Activation:   taskRailActivation,
}

func taskRailActivation(ctx map[string]any) float64 {
	// The lobe is on whenever a rail exists. A no-rail turn ⇒ 0.
	sp, ok := ctx["scratchpad"]
	if !ok || sp == nil {
		return 0
	}
	if _, has := asScratchpad(sp).asList("todos"); has {
		return 1
	}
	return 0
}

// TaskRailLobeSpec compiles TaskRailLOBE to its internal spec.Lobe form.
func TaskRailLobeSpec() spec.Lobe { return TaskRailLOBE.Spec() }

// TaskRailLobe is the explicit Go mirror of the Python TaskRailLobe
// class — `prompt(ctx) -> [PromptContribution]`. Lobe clients that
// prefer the structured form can use this directly.
type TaskRailLobe struct{}

// Prompt renders the rail (or returns an empty slice if no rail
// exists). Mirrors `TaskRailLobe.prompt`.
func (TaskRailLobe) Prompt(ctx any) []TaskRailRender {
	return taskRailRender(ctx)
}

// PromptContribution is the rich render output. Mirrors Python's
// (text, source) tuple shape.
type TaskRailRender struct {
	Text   string
	Source string
}

// TaskRailRenderString returns the rendered text (the simpler shape
// tests assert against). Returns "" if the rail is empty.
func TaskRailRenderString(ctx any) string {
	contribs := taskRailRender(ctx)
	parts := []string{}
	for _, c := range contribs {
		parts = append(parts, c.Text)
	}
	return strings.Join(parts, "")
}

// scratchpadShape is the structural shape the lobe reads.
type scratchpadShape struct {
	data map[string]any
}

func (s scratchpadShape) asList(k string) ([]map[string]any, bool) {
	v, ok := s.data[k]
	if !ok {
		return nil, false
	}
	switch x := v.(type) {
	case []map[string]any:
		return x, true
	case []any:
		out := []map[string]any{}
		for _, e := range x {
			if m, ok := e.(map[string]any); ok {
				out = append(out, m)
			}
		}
		if len(out) > 0 {
			return out, true
		}
	}
	return nil, false
}

func asScratchpad(v any) scratchpadShape {
	switch x := v.(type) {
	case scratchpadShape:
		return x
	case map[string]any:
		return scratchpadShape{data: x}
	case *scratchpadShape:
		if x != nil {
			return *x
		}
	}
	return scratchpadShape{}
}

func taskRailRender(ctx any) []TaskRailRender {
	spVal := readField(ctx, "scratchpad")
	if spVal == nil {
		return nil
	}
	sp := asScratchpad(spVal)
	todos, ok := sp.asList("todos")
	if !ok || len(todos) == 0 {
		return nil
	}
	results, _ := sp.asList("todos_results")
	resultsByID := map[string]string{}
	for _, r := range results {
		id, _ := r["label"].(string)
		if id == "" {
			id, _ = r["id"].(string)
		}
		val, _ := r["result"].(string)
		if id != "" {
			resultsByID[id] = val
		}
	}
	// Sort the rail by id for determinism.
	sort.Slice(todos, func(i, j int) bool {
		return idOf(todos[i]) < idOf(todos[j])
	})
	lines := []string{"Task checklist:"}
	for _, t := range todos {
		id := idOf(t)
		title, _ := t["input"].(string)
		if title == "" {
			title, _ = t["title"].(string)
		}
		mark := "[ ]"
		if _, done := resultsByID[id]; done {
			mark = "[x]"
		}
		deps, _ := t["deps"].([]any)
		line := fmt.Sprintf("%s %s: %s", mark, id, title)
		if len(deps) > 0 {
			depStrs := []string{}
			for _, d := range deps {
				if s, ok := d.(string); ok {
					depStrs = append(depStrs, s)
				}
			}
			if len(depStrs) > 0 {
				line += " (needs " + strings.Join(depStrs, ",") + ")"
			}
		}
		lines = append(lines, line)
	}
	return []TaskRailRender{{Text: strings.Join(lines, "\n"), Source: "task_rail"}}
}

func idOf(t map[string]any) string {
	if s, ok := t["id"].(string); ok {
		return s
	}
	return ""
}

// readField does a duck-typed field read. The test side passes a
// SimpleNamespace-style struct; the engine side passes a
// *contracts.TurnContext. Both expose "scratchpad".
func readField(v any, field string) any {
	switch x := v.(type) {
	case map[string]any:
		return x[field]
	}
	// Reflection fallback for structs with a `Scratchpad` field.
	return structReadField(v, field)
}
