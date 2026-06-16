package context

import (
	stdctx "context"
	"reflect"
	"strings"
	"testing"

	"github.com/nccasia/agent-sdk-go/agent_sdk/memory"
)

func ctxWithMemory() *AgentContext {
	return New(Options{Query: "hi", Memory: memory.NewMemory(nil, nil)})
}

func TestTurnScopeRoundtripsThroughScratchpad(t *testing.T) {
	bg := stdctx.Background()
	ctx := New(Options{})
	if err := ctx.Set(bg, "plan", []any{"a", "b"}, ScopeTurn); err != nil {
		t.Fatal(err)
	}
	got, _ := ctx.Get(bg, "plan", ScopeTurn, nil)
	if !reflect.DeepEqual(got, []any{"a", "b"}) {
		t.Fatalf("turn roundtrip: %v", got)
	}
	if !reflect.DeepEqual(ctx.Scratchpad().Get("plan", nil), []any{"a", "b"}) {
		t.Fatal("same scratchpad a tool reaches")
	}
	ok, _ := ctx.Delete(bg, "plan", ScopeTurn)
	if !ok {
		t.Fatal("delete should report existence")
	}
	gone, _ := ctx.Get(bg, "plan", ScopeTurn, "gone")
	if gone != "gone" {
		t.Fatalf("deleted key default: %v", gone)
	}
}

func TestTurnScopeIsTheDefault(t *testing.T) {
	bg := stdctx.Background()
	ctx := New(Options{})
	if err := ctx.Set(bg, "lang", "vi", ""); err != nil {
		t.Fatal(err)
	}
	got, _ := ctx.Get(bg, "lang", "", nil)
	if got != "vi" {
		t.Fatalf("default scope: %v", got)
	}
	if ctx.Scratchpad().Get("lang", nil) != "vi" {
		t.Fatal("default scope writes to scratchpad")
	}
}

func TestDurableScopesRoundtripThroughMemory(t *testing.T) {
	bg := stdctx.Background()
	for _, scope := range []Scope{ScopeConversation, ScopeChannel, ScopeUser, ScopeBot} {
		ctx := ctxWithMemory()
		if err := ctx.Set(bg, "ui_pref", "dark", scope); err != nil {
			t.Fatal(err)
		}
		got, _ := ctx.Get(bg, "ui_pref", scope, nil)
		if got != "dark" {
			t.Fatalf("scope %s roundtrip: %v", scope, got)
		}
		def, _ := ctx.Get(bg, "missing", scope, "d")
		if def != "d" {
			t.Fatalf("missing default: %v", def)
		}
		ok, _ := ctx.Delete(bg, "ui_pref", scope)
		if !ok {
			t.Fatal("delete should report existence")
		}
		gone, _ := ctx.Get(bg, "ui_pref", scope, nil)
		if gone != nil {
			t.Fatalf("deleted durable key: %v", gone)
		}
	}
}

func TestScopesAreIsolated(t *testing.T) {
	bg := stdctx.Background()
	ctx := ctxWithMemory()
	_ = ctx.Set(bg, "k", "turn-val", ScopeTurn)
	_ = ctx.Set(bg, "k", "user-val", ScopeUser)
	_ = ctx.Set(bg, "k", "bot-val", ScopeBot)
	if v, _ := ctx.Get(bg, "k", ScopeTurn, nil); v != "turn-val" {
		t.Fatalf("turn: %v", v)
	}
	if v, _ := ctx.Get(bg, "k", ScopeUser, nil); v != "user-val" {
		t.Fatalf("user: %v", v)
	}
	if v, _ := ctx.Get(bg, "k", ScopeBot, nil); v != "bot-val" {
		t.Fatalf("bot: %v", v)
	}
}

func TestScopeAcceptsPlainStrings(t *testing.T) {
	bg := stdctx.Background()
	ctx := ctxWithMemory()
	_ = ctx.Set(bg, "k", "v", CoerceScope("user"))
	if v, _ := ctx.Get(bg, "k", CoerceScope("user"), nil); v != "v" {
		t.Fatalf("plain string scope: %v", v)
	}
}

func TestSearchTurnScopeScansScratchpad(t *testing.T) {
	bg := stdctx.Background()
	ctx := New(Options{})
	_ = ctx.Set(bg, "deploy_window", "Thursday 14:00", "")
	_ = ctx.Set(bg, "owner", "lan", "")
	hits, _ := ctx.Search(bg, "deploy", ScopeTurn, 5)
	found := false
	for _, h := range hits {
		if h.Key == "deploy_window" {
			found = true
		}
	}
	if !found {
		t.Fatalf("scratchpad search: %v", hits)
	}
}

func TestSearchDurableScopeDelegatesToBackend(t *testing.T) {
	bg := stdctx.Background()
	ctx := ctxWithMemory()
	_ = ctx.Set(bg, "pref", "user prefers dark mode", ScopeUser)
	hits, _ := ctx.Search(bg, "dark mode", ScopeUser, 5)
	if len(hits) == 0 || hits[0].Key != "pref" {
		t.Fatalf("durable search: %v", hits)
	}
}

func TestDurableScopeWithoutMemoryRaises(t *testing.T) {
	bg := stdctx.Background()
	ctx := New(Options{})
	if ctx.HasDurable() {
		t.Fatal("no durable expected")
	}
	if _, err := ctx.Get(bg, "k", ScopeUser, nil); err == nil || !strings.Contains(err.Error(), "no durable Memory") {
		t.Fatalf("expected durable error, got %v", err)
	}
}

func TestScopeAllowlistIsEnforcedByMemory(t *testing.T) {
	bg := stdctx.Background()
	ctx := New(Options{Memory: memory.NewMemory(nil, []string{"conversation"})})
	if err := ctx.Set(bg, "k", "v", ScopeBot); err == nil || !strings.Contains(err.Error(), "not in allowed scopes") {
		t.Fatalf("expected allowlist error, got %v", err)
	}
}

func TestAmbientViewsAreExposed(t *testing.T) {
	ctx := New(Options{
		Query:    "who am i",
		Identity: map[string]any{"user_id": "u1", "tenant_id": "t1"},
		Channel:  map[string]any{"channel_id": "c1"},
		Session:  "SESSION_STATE",
	})
	if ctx.Query != "who am i" {
		t.Fatal("query")
	}
	if ctx.Identity()["user_id"] != "u1" {
		t.Fatal("identity")
	}
	if ctx.Channel()["channel_id"] != "c1" {
		t.Fatal("channel")
	}
	if ctx.Session() != "SESSION_STATE" {
		t.Fatal("session")
	}
}

func TestEvidenceDedupesByChunkID(t *testing.T) {
	ev := NewEvidence()
	if !ev.Add(map[string]any{"chunk_id": "a", "text": "x"}) {
		t.Fatal("first add new")
	}
	if ev.Add(map[string]any{"chunk_id": "a", "text": "x again"}) {
		t.Fatal("dupe should be false")
	}
	if !ev.Add(map[string]any{"chunk_id": "b", "text": "y"}) {
		t.Fatal("third add new")
	}
	if ev.Len() != 2 {
		t.Fatalf("len: %d", ev.Len())
	}
	if _, ok := ev.AlreadyRead["a"]; !ok {
		t.Fatal("a tracked")
	}
	if _, ok := ev.AlreadyRead["b"]; !ok {
		t.Fatal("b tracked")
	}
}

func TestFromTurnWrapsTurnState(t *testing.T) {
	chunks := []map[string]any{}
	seen := map[string]struct{}{}
	turn := Turn{
		Query:           "q",
		StageID:         "synthesize",
		ActivePath:      "qna",
		Identity:        map[string]any{"user_id": "u9"},
		Channel:         map[string]any{"channel_id": "c9"},
		SessionMemory:   "S",
		RetrievedChunks: chunks,
		AlreadyRead:     seen,
	}
	ctx := FromTurn(turn, nil)
	if ctx.Query != "q" || ctx.StageID != "synthesize" || ctx.Path != "qna" {
		t.Fatalf("turn fields: %+v", ctx)
	}
	if ctx.Identity()["user_id"] != "u9" || ctx.Session() != "S" {
		t.Fatal("ambient")
	}
	ctx.EvidenceCh().Add(map[string]any{"chunk_id": "z", "text": "t"})
	if len(ctx.EvidenceCh().RetrievedChunks) != 1 {
		t.Fatal("evidence shared via context")
	}
	if _, ok := seen["z"]; !ok {
		t.Fatal("shared already_read updated")
	}
}

func TestCurrentContextSeamBindsAndRestores(t *testing.T) {
	bg := stdctx.Background()
	if Current() != nil {
		t.Fatal("starts unbound")
	}
	ctx := New(Options{Query: "bound"})
	restore := Bind(ctx)
	if Current() != ctx {
		t.Fatal("bound")
	}
	_ = Current().Set(bg, "note", "from a tool", "")
	if ctx.Scratchpad().Get("note", nil) != "from a tool" {
		t.Fatal("tool reached shared state")
	}
	restore()
	if Current() != nil {
		t.Fatal("restored")
	}
}

func TestBindContextNests(t *testing.T) {
	outer := New(Options{Query: "outer"})
	inner := New(Options{Query: "inner"})
	rOuter := Bind(outer)
	if Current() != outer {
		t.Fatal("outer bound")
	}
	rInner := Bind(inner)
	if Current() != inner {
		t.Fatal("inner bound")
	}
	rInner()
	if Current() != outer {
		t.Fatal("inner restored")
	}
	rOuter()
}
