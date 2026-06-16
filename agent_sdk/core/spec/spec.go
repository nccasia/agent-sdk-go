// Package spec holds the serializable PreAct configuration as data plus the
// structural network types (Lobe/Stage/Flow/Path) and the deterministic
// network validator. Ported from agent_sdk/spec.py and the spec-typed half of
// agent_sdk/network/activation.py.
//
// The deterministic core (intent recognition, activation, attention/budget,
// flow resolution) is a pure function of (spec, context). Spec captures an
// agent's network as JSON and round-trips exactly.
package spec

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/nccasia/agent-sdk-go/agent_sdk/contracts"
)

// SpecVersion is the serialized spec schema version.
const SpecVersion = "1"

// PinnedLobes is the canonical pinned output-contract lobe set ({cite, filter}).
// Re-exported from contracts so the activation machinery and the policy
// validator share one source of truth.
var PinnedLobes = contracts.PinnedLobes

// SortedPinnedLobes returns the pinned ids in deterministic sorted order.
func SortedPinnedLobes() []string { return contracts.SortedPinnedLobes() }

// ── Layers: the reasoning process, brain-shaped ──────────────────────────────

const (
	LayerInstinct   = 0 // B0 — reflexes (CORE, never lobes)
	LayerPerception = 1 // B1 — deterministic feature extraction (CORE)
	LayerMemory     = 2 // B2 — recall lobes
	LayerSkill      = 3 // B3 — learned procedure selection
	LayerCognition  = 4 // B4 — deliberate behavior
	LayerExpression = 5 // B5 — output contract: cite/filter pinned, format
)

// Layers maps a layer index to its name.
var Layers = map[int]string{
	LayerInstinct:   "instinct",
	LayerPerception: "perception",
	LayerMemory:     "memory",
	LayerSkill:      "skill",
	LayerCognition:  "cognition",
	LayerExpression: "expression",
}

// lobeLayers — lobes may only occupy B2..B5.
var lobeLayers = map[int]struct{}{
	LayerMemory: {}, LayerSkill: {}, LayerCognition: {}, LayerExpression: {},
}

// OutputContractLobes are the grounding output-contract lobe ids whose
// activation is driven by the resolved path's grounding flag.
var OutputContractLobes = map[string]struct{}{"cite": {}, "filter": {}}

// SignalFn is a lobe's deterministic, free signal extractor: ctx -> named
// signal values. Never an LLM call.
type SignalFn func(ctx map[string]any) map[string]float64

// Lobe is one behavior per module — a registry row, never an interpreter
// branch. (Python LobeSpec.) edges target strictly later (layer, order)
// positions; order is the intra-layer execution rank.
type Lobe struct {
	ID       string
	Behavior string // "recall", "rewrite", "route", "decompose", …
	Layer    int
	Prior    float64
	Pinned   bool
	// Attends ContextBound is held opaquely as the receptive field; the
	// activate/attention packages interpret it. Kept as any to avoid an import
	// cycle through attention.
	Attends       any
	Signals       SignalFn
	SignalWeights map[string]float64
	Edges         map[string]float64 // lobe_id -> weight
	Writes        []string
	MinActivation float64
	Order         int
	BuildContext  bool
}

// Validate enforces the lobe-layer invariant (lobes live in B2..B5).
func (l Lobe) Validate() error {
	if _, ok := lobeLayers[l.Layer]; !ok {
		return fmt.Errorf("lobe %q: layer %d is core machinery, not a lobe layer (lobes live in B2..B5)", l.ID, l.Layer)
	}
	return nil
}

// RecognizerFn scores a path from free B1 signals (0..1).
type RecognizerFn func(ctx map[string]any) float64

// Path is a well-known reasoning path — a labeled subgraph template over lobes.
// (Python PathSpec.) Recognition BIASES member lobes; it never hard-gates.
type Path struct {
	Name       string
	Members    []string
	Recognizer RecognizerFn
	Bias       map[string]float64 // lobe_id -> default bias
	Threshold  float64            // recognition floor (default 0.5)
	StageNames []string
	Grounds    bool // whether this path produces a grounded (KB-answering) reply
}

// Stage is one ordered pipeline step over a lobe slice (the OX/time axis).
type Stage struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	UseWhen      string   `json:"use_when"`
	Lobes        []string `json:"lobes"`
	Loop         string   `json:"loop"`
	Tools        []string `json:"tools"`
	FanoutKey    string   `json:"fanout_key"`
	Threshold    float64  `json:"threshold"`
	Model        *string  `json:"model"`
	Temperature  *float64 `json:"temperature"`
	MaxTokens    *int     `json:"max_tokens"`
	Hops         *int     `json:"hops"`
	SystemPrompt *string  `json:"system_prompt"`
}

// Flow is an ordered pipeline of stages.
type Flow struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	UseWhen     string   `json:"use_when"`
	Stages      []string `json:"stages"`
	Threshold   float64  `json:"threshold"`
	Grounds     bool     `json:"grounds"`
	Signal      any      `json:"signal"`
}

// Spec is the whole PreAct configuration as data (the Python PreactSpec). Rows
// are JSON-shaped maps so the spec round-trips exactly.
type Spec struct {
	Version          string           `json:"version"`
	Instructions     string           `json:"instructions"`
	Lobes            []map[string]any `json:"lobes"`
	Stages           []map[string]any `json:"stages"`
	Flows            []map[string]any `json:"flows"`
	Skills           []map[string]any `json:"skills"`
	Weights          map[string]any   `json:"weights"`
	Budgets          map[string]any   `json:"budgets"`
	FlowLobeWeights  map[string]any   `json:"flow_lobe_weights"`
	FlowLayerBudgets map[string]any   `json:"flow_layer_budgets"`
	PinnedLobes      []string         `json:"pinned_lobes"`
	RequireCitations bool             `json:"require_citations"`
	TZ               string           `json:"tz"`
	Lang             string           `json:"lang"`
}

// NewSpec builds a Spec with the Python dataclass defaults (version "1",
// pinned_lobes sorted, tz UTC, lang en, empty collections non-nil).
func NewSpec() *Spec {
	return &Spec{
		Version:          SpecVersion,
		Lobes:            []map[string]any{},
		Stages:           []map[string]any{},
		Flows:            []map[string]any{},
		Skills:           []map[string]any{},
		Weights:          map[string]any{},
		Budgets:          map[string]any{},
		FlowLobeWeights:  map[string]any{},
		FlowLayerBudgets: map[string]any{},
		PinnedLobes:      SortedPinnedLobes(),
		RequireCitations: false,
		TZ:               "UTC",
		Lang:             "en",
	}
}

// ToJSON serializes the spec to a map (the Python to_json shape).
func (s *Spec) ToJSON() map[string]any {
	b, _ := json.Marshal(s)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

// ToJSONStr serializes the spec to an indented JSON string.
func (s *Spec) ToJSONStr() (string, error) {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// FromJSON rebuilds a Spec from a JSON byte slice, ignoring unknown keys.
func FromJSON(data []byte) (*Spec, error) {
	s := NewSpec()
	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	return s, nil
}

// ── Network validation ───────────────────────────────────────────────────────

// ValidateNetwork rejects malformed networks at registration time.
//
// Forward DAG: every edge targets an existing lobe at a strictly later (layer,
// order) position. Pinned protection: no negative (inhibitory) edge may target
// a pinned lobe.
func ValidateNetwork(lobes []Lobe) error {
	byID := make(map[string]Lobe, len(lobes))
	for _, lobe := range lobes {
		if _, dup := byID[lobe.ID]; dup {
			return fmt.Errorf("duplicate lobe id %q", lobe.ID)
		}
		byID[lobe.ID] = lobe
	}
	// Deterministic edge iteration order for stable error messages.
	for _, lobe := range lobes {
		targets := make([]string, 0, len(lobe.Edges))
		for t := range lobe.Edges {
			targets = append(targets, t)
		}
		sort.Strings(targets)
		for _, targetID := range targets {
			weight := lobe.Edges[targetID]
			target, ok := byID[targetID]
			if !ok {
				return fmt.Errorf("lobe %q: edge to unknown lobe %q", lobe.ID, targetID)
			}
			if lessEqual(target.Layer, target.Order, lobe.Layer, lobe.Order) {
				return fmt.Errorf(
					"lobe %q: edge to %q is not forward ((%d,%d) -> (%d,%d)) — the network is a forward DAG",
					lobe.ID, targetID, lobe.Layer, lobe.Order, target.Layer, target.Order)
			}
			if weight < 0 && target.Pinned {
				return fmt.Errorf(
					"lobe %q: inhibitory edge to pinned lobe %q (a weight can never express 'skip the ground-or-refuse contract')",
					lobe.ID, targetID)
			}
		}
	}
	return nil
}

// lessEqual reports (la,lo) <= (rla,rlo) on the (layer, order) tuple.
func lessEqual(la, lo, rla, rlo int) bool {
	if la != rla {
		return la < rla
	}
	return lo <= rlo
}
