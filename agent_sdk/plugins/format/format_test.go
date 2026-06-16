// Format plugin — basic contract: lobes = [format], install adds
// the lobe; BuildRequirements assembles the language/tone/voice/
// format/Mezon requirement lines.
package format

import (
	"strings"
	"testing"

	"github.com/mezon/agent-sdk-go/agent_sdk/agent"
)

func TestFormatPluginLobesIsFormatOnly(t *testing.T) {
	fp := NewFormatPlugin()
	lobes := fp.Lobes()
	if len(lobes) != 1 || lobes[0].ID != "format" {
		t.Fatalf("expected [format], got %+v", lobes)
	}
}

func TestFormatPluginInstallAddsLobe(t *testing.T) {
	setup := agent.NewAgentSetup()
	NewFormatPlugin().Install(setup)
	if len(setup.Lobes) != 1 || setup.Lobes[0].ID != "format" {
		t.Fatalf("expected setup.Lobes=[format], got %+v", setup.Lobes)
	}
}

func TestBuildRequirementsMezonDeployment(t *testing.T) {
	reqs := BuildRequirements(map[string]any{"language": "vi", "tone": "casual"}, "mezon")
	if !strings.Contains(reqs, "Vietnamese") {
		t.Fatalf("expected Vietnamese in requirements, got %q", reqs)
	}
	if !strings.Contains(reqs, "casual") {
		t.Fatalf("expected casual tone, got %q", reqs)
	}
	if !strings.Contains(reqs, "MEZON FORMAT") {
		t.Fatalf("expected MEZON FORMAT, got %q", reqs)
	}
}

func TestBuildRequirementsEnglishProfessionalDefault(t *testing.T) {
	reqs := BuildRequirements(map[string]any{}, "")
	if reqs != "" {
		t.Fatalf("expected empty requirements for default English/professional, got %q", reqs)
	}
}
