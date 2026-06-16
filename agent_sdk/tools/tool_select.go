package tools

import (
	"math"

	"github.com/nccasia/agent-sdk-go/agent_sdk/core/attention"
	"github.com/nccasia/agent-sdk-go/agent_sdk/core/spec"
	"github.com/nccasia/agent-sdk-go/agent_sdk/lobes"
)

// ToolSelectLobe is the activation-only control lobe that adaptively exposes only
// the tools a turn needs. Inert by default (tool_strategy != "adaptive"); at
// default weights the lobe is dark, so the network is byte-identical at parity.
// Ported from agent_sdk/tools/lobes/tool_select.py.
type ToolSelectLobe struct{}

// Lobe returns the authoring-API lobe (id/name/activation), mirroring the Python
// LOBE singleton.
func (ToolSelectLobe) Lobe() lobes.Lobe {
	return lobes.Lobe{
		ID:          "tool_select",
		Name:        "Tool Select",
		Description: "Expose only the tools relevant to the turn, always keeping essentials.",
		UseWhen:     "the turn needs only a few tools, not the whole toolset",
		Behavior:    "select",
		Layer:       spec.LayerSkill,
		Order:       2, // after skill_select (0) + skill_active (1) in the SKILL layer
		Activation: func(ctx map[string]any) float64 {
			if s, _ := ctx["tool_strategy"].(string); s == "adaptive" {
				return 1.0
			}
			return 0.0
		},
	}
}

// KeptMeta is one kept tool's selection record.
type KeptMeta struct {
	Name       string  `json:"name"`
	Essential  bool    `json:"essential"`
	L1         float64 `json:"l1,omitempty"`
	L2         float64 `json:"l2,omitempty"`
	Activation float64 `json:"activation,omitempty"`
}

// DroppedMeta is one dropped tool's selection record.
type DroppedMeta struct {
	Name       string  `json:"name"`
	L1         float64 `json:"l1"`
	L2         float64 `json:"l2"`
	Activation float64 `json:"activation"`
	Reason     string  `json:"reason"` // below_floor | max_tools
}

// SelectTrace is the {kept, dropped} record placed on trace.tool_selection.
type SelectTrace struct {
	Kept    []KeptMeta    `json:"kept"`
	Dropped []DroppedMeta `json:"dropped"`
}

// Select trims specs to the relevant tools under a budget, keeping essentials
// unconditionally. essential(name) decides the always-kept set; everything else
// is scored by relevance (the shared L1+L2 ScoreText) and kept by floor + budget.
// Deterministic — no LLM call. Ported from ToolSelectLobe.select.
//
// embedOne returns the cached embedding for a text (nil to skip L2). qVec is the
// turn-query vector (nil ⇒ L1-only).
func (ToolSelectLobe) Select(
	specs []map[string]any,
	query string,
	qVec []float64,
	embedOne func(string) []float64,
	essential func(string) bool,
	weights map[string]float64,
	minActivation float64,
	maxTools int,
) ([]map[string]any, SelectTrace) {
	type scored struct {
		act  float64
		res  attention.ScoreResult
		spec map[string]any
		name string
	}

	var essentials []map[string]any
	var rest []scored
	for _, s := range specs {
		name, _ := s["name"].(string)
		if essential(name) {
			essentials = append(essentials, s)
			continue
		}
		desc, _ := s["description"].(string)
		var textVec []float64
		if qVec != nil && embedOne != nil {
			textVec = embedOne(name + " " + desc)
		}
		res := attention.ScoreText(query, qVec, name+" "+desc, textVec, weights, 0.0)
		rest = append(rest, scored{act: res.Activation, res: res, spec: s, name: name})
	}
	// Stable sort by descending activation (preserve input order on ties).
	for i := 1; i < len(rest); i++ {
		for j := i; j > 0 && rest[j].act > rest[j-1].act; j-- {
			rest[j], rest[j-1] = rest[j-1], rest[j]
		}
	}

	out := append([]map[string]any(nil), essentials...)
	tr := SelectTrace{}
	for _, s := range essentials {
		name, _ := s["name"].(string)
		tr.Kept = append(tr.Kept, KeptMeta{Name: name, Essential: true})
	}
	budget := maxTools - len(essentials)
	if budget < 0 {
		budget = 0
	}
	nonEssKept := 0
	for _, sc := range rest {
		meta := DroppedMeta{
			Name:       sc.name,
			L1:         round3(sc.res.L1),
			L2:         round3(sc.res.L2),
			Activation: round3(sc.act),
		}
		switch {
		case sc.act < minActivation:
			meta.Reason = "below_floor"
			tr.Dropped = append(tr.Dropped, meta)
		case nonEssKept >= budget:
			meta.Reason = "max_tools"
			tr.Dropped = append(tr.Dropped, meta)
		default:
			out = append(out, sc.spec)
			nonEssKept++
			tr.Kept = append(tr.Kept, KeptMeta{
				Name: sc.name, L1: round3(sc.res.L1), L2: round3(sc.res.L2), Activation: round3(sc.act),
			})
		}
	}
	return out, tr
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
