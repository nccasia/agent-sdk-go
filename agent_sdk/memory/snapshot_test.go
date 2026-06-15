package memory

import (
	"strings"
	"testing"
)

func populate() *MemoryStore {
	s := NewMemoryStore()
	s.Remember("tool_result", "ephemeral search output", RememberOpts{Scope: FlashScope})
	s.Remember("fact", "the deploy window is Thursday 14:00 UTC", RememberOpts{Scope: "conversation", Key: "deploy-window"})
	big := "# Plan\n\nROOT CAUSE in src/export/csv.py line 142\n\n" + strings.Repeat("detail line\n", 400)
	s.Remember("artifact", big, RememberOpts{Scope: "conversation", Key: "plan-doc"})
	return s
}

func TestLongTermRoundtripsFlashDropped(t *testing.T) {
	s := populate()
	blob := s.ToJSON(SnapshotOpts{})
	restored := MemoryStoreFromJSON(blob, nil)
	if restored.Stats()["flash"] != 0 {
		t.Fatal("flash must be dropped")
	}
	if restored.Stats()["long_term"] != s.Stats()["long_term"] {
		t.Fatalf("long-term mismatch: %d vs %d", restored.Stats()["long_term"], s.Stats()["long_term"])
	}
	fact := restored.Get("mem://fact/conversation/deploy-window")
	if fact == nil {
		t.Fatal("fact missing")
	}
	if fact.Body != "the deploy window is Thursday 14:00 UTC" {
		t.Fatalf("body: %q", fact.Body)
	}
	if fact.Scope != "conversation" {
		t.Fatalf("scope: %q", fact.Scope)
	}
}

func TestOffloadedBodyRefetchableAfterRestore(t *testing.T) {
	s := populate()
	handle := "mem://artifact/conversation/plan-doc"
	if !s.Get(handle).Offloaded {
		t.Fatal("should be offloaded before snapshot")
	}
	restored := MemoryStoreFromJSON(s.ToJSON(SnapshotOpts{}), nil)
	e := restored.Get(handle)
	if e == nil || !e.Offloaded {
		t.Fatal("offloaded entry must survive restore")
	}
	matches := restored.Grep(handle, "ROOT CAUSE", 50)
	if len(matches) == 0 {
		t.Fatal("offloaded body must survive snapshot/restore")
	}
	if !strings.Contains(matches[0]["line"].(string), "csv.py") {
		t.Fatalf("match: %v", matches[0])
	}
	if !strings.Contains(restored.Read(handle), "ROOT CAUSE") {
		t.Fatal("full read after restore")
	}
}

func TestRecallOrderAndSeqPreserved(t *testing.T) {
	s := populate()
	blob := s.ToJSON(SnapshotOpts{})
	restored := MemoryStoreFromJSON(blob, nil)
	if restored.Seq() < blob.Seq {
		t.Fatalf("seq should advance: %d < %d", restored.Seq(), blob.Seq)
	}
	before := restored.Stats()["long_term"]
	restored.Remember("note", "a brand new note", RememberOpts{Scope: "conversation", Key: "new"})
	if restored.Stats()["long_term"] != before+1 {
		t.Fatal("new write should add one")
	}
	found := restored.Recall(RecallOpts{Query: "deploy window"})
	if !anyHandle(found, "mem://fact/conversation/deploy-window") {
		t.Fatal("recall after restore")
	}
}

func TestEstablishedFactSurvivesSnapshot(t *testing.T) {
	s := NewMemoryStore()
	s.Remember("fact", "user prefers metric units", RememberOpts{Scope: "conversation", Key: "units"})
	restored := MemoryStoreFromJSON(s.ToJSON(SnapshotOpts{}), nil)
	e := restored.Get("mem://fact/conversation/units")
	if e == nil || e.Body != "user prefers metric units" {
		t.Fatal("established fact should survive")
	}
}

func TestSnapshotBoundKeepsPinnedDropsOverflow(t *testing.T) {
	s := NewMemoryStore()
	s.Remember("decision", "pinned decision", RememberOpts{Scope: "conversation", Key: "keep", Pinned: true})
	for i := 0; i < 10; i++ {
		s.Remember("note", "note "+itoa(i), RememberOpts{Scope: "conversation", Key: "n" + itoa(i)})
	}
	blob := s.ToJSON(SnapshotOpts{MaxEntries: 3})
	if len(blob.Long) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(blob.Long))
	}
	handles := map[string]struct{}{}
	for _, e := range blob.Long {
		handles[e["handle"].(string)] = struct{}{}
	}
	if _, ok := handles["mem://decision/conversation/keep"]; !ok {
		t.Fatal("pinned must always survive")
	}
}

func TestResetClearsAll(t *testing.T) {
	s := populate()
	s.Reset()
	st := s.Stats()
	if st["flash"] != 0 || st["long_term"] != 0 || st["flash_tokens"] != 0 || st["long_term_tokens"] != 0 {
		t.Fatalf("reset should clear all: %v", st)
	}
}
