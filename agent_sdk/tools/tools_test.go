package tools

import (
	"context"
	"errors"
	"testing"
)

// TestBareDecoratorSchemaFromSignature mirrors
// test_bare_decorator_schema_from_signature: a tool built from a name + params
// exposes a {name, description, input_schema} spec.
func TestBareDecoratorSchemaFromSignature(t *testing.T) {
	search := Tool("search",
		func(ctx context.Context, in map[string]any) (any, error) { return "ok", nil },
		Desc("Search the knowledge base."),
		Param("query", "string", true, nil),
		Param("top_k", "integer", false, 5),
	)
	spec := search.Spec()
	if spec["name"] != "search" {
		t.Fatalf("name = %v", spec["name"])
	}
	if spec["description"] != "Search the knowledge base." {
		t.Fatalf("description = %v", spec["description"])
	}
	props := spec["input_schema"].(map[string]any)["properties"].(map[string]any)
	q := props["query"].(map[string]any)
	if q["type"] != "string" {
		t.Fatalf("query type = %v", q["type"])
	}
	tk := props["top_k"].(map[string]any)
	if tk["type"] != "integer" {
		t.Fatalf("top_k type = %v", tk["type"])
	}
	if tk["default"] != 5 {
		t.Fatalf("top_k default = %v", tk["default"])
	}
	req := spec["input_schema"].(map[string]any)["required"].([]string)
	if len(req) != 1 || req[0] != "query" {
		t.Fatalf("required = %v", req)
	}
}

// TestExplicitNameAndRequires mirrors test_explicit_name_and_requires.
func TestExplicitNameAndRequires(t *testing.T) {
	create := Tool("tickets.create",
		func(ctx context.Context, in map[string]any) (any, error) { return "ok", nil },
		Requires("acl"),
		Param("title", "string", true, nil),
	)
	if create.Name != "tickets.create" {
		t.Fatalf("name = %q", create.Name)
	}
	if len(create.Requires) != 1 || create.Requires[0] != "acl" {
		t.Fatalf("requires = %v", create.Requires)
	}
}

// TestInvokeAsyncAndSync mirrors test_invoke_async_and_sync (Go has no async; a
// function returns (any, error) directly).
func TestInvokeAsyncAndSync(t *testing.T) {
	a := Tool("a", func(ctx context.Context, in map[string]any) (any, error) {
		return in["x"].(int) + 1, nil
	}, Param("x", "integer", true, nil))
	b := Tool("b", func(ctx context.Context, in map[string]any) (any, error) {
		return in["x"].(int) * 2, nil
	}, Param("x", "integer", true, nil))
	out, err := a.Invoke(context.Background(), map[string]any{"x": 1})
	if err != nil || out != 2 {
		t.Fatalf("a.Invoke = %v, %v", out, err)
	}
	out, err = b.Invoke(context.Background(), map[string]any{"x": 3})
	if err != nil || out != 6 {
		t.Fatalf("b.Invoke = %v, %v", out, err)
	}
}

// TestRuntimeSpecsAndCall mirrors test_runtime_specs_and_call.
func TestRuntimeSpecsAndCall(t *testing.T) {
	search := Tool("search", func(ctx context.Context, in map[string]any) (any, error) {
		return "results for " + in["query"].(string), nil
	}, Param("query", "string", true, nil))
	rt := NewFunctionToolRuntime(search)
	specs := rt.GetToolSpecs()
	if specs[0]["name"] != "search" {
		t.Fatalf("spec name = %v", specs[0]["name"])
	}
	out, err := rt.CallTool(context.Background(), "search", map[string]any{"query": "x"}, nil, nil)
	if err != nil || out != "results for x" {
		t.Fatalf("call = %q, %v", out, err)
	}
}

// TestRuntimeUnknownTool mirrors test_runtime_unknown_tool.
func TestRuntimeUnknownTool(t *testing.T) {
	rt := NewFunctionToolRuntime()
	out, _ := rt.CallTool(context.Background(), "nope", nil, nil, nil)
	if !contains(out, "unknown tool") {
		t.Fatalf("out = %q", out)
	}
}

// TestRuntimeStringifiesNonStrReturn mirrors test_runtime_stringifies_non_str_return.
func TestRuntimeStringifiesNonStrReturn(t *testing.T) {
	nums := Tool("nums", func(ctx context.Context, in map[string]any) (any, error) {
		return []int{1, 2, 3}, nil
	})
	rt := NewFunctionToolRuntime(nums)
	out, _ := rt.CallTool(context.Background(), "nums", nil, nil, nil)
	if out != "[1,2,3]" {
		t.Fatalf("out = %q", out)
	}
}

// TestRuntimeToolErrorIsSurfaced mirrors test_runtime_tool_error_is_surfaced.
func TestRuntimeToolErrorIsSurfaced(t *testing.T) {
	boom := Tool("boom", func(ctx context.Context, in map[string]any) (any, error) {
		return nil, errors.New("kaboom")
	})
	rt := NewFunctionToolRuntime(boom)
	out, _ := rt.CallTool(context.Background(), "boom", nil, nil, nil)
	if !contains(out, "kaboom") {
		t.Fatalf("out = %q", out)
	}
}

// TestMissingRequiredArgReturnsCleanError mirrors
// test_missing_required_arg_returns_clean_error.
func TestMissingRequiredArgReturnsCleanError(t *testing.T) {
	write := Tool("Write", func(ctx context.Context, in map[string]any) (any, error) {
		return "wrote " + in["file_path"].(string), nil
	}, Param("file_path", "string", true, nil), Param("content", "string", true, nil))
	rt := NewFunctionToolRuntime(write)
	out, _ := rt.CallTool(context.Background(), "Write", map[string]any{"content": "x"}, nil, nil)
	if !contains(out, "requires argument") || !contains(out, "'file_path'") {
		t.Fatalf("out = %q", out)
	}
	out, _ = rt.CallTool(context.Background(), "Write", map[string]any{"file_path": "A.md", "content": "x"}, nil, nil)
	if out != "wrote A.md" {
		t.Fatalf("well-formed call = %q", out)
	}
}

// TestMissingRequiredListsAllAbsentArgs mirrors
// test_missing_required_lists_all_absent_args.
func TestMissingRequiredListsAllAbsentArgs(t *testing.T) {
	write := Tool("Write", func(ctx context.Context, in map[string]any) (any, error) { return "ok", nil },
		Param("file_path", "string", true, nil), Param("content", "string", true, nil))
	if got := write.MissingRequired(nil); !eqStrs(got, []string{"file_path", "content"}) {
		t.Fatalf("missing(nil) = %v", got)
	}
	if got := write.MissingRequired(map[string]any{"file_path": "A"}); !eqStrs(got, []string{"content"}) {
		t.Fatalf("missing = %v", got)
	}
	if got := write.MissingRequired(map[string]any{"file_path": "A", "content": "x"}); len(got) != 0 {
		t.Fatalf("missing = %v", got)
	}
}

// TestOptionalAnnotation mirrors test_optional_annotation: an optional param has
// a type but is not required.
func TestOptionalAnnotation(t *testing.T) {
	f := Tool("f", func(ctx context.Context, in map[string]any) (any, error) { return "", nil },
		Param("x", "string", false, nil))
	props := f.InputSchema["properties"].(map[string]any)
	if props["x"].(map[string]any)["type"] != "string" {
		t.Fatalf("x type = %v", props["x"])
	}
	if req := f.InputSchema["required"].([]string); len(req) != 0 {
		t.Fatalf("required = %v", req)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func eqStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
