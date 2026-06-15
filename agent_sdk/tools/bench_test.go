package tools

import (
	"context"
	"testing"
)

func BenchmarkSelect(b *testing.B) {
	lobe := ToolSelectLobe{}
	specs := selTools()
	essential := func(name string) bool { return name == "memory" }
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = lobe.Select(specs, "search the knowledge base for facts", nil, nil, essential, nil, 0.01, 3)
	}
}

func BenchmarkCallTool(b *testing.B) {
	search := Tool("search", func(ctx context.Context, in map[string]any) (any, error) {
		return "results for " + in["query"].(string), nil
	}, Param("query", "string", true, nil))
	rt := NewFunctionToolRuntime(search)
	in := map[string]any{"query": "x"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = rt.CallTool(context.Background(), "search", in, nil, nil)
	}
}
