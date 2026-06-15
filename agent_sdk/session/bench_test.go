package session

import "testing"

func BenchmarkMessagesShort(b *testing.B) {
	st := SessionState{History: []Turn{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = st.Messages(1, 6, 2000)
	}
}

func BenchmarkMessagesLong(b *testing.B) {
	var hist []Turn
	for i := 0; i < 40; i++ {
		hist = append(hist, Turn{Role: "user", Content: "a fairly long conversation turn body"})
	}
	st := SessionState{History: hist, Summary: "rolling summary"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = st.Messages(1, 6, 2000)
	}
}

func BenchmarkSessionStateRoundTrip(b *testing.B) {
	st := SessionState{
		History:     []Turn{{Role: "user", Content: "x"}, {Role: "assistant", Content: "y"}},
		Summary:     "s",
		SkillsInUse: []string{"k"},
		Memory:      map[string]any{"long": []any{}},
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = SessionStateFromJSON(st.ToJSON())
	}
}
