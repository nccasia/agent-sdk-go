// Declarative per-stage overrides + the grounded-stage temperature invariant.
//
// A host tunes the built-in network per stage from config — a
// {stage_name: {system_prompt?, temperature?, max_tokens?, loop?, budget:{hops?}}}
// map — without re-authoring stages in code. ApplyStageOverrides patches the
// matching production Stage objects; unset knobs keep the production default.
// Matching is exact stage-id match, else the bare logical-step suffix
// (qna:synthesize ← "synthesize"), so one override tunes every flow's step of
// that name.
//
// AssertGroundedStagesZeroTemp enforces the grounding invariant — synthesize /
// cite / filter must run at temperature == 0 — re-asserted after patching so an
// override can never break it.
//
// Ported from agent_sdk/stage_overrides.py.
package engine

import (
	"fmt"
	"strings"
)

// GroundedStages must run at temperature 0.
var GroundedStages = [...]string{"synthesize", "cite", "filter"}

var overrideLoops = map[string]struct{}{
	"none":    {},
	"single":  {},
	"agentic": {},
	"map":     {},
}

// GroundedTempError is raised when a grounded stage has a non-zero temperature.
type GroundedTempError struct{ Stage string }

func (e *GroundedTempError) Error() string {
	return fmt.Sprintf("stage %q must be temperature==0", e.Stage)
}

func bareName(name string) string {
	if i := strings.LastIndex(name, ":"); i >= 0 {
		return name[i+1:]
	}
	return name
}

// AssertGroundedStagesZeroTemp enforces the hard invariant that synthesize /
// cite / filter run at temperature 0. Returns a *GroundedTempError on breach.
func AssertGroundedStagesZeroTemp(stages []*Stage) error {
	for _, s := range stages {
		name := s.NameField
		if name == "" {
			name = s.IDField
		}
		bare := bareName(name)
		if !isGrounded(bare) {
			continue
		}
		if s.Temperature != nil && *s.Temperature != 0 {
			return &GroundedTempError{Stage: name}
		}
	}
	return nil
}

func isGrounded(bare string) bool {
	for _, g := range GroundedStages {
		if g == bare {
			return true
		}
	}
	return false
}

// overrideFor returns the override config for a stage id — exact full-id match,
// else bare-suffix match (qna:synthesize → overrides["synthesize"]).
func overrideFor(stageID string, overrides map[string]any) map[string]any {
	cfg, ok := overrides[stageID]
	if !ok {
		cfg = overrides[bareName(stageID)]
	}
	m, _ := cfg.(map[string]any)
	return m
}

// clone returns a shallow copy of st (all fields preserved) so an override never
// silently drops a stage attribute.
func clone(st *Stage) *Stage {
	cp := *st
	return &cp
}

func patch(st *Stage, cfg map[string]any) *Stage {
	n := clone(st)
	budget, _ := cfg["budget"].(map[string]any)
	if sp, ok := cfg["system_prompt"]; ok {
		if s, ok := sp.(string); ok && s != "" {
			n.SystemPrompt = &s
		}
	}
	if t, ok := cfg["temperature"]; ok && t != nil {
		if f, ok := asFloat(t); ok {
			n.Temperature = &f
		}
	}
	if mt, ok := cfg["max_tokens"]; ok && mt != nil {
		if i, ok := asInt(mt); ok {
			n.MaxTokens = &i
		}
	}
	if budget != nil {
		if h, ok := budget["hops"]; ok && h != nil {
			if i, ok := asInt(h); ok {
				n.Hops = &i
			}
		}
	}
	if loop, ok := cfg["loop"].(string); ok {
		if _, valid := overrideLoops[loop]; valid {
			n.Loop = loop
		}
	}
	return n
}

// ApplyStageOverrides returns stages patched from an overrides map
// ({stage_name: {system_prompt?, temperature?, max_tokens?, loop?, budget:{hops?}}}).
// No-op (a fresh slice) when there are no overrides. Re-asserts the
// grounded-stage zero-temperature invariant, returning an error on breach.
//
// model is intentionally not applied here — per-stage model dispatch is an
// engine/client concern, not a stage-clone concern.
func ApplyStageOverrides(stages []*Stage, overrides map[string]any) ([]*Stage, error) {
	if len(overrides) == 0 {
		return append([]*Stage(nil), stages...), nil
	}
	out := make([]*Stage, 0, len(stages))
	for _, st := range stages {
		if cfg := overrideFor(st.IDField, overrides); cfg != nil {
			out = append(out, patch(st, cfg))
		} else {
			out = append(out, st)
		}
	}
	if err := AssertGroundedStagesZeroTemp(out); err != nil {
		return nil, err
	}
	return out, nil
}

func asFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	}
	return 0, false
}

func asInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	}
	return 0, false
}
