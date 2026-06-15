package events

import "testing"

func BenchmarkToolCallToJSON(b *testing.B) {
	ev := &ToolCall{ID: "1", Name: "search", Input: map[string]any{"q": "x", "k": 5}}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ev.ToJSON()
	}
}

func BenchmarkStamp(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Stamp(&RunStart{}, "trace-9")
	}
}
