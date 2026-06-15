package memory

import (
	"context"
	"testing"

	"github.com/mezon/agent-sdk-go/agent_sdk/session"
)

func summarizer(turns []session.Turn) (string, error) {
	return "[earlier turns]", nil
}

// ── SessionStore ─────────────────────────────────────────────────────────────
func TestSessionAppendAndLoadInMemory(t *testing.T) {
	ctx := context.Background()
	s := session.New("conv-1", NewSessionStoreInMemory())
	if err := s.Append(ctx, session.Turn{Role: "user", Content: "hello"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(ctx, session.Turn{Role: "assistant", Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	state, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.History) != 2 || state.History[0].Content != "hello" || state.History[1].Content != "hi" {
		t.Errorf("history = %+v", state.History)
	}
	msgs := state.Messages(1, 6, 2000)
	if msgs[0]["role"] != "user" {
		t.Errorf("msgs[0] = %v", msgs[0])
	}
}

func TestSessionCompactRollsIntoSummary(t *testing.T) {
	ctx := context.Background()
	s := session.New("c", NewSessionStoreInMemory())
	for i := 0; i < 10; i++ {
		if err := s.Append(ctx, session.Turn{Role: "user", Content: "m"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Compact(ctx, summarizer, 3); err != nil {
		t.Fatal(err)
	}
	state, _ := s.Load(ctx)
	if len(state.History) != 3 {
		t.Errorf("history len = %d", len(state.History))
	}
	if state.Summary == "" || !contains(state.Summary, "earlier turns") {
		t.Errorf("summary = %q", state.Summary)
	}
}

func TestSessionDefaultStoreIsInMemory(t *testing.T) {
	s := session.New("x", nil)
	if s.Store == nil {
		t.Fatal("default store is nil")
	}
}

func TestSessionSQLStoreRoundtrip(t *testing.T) {
	ctx := context.Background()
	store, err := NewSessionStoreSQL(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(ctx, "c1", session.Turn{Role: "user", Content: "persisted"}); err != nil {
		t.Fatal(err)
	}
	state, err := store.Load(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.History) == 0 || state.History[0].Content != "persisted" {
		t.Errorf("history = %+v", state.History)
	}
}

func TestSessionStoreSaveWholeState(t *testing.T) {
	ctx := context.Background()
	store := NewSessionStoreInMemory()
	st := session.SessionState{
		History: []session.Turn{{Role: "user", Content: "Remember"}},
		Memory:  map[string]any{"long": []any{map[string]any{"body": "the window is thursday"}}},
	}
	if err := store.Save(ctx, "s1", st); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Load(ctx, "s1")
	long, _ := got.Memory["long"].([]any)
	if len(long) == 0 {
		t.Errorf("memory not persisted: %v", got.Memory)
	}
	if len(got.History) != 1 {
		t.Errorf("history = %+v", got.History)
	}
}

// ── MemoryStore ──────────────────────────────────────────────────────────────
func TestMemoryStoreWriteReadSearchForget(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStoreInMemory()
	if err := st.Write(ctx, "user", "deploy_day", "Friday"); err != nil {
		t.Fatal(err)
	}
	v, _ := st.Read(ctx, "user", "deploy_day")
	if v != "Friday" {
		t.Errorf("read = %v", v)
	}
	items, _ := st.Search(ctx, "user", "deploy", 5)
	if len(items) == 0 || items[0].Key != "deploy_day" {
		t.Errorf("search = %+v", items)
	}
	if items[0].Scope != "user" {
		t.Errorf("scope = %q", items[0].Scope)
	}
	ok, _ := st.Forget(ctx, "user", "deploy_day")
	if !ok {
		t.Error("forget returned false")
	}
	v2, _ := st.Read(ctx, "user", "deploy_day")
	if v2 != nil {
		t.Errorf("read after forget = %v", v2)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
