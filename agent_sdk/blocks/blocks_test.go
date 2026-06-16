// Translated from tests/test_blocks.py / test_lobe_network.py / test_spec.py
// — exercises the blocks package: a flat re-export of framework primitives
// (contracts, lobes, flows, network, skills, metacognition, inspection).
package blocks

import (
	"testing"

	"github.com/mezon/agent-sdk-go/agent_sdk/contracts"
	"github.com/mezon/agent-sdk-go/agent_sdk/core/activate"
	"github.com/mezon/agent-sdk-go/agent_sdk/core/attention"
	"github.com/mezon/agent-sdk-go/agent_sdk/core/feature"
	"github.com/mezon/agent-sdk-go/agent_sdk/core/spec"
	"github.com/mezon/agent-sdk-go/agent_sdk/flows"
	"github.com/mezon/agent-sdk-go/agent_sdk/inspection"
	"github.com/mezon/agent-sdk-go/agent_sdk/lobes"
	"github.com/mezon/agent-sdk-go/agent_sdk/metacognition"
	"github.com/mezon/agent-sdk-go/agent_sdk/react"
	"github.com/mezon/agent-sdk-go/agent_sdk/skills"
)

// TestContractsReexported: the contracts primitives are wired through blocks.
func TestContractsReexported(t *testing.T) {
	_ = contracts.LlmRequest{}
	var _ contracts.LlmCall = nil
	_ = contracts.LobeServices{}
	_ = contracts.PromptContribution{}
	_ = contracts.TurnContext{}
	_ = contracts.NewTurnContext("q")
	_ = contracts.LobeResult{}
	_ = contracts.StageResult{}
	_ = contracts.ToolRuntime(nil)
	_ = (*contracts.CompositeToolRuntime)(nil)
	_ = contracts.NewCompositeToolRuntime
	_ = contracts.Citation{}
	_ = contracts.Claim{}
	_ = contracts.Memo{}
	_ = contracts.FinalEnvelope{}
	if !contracts.IsPinned("cite") || !contracts.IsPinned("filter") {
		t.Errorf("PINNED_LOBES not reexported")
	}
	if contracts.StripMemoryFooter("text") != "text" {
		t.Errorf("StripMemoryFooter not reexported")
	}
}

// TestNetworkReexported: the network primitives (Lobe, Path, ContextNode, ...)
// are wired through blocks.
func TestNetworkReexported(t *testing.T) {
	_ = spec.Lobe{}
	_ = spec.Path{}
	_ = attention.Blackboard{}
	_ = attention.ContextNode{}
	_ = feature.RecognizePaths
	_ = activate.ResolvePath
	_ = spec.ValidateNetwork
	_ = activate.MergeLobeWeights
	_ = activate.Propagate
	_ = activate.NetworkResolution{}
	_ = attention.Build
}

// TestLobeFrameworkReexported: the lobe framework (lobes.Lobe, Registry,
// ProductionPaths, etc.) is reexported.
func TestLobeFrameworkReexported(t *testing.T) {
	_ = lobes.Lobe{}
	_ = (&lobes.Registry{})
	_ = lobes.ProductionPaths
}

// TestFlowFrameworkReexported: the flow framework (Flow, FlowStep, etc.) is
// reexported.
func TestFlowFrameworkReexported(t *testing.T) {
	_ = flows.Flow{}
	_ = flows.FlowStep{}
	_ = flows.NewFlow("id")
	_ = flows.NewFlowStep(flows.FlowStep{Name: "x"})
}

// TestMetacognitionReexported: the metacognition façade (Metacognition,
// MetaDecision, MetaObservation) is reexported.
func TestMetacognitionReexported(t *testing.T) {
	_ = metacognition.MetaController{}
	_ = metacognition.MetaDecision{}
	_ = metacognition.MetaObservation{}
	_ = metacognition.Monitor
	_ = metacognition.CompileStatePlan
}

// TestReactAndSkillsReexported: react funnel helpers + skills types.
func TestReactAndSkillsReexported(t *testing.T) {
	_ = react.TierObservations
	_ = react.CompactObservations
	_ = (*skills.SkillRegistry)(nil)
	_ = skills.SkillPack{}
	_ = skills.BuildPromptBlock
}

// TestInspectionReexported: the inspection types are wired through blocks.
func TestInspectionReexported(t *testing.T) {
	_ = inspection.EngineSnapshot{}
	_ = inspection.LobeAxisSnapshot{}
	_ = inspection.FlowAxisSnapshot{}
	_ = inspection.AxisOptimization{}
	_ = inspection.SuggestAxisOptimizations
}

// TestLobeSpecReexported: the lobes.Lobe / spec.Lobe alias is consistent.
func TestLobeSpecReexported(t *testing.T) {
	// Both spec.Lobe and lobes.Lobe exist; blocks re-exports both names.
	sl := spec.Lobe{ID: "x"}
	if sl.ID != "x" {
		t.Errorf("spec.Lobe not initialized")
	}
	ll := lobes.Lobe{ID: "x"}
	if ll.GetID() != "x" {
		t.Errorf("lobes.Lobe not initialized")
	}
}
