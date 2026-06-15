package tools

import "testing"

func selTools() []map[string]any {
	return []map[string]any{
		{"name": "search_kb", "description": "search the knowledge base for facts and documents"},
		{"name": "weather", "description": "current weather forecast for a city"},
		{"name": "calculator", "description": "evaluate an arithmetic expression"},
		{"name": "translate", "description": "translate text between languages"},
		{"name": "send_email", "description": "send an email message to a recipient"},
		{"name": "memory", "description": "recall durable memory"},
	}
}

// TestSelectDropsIrrelevantKeepsEssentialsAndRelevant mirrors
// test_adaptive_drops_irrelevant_keeps_essentials_and_relevant at the lobe level.
func TestSelectDropsIrrelevantKeepsEssentialsAndRelevant(t *testing.T) {
	lobe := ToolSelectLobe{}
	essential := func(name string) bool { return name == "memory" }
	out, tr := lobe.Select(selTools(), "search the knowledge base for facts", nil, nil,
		essential, nil, 0.01, 3)
	kept := map[string]bool{}
	for _, s := range out {
		kept[s["name"].(string)] = true
	}
	if !kept["memory"] {
		t.Fatalf("essential 'memory' dropped: %v", kept)
	}
	if !kept["search_kb"] {
		t.Fatalf("relevant 'search_kb' dropped: %v", kept)
	}
	if len(tr.Dropped) == 0 {
		t.Fatalf("expected an irrelevant tool to drop under the budget")
	}
	for _, d := range tr.Dropped {
		if kept[d.Name] {
			t.Fatalf("dropped %q also in kept payload", d.Name)
		}
	}
}

// TestSelectEssentialsNeverScored verifies the essentials guard keeps the
// allowlist regardless of relevance.
func TestSelectEssentialsNeverScored(t *testing.T) {
	lobe := ToolSelectLobe{}
	essential := func(name string) bool { return name == "send_email" || name == "memory" }
	out, _ := lobe.Select(selTools(), "search the knowledge base", nil, nil,
		essential, nil, 0.01, 2)
	kept := map[string]bool{}
	for _, s := range out {
		kept[s["name"].(string)] = true
	}
	if !kept["send_email"] || !kept["memory"] {
		t.Fatalf("essentials not all kept: %v", kept)
	}
}

// TestSelectActivationDarkByDefault mirrors the inert-by-default behavior.
func TestSelectActivationDarkByDefault(t *testing.T) {
	act := ToolSelectLobe{}.Lobe().Activation
	if act(map[string]any{}) != 0.0 {
		t.Fatalf("expected dark by default")
	}
	if act(map[string]any{"tool_strategy": "adaptive"}) != 1.0 {
		t.Fatalf("expected active under adaptive strategy")
	}
}
