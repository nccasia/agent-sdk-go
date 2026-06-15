package memory

import (
	"context"
	"strings"
	"testing"
)

func TestRememberRecallRoundtrip(t *testing.T) {
	s := NewMemoryStore()
	h := s.Remember("note", "the deploy window is Thursday 14:00 UTC", RememberOpts{Scope: FlashScope})
	if !strings.HasPrefix(h, "mem://note/turn/") {
		t.Fatalf("handle prefix: %q", h)
	}
	if got := s.Read(h); got != "the deploy window is Thursday 14:00 UTC" {
		t.Fatalf("read: %q", got)
	}
	if s.Get(h).Digest == "" {
		t.Fatal("a gist should have been produced")
	}
	found := s.Recall(RecallOpts{Query: "deploy window"})
	if !anyHandle(found, h) {
		t.Fatal("recall did not find the entry")
	}
}

func TestDigestPreservesNeedles(t *testing.T) {
	body := "ROOT CAUSE: the csv exporter held the full result set in memory.\n" +
		"fix in src/export/csv.py line 142\n" +
		"thanks everyone, nice work!"
	dg := DeterministicDigest("tool_result", map[string]any{"tool": "read_file"}, body, 240, 3)
	for _, needle := range []string{"src/export/csv.py", "142", "ROOT CAUSE"} {
		if !strings.Contains(dg, needle) {
			t.Fatalf("digest dropped needle %q: %q", needle, dg)
		}
	}
}

func TestDigestCompressesLargeBody(t *testing.T) {
	body := "DEADLINE: 2026-07-15 owner @lan in src/plan.py\n" +
		strings.Repeat("routine status chatter, nothing decision-relevant here. ", 200)
	dg := DeterministicDigest("note", map[string]any{}, body, 240, 3)
	if !strings.Contains(dg, "2026-07-15") || !strings.Contains(dg, "src/plan.py") {
		t.Fatalf("needles must survive: %q", dg)
	}
	if CompressionRatio(dg, body) >= 0.2 {
		t.Fatalf("bulk should be dropped, ratio=%v", CompressionRatio(dg, body))
	}
}

func TestLargeBodyOffloadsAndSlices(t *testing.T) {
	s := NewMemoryStore(WithLargeBodyChars(200))
	body := "# Overview\nsmall intro\n\n# Details\n" + strings.Repeat("detail line with data 42\n", 60)
	h := s.Remember("temp_file", body, RememberOpts{Scope: FlashScope, Meta: map[string]any{"path": "report.md"}})
	if !s.Get(h).Offloaded {
		t.Fatal("large body should offload")
	}
	if s.Read(h) != body {
		t.Fatal("full body still available")
	}
	hits := s.Grep(h, "data 42", 50)
	if len(hits) == 0 {
		t.Fatal("grep returns matching lines")
	}
	outline := s.Outline(h)
	foundDetails := false
	for _, sec := range outline {
		if sec["heading"] == "Details" {
			foundDetails = true
		}
	}
	if !foundDetails {
		t.Fatal("outline should contain Details section")
	}
	id, _ := outline[1]["id"].(string)
	section := s.ReadSection(h, id)
	if !strings.Contains(section, "detail line") {
		t.Fatalf("section read: %q", section)
	}
}

func TestTwoTiersAndPromote(t *testing.T) {
	s := NewMemoryStore()
	flash := s.Remember("fact", "user prefers dark mode", RememberOpts{Scope: FlashScope})
	if !s.Get(flash).IsFlash() {
		t.Fatal("flash entry should be flash")
	}
	promoted := s.Promote(flash, "user", "ui_pref")
	if !strings.HasPrefix(promoted, "mem://fact/user/") {
		t.Fatalf("promoted handle: %q", promoted)
	}
	if s.Get(promoted).IsFlash() {
		t.Fatal("promoted entry should not be flash")
	}
	s.ResetFlash()
	if s.Get(flash) != nil {
		t.Fatal("flash dropped at turn end")
	}
	if s.Read(promoted) != "user prefers dark mode" {
		t.Fatal("promoted long-term entry survives")
	}
}

func TestTieringPinsAndNoSilentDrop(t *testing.T) {
	s := NewMemoryStore()
	pin := s.Get(s.Remember("decision", "ship behind a flag", RememberOpts{Pinned: true}))
	rel := s.Get(s.Remember("fact", "the zephyr deadline is 2026-07-15", RememberOpts{}))
	junk := s.Get(s.Remember("tool_result", strings.Repeat("x ", 400), RememberOpts{Meta: map[string]any{"tool": "noise"}}))
	s.Tier([]*MemoryEntry{pin, rel, junk}, "zephyr deadline", 10000, 0.30, 0.12)
	if pin.Tier != 1 {
		t.Fatalf("pinned floors to tier 1, got %d", pin.Tier)
	}
	if rel.Tier != 1 {
		t.Fatalf("relevant+small → tier 1, got %d", rel.Tier)
	}
	if junk.Tier != 2 && junk.Tier != 3 {
		t.Fatalf("large+off-topic → tier 2/3, got %d", junk.Tier)
	}
}

func TestCompactionSummarizerOffloadsAndIsRefetchable(t *testing.T) {
	s := NewMemoryStore()
	summarize := s.CompactionSummarizer()
	raw := "DEPLOY WINDOW: Thursday 14:00 UTC owner @lan ticket OPS-1102 " + strings.Repeat("detail ", 50)
	digest := summarize("retrieve_kb", map[string]any{"q": "deploy"}, raw)
	if !strings.Contains(digest, "read('mem://tool_result/turn/") {
		t.Fatalf("digest should name the handle: %q", digest)
	}
	handle := strings.SplitN(strings.SplitN(digest, "read('", 2)[1], "')", 2)[0]
	if s.Read(handle) != raw {
		t.Fatal("offloaded body should be re-fetchable")
	}
}

func TestRenderIndexIsTheDiscoverableMenu(t *testing.T) {
	s := NewMemoryStore()
	s.Remember("decision", "ship behind the flag", RememberOpts{Scope: "conversation", Key: "ship"})
	for i := 0; i < 20; i++ {
		s.Remember("tool_result", "result "+itoa(i)+" about deploy", RememberOpts{Meta: map[string]any{"tool": "fetch"}})
	}
	idx := s.RenderIndex(RenderIndexOpts{BudgetTokens: 200, MaxPerKind: 5})
	if !strings.HasPrefix(idx, "## Memory") {
		t.Fatalf("index header: %q", idx)
	}
	if !strings.Contains(idx, "ship behind the flag") {
		t.Fatal("decision should be listed")
	}
	if !strings.Contains(idx, "recall(query") {
		t.Fatal("capped entries should be announced")
	}
	hits := s.Recall(RecallOpts{Query: "result 2 about deploy"})
	found := false
	for _, e := range hits {
		if strings.Contains(s.Read(e.Handle), "result 2") {
			found = true
		}
	}
	if !found {
		t.Fatal("capped entry still findable by search")
	}
}

func TestRecallToolReadAndWrite(t *testing.T) {
	s := NewMemoryStore()
	tool := NewRecallToolRuntime(s)
	out, _ := tool.CallTool(context.Background(), "note", map[string]any{"content": "use the funnel approach", "kind": "decision"})
	if !strings.Contains(out, "Noted decision") {
		t.Fatalf("note output: %q", out)
	}
	if len(tool.Writes) == 0 || tool.Writes[0]["kind"] != "decision" {
		t.Fatalf("writes: %v", tool.Writes)
	}
	found, _ := tool.CallTool(context.Background(), "recall", map[string]any{"query": "funnel approach"})
	if !strings.Contains(found, "decision") {
		t.Fatalf("recall: %q", found)
	}
	h := s.Remember("tool_result", "the full body here", RememberOpts{Meta: map[string]any{"tool": "fetch"}})
	body, _ := tool.CallTool(context.Background(), "recall", map[string]any{"handle": h, "full": true})
	if body != "the full body here" {
		t.Fatalf("full read: %q", body)
	}
}

func anyHandle(es []*MemoryEntry, h string) bool {
	for _, e := range es {
		if e.Handle == h {
			return true
		}
	}
	return false
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	return string(b[p:])
}
