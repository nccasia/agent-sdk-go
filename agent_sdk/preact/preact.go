// Package preact exposes the built-in PreAct network as namespaces: Lobes,
// Stages, and Flows, each with a Default() (the faithfully-ported agent-core
// production network — 15 lobes, 5 paths, the named flows) and a Minimal() (the
// small qna/research/clarify network kept for lightweight agents/tests). Ported
// from agent_sdk/preact/{production,defaults,lobes}.py.
package preact

import (
	"github.com/mezon/agent-sdk-go/agent_sdk/core/spec"
	"github.com/mezon/agent-sdk-go/agent_sdk/lobes"
	"github.com/mezon/agent-sdk-go/agent_sdk/network"
)

// Lobes is the lobe-network namespace.
type Lobes struct{}

// Default returns the 15 ported production lobes (memory/skill/tool/cognition/
// expression), signal-extractors attached.
func (Lobes) Default() []spec.Lobe { return network.ProductionLobes() }

// Minimal returns the small qna/research/clarify lobe network for lightweight
// agents and tests (preact/lobes.py:default_lobes).
func (Lobes) Minimal() []spec.Lobe {
	out := make([]spec.Lobe, 0, len(minimalLobes))
	for _, l := range minimalLobes {
		out = append(out, l.Spec())
	}
	return out
}

// Paths returns the production path recognizers (shared by Default/Minimal).
func (Lobes) Paths() []spec.Path { return network.ProductionPaths() }

// Stages is the stage (OX-axis) namespace.
type Stages struct{}

// Default returns the production stages (flow-qualified) from the embedded net.
func (Stages) Default() []spec.Stage {
	raw := network.ProductionStages()
	out := make([]spec.Stage, 0, len(raw))
	for _, s := range raw {
		out = append(out, spec.Stage{
			ID:          s.ID,
			Name:        s.Name,
			Description: s.Description,
			Lobes:       append([]string(nil), s.Lobes...),
			Loop:        s.Loop,
			Tools:       append([]string(nil), s.Tools...),
			FanoutKey:   s.FanoutKey,
			Hops:        s.Hops,
		})
	}
	return out
}

// Minimal returns the small plan/research/synthesize/clarify stage set.
func (Stages) Minimal() []spec.Stage {
	return []spec.Stage{
		{ID: "plan", Name: "plan", Lobes: []string{"plan"}, Loop: "single",
			Description: "Decompose the question into sub-questions."},
		{ID: "research", Name: "research", Lobes: []string{"research"}, Loop: "agentic",
			Description: "Gather evidence with tools."},
		{ID: "synthesize", Name: "synthesize", Lobes: []string{"classify", "synthesize", "cite", "filter"},
			Loop: "single", Description: "Compose the grounded answer."},
		{ID: "clarify", Name: "clarify", Lobes: []string{"clarify"}, Loop: "single",
			UseWhen: "an ambiguous follow-up", Description: "Ask one clarifying question."},
	}
}

// Flows is the flow (OX-axis) namespace.
type Flows struct{}

// Default returns the production flows from the embedded net, wired to the
// production paths' grounding flags and thresholds.
func (Flows) Default() []spec.Flow {
	raw := network.ProductionFlows()
	out := make([]spec.Flow, 0, len(raw))
	for _, f := range raw {
		out = append(out, spec.Flow{
			ID:          f.Name,
			Name:        f.Name,
			Description: f.Description,
			Stages:      append([]string(nil), f.Stages...),
			Grounds:     f.Grounds,
			Threshold:   f.Threshold,
		})
	}
	return out
}

// Minimal returns the small research/clarify/qna flow set.
func (Flows) Minimal() []spec.Flow {
	return []spec.Flow{
		{ID: "research", Name: "research", UseWhen: "multi-step questions needing sources",
			Stages: []string{"plan", "research", "synthesize"}, Threshold: 0.5,
			Signal: map[string]any{"any": []any{
				map[string]any{"lexical": []any{"compare", "vs", "versus", "research", "analyze"}},
				map[string]any{"min_words": 12.0},
			}}},
		{ID: "clarify", Name: "clarify", UseWhen: "an ambiguous follow-up",
			Stages: []string{"clarify"}, Threshold: 0.5, Grounds: false,
			Signal: map[string]any{"flag": "ambiguous"}},
		{ID: "qna", Name: "qna", UseWhen: "a direct question",
			Stages: []string{"synthesize"}, Threshold: 0.4,
			Signal: map[string]any{"const": 0.5}},
	}
}

// minimalLobes is the small qna/research/clarify lobe network (the B2..B5
// lightweight default). Ported from preact/lobes.py.
var minimalLobes = []lobes.Lobe{
	{ID: "classify", Name: "Classify", Layer: spec.LayerCognition, Behavior: "select",
		Description: "Route the turn: a direct answer vs. multi-step research.",
		UseWhen:     "every answer-producing turn",
		Activation:  func(map[string]any) float64 { return 1.0 }},
	{ID: "plan", Name: "Plan", Layer: spec.LayerCognition, Behavior: "decompose", Order: 1,
		Description: "Decompose a complex question into sub-questions.",
		UseWhen:     "a multi-step question that needs a plan"},
	{ID: "research", Name: "Research", Layer: spec.LayerCognition, Behavior: "gather", Order: 2,
		Description: "Gather evidence from tools/sources before answering.",
		UseWhen:     "the question needs external facts"},
	{ID: "synthesize", Name: "Synthesize", Layer: spec.LayerCognition, Behavior: "compose", Order: 3,
		Description: "Compose the final answer from what was gathered.",
		UseWhen:     "producing the answer",
		Activation:  func(map[string]any) float64 { return 1.0 }},
	{ID: "clarify", Name: "Clarify", Layer: spec.LayerCognition, Behavior: "clarify", Order: 4,
		Description: "Ask one focused clarifying question when the turn is ambiguous.",
		UseWhen:     "an ambiguous follow-up"},
	{ID: "cite", Name: "Cite", Layer: spec.LayerExpression, Behavior: "ground", Order: 8,
		Description: "Attach grounding to claims (the output contract).",
		UseWhen:     "grounding (KB-answering) turns"},
	{ID: "filter", Name: "Filter", Layer: spec.LayerExpression, Behavior: "filter", Order: 9,
		Description: "Refuse rather than emit ungrounded claims (ground-or-refuse).",
		UseWhen:     "grounding (KB-answering) turns"},
}
