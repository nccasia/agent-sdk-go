// Translated from the SQLite store contract in agent_sdk/stores/session.py:
// load (empty when absent), append (loads + mutates + saves), compact
// (loads + compacts in place + saves), save (whole-state atomic write), and
// a round-trip across processes via a file DSN. The Go port uses
// modernc.org/sqlite (pure-Go, no CGO).
package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nccasia/agent-sdk-go/agent_sdk/session"
)

func TestStoreLoadEmptyWhenAbsent(t *testing.T) {
	s, err := NewStore(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer s.Close()
	st, err := s.Load(context.Background(), "absent")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(st.History) != 0 || st.Summary != "" {
		t.Errorf("empty state expected, got %+v", st)
	}
}

func TestStoreAppendThenLoad(t *testing.T) {
	s, err := NewStore(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.Append(ctx, "s1", session.Turn{Role: "user", Content: "hi"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := s.Append(ctx, "s1", session.Turn{Role: "assistant", Content: "hello"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	st, err := s.Load(ctx, "s1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(st.History) != 2 {
		t.Errorf("history len = %d, want 2", len(st.History))
	}
	if st.History[0].Role != "user" || st.History[1].Role != "assistant" {
		t.Errorf("roles = %q/%q", st.History[0].Role, st.History[1].Role)
	}
}

func TestStoreSavePersistsWholeState(t *testing.T) {
	s, err := NewStore(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer s.Close()
	ctx := context.Background()
	state := session.SessionState{
		History:     []session.Turn{{Role: "user", Content: "q"}, {Role: "assistant", Content: "a"}},
		Summary:     "sum",
		SkillsInUse: []string{"k"},
		Memory:      map[string]any{"seq": 7, "long": []any{}},
	}
	if err := s.Save(ctx, "s1", state); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := s.Load(ctx, "s1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Summary != "sum" {
		t.Errorf("summary = %q, want 'sum'", got.Summary)
	}
	if len(got.SkillsInUse) != 1 || got.SkillsInUse[0] != "k" {
		t.Errorf("skills_in_use = %v, want [k]", got.SkillsInUse)
	}
	if len(got.History) != 2 {
		t.Errorf("history len = %d, want 2", len(got.History))
	}
	if v, _ := got.Memory["seq"].(int); v != 7 {
		if vf, ok := got.Memory["seq"].(float64); !ok || int(vf) != 7 {
			t.Errorf("memory.seq = %v (%T), want 7", got.Memory["seq"], got.Memory["seq"])
		}
	}
}

func TestStoreCompactFoldsOlderTurnsIntoSummary(t *testing.T) {
	s, err := NewStore(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer s.Close()
	ctx := context.Background()
	for i := 0; i < 8; i++ {
		if err := s.Append(ctx, "s1", session.Turn{Role: "user", Content: "u" + itoa(i)}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	sum := func(turns []session.Turn) (string, error) {
		out := ""
		for _, tn := range turns {
			out += tn.Content + " "
		}
		return "FOLDED:" + out, nil
	}
	if err := s.Compact(ctx, "s1", sum, 3); err != nil {
		t.Fatalf("compact: %v", err)
	}
	got, err := s.Load(ctx, "s1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Summary == "" || got.Summary[:7] != "FOLDED:" {
		t.Errorf("summary = %q, want FOLDED:…", got.Summary)
	}
	if len(got.History) != 3 {
		t.Errorf("history len = %d, want 3", len(got.History))
	}
}

func TestStoreFileDSNRoundtrip(t *testing.T) {
	dir := t.TempDir()
	dsn := filepath.Join(dir, "sessions.db")
	s1, err := NewStore(dsn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := s1.Save(context.Background(), "k", session.SessionState{Summary: "hello"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	s1.Close()

	s2, err := NewStore(dsn)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	got, err := s2.Load(context.Background(), "k")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Summary != "hello" {
		t.Errorf("summary = %q, want 'hello'", got.Summary)
	}
	_ = os.Stat
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	digits := []byte{}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}
