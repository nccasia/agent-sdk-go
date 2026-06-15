package metacognition

import (
	"os"
	"strings"
)

// MetaMode is the metacognition mode. Metacognition is ALWAYS ON (ENGINE
// 0.7.1) — "observe" is the floor (monitor + trace, never mutate); "apply"
// additionally applies the allow-listed actions.
type MetaMode = string

const (
	ModeObserve MetaMode = "observe"
	ModeApply   MetaMode = "apply"
)

// observeTokens map to "observe" (including the legacy "disabled" vocabulary —
// the mutation kill switch survives, observability never turns off).
var observeTokens = map[string]struct{}{
	"observe": {}, "observability": {}, "shadow": {}, "dry_run": {}, "dry-run": {},
	"log": {}, "off": {}, "0": {}, "false": {}, "disabled": {}, "disable": {},
}

var applyTokens = map[string]struct{}{
	"on": {}, "1": {}, "true": {}, "enabled": {}, "enable": {}, "apply": {},
}

// ApplyCapableActions are the actions the interpreter implements an apply seam
// for. meta_review has no apply seam — observe-only by construction.
var ApplyCapableActions = map[MetaAction]struct{}{
	ActionAdjustLobeSlice: {},
	ActionSkipStep:        {},
	ActionRetryStep:       {},
	ActionRedoPhase:       {},
	ActionGotoPhase:       {},
}

// DefaultApplyActions is the production default — trim-only. Widening is a
// per-policy opt-in (metacognition_apply_actions); keeping the Navigator out of
// the default set preserves default-network parity.
var DefaultApplyActions = map[MetaAction]struct{}{
	ActionAdjustLobeSlice: {},
}

func normalizeMode(value any) (MetaMode, bool) {
	if value == nil {
		return "", false
	}
	raw := strings.ToLower(strings.TrimSpace(toStr(value)))
	if _, ok := observeTokens[raw]; ok {
		return ModeObserve, true
	}
	if _, ok := applyTokens[raw]; ok {
		return ModeApply, true
	}
	return "", false
}

// MetacognitionMode resolves the mode from $METACOGNITION, then policy. Policy
// may carry "metacognition_mode" (string) or the legacy boolean
// "metacognition_enabled". Defaults to "apply".
func MetacognitionMode(policy map[string]any) MetaMode {
	if m, ok := normalizeMode(os.Getenv("METACOGNITION")); ok {
		return m
	}
	if policy != nil {
		if m, ok := normalizeMode(policy["metacognition_mode"]); ok {
			return m
		}
		if v, present := policy["metacognition_enabled"]; present {
			if truthy(v) {
				return ModeApply
			}
			return ModeObserve
		}
	}
	return ModeApply
}

// MetacognitionEnabled is always true since ENGINE 0.7.1 — metacognition cannot
// be turned off. Kept for back-compat callers.
func MetacognitionEnabled(policy map[string]any) bool { return true }

// allowedActions resolves the policy's metacognition_apply_actions allow-list,
// intersected with ApplyCapableActions. nil/absent → DefaultApplyActions.
func allowedActions(policy map[string]any) map[MetaAction]struct{} {
	if policy == nil {
		return DefaultApplyActions
	}
	raw, present := policy["metacognition_apply_actions"]
	if !present || raw == nil {
		return DefaultApplyActions
	}
	var items []string
	switch v := raw.(type) {
	case string:
		for _, part := range strings.Split(v, ",") {
			items = append(items, strings.TrimSpace(part))
		}
	case []string:
		for _, part := range v {
			items = append(items, strings.TrimSpace(part))
		}
	case []any:
		for _, part := range v {
			items = append(items, strings.TrimSpace(toStr(part)))
		}
	default:
		return DefaultApplyActions
	}
	allowed := map[MetaAction]struct{}{}
	for _, item := range items {
		if _, ok := ApplyCapableActions[item]; ok {
			allowed[item] = struct{}{}
		}
	}
	return allowed
}

// MetaController monitors and regulates object-level OX/OY thinking.
type MetaController struct {
	Mode         MetaMode
	Enabled      bool // always true — kept for back-compat
	ApplyActions map[MetaAction]struct{}
}

// NewMetaController builds a controller. mode "" derives from the legacy
// enabled flag (enabledFalse → observe, else apply). applyActions nil →
// DefaultApplyActions.
func NewMetaController(mode MetaMode, enabledFalse bool, applyActions map[MetaAction]struct{}) *MetaController {
	if mode == "" {
		if enabledFalse {
			mode = ModeObserve
		} else {
			mode = ModeApply
		}
	}
	if applyActions == nil {
		applyActions = DefaultApplyActions
	}
	return &MetaController{Mode: mode, Enabled: true, ApplyActions: applyActions}
}

// MetaControllerFromPolicy builds a controller from a policy map.
func MetaControllerFromPolicy(policy map[string]any) *MetaController {
	return NewMetaController(MetacognitionMode(policy), false, allowedActions(policy))
}

// ShouldApply reports whether the action is applied (mode is apply and the
// action is allow-listed).
func (c *MetaController) ShouldApply(action MetaAction) bool {
	if c.Mode != ModeApply {
		return false
	}
	_, ok := c.ApplyActions[action]
	return ok
}

// PlanNextInput carries the snapshots + regulation context for PlanNext.
type PlanNextInput struct {
	MonitorInput
	TargetFlow   *string
	TargetStep   *string
	CurrentLobes []string
}

// PlanNext runs monitor → regulate.
func (c *MetaController) PlanNext(in PlanNextInput) MetaDecision {
	observations := Monitor(in.MonitorInput)
	return Regulate(observations, RegulateInput{
		TargetFlow:   in.TargetFlow,
		TargetStep:   in.TargetStep,
		CurrentLobes: in.CurrentLobes,
	})
}

func toStr(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "True"
		}
		return "False"
	}
	return ""
}

func truthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x != ""
	case int:
		return x != 0
	case float64:
		return x != 0
	case nil:
		return false
	}
	return true
}
