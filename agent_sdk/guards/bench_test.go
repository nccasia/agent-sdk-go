package guards

import "testing"

// AnswerLeakViolation runs on every produced answer (the post-filter hot path).
func BenchmarkAnswerLeakViolation(b *testing.B) {
	answer := "The capital of France is Paris, and revenue grew across all three regions last quarter."
	forbidden := []string{"internal-only", "do-not-share"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = AnswerLeakViolation(answer, forbidden, 0)
	}
}

// GoldenHit runs once per turn over the golden head (cosine scan).
func BenchmarkGoldenHit(b *testing.B) {
	items := make([]GoldenItem, 64)
	vecs := make([]Vec, 64)
	for i := range items {
		items[i] = GoldenItem{CaseID: "g", Query: "q", ExpectedBehavior: "a"}
		vecs[i] = Vec{float32(i) / 64, 1 - float32(i)/64}
	}
	h := NewGoldenHead(items, vecs, "m")
	q := Vec{0.5, 0.5}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GoldenHit(q, h, 0.86)
	}
}
