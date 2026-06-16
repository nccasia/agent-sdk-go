// PluginRegistry tests — the control surface for plugins (enable /
// disable / override / accept a registry in PreactAgent).
//
// Mirrors tests/test_plugin_registry.py from the Python port.
package plugins

import (
	"strings"
	"testing"

	"github.com/nccasia/agent-sdk-go/agent_sdk/agent"
	"github.com/nccasia/agent-sdk-go/agent_sdk/clients"
)

func lobesSet(ag *agent.PreactAgent) map[string]struct{} {
	out := map[string]struct{}{}
	for _, lb := range ag.Engine().Lobes {
		out[lb.ID] = struct{}{}
	}
	return out
}

func TestRegisterEnableDisable(t *testing.T) {
	p := NewPluginSupportTriage()
	reg := NewPluginRegistry(p)
	active := reg.Active()
	if len(active) != 1 || active[0] != p {
		t.Fatalf("expected 1 active = support_triage, got %v", active)
	}
	if !reg.IsEnabled("support_triage") {
		t.Fatalf("expected support_triage enabled")
	}
	reg.Disable("support_triage")
	if len(reg.Active()) != 0 || reg.IsEnabled("support_triage") {
		t.Fatalf("expected support_triage disabled")
	}
	reg.Enable("support_triage")
	active = reg.Active()
	if len(active) != 1 || active[0] != p {
		t.Fatalf("expected support_triage re-enabled, got %v", active)
	}
}

type otherTriage struct{ name string }

func (o *otherTriage) Name() string          { return o.name }
func (o *otherTriage) Install(s *AgentSetup) {}

func TestOverrideByName(t *testing.T) {
	reg := NewPluginRegistry(NewPluginSupportTriage())
	other := &otherTriage{name: "support_triage"}
	reg.Override(other) // same name → replaces in place
	if reg.Get("support_triage") == nil || reg.Get("support_triage") != Plugin(other) {
		t.Fatalf("expected override to take effect, got %v", reg.Get("support_triage"))
	}
	names := reg.Names()
	if len(names) != 1 || names[0] != "support_triage" {
		t.Fatalf("expected names=[support_triage], got %v", names)
	}
}

func TestPreactAgentAcceptsARegistry(t *testing.T) {
	reg := NewPluginRegistry(NewPluginSupportTriage())
	ag, err := agent.NewPreactAgent(agent.Config{
		Client:          clients.NewFakeClient(nil, nil),
		Plugins:         reg,
		UniversalMemory: false,
	})
	if err != nil {
		t.Fatalf("NewPreactAgent: %v", err)
	}
	if _, ok := lobesSet(ag)["triage"]; !ok {
		t.Fatalf("expected `triage` lobe in agent (registry was applied)")
	}
}

func TestDisabledPluginInRegistryIsNotApplied(t *testing.T) {
	reg := NewPluginRegistry(NewPluginSupportTriage())
	reg.Disable("support_triage")
	ag, err := agent.NewPreactAgent(agent.Config{
		Client:          clients.NewFakeClient(nil, nil),
		Plugins:         reg,
		UniversalMemory: false,
	})
	if err != nil {
		t.Fatalf("NewPreactAgent: %v", err)
	}
	if _, ok := lobesSet(ag)["triage"]; ok {
		t.Fatalf("expected `triage` lobe NOT present (plugin was disabled)")
	}
}

func TestBuiltinRegistrySeedsInfraPlugins(t *testing.T) {
	reg := BuiltinRegistry()
	names := reg.Names()
	got := map[string]bool{}
	for _, n := range names {
		got[n] = true
	}
	if !got["otel"] {
		t.Fatalf("expected otel in builtin registry, got %v", names)
	}
	if !got["guardrails"] {
		t.Fatalf("expected guardrails in builtin registry, got %v", names)
	}
	if !reg.IsEnabled("otel") {
		t.Fatalf("expected otel enabled")
	}
	if !reg.IsEnabled("guardrails") {
		t.Fatalf("expected guardrails enabled")
	}
}

// Suppress unused-import warnings for strings if no test uses it.
var _ = strings.Join
