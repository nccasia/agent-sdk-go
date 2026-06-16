package memory

import (
	"context"
	"strings"
	"testing"

	storemem "github.com/nccasia/agent-sdk-go/agent_sdk/stores/memory"
)

func seededMemory(t *testing.T) *Memory {
	t.Helper()
	m := NewMemory(storemem.NewMemoryStoreInMemory(), []string{"bot", "user", "channel", "conversation"})
	ctx := context.Background()
	if err := m.Write(ctx, "channel", "deploy_freeze", "deploy freeze until Monday June 15"); err != nil {
		t.Fatal(err)
	}
	if err := m.Write(ctx, "user", "review_language", "write code reviews in Vietnamese"); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestHookReturnsScopeOrderedItems(t *testing.T) {
	m := seededMemory(t)
	hook := MemoryPrefetchHook(m, PrefetchOpts{K: 5})
	items, err := hook(context.Background(), "when is the deploy freeze", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Fatal("matching facts should be prefetched")
	}
	foundDeploy := false
	for _, i := range items {
		if i["key"] == "deploy_freeze" && strings.Contains(toStr(i["value"]), "Monday") {
			foundDeploy = true
		}
	}
	if !foundDeploy {
		t.Fatal("deploy_freeze fact should be prefetched")
	}
	order := map[string]int{"bot": 0, "user": 1, "channel": 2, "conversation": 3}
	prev := -1
	for _, i := range items {
		r, ok := order[toStr(i["scope"])]
		if !ok {
			r = 9
		}
		if r < prev {
			t.Fatalf("scope order broad→specific violated: %v", items)
		}
		prev = r
	}
}

func TestHookOverBudgetValueDegradesToHint(t *testing.T) {
	m := NewMemory(storemem.NewMemoryStoreInMemory(), []string{"bot"})
	if err := m.Write(context.Background(), "bot", "manual", "deploy "+strings.Repeat("x", 5000)); err != nil {
		t.Fatal(err)
	}
	hook := MemoryPrefetchHook(m, PrefetchOpts{ValueBudgetChars: 100})
	items, err := hook(context.Background(), "deploy", nil)
	if err != nil {
		t.Fatal(err)
	}
	var item map[string]any
	for _, i := range items {
		if i["key"] == "manual" {
			item = i
		}
	}
	if item == nil {
		t.Fatal("manual item not found")
	}
	if toStr(item["value"]) != "" {
		t.Fatalf("over-budget value should be cleared, got %q", item["value"])
	}
	if !strings.Contains(toStr(item["description"]), "recall to read") {
		t.Fatalf("should surface as a hint: %v", item)
	}
}

func TestHookEmptyStoreIsNoop(t *testing.T) {
	m := NewMemory(storemem.NewMemoryStoreInMemory(), []string{"bot", "user"})
	items, err := MemoryPrefetchHook(m, PrefetchOpts{})(context.Background(), "anything at all", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("empty store should be a no-op, got %v", items)
	}
}

func toStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
