package react

import (
	"strings"
	"testing"
)

func BenchmarkTierObservations(b *testing.B) {
	var msgs []map[string]any
	for i := 0; i < 40; i++ {
		text := strings.Repeat("observation number x with some detail ", 8)
		msgs = append(msgs, obsBench("t"+itoaInt(i), text)...)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TierObservations(msgs, TierOpts{Hop: 40, KeepLastFull: 2, KeepErrorsFull: true})
	}
}

func BenchmarkScoreObservations(b *testing.B) {
	var msgs []map[string]any
	for i := 0; i < 40; i++ {
		text := strings.Repeat("deployment release production rollout step ", 6)
		msgs = append(msgs, obsBench("t"+itoaInt(i), text)...)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ScoreObservations(msgs, "deploy the release to production", nil, nil, 4)
	}
}

func obsBench(tid, text string) []map[string]any {
	return []map[string]any{
		{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": tid, "name": "search", "input": map[string]any{"q": "x"}}}},
		{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": tid, "content": text}}},
	}
}
