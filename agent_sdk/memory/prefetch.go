package memory

import "context"

// ScopeOrder is the rendered prefetch order: broad → specific. The attention
// builder boosts conversation > channel > user > bot, so the most specific fact
// still wins on conflict regardless of this order.
var ScopeOrder = []string{"bot", "user", "channel", "conversation"}

func orderedScopes(scopes []string) []string {
	have := map[string]struct{}{}
	for _, s := range scopes {
		have[s] = struct{}{}
	}
	var known []string
	for _, s := range ScopeOrder {
		if _, ok := have[s]; ok {
			known = append(known, s)
		}
	}
	inOrder := map[string]struct{}{}
	for _, s := range ScopeOrder {
		inOrder[s] = struct{}{}
	}
	var rest []string
	for _, s := range scopes {
		if _, ok := inOrder[s]; !ok {
			rest = append(rest, s)
		}
	}
	return append(known, rest...)
}

// PrefetchHook is the always-on memory prefetch hook: it searches each scope for
// the turn query and renders the hits as [{scope, key, value, description?}].
type PrefetchHook func(ctx context.Context, query string, state any) ([]map[string]any, error)

// PrefetchOpts configures the prefetch hook.
type PrefetchOpts struct {
	Scopes           []string
	K                int
	ValueBudgetChars int
}

// MemoryPrefetchHook builds the always-on memory prefetch hook for a Memory.
// Over-budget values are cleared and flagged "recall to read" so they surface as
// Tier-2 hints, not Tier-1 bodies.
func MemoryPrefetchHook(memory *Memory, opts PrefetchOpts) PrefetchHook {
	k := opts.K
	if k == 0 {
		k = 5
	}
	budget := opts.ValueBudgetChars
	if budget == 0 {
		budget = 1200
	}
	scopes := opts.Scopes
	if len(scopes) == 0 {
		scopes = memory.Scopes
	}
	scopeList := orderedScopes(scopes)

	return func(ctx context.Context, query string, _ any) ([]map[string]any, error) {
		items := []map[string]any{}
		for _, scope := range scopeList {
			found, err := memory.Search(ctx, scope, query, k)
			if err != nil {
				continue
			}
			for _, it := range found {
				items = append(items, map[string]any{
					"scope": it.Scope,
					"key":   it.Key,
					"value": it.Value,
				})
			}
		}
		used := 0
		for _, d := range items {
			v := ""
			if d["value"] != nil {
				v = stringify(d["value"])
			}
			if used+len(v) > budget {
				d["description"] = stringify(d["key"]) + " (recall to read)"
				d["value"] = ""
			} else {
				used += len(v)
			}
		}
		return items, nil
	}
}
