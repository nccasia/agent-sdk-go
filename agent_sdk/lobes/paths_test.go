// Path-recognizer parity: each named recognizer reproduces Python's exact score
// (agent_sdk/paths/*). Golden values exported from the Python SDK.
package lobes

import "testing"

func recognizerByName(name string) func(map[string]any) float64 {
	for _, p := range ProductionPaths() {
		if p.Name == name {
			return p.Recognizer
		}
	}
	return nil
}

func TestRecognizerGoldenScores(t *testing.T) {
	cases := []struct {
		name string
		ctx  map[string]any
		want map[string]float64
	}{
		{"qna", map[string]any{"query": "what is the capital of France?"},
			map[string]float64{"qna": 0.8, "research": 0.0, "clarify": 0.0, "relational": 0.0, "onboarding": 0.0}},
		{"qna_zero_comparative", map[string]any{"query": "compare a vs b"},
			map[string]float64{"qna": 0.0, "research": 0.7}},
		{"research", map[string]any{"query": "compare and contrast microservices versus monoliths in great detail across the team and every dimension"},
			map[string]float64{"research": 1.0, "qna": 0.0}},
		{"research_reminder_zero", map[string]any{"query": "schedule a summary versus report every morning"},
			map[string]float64{"research": 0.0, "qna": 0.0}},
		{"clarify", map[string]any{"query": "what about that?", "has_history": true, "prev_path": "qna"},
			map[string]float64{"clarify": 1.0, "qna": 0.8}},
		{"clarify_no_history", map[string]any{"query": "what about that?"},
			map[string]float64{"clarify": 0.0}},
		{"relational_greeting", map[string]any{"query": "hello there"},
			map[string]float64{"relational": 0.9}},
		{"relational_long_question", map[string]any{"query": "hi can you tell me what is the meaning of life in detail please"},
			map[string]float64{"relational": 0.0, "qna": 0.8}},
		{"onboarding", map[string]any{"query": "onboarding", "config_mode": true},
			map[string]float64{"onboarding": 1.0}},
		{"onboarding_no_flag", map[string]any{"query": "onboarding"},
			map[string]float64{"onboarding": 0.0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for path, want := range c.want {
				fn := recognizerByName(path)
				if fn == nil {
					t.Fatalf("no recognizer %q", path)
				}
				got := round4(fn(c.ctx))
				if got != want {
					t.Errorf("%s(%q) = %v, want %v", path, c.ctx["query"], got, want)
				}
			}
		})
	}
}

func round4(v float64) float64 {
	return float64(int64(v*1e4+0.5)) / 1e4
}

func TestIsRecurringSchedule(t *testing.T) {
	if !isRecurringSchedule("tổng hợp tin tức mỗi tối lúc 21h") {
		t.Error("recurring cadence+clock should be a schedule")
	}
	if isRecurringSchedule("compare a vs b") {
		t.Error("plain comparison is not a schedule")
	}
}

func BenchmarkRecognizeResearch(b *testing.B) {
	ctx := map[string]any{"query": "compare and contrast microservices versus monoliths in great detail across the team"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = recognizeResearch(ctx)
	}
}
