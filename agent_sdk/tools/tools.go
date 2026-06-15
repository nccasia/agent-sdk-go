// Package tools ports agent_sdk/tools — a typed function becomes a
// provider-compatible tool ({name, description, input_schema}) that runs inside
// the engine's tool loop, plus the FunctionToolRuntime / CompositeToolRuntime
// that plug a set of such tools into the ToolRuntime boundary.
//
// Go has no runtime reflection of a function's named parameters the way Python's
// inspect.signature gives, and no Pydantic. So a tool's input schema is declared
// explicitly via Param options (the hand-rolled object schema — the Pydantic
// substitute). The tool function is a single (ctx, map[string]any) -> (any, error)
// closure: the engine already passes tool input as a JSON object.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Fn is a tool body: it receives the call's JSON arguments and returns either a
// model-visible result (stringified by the runtime) or an error (surfaced to the
// model, not crashing the turn).
type Fn func(ctx context.Context, in map[string]any) (any, error)

// param is one declared input property.
type param struct {
	name     string
	typ      string
	required bool
	def      any // default value; nil ⇒ no default emitted
}

// ToolDef is a typed function wrapped as a provider-compatible tool — the Go
// equivalent of the Python Tool class.
type ToolDef struct {
	Name        string
	Description string
	Requires    []string
	Fn          Fn

	params      []param
	InputSchema map[string]any
}

// Option configures a Tool at build time.
type Option func(*ToolDef)

// Desc sets the tool description (Python: the function docstring).
func Desc(d string) Option { return func(t *ToolDef) { t.Description = strings.TrimSpace(d) } }

// Requires declares capability gates the tool needs (Python: requires=[...]).
func Requires(names ...string) Option {
	return func(t *ToolDef) { t.Requires = append(t.Requires, names...) }
}

// Param declares one input property: its JSON-schema type, whether it is
// required, and an optional default (use nil for none). Replaces Python's
// signature introspection.
func Param(name, typ string, required bool, def any) Option {
	return func(t *ToolDef) {
		t.params = append(t.params, param{name: name, typ: typ, required: required, def: def})
	}
}

// Tool builds a ToolDef from a name, a function, and declared options — the Go
// equivalent of the @tool decorator (a builder, since Go has no decorators).
func Tool(name string, fn Fn, opts ...Option) *ToolDef {
	t := &ToolDef{Name: name, Fn: fn}
	for _, o := range opts {
		o(t)
	}
	t.InputSchema = t.buildInputSchema()
	return t
}

func (t *ToolDef) buildInputSchema() map[string]any {
	properties := map[string]any{}
	required := []string{}
	for _, p := range t.params {
		frag := map[string]any{}
		if p.typ != "" {
			frag["type"] = p.typ
		}
		if p.required {
			required = append(required, p.name)
		} else if p.def != nil {
			frag["default"] = p.def
		}
		properties[p.name] = frag
	}
	return map[string]any{"type": "object", "properties": properties, "required": required}
}

// Spec returns the {name, description, input_schema} provider spec.
func (t *ToolDef) Spec() map[string]any {
	return map[string]any{
		"name":         t.Name,
		"description":  t.Description,
		"input_schema": t.InputSchema,
	}
}

// MissingRequired returns the required properties absent from inp (empty ⇒ the
// call is well-formed). Mirrors Tool.missing_required.
func (t *ToolDef) MissingRequired(inp map[string]any) []string {
	var out []string
	for _, p := range t.params {
		if !p.required {
			continue
		}
		if _, ok := inp[p.name]; !ok {
			out = append(out, p.name)
		}
	}
	return out
}

// Invoke runs the tool body with the given arguments.
func (t *ToolDef) Invoke(ctx context.Context, inp map[string]any) (any, error) {
	if inp == nil {
		inp = map[string]any{}
	}
	return t.Fn(ctx, inp)
}

// stringify renders a tool result as model-visible text (Python: _stringify).
func stringify(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// FunctionToolRuntime is a ToolRuntime over a set of ToolDefs — it implements
// GetToolSpecs / CallTool so plain typed functions plug straight into the tool
// loop (and compose with MCP via CompositeToolRuntime).
type FunctionToolRuntime struct {
	tools map[string]*ToolDef
	order []string
}

// NewFunctionToolRuntime builds a runtime over the given tools (insertion order
// preserved for stable spec ordering).
func NewFunctionToolRuntime(tools ...*ToolDef) *FunctionToolRuntime {
	r := &FunctionToolRuntime{tools: map[string]*ToolDef{}}
	for _, t := range tools {
		r.Add(t)
	}
	return r
}

// Add registers a tool (re-adding a name overwrites in place, keeping order).
func (r *FunctionToolRuntime) Add(t *ToolDef) {
	if _, dup := r.tools[t.Name]; !dup {
		r.order = append(r.order, t.Name)
	}
	r.tools[t.Name] = t
}

// Get returns the named tool, or nil.
func (r *FunctionToolRuntime) Get(name string) *ToolDef { return r.tools[name] }

// Names returns the tool names in insertion order.
func (r *FunctionToolRuntime) Names() []string { return append([]string(nil), r.order...) }

// GetToolSpecs returns the specs in insertion order.
func (r *FunctionToolRuntime) GetToolSpecs() []map[string]any {
	out := make([]map[string]any, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.tools[name].Spec())
	}
	return out
}

// CallTool validates required args, runs the tool, and stringifies the result —
// surfacing a clean, model-actionable message on a missing arg or a tool error
// instead of crashing the turn. Mirrors FunctionToolRuntime.call_tool.
func (r *FunctionToolRuntime) CallTool(ctx context.Context, name string, inp map[string]any, retrievedChunks []map[string]any, alreadyRead map[string]struct{}) (string, error) {
	t := r.tools[name]
	if t == nil {
		return fmt.Sprintf("Error: unknown tool '%s'. Use only the provided tools.", name), nil
	}
	if missing := t.MissingRequired(inp); len(missing) > 0 {
		plural := ""
		them := "it"
		if len(missing) > 1 {
			plural = "s"
			them = "them"
		}
		quoted := make([]string, len(missing))
		for i, m := range missing {
			quoted[i] = "'" + m + "'"
		}
		return fmt.Sprintf("Error: tool '%s' requires argument%s %s. Provide %s and call again.",
			name, plural, strings.Join(quoted, ", "), them), nil
	}
	result, err := t.Invoke(ctx, inp)
	if err != nil {
		return fmt.Sprintf("Error calling tool '%s': %s", name, err.Error()), nil
	}
	return stringify(result), nil
}
