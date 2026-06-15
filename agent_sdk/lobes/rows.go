package lobes

import "regexp"

// CompileRowSignals compiles a declarative signal spec (a registry row's
// "signals" value) into a deterministic extractor. Supported entries, keyed by
// signal name:
//
//	{"regex": "<pattern>"}   1.0 when the pattern matches ctx["query"]
//	{"flag": "<ctx key>"}    1.0 when ctx[key] is truthy
//	{"const": <float>}       a constant
//
// Rows carry data, never code — a new capability is a row, not a branch. Ported
// from agent_sdk/lobes/rows.py:compile_row_signals.
func CompileRowSignals(spec map[string]any) func(map[string]any) map[string]float64 {
	type entry struct {
		name string
		fn   func(map[string]any) float64
	}
	var entries []entry
	for name, raw := range spec {
		rule, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		entries = append(entries, entry{name, compileRule(rule)})
	}
	return func(ctx map[string]any) map[string]float64 {
		out := make(map[string]float64, len(entries))
		for _, e := range entries {
			out[e.name] = e.fn(ctx)
		}
		return out
	}
}

// CompileRowRecognizer compiles a declarative path recognizer row
// ({"regex": …} | {"flag": …}). Ported from rows.py:compile_row_recognizer.
func CompileRowRecognizer(rule map[string]any) func(map[string]any) float64 {
	if rule == nil {
		return func(map[string]any) float64 { return 0.0 }
	}
	if pat, ok := rule["regex"].(string); ok {
		re := regexp.MustCompile("(?i)" + pat)
		return func(ctx map[string]any) float64 {
			if re.MatchString(ctxStr(ctx, "query")) {
				return 1.0
			}
			return 0.0
		}
	}
	if key, ok := rule["flag"].(string); ok {
		return func(ctx map[string]any) float64 {
			if ctxBool(ctx, key) {
				return 1.0
			}
			return 0.0
		}
	}
	return func(map[string]any) float64 { return 0.0 }
}

func compileRule(rule map[string]any) func(map[string]any) float64 {
	if pat, ok := rule["regex"].(string); ok {
		re := regexp.MustCompile("(?i)" + pat)
		return func(ctx map[string]any) float64 {
			if re.MatchString(ctxStr(ctx, "query")) {
				return 1.0
			}
			return 0.0
		}
	}
	if key, ok := rule["flag"].(string); ok {
		return func(ctx map[string]any) float64 {
			if ctxBool(ctx, key) {
				return 1.0
			}
			return 0.0
		}
	}
	c := 0.0
	if v, ok := asFloat(rule["const"]); ok {
		c = v
	}
	return func(map[string]any) float64 { return c }
}

func asFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	}
	return 0, false
}
