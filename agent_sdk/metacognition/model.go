// Package metacognition is the meta level over the object-level OX/OY engine:
// lobes optimize context, flow stages optimize progressive execution, and the
// meta layer monitors both before deciding what to think about next. All of the
// decision logic here is PURE — the tables are ported 1:1 from
// agent_sdk/metacognition/{model,monitor,regulator,controller}.py and
// agent_sdk/metacognition_facade.py.
package metacognition

import "strings"

// MetaAction is one of the canonical meta-level moves.
type MetaAction = string

// The MetaAction values (Literal in Python).
const (
	ActionContinue         MetaAction = "continue"
	ActionAdjustLobeSlice  MetaAction = "adjust_lobe_slice"
	ActionRetryStep        MetaAction = "retry_step"
	ActionSkipStep         MetaAction = "skip_step"
	ActionAskClarification MetaAction = "ask_clarification"
	ActionMetaReview       MetaAction = "meta_review"
	ActionRedoPhase        MetaAction = "redo_phase"
	ActionGotoPhase        MetaAction = "goto_phase"
	ActionExpand           MetaAction = "expand"
)

// StatePlanKey is the scratchpad key the compiled state plan lives under.
const StatePlanKey = "state_plan"

// StatePlanStep is one scheduled structural state ({state, subject}).
type StatePlanStep struct {
	State   string  `json:"state"`
	Subject *string `json:"subject"`
}

// CompileStatePlan compiles a plan into a dynamic sequence of structural states
// (Layer 1). Pure + deterministic: each aspect becomes an "act" state scoped to
// that aspect's subject; >1 aspect adds a single "synthesize"; when the turn
// grounds, the pinned "cite" then "filter" states are appended (never dropped).
// One aspect (or none) degrades to a single "act".
//
// aspects elements may be strings or maps carrying question/subject/id.
func CompileStatePlan(aspects []any, grounds bool) []StatePlanStep {
	var subjects []string
	for _, a := range aspects {
		switch v := a.(type) {
		case string:
			subjects = appendNonEmpty(subjects, strings.TrimSpace(v))
		case map[string]any:
			s := firstString(v, "question", "subject", "id")
			subjects = appendNonEmpty(subjects, strings.TrimSpace(s))
		}
	}

	var plan []StatePlanStep
	if len(subjects) <= 1 {
		var subj *string
		if len(subjects) == 1 {
			subj = &subjects[0]
		}
		plan = append(plan, StatePlanStep{State: "act", Subject: subj})
	} else {
		for i := range subjects {
			s := subjects[i]
			plan = append(plan, StatePlanStep{State: "act", Subject: &s})
		}
		plan = append(plan, StatePlanStep{State: "synthesize", Subject: nil})
	}
	if grounds {
		plan = append(plan, StatePlanStep{State: "cite", Subject: nil})
		plan = append(plan, StatePlanStep{State: "filter", Subject: nil})
	}
	return plan
}

func appendNonEmpty(xs []string, s string) []string {
	if s != "" {
		return append(xs, s)
	}
	return xs
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// MetaObservation is one monitor finding.
type MetaObservation struct {
	ID       string  `json:"id"`
	Kind     string  `json:"kind"`
	Target   string  `json:"target"`
	Severity float64 `json:"severity"`
	Detail   string  `json:"detail"`
}

// ToPayload renders the observation as a wire-stable map.
func (o MetaObservation) ToPayload() map[string]any {
	return map[string]any{
		"id": o.ID, "kind": o.Kind, "target": o.Target,
		"severity": o.Severity, "detail": o.Detail,
	}
}

// MetaQueueItem is one prioritized follow-up target.
type MetaQueueItem struct {
	Target   string  `json:"target"`
	Reason   string  `json:"reason"`
	Priority float64 `json:"priority"`
}

// ToPayload renders the queue item as a wire-stable map.
func (q MetaQueueItem) ToPayload() map[string]any {
	return map[string]any{"target": q.Target, "reason": q.Reason, "priority": q.Priority}
}

// MetaDecision is the next-thinking decision. Zero value mirrors the Python
// default (action="continue", confidence 0; callers use NewMetaDecision for the
// confidence=1.0 default).
type MetaDecision struct {
	Action       MetaAction         `json:"action"`
	TargetFlow   *string            `json:"target_flow"`
	TargetStep   *string            `json:"target_step"`
	TargetLobes  []string           `json:"target_lobes"`
	WeightPatch  map[string]float64 `json:"weight_patch"`
	Reason       string             `json:"reason"`
	Confidence   float64            `json:"confidence"`
	Queue        []MetaQueueItem    `json:"queue"`
	Observations []MetaObservation  `json:"observations"`
}

// DefaultMetaDecision returns the Python field-default decision (action
// "continue", confidence 1.0, the disabled reason).
func DefaultMetaDecision() MetaDecision {
	return MetaDecision{
		Action:     ActionContinue,
		Reason:     "metacognition disabled or no regulation needed",
		Confidence: 1.0,
	}
}

// ToPayload renders the decision as a wire-stable map.
func (d MetaDecision) ToPayload() map[string]any {
	queue := make([]map[string]any, 0, len(d.Queue))
	for _, q := range d.Queue {
		queue = append(queue, q.ToPayload())
	}
	obs := make([]map[string]any, 0, len(d.Observations))
	for _, o := range d.Observations {
		obs = append(obs, o.ToPayload())
	}
	return map[string]any{
		"action":       d.Action,
		"target_flow":  derefOrNil(d.TargetFlow),
		"target_step":  derefOrNil(d.TargetStep),
		"target_lobes": orEmptyStrings(d.TargetLobes),
		"weight_patch": orEmptyFloats(d.WeightPatch),
		"reason":       d.Reason,
		"confidence":   d.Confidence,
		"queue":        queue,
		"observations": obs,
	}
}

// MetaState bundles the enabled flag, observations, and the decision.
type MetaState struct {
	Enabled      bool
	Observations []MetaObservation
	Decision     MetaDecision
}

// ToPayload renders the meta state as a wire-stable map.
func (s MetaState) ToPayload() map[string]any {
	obs := make([]map[string]any, 0, len(s.Observations))
	for _, o := range s.Observations {
		obs = append(obs, o.ToPayload())
	}
	return map[string]any{
		"enabled":      s.Enabled,
		"observations": obs,
		"decision":     s.Decision.ToPayload(),
	}
}

func strptr(s string) *string { return &s }

func derefOrNil(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func orEmptyStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func orEmptyFloats(m map[string]float64) map[string]float64 {
	if m == nil {
		return map[string]float64{}
	}
	return m
}
