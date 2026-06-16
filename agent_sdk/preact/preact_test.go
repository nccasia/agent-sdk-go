// Translated from agent_sdk/preact/defaults.py + preact/lobes.py — the
// Lobes/Stages/Flows namespaces' Default()/Minimal() networks.
package preact

import (
	"testing"

	"github.com/nccasia/agent-sdk-go/agent_sdk/core/spec"
)

func TestLobesDefaultIsProductionNetwork(t *testing.T) {
	got := Lobes{}.Default()
	if len(got) != 15 {
		t.Fatalf("default lobes = %d, want 15", len(got))
	}
	if err := spec.ValidateNetwork(got); err != nil {
		t.Errorf("default network invalid: %v", err)
	}
}

func TestLobesMinimalIsSmallNetwork(t *testing.T) {
	got := Lobes{}.Minimal()
	wantIDs := map[string]bool{"classify": true, "plan": true, "research": true,
		"synthesize": true, "clarify": true, "cite": true, "filter": true}
	if len(got) != len(wantIDs) {
		t.Fatalf("minimal lobes = %d, want %d", len(got), len(wantIDs))
	}
	for _, l := range got {
		if !wantIDs[l.ID] {
			t.Errorf("unexpected minimal lobe %q", l.ID)
		}
	}
	if err := spec.ValidateNetwork(got); err != nil {
		t.Errorf("minimal network invalid: %v", err)
	}
}

func TestStagesDefaultAndMinimal(t *testing.T) {
	if got := len(Stages{}.Default()); got != 8 {
		t.Errorf("default stages = %d, want 8", got)
	}
	min := Stages{}.Minimal()
	if len(min) != 4 {
		t.Fatalf("minimal stages = %d, want 4", len(min))
	}
	if min[0].ID != "plan" || min[2].Loop != "single" {
		t.Errorf("minimal stage shape: %+v", min)
	}
}

func TestFlowsDefaultAndMinimal(t *testing.T) {
	def := Flows{}.Default()
	if len(def) != 6 {
		t.Fatalf("default flows = %d, want 6", len(def))
	}
	byName := map[string]spec.Flow{}
	for _, f := range def {
		byName[f.Name] = f
	}
	if !byName["qna"].Grounds || !byName["research"].Grounds {
		t.Error("qna/research flows must ground")
	}
	if byName["clarify"].Grounds || byName["relational"].Grounds {
		t.Error("clarify/relational flows must not ground")
	}

	min := Flows{}.Minimal()
	if len(min) != 3 {
		t.Fatalf("minimal flows = %d, want 3", len(min))
	}
}

func TestLobesPaths(t *testing.T) {
	if got := len(Lobes{}.Paths()); got != 5 {
		t.Errorf("paths = %d, want 5", got)
	}
}
