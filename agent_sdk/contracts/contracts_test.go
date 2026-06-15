package contracts

import (
	"context"
	"reflect"
	"testing"
)

func TestPinnedLobes(t *testing.T) {
	if !IsPinned("cite") || !IsPinned("filter") {
		t.Error("cite and filter must be pinned")
	}
	if IsPinned("synthesize") {
		t.Error("synthesize must not be pinned")
	}
	if got := SortedPinnedLobes(); !reflect.DeepEqual(got, []string{"cite", "filter"}) {
		t.Errorf("SortedPinnedLobes = %v", got)
	}
}

func TestStripMemoryFooter(t *testing.T) {
	in := "Here is the answer.\n• Đã ghi nhớ: your timezone"
	if got := StripMemoryFooter(in); got != "Here is the answer." {
		t.Errorf("StripMemoryFooter = %q", got)
	}
	if got := StripMemoryFooter("no footer here"); got != "no footer here" {
		t.Errorf("StripMemoryFooter(plain) = %q", got)
	}
	if got := StripMemoryFooter(""); got != "" {
		t.Errorf("StripMemoryFooter(empty) = %q", got)
	}
}

type stubRuntime struct {
	specs []map[string]any
	calls map[string]string
}

func (s *stubRuntime) GetToolSpecs() []map[string]any { return s.specs }
func (s *stubRuntime) CallTool(_ context.Context, name string, _ map[string]any, _ []map[string]any, _ map[string]struct{}) (string, error) {
	return s.calls[name], nil
}

func TestCompositeToolRuntimeDedupAndDispatch(t *testing.T) {
	a := &stubRuntime{
		specs: []map[string]any{{"name": "search"}, {"name": "shared"}},
		calls: map[string]string{"search": "A:search", "shared": "A:shared"},
	}
	b := &stubRuntime{
		specs: []map[string]any{{"name": "shared"}, {"name": "fetch"}},
		calls: map[string]string{"shared": "B:shared", "fetch": "B:fetch"},
	}
	c := NewCompositeToolRuntime([]ToolRuntime{a, b})
	specs := c.GetToolSpecs()
	names := []string{}
	for _, s := range specs {
		names = append(names, s["name"].(string))
	}
	if !reflect.DeepEqual(names, []string{"search", "shared", "fetch"}) {
		t.Errorf("specs = %v", names)
	}
	// First owner of "shared" (a) wins.
	out, _ := c.CallTool(context.Background(), "shared", nil, nil, nil)
	if out != "A:shared" {
		t.Errorf("shared dispatch = %q", out)
	}
	out, _ = c.CallTool(context.Background(), "fetch", nil, nil, nil)
	if out != "B:fetch" {
		t.Errorf("fetch dispatch = %q", out)
	}
	out, _ = c.CallTool(context.Background(), "unknown", nil, nil, nil)
	if out != "Error: unknown tool 'unknown'. Use only the provided tools." {
		t.Errorf("unknown dispatch = %q", out)
	}
}

func TestNewTurnContextDefaults(t *testing.T) {
	tc := NewTurnContext("hi?")
	if tc.Query != "hi?" {
		t.Errorf("query = %q", tc.Query)
	}
	if tc.Policy == nil || tc.ActiveLobes == nil || tc.RetrievedChunks == nil || tc.AlreadyRead == nil {
		t.Error("default collections must be non-nil")
	}
}
