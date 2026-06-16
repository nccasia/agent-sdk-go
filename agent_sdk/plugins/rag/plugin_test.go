// RAG plugin — install + safety contract. Mirrors test_rag_plugin.py.
package rag

import (
	"testing"

	"github.com/mezon/agent-sdk-go/agent_sdk/agent"
	"github.com/mezon/agent-sdk-go/agent_sdk/clients"
)

// TestDefaultAgentHasSafetyNotGrounding mirrors test_default_agent_has_safety_not_grounding.
func TestDefaultAgentHasSafetyNotGrounding(t *testing.T) {
	ag, err := agent.NewPreactAgent(agent.Config{Client: clients.NewFakeClient(nil, nil), UniversalMemory: false})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]struct{}{}
	for _, lb := range ag.Engine().Lobes {
		ids[lb.ID] = struct{}{}
	}
	if _, ok := ids["filter"]; !ok {
		t.Fatalf("expected `filter` (safety) in default agent")
	}
	if _, ok := ids["cite"]; ok {
		t.Fatalf("expected `cite` (RAG) absent from default agent")
	}
	if len(ag.Engine().FinalizeHooks) != 0 {
		t.Fatalf("expected no finalize hooks in default agent, got %d", len(ag.Engine().FinalizeHooks))
	}
	if len(ag.Engine().ToolResultHooks) != 0 {
		t.Fatalf("expected no tool-result hooks in default agent, got %d", len(ag.Engine().ToolResultHooks))
	}
}

// TestRagPluginAddsCiteAndHooks mirrors test_rag_plugin_adds_cite_and_hooks.
func TestRagPluginAddsCiteAndHooks(t *testing.T) {
	ag, err := agent.NewPreactAgent(agent.Config{
		Client:          clients.NewFakeClient(nil, nil),
		Plugins:         []agent.Plugin{NewRagPlugin()},
		UniversalMemory: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]struct{}{}
	for _, lb := range ag.Engine().Lobes {
		ids[lb.ID] = struct{}{}
	}
	if _, ok := ids["cite"]; !ok {
		t.Fatalf("expected `cite` in agent with RagPlugin")
	}
	if len(ag.Engine().FinalizeHooks) != 1 {
		t.Fatalf("expected 1 finalize hook, got %d", len(ag.Engine().FinalizeHooks))
	}
	if len(ag.Engine().ToolResultHooks) != 1 {
		t.Fatalf("expected 1 tool-result hook, got %d", len(ag.Engine().ToolResultHooks))
	}
}

// TestRequireCitationsAutoEnablesRag mirrors test_require_citations_auto_enables_rag.
func TestRequireCitationsAutoEnablesRag(t *testing.T) {
	ag, err := agent.NewPreactAgent(agent.Config{
		Client:           clients.NewFakeClient(nil, nil),
		RequireCitations: true,
		UniversalMemory:  false,
	})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]struct{}{}
	for _, lb := range ag.Engine().Lobes {
		ids[lb.ID] = struct{}{}
	}
	if _, ok := ids["cite"]; !ok {
		t.Fatalf("expected `cite` in agent with require_citations=True")
	}
}

// TestSafetyPluginOwnsFilterOnly mirrors test_safety_plugin_owns_filter_only.
func TestSafetyPluginOwnsFilterOnly(t *testing.T) {
	// SafetyPlugin from the safety package — kept in this file as a
	// comment because the test enforces the contract; the safety
	// plugin is in a separate package.
	t.Skip("SafetyPlugin contract is exercised in agent_sdk/plugins/safety tests")
}
