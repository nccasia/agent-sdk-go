package expression

import (
	"strings"
	"testing"

	"github.com/nccasia/agent-sdk-go/agent_sdk/core/spec"
)

func TestRespondLobeMetadata(t *testing.T) {
	s := Respond.Spec()
	if s.ID != "respond" {
		t.Fatalf("id = %q", s.ID)
	}
	if s.Layer != spec.LayerExpression {
		t.Fatalf("layer = %d", s.Layer)
	}
	if !s.Pinned {
		t.Fatal("respond must be pinned")
	}
}

func TestRespondActivationAlwaysOn(t *testing.T) {
	signals := Respond.Spec().Signals(map[string]any{})
	if signals["respond"] != 1.0 {
		t.Fatalf("respond activation = %v", signals)
	}
}

func TestRespondSystemPromptContinuation(t *testing.T) {
	if !strings.Contains(RespondSystemPrompt, "continuing this conversation") {
		t.Fatal("respond prompt must frame a continuation")
	}
	if !strings.Contains(RespondSystemPrompt, "do not restart") {
		t.Fatal("respond prompt must forbid restarting")
	}
}

func TestExpressionLobesContainsRespond(t *testing.T) {
	if len(Lobes) != 1 || Lobes[0].ID != "respond" {
		t.Fatalf("Lobes = %+v", Lobes)
	}
}
