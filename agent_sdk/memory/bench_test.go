package memory

import (
	"strings"
	"testing"
)

func BenchmarkDeterministicDigest(b *testing.B) {
	body := "DEADLINE: 2026-07-15 owner @lan in src/plan.py\n" +
		strings.Repeat("routine status chatter, nothing decision-relevant here. ", 100)
	meta := map[string]any{"tool": "read_file"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = DeterministicDigest("tool_result", meta, body, 240, 3)
	}
}

func BenchmarkRememberRecall(b *testing.B) {
	s := NewMemoryStore()
	for i := 0; i < 200; i++ {
		s.Remember("tool_result", "result "+itoaInt(i)+" about deploy window thursday", RememberOpts{Meta: map[string]any{"tool": "fetch"}})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.Recall(RecallOpts{Query: "deploy window", K: 8})
	}
}

func BenchmarkTier(b *testing.B) {
	s := NewMemoryStore()
	var entries []*MemoryEntry
	for i := 0; i < 64; i++ {
		h := s.Remember("tool_result", "observation "+itoaInt(i)+" about deploy and rollout", RememberOpts{Meta: map[string]any{"tool": "fetch"}})
		entries = append(entries, s.Get(h))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Tier(entries, "deploy rollout", 4000, 0.30, 0.12)
	}
}
