// SupportTriage plugin — basic contract: install contributes
// lobe/stage/flow/path/skill/tool.
package support_triage

import (
	"testing"

	"github.com/nccasia/agent-sdk-go/agent_sdk/agent"
)

func TestSupportTriagePluginInstallContributes(t *testing.T) {
	setup := agent.NewAgentSetup()
	NewPluginSupportTriage().Install(setup)
	if len(setup.Lobes) != 1 || setup.Lobes[0].ID != "triage" {
		t.Fatalf("expected lobe=triage, got %+v", setup.Lobes)
	}
	if len(setup.Stages) != 1 {
		t.Fatalf("expected 1 stage, got %d", len(setup.Stages))
	}
	if len(setup.Flows) != 1 || setup.Flows[0].ID() != "triage" {
		t.Fatalf("expected flow=triage, got %+v", setup.Flows)
	}
	if len(setup.Paths) != 1 || setup.Paths[0].Name != "triage" {
		t.Fatalf("expected path=triage, got %+v", setup.Paths)
	}
	if len(setup.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(setup.Skills))
	}
	if len(setup.ToolRuntimes) != 1 {
		t.Fatalf("expected 1 tool runtime, got %d", len(setup.ToolRuntimes))
	}
}

func TestTriageRecognizerFiredPrompt(t *testing.T) {
	if got := TriageRecognizer(map[string]any{"fired_prompt": true}); got != 1.0 {
		t.Fatalf("expected 1.0, got %v", got)
	}
}

func TestLookupTicketTool(t *testing.T) {
	rt := NewLookupTicketToolRuntime()
	specs := rt.GetToolSpecs()
	if len(specs) != 1 || specs[0]["name"] != "lookup_ticket" {
		t.Fatalf("expected one `lookup_ticket` tool, got %v", specs)
	}
}
