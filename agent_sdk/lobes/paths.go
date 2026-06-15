package lobes

import "github.com/mezon/agent-sdk-go/agent_sdk/core/spec"

// taskishPaths / infoishPaths — previous-turn path classes (paths/common.py).
var infoishPaths = map[string]struct{}{"qna": {}, "research": {}, "clarify": {}}

// ── Named-path recognizers (free, deterministic) ─────────────────────────────
// Ported from agent_sdk/paths/{qna,research,clarify,relational,onboarding}.py.

func recognizeQna(ctx map[string]any) float64 { return qnaScore(ctx) }

func recognizeResearch(ctx map[string]any) float64 {
	query := ctxStr(ctx, "query")
	if firedPromptRE.MatchString(query) {
		return 0.0
	}
	if reminderRE.MatchString(query) || mutationRE.MatchString(query) || isRecurringSchedule(query) {
		return 0.0
	}
	score := 0.0
	if comparativeRE.MatchString(query) {
		score += 0.7
	}
	if wordCount(query) >= 15 {
		score += 0.3
	}
	return score
}

func recognizeClarify(ctx map[string]any) float64 {
	query := ctxStr(ctx, "query")
	if !ctxBool(ctx, "has_history") {
		return 0.0
	}
	if reminderRE.MatchString(query) || mutationRE.MatchString(query) || firedPromptRE.MatchString(query) {
		return 0.0
	}
	if softCancelRE.MatchString(query) {
		return 0.0
	}
	score := 0.0
	if anaphoraRE.MatchString(query) {
		score += 0.5
	}
	if wordCount(query) <= 8 {
		score += 0.3
	}
	if interrogativeRE.MatchString(query) {
		score += 0.2
	}
	if _, ok := infoishPaths[ctxStr(ctx, "prev_path")]; ok {
		score += 0.15
	}
	if score < 0.5 {
		return 0.0
	}
	if score > 1.0 {
		return 1.0
	}
	return score
}

func recognizeRelational(ctx map[string]any) float64 {
	query := ctxStr(ctx, "query")
	isGreeting := greetingRE.MatchString(query)
	isSelfref := selfRefRE.MatchString(query) && wordCount(query) <= 12
	if !(isGreeting || isSelfref) {
		return 0.0
	}
	if isGreeting && !isSelfref && interrogativeRE.MatchString(query) && wordCount(query) > 6 {
		return 0.0
	}
	if wordCount(query) <= 8 {
		return 0.9
	}
	return 0.5
}

func recognizeOnboarding(ctx map[string]any) float64 {
	if !ctxBool(ctx, "config_mode") {
		return 0.0
	}
	if firedPromptRE.MatchString(ctxStr(ctx, "query")) {
		return 0.0
	}
	return 1.0
}

// ProductionPaths returns the 5 built-in production path recognizers, in
// recognition order (paths.PATHS). Each path BIASES its member lobes when
// recognized; it never hard-gates.
func ProductionPaths() []spec.Path {
	return []spec.Path{
		{
			Name:       "qna",
			Recognizer: recognizeQna,
			Members:    []string{"synthesize", "cite", "filter"},
			Bias:       map[string]float64{"synthesize": 0.1},
			Threshold:  0.5,
			Grounds:    true,
		},
		{
			Name:       "research",
			Recognizer: recognizeResearch,
			Members:    []string{"classify", "plan", "research", "synthesize", "cite", "filter"},
			Bias:       map[string]float64{"classify": 0.2, "plan": 0.2, "research": 0.2},
			Threshold:  0.5,
			Grounds:    true,
		},
		{
			Name:       "clarify",
			Recognizer: recognizeClarify,
			Members:    []string{"condense", "scope_check", "synthesize"},
			Bias:       map[string]float64{"condense": 0.25},
			Threshold:  0.5,
			Grounds:    false,
		},
		{
			Name:       "relational",
			Recognizer: recognizeRelational,
			Members:    []string{"synthesize"},
			Bias:       map[string]float64{},
			Threshold:  0.5,
			Grounds:    false,
		},
		{
			Name:       "onboarding",
			Recognizer: recognizeOnboarding,
			Members:    []string{"skill_select", "skill_active", "task_state"},
			Bias:       map[string]float64{"skill_select": 0.2, "skill_active": 0.2, "task_state": 0.1},
			Threshold:  0.5,
			Grounds:    false,
		},
	}
}
