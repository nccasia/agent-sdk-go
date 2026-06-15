package react

import (
	"strings"
	"testing"
)

// obs builds one think→act→observe exchange (assistant tool_use + user tool_result).
func obs(tid, text string, name string, inp map[string]any, isError bool) []map[string]any {
	tr := map[string]any{"type": "tool_result", "tool_use_id": tid, "content": text}
	if isError {
		tr["is_error"] = true
	}
	if name == "" {
		name = "f"
	}
	if inp == nil {
		inp = map[string]any{}
	}
	return []map[string]any{
		{
			"role":    "assistant",
			"content": []any{map[string]any{"type": "tool_use", "id": tid, "name": name, "input": inp}},
		},
		{
			"role":    "user",
			"content": []any{tr},
		},
	}
}

func TestScoreObservationsValueBeatsRecency(t *testing.T) {
	msgs := append(
		obs("t1", "deployment release production rollout steps are prepared", "search", map[string]any{"q": "deploy release production"}, false),
		obs("t2", "sunny twenty five degrees clear sky weather forecast today", "weather", map[string]any{"q": "weather"}, false)...,
	)
	keep := ScoreObservations(msgs, "deploy the release to production", nil, nil, 1)
	if len(keep) != 1 {
		t.Fatalf("expected 1 id, got %v", keep)
	}
	if _, ok := keep["t1"]; !ok {
		t.Fatalf("value (on-goal) should beat recency: got %v", keep)
	}
}

func TestScoreObservationsKeepTopZero(t *testing.T) {
	msgs := obs("t1", "anything", "", nil, false)
	if got := ScoreObservations(msgs, "x", nil, nil, 0); len(got) != 0 {
		t.Fatalf("keep_top=0 should be empty, got %v", got)
	}
}

func TestPinnedObservationStaysFullAgainstRecency(t *testing.T) {
	msgs := obs("old", "CRITICAL the database password rotates at midnight UTC", "", nil, false)
	msgs = append(msgs, obs("a", strings.Repeat("x", 50), "", nil, false)...)
	msgs = append(msgs, obs("b", strings.Repeat("y", 50), "", nil, false)...)
	msgs = append(msgs, obs("c", strings.Repeat("z", 50), "", nil, false)...)
	out := TierObservations(msgs, TierOpts{Hop: 4, KeepLastFull: 1, KeepFullIDs: map[string]struct{}{"old": {}}, KeepErrorsFull: true})
	content := findToolResult(t, out, "old")
	if !strings.Contains(content, "CRITICAL the database password") {
		t.Fatalf("pinned observation should keep full body, got %q", content)
	}
}

func TestErrorObservationStaysFull(t *testing.T) {
	msgs := obs("err", "Traceback: ConnectionRefused on port 5432", "", nil, true)
	msgs = append(msgs, obs("a", strings.Repeat("x", 80), "", nil, false)...)
	msgs = append(msgs, obs("b", strings.Repeat("y", 80), "", nil, false)...)
	out := TierObservations(msgs, TierOpts{Hop: 3, KeepLastFull: 1, KeepErrorsFull: true})
	content := findToolResult(t, out, "err")
	if !strings.Contains(content, "Traceback") {
		t.Fatalf("error body should survive funnel, got %q", content)
	}
}

func TestBudgetCompactionBoundsTailVsRecencyOnly(t *testing.T) {
	var msgsR, msgsB []map[string]any
	for i := 0; i < 40; i++ {
		text := strings.Repeat("observation number x with some detail ", 8)
		msgsR = append(msgsR, obs("t"+itoa(i), text, "", nil, false)...)
		msgsB = append(msgsB, obs("t"+itoa(i), text, "", nil, false)...)
		msgsR = TierObservations(msgsR, TierOpts{Hop: i, KeepLastFull: 2, KeepErrorsFull: true})
		msgsB = TierObservations(msgsB, TierOpts{Hop: i, KeepLastFull: 2, KeepErrorsFull: true})
		if ObsTailTokens(msgsB) > 150 {
			msgsB = CompactObservations(msgsB, CompactOpts{KeepLastFull: 2, MaxSpent: 6, KeepErrorsFull: true})
		}
	}
	recencyTail := ObsTailTokens(msgsR)
	budgetedTail := ObsTailTokens(msgsB)
	if !(budgetedTail < recencyTail) {
		t.Fatalf("budget should bound the tail: budgeted=%d recency=%d", budgetedTail, recencyTail)
	}
	if budgetedTail > 600 {
		t.Fatalf("budgeted tail should stay near working-set budget, got %d", budgetedTail)
	}
}

// findToolResult finds the tool_result content string for a given tool_use_id.
func findToolResult(t *testing.T, msgs []map[string]any, tid string) string {
	t.Helper()
	for _, m := range msgs {
		content, _ := m["content"].([]any)
		for _, b := range content {
			bm, ok := b.(map[string]any)
			if !ok {
				continue
			}
			if s, _ := bm["tool_use_id"].(string); s == tid {
				c, _ := bm["content"].(string)
				return c
			}
		}
	}
	t.Fatalf("tool_use_id %q not found", tid)
	return ""
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		p--
		b[p] = '-'
	}
	return string(b[p:])
}
