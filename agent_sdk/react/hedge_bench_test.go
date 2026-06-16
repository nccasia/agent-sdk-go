// Micro-benchmarks for the hedge retry hot path (called once per turn by
// the engine's answer-retry seam).
package react

import "testing"

// BenchmarkMakeHedgeRetryDirectAnswer measures the no-retry path (the
// dominant case for a working agent).
func BenchmarkMakeHedgeRetryDirectAnswer(b *testing.B) {
	retry := MakeHedgeRetry()
	ans := "The deadline is March 1, per [c12]."
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = retry(ans)
	}
}

// BenchmarkMakeHedgeRetryHedge measures the hedge path.
func BenchmarkMakeHedgeRetryHedge(b *testing.B) {
	retry := MakeHedgeRetry()
	ans := "Sorry, I couldn't find specifics on that."
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = retry(ans)
	}
}
