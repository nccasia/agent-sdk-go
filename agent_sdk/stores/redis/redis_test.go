// Translated from the Redis store contract in agent_sdk/stores/session.py:
// Load (empty when absent), Save (whole-state JSON), Append (load + mutate +
// save), Compact (load + compact + save). The Go port speaks to an embedded
// fakeredis or any go-redis-compatible client.
//
// Tests use an in-process fake client (map-backed) so they run without a
// Redis server — the production path is exercised by plugging a real
// go-redis client into NewStoreWithClient.
package redis

import (
	"context"
	"testing"

	"github.com/nccasia/agent-sdk-go/agent_sdk/session"
)

// fakeClient is a stand-in for go-redis that satisfies the small interface
// this package uses (GET / SET / DEL on string keys). It lets the tests run
// hermetically, no network.
type fakeClient struct {
	data map[string]string
}

func newFakeClient() *fakeClient { return &fakeClient{data: map[string]string{}} }

func (f *fakeClient) Get(_ context.Context, key string) (string, bool, error) {
	v, ok := f.data[key]
	return v, ok, nil
}
func (f *fakeClient) Set(_ context.Context, key, value string) error {
	f.data[key] = value
	return nil
}
func (f *fakeClient) Del(_ context.Context, key string) (bool, error) {
	if _, ok := f.data[key]; ok {
		delete(f.data, key)
		return true, nil
	}
	return false, nil
}

func TestStoreLoadEmptyWhenAbsent(t *testing.T) {
	fc := newFakeClient()
	s := NewStoreWithClient(fc, "session:")
	st, err := s.Load(context.Background(), "absent")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(st.History) != 0 || st.Summary != "" {
		t.Errorf("empty state expected, got %+v", st)
	}
}

func TestStoreSavePersistsWholeState(t *testing.T) {
	fc := newFakeClient()
	s := NewStoreWithClient(fc, "session:")
	state := session.SessionState{
		History:     []session.Turn{{Role: "user", Content: "q"}, {Role: "assistant", Content: "a"}},
		Summary:     "sum",
		SkillsInUse: []string{"k"},
		Memory:      map[string]any{"seq": 7, "long": []any{}},
	}
	if err := s.Save(context.Background(), "s1", state); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := s.Load(context.Background(), "s1")
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

func TestStoreAppendLoadsMutatesAndSaves(t *testing.T) {
	fc := newFakeClient()
	s := NewStoreWithClient(fc, "session:")
	ctx := context.Background()
	if err := s.Append(ctx, "s1", session.Turn{Role: "user", Content: "hi"}); err != nil {
		t.Fatalf("append 1: %v", err)
	}
	if err := s.Append(ctx, "s1", session.Turn{Role: "assistant", Content: "hello"}); err != nil {
		t.Fatalf("append 2: %v", err)
	}
	st, err := s.Load(ctx, "s1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(st.History) != 2 {
		t.Errorf("history len = %d, want 2", len(st.History))
	}
}

func TestStoreCompactFoldsOlderTurnsIntoSummary(t *testing.T) {
	fc := newFakeClient()
	s := NewStoreWithClient(fc, "session:")
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
