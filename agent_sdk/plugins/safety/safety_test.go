// Safety plugin — basic contract: lobes = [filter], install adds
// the lobe to the AgentSetup.
package safety

import (
	"testing"

	"github.com/nccasia/agent-sdk-go/agent_sdk/agent"
)

func TestSafetyPluginLobesIsFilterOnly(t *testing.T) {
	sp := NewSafetyPlugin()
	lobes := sp.Lobes()
	if len(lobes) != 1 || lobes[0].ID != "filter" {
		t.Fatalf("expected [filter], got %+v", lobes)
	}
}

func TestSafetyPluginInstallAddsLobe(t *testing.T) {
	setup := agent.NewAgentSetup()
	NewSafetyPlugin().Install(setup)
	if len(setup.Lobes) != 1 || setup.Lobes[0].ID != "filter" {
		t.Fatalf("expected setup.Lobes=[filter], got %+v", setup.Lobes)
	}
}

func TestFilterLobeIsPinned(t *testing.T) {
	if !FilterLOBE.Pinned {
		t.Fatalf("expected filter lobe to be pinned")
	}
}
