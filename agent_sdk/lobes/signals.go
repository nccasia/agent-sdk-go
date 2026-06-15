package lobes

import "strings"

// ctxStr returns ctx[key] as a string ("" when absent or non-string).
func ctxStr(ctx map[string]any, key string) string {
	if v, ok := ctx[key].(string); ok {
		return v
	}
	return ""
}

// ctxBool returns whether ctx[key] is truthy (non-nil, non-empty, non-zero).
func ctxBool(ctx map[string]any, key string) bool {
	return truthy(ctx[key])
}

func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case int:
		return x != 0
	case float64:
		return x != 0
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	default:
		return true
	}
}

// stagesSet returns ctx["stages"] as a membership set.
func stagesSet(ctx map[string]any) map[string]struct{} {
	out := map[string]struct{}{}
	switch v := ctx["stages"].(type) {
	case []any:
		for _, s := range v {
			if str, ok := s.(string); ok {
				out[str] = struct{}{}
			}
		}
	case []string:
		for _, s := range v {
			out[s] = struct{}{}
		}
	}
	return out
}

// ── Per-lobe free signal extractors ──────────────────────────────────────────
// Ported from each domain lobe module's module-level signals(ctx) function.
// SignalFn is spec.SignalFn (ctx -> {name: value}).

// memoryRecallSignals — memory_enabled gates (parity); per-scope presence flags
// are tuning levers (weight 0 at parity).
func memoryRecallSignals(ctx map[string]any) map[string]float64 {
	enabled := true
	if v, ok := ctx["memory_enabled"]; ok {
		enabled = truthy(v)
	}
	out := map[string]float64{"memory_enabled": boolf(enabled)}
	scopes, _ := ctx["memory_scopes"].(map[string]any)
	for _, scope := range []string{"conversation", "channel", "user", "bot"} {
		v := false
		if scopes != nil {
			v = truthy(scopes[scope])
		}
		out["mem_"+scope] = boolf(v)
	}
	return out
}

// emptySignals — prior/edge-driven lobes that emit no own signals.
func emptySignals(map[string]any) map[string]float64 { return map[string]float64{} }

// skillSelectSignals fires on a declared-skills turn.
func skillSelectSignals(ctx map[string]any) map[string]float64 {
	return map[string]float64{"skill_select": boolf(ctxBool(ctx, "skills_declared"))}
}

// skillActiveSignals fires when skills are in use.
func skillActiveSignals(ctx map[string]any) map[string]float64 {
	return map[string]float64{"skill_active": boolf(ctxBool(ctx, "skills_in_use"))}
}

// toolSelectSignals fires only when the bot opts into adaptive tool exposure.
func toolSelectSignals(ctx map[string]any) map[string]float64 {
	return map[string]float64{"tool_select": boolf(ctxStr(ctx, "tool_strategy") == "adaptive")}
}

// scopeCheckSignals fires on the scope_gate flag.
func scopeCheckSignals(ctx map[string]any) map[string]float64 {
	return map[string]float64{"scope_check": boolf(ctxBool(ctx, "scope_gate"))}
}

// condenseSignals — gated on stage presence + history, then anaphora OR short.
func condenseSignals(ctx map[string]any) map[string]float64 {
	query := ctxStr(ctx, "query")
	stages := stagesSet(ctx)
	_, hasCondense := stages["condense"]
	eligible := hasCondense && ctxBool(ctx, "has_history")
	minTokens := 6
	if v, ok := asInt(ctx["condense_min_tokens"]); ok && v > 0 {
		minTokens = v
	}
	anaphora := 0.0
	if eligible && anaphoraRE.MatchString(query) {
		anaphora = 1.0
	}
	short := 0.0
	if eligible && wordCount(query) < minTokens {
		short = 1.0
	}
	return map[string]float64{
		"anaphora":    anaphora,
		"short_query": short,
		"has_history": boolf(ctxBool(ctx, "has_history")),
	}
}

// classifySignals — has_stage_classify gates the router; simple_shape inhibits.
func classifySignals(ctx map[string]any) map[string]float64 {
	stages := stagesSet(ctx)
	_, hasClassify := stages["classify"]
	return map[string]float64{
		"has_stage_classify": boolf(hasClassify),
		"simple_shape":       boolf(isSimpleShape(ctx)),
	}
}

// planSignals — route=="complex".
func planSignals(ctx map[string]any) map[string]float64 {
	return map[string]float64{"route_complex": boolf(ctxStr(ctx, "route") == "complex")}
}

// formatSignals — fixed_format flag.
func formatSignals(ctx map[string]any) map[string]float64 {
	return map[string]float64{"format": boolf(ctxBool(ctx, "fixed_format"))}
}

// respondSignals — pinned-style unconditional 1.0.
func respondSignals(map[string]any) map[string]float64 {
	return map[string]float64{"respond": 1.0}
}

// signalFns maps a production lobe id to its free signal extractor.
var signalFns = map[string]func(map[string]any) map[string]float64{
	"memory_recall":  memoryRecallSignals,
	"session_recall": emptySignals,
	"ctxvar_resolve": emptySignals,
	"skill_select":   skillSelectSignals,
	"skill_active":   skillActiveSignals,
	"tool_select":    toolSelectSignals,
	"condense":       condenseSignals,
	"scope_check":    scopeCheckSignals,
	"classify":       classifySignals,
	"plan":           planSignals,
	"research":       emptySignals,
	"synthesize":     emptySignals,
	"filter":         emptySignals,
	"format":         formatSignals,
	"respond":        respondSignals,
}

// SignalFor returns the free signal extractor for a production lobe id, or an
// empty-signal extractor for an unknown id. This is the OY perception seam the
// network package reattaches to the embedded structural rows.
func SignalFor(id string) func(map[string]any) map[string]float64 {
	if fn, ok := signalFns[id]; ok {
		return fn
	}
	return emptySignals
}

// isSimpleShape — classify-skip: a high-confidence SIMPLE query needs no LLM
// router. Conservative: only a strongly qna-shaped query (interrogative AND
// short, with every excluder) qualifies; anything ambiguous still pays.
func isSimpleShape(ctx map[string]any) bool {
	query := ctxStr(ctx, "query")
	if ctxBool(ctx, "fired_prompt") || firedPromptRE.MatchString(query) {
		return false
	}
	if qnaScore(ctx) < 0.8 {
		return false
	}
	// Referents may hide multi-hop work.
	if anaphoraRE.MatchString(query) {
		return false
	}
	return true
}

// qnaScore is the qna path recognizer's raw score (interrogative+0.6, short+0.2,
// with the comparative/fired/reminder/mutation/greeting excluders). is_simple_shape
// consumes this (classify-skip requires >= 0.8).
func qnaScore(ctx map[string]any) float64 {
	query := ctxStr(ctx, "query")
	if comparativeRE.MatchString(query) || firedPromptRE.MatchString(query) {
		return 0.0
	}
	if reminderRE.MatchString(query) || mutationRE.MatchString(query) {
		return 0.0
	}
	score := 0.0
	if interrogativeRE.MatchString(query) || infoRequestRE.MatchString(query) {
		score += 0.6
	}
	if wc := wordCount(query); wc > 0 && wc <= 14 {
		score += 0.2
	}
	if greetingRE.MatchString(query) && !interrogativeRE.MatchString(query) {
		return 0.0
	}
	return score
}

func boolf(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}

func asInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case float64:
		return int(x), true
	case string:
		x = strings.TrimSpace(x)
		_ = x
		return 0, false
	}
	return 0, false
}
