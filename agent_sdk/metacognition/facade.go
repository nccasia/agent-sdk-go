package metacognition

import "fmt"

// PinnedUnskippable: the engine never lets metacognition skip these, regardless
// of subclass. Ported from metacognition_facade.PINNED_UNSKIPPABLE.
var PinnedUnskippable = map[string]struct{}{"cite": {}, "filter": {}}

// Metacognition is the first-class reasoning-control object. It monitors the
// object-level state and regulates the next step within an allow-list; cite /
// filter stay pinned and are never skippable. Ported from
// agent_sdk/metacognition_facade.py:Metacognition.
type Metacognition struct {
	Mode         MetaMode
	ApplyActions map[MetaAction]struct{}
	controller   *MetaController
}

// NewMetacognition builds a Metacognition. mode must be "apply" or "observe".
// applyActions nil → {adjust_lobe_slice}; non-capable actions are dropped.
func NewMetacognition(mode MetaMode, applyActions map[MetaAction]struct{}) (*Metacognition, error) {
	if mode != ModeApply && mode != ModeObserve {
		return nil, fmt.Errorf("mode must be 'apply' or 'observe'")
	}
	if applyActions == nil {
		applyActions = map[MetaAction]struct{}{ActionAdjustLobeSlice: {}}
	}
	filtered := map[MetaAction]struct{}{}
	for a := range applyActions {
		if _, ok := ApplyCapableActions[a]; ok {
			filtered[a] = struct{}{}
		}
	}
	return &Metacognition{
		Mode:         mode,
		ApplyActions: filtered,
		controller:   NewMetaController(mode, false, filtered),
	}, nil
}

// CoerceMetacognition coerces a value to a *Metacognition. nil → observe-mode;
// a string → that mode; a *Metacognition → itself.
func CoerceMetacognition(value any) (*Metacognition, error) {
	switch v := value.(type) {
	case nil:
		return NewMetacognition(ModeObserve, nil)
	case *Metacognition:
		return v, nil
	case string:
		return NewMetacognition(v, nil)
	}
	return nil, fmt.Errorf("cannot coerce %v to Metacognition", value)
}

// ShouldApply reports whether the action would be applied.
func (m *Metacognition) ShouldApply(action MetaAction) bool {
	return m.controller.ShouldApply(action)
}

// PlanNext runs monitor → regulate, then enforces the pinned-unskippable guard:
// a skip_step targeting cite/filter is rewritten to a continue.
func (m *Metacognition) PlanNext(in PlanNextInput) MetaDecision {
	decision := m.controller.PlanNext(in)
	if decision.Action == ActionSkipStep && in.TargetStep != nil {
		if _, pinned := PinnedUnskippable[*in.TargetStep]; pinned {
			return MetaDecision{
				Action:     ActionContinue,
				Reason:     "pinned:never_skip",
				TargetStep: in.TargetStep,
			}
		}
	}
	return decision
}

// String mirrors the Python __repr__.
func (m *Metacognition) String() string {
	return fmt.Sprintf("Metacognition(mode=%q, apply_actions=%v)", m.Mode, sortedActions(m.ApplyActions))
}

func sortedActions(m map[MetaAction]struct{}) []MetaAction {
	out := make([]MetaAction, 0, len(m))
	for a := range m {
		out = append(out, a)
	}
	return out
}
