// Package network is the built-in production network — the agent-core 15-lobe /
// 5-path / named-flow network exported from Python's preact/production.py and
// embedded as JSON via go:embed. It is the source-of-truth default for the
// PreAct agent: the structural rows are regenerated from Python, while the per-
// lobe free signal extractors and path recognizers live in the lobes package
// (the OY perception layer).
package network

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/mezon/agent-sdk-go/agent_sdk/core/spec"
	"github.com/mezon/agent-sdk-go/agent_sdk/lobes"
)

//go:embed production.json
var productionJSON []byte

// rawNetwork is the embedded structural network (signals reattached at load).
type rawNetwork struct {
	Lobes  []rawLobe  `json:"lobes"`
	Paths  []rawPath  `json:"paths"`
	Flows  []RawFlow  `json:"flows"`
	Stages []RawStage `json:"stages"`
}

type rawLobe struct {
	ID            string             `json:"id"`
	Behavior      string             `json:"behavior"`
	Layer         int                `json:"layer"`
	Order         int                `json:"order"`
	Prior         float64            `json:"prior"`
	Pinned        bool               `json:"pinned"`
	MinActivation float64            `json:"min_activation"`
	Edges         map[string]float64 `json:"edges"`
	SignalWeights map[string]float64 `json:"signal_weights"`
	Writes        []string           `json:"writes"`
}

type rawPath struct {
	Name      string             `json:"name"`
	Members   []string           `json:"members"`
	Bias      map[string]float64 `json:"bias"`
	Threshold float64            `json:"threshold"`
	Grounds   bool               `json:"grounds"`
}

// RawFlow is one embedded production flow row.
type RawFlow struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Stages      []string `json:"stages"`
	Grounds     bool     `json:"grounds"`
	Threshold   float64  `json:"threshold"`
}

// RawStage is one embedded production stage row.
type RawStage struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Lobes       []string `json:"lobes"`
	Loop        string   `json:"loop"`
	Tools       []string `json:"tools"`
	FanoutKey   string   `json:"fanout_key"`
	Description string   `json:"description"`
	Hops        *int     `json:"hops"`
}

var (
	loadOnce sync.Once
	loaded   rawNetwork
	loadErr  error
)

func load() rawNetwork {
	loadOnce.Do(func() {
		loadErr = json.Unmarshal(productionJSON, &loaded)
	})
	if loadErr != nil {
		panic(fmt.Sprintf("network: malformed embedded production.json: %v", loadErr))
	}
	return loaded
}

// ProductionLobes returns the 15 production lobe specs with their free signal
// extractors reattached from the lobes package. The structural rows come from
// the embedded JSON; the signals come from lobes.SignalFor.
func ProductionLobes() []spec.Lobe {
	raw := load()
	out := make([]spec.Lobe, 0, len(raw.Lobes))
	for _, r := range raw.Lobes {
		out = append(out, spec.Lobe{
			ID:            r.ID,
			Behavior:      r.Behavior,
			Layer:         r.Layer,
			Order:         r.Order,
			Prior:         r.Prior,
			Pinned:        r.Pinned,
			MinActivation: r.MinActivation,
			Edges:         cloneFloat(r.Edges),
			SignalWeights: cloneFloat(r.SignalWeights),
			Writes:        append([]string(nil), r.Writes...),
			Signals:       lobes.SignalFor(r.ID),
		})
	}
	return out
}

// ProductionPaths returns the 5 production path recognizers (delegates to the
// lobes package, which owns the deterministic recognizers).
func ProductionPaths() []spec.Path { return lobes.ProductionPaths() }

// ProductionFlows returns the embedded production flow rows.
func ProductionFlows() []RawFlow {
	raw := load()
	return append([]RawFlow(nil), raw.Flows...)
}

// ProductionStages returns the embedded production stage rows.
func ProductionStages() []RawStage {
	raw := load()
	return append([]RawStage(nil), raw.Stages...)
}

// DefaultWeights builds the flat sparse weight surface from the production
// network — priors, thresholds, edges, and path biases. Ported from
// agent_sdk/lobes/weights.py:_build_default_weights (the non-tuning, network-
// derived rows; the global tuning levers default to 1.0/0.0 in the engine).
func DefaultWeights() map[string]float64 {
	weights := map[string]float64{
		"w_memory_enabled":     1.0,
		"w_mem_conversation":   0.0,
		"w_mem_channel":        0.0,
		"w_mem_user":           0.0,
		"w_mem_bot":            0.0,
		"w_skills_declared":    1.0,
		"w_anaphora":           0.6,
		"w_short_query":        0.6,
		"w_has_history":        0.0,
		"w_scope_gate":         1.0,
		"w_has_stage_classify": 1.0,
		"w_simple_shape":       -0.6,
		"w_route_complex":      1.0,
		"w_tools_used":         0.0,
		"w_fixed_format":       1.0,
		"budget_memory":        800,
		"budget_skill":         400,
		"budget_cognition":     600,
	}
	for _, l := range ProductionLobes() {
		weights["prior_"+l.ID] = l.Prior
		weights["min_"+l.ID] = l.MinActivation
		for dst, w := range l.Edges {
			weights["edge_"+l.ID+"__"+dst] = w
		}
	}
	for _, p := range ProductionPaths() {
		for _, member := range p.Members {
			weights["path_"+p.Name+"__"+member] = p.Bias[member]
		}
	}
	return weights
}

func cloneFloat(m map[string]float64) map[string]float64 {
	if m == nil {
		return map[string]float64{}
	}
	out := make(map[string]float64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
