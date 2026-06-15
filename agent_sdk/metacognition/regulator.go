package metacognition

import (
	"sort"
	"strings"

	"github.com/mezon/agent-sdk-go/agent_sdk/contracts"
)

// TrimmableLobes are the optional recall/skill lobes a context-tight regulation
// drops first. Ported 1:1 from regulator._TRIMMABLE_LOBES.
var TrimmableLobes = map[string]struct{}{
	"skill_select":   {},
	"skill_active":   {},
	"memory_recall":  {},
	"session_recall": {},
	"ctxvar_resolve": {},
}

// pinnedStep reports whether a step name coincides with a pinned lobe id
// (cite/filter) — a pinned step is never skippable.
func pinnedStep(name string) bool {
	return contracts.IsPinned(name)
}

func splitTarget(target string) (flow, step string) {
	if !strings.Contains(target, ".") {
		return "", ""
	}
	parts := strings.SplitN(target, ".", 2)
	return parts[0], parts[1]
}

// RegulateInput carries the regulation context (mirrors the keyword args).
type RegulateInput struct {
	TargetFlow   *string
	TargetStep   *string
	CurrentLobes []string
}

// Regulate turns observations into a next-thinking decision. The decision table
// is ported 1:1 from regulator.regulate.
func Regulate(observations []MetaObservation, in RegulateInput) MetaDecision {
	if len(observations) == 0 {
		return MetaDecision{
			Action:      ActionContinue,
			TargetFlow:  in.TargetFlow,
			TargetStep:  in.TargetStep,
			TargetLobes: in.CurrentLobes,
			Reason:      "object-level state is healthy",
			Confidence:  1.0,
		}
	}

	// queue: observations sorted by (-severity, id).
	sorted := make([]MetaObservation, len(observations))
	copy(sorted, observations)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Severity != sorted[j].Severity {
			return sorted[i].Severity > sorted[j].Severity
		}
		return sorted[i].ID < sorted[j].ID
	})
	queue := make([]MetaQueueItem, 0, len(sorted))
	for _, o := range sorted {
		reason := o.Detail
		if reason == "" {
			reason = o.Kind
		}
		queue = append(queue, MetaQueueItem{Target: o.Target, Reason: reason, Priority: o.Severity})
	}

	// by_kind: first observation per kind, in iteration order (dict semantics:
	// last write wins). Python builds {obs.kind: obs} over the original order, so
	// the LAST observation of a kind wins.
	byKind := map[string]MetaObservation{}
	for _, o := range observations {
		byKind[o.Kind] = o
	}

	if obs, ok := byKind["low_confidence_path"]; ok {
		return MetaDecision{
			Action:       ActionMetaReview,
			TargetFlow:   in.TargetFlow,
			TargetStep:   in.TargetStep,
			TargetLobes:  in.CurrentLobes,
			Reason:       obs.Detail,
			Confidence:   0.45,
			Queue:        queue,
			Observations: observations,
		}
	}

	if obs, ok := byKind["context_tight"]; ok {
		flow, step := splitTarget(obs.Target)
		narrowed := narrow(in.CurrentLobes)
		if len(narrowed) > 0 && !equalStrings(narrowed, in.CurrentLobes) {
			return MetaDecision{
				Action:       ActionAdjustLobeSlice,
				TargetFlow:   firstFlow(flow, in.TargetFlow),
				TargetStep:   firstStep(step, in.TargetStep),
				TargetLobes:  narrowed,
				Reason:       "context window is tight; trim optional recall/skill lobes for this step",
				Confidence:   0.8,
				Queue:        queue,
				Observations: observations,
			}
		}
	}

	if obs, ok := byKind["empty_lobe_slice"]; ok {
		flow, step := splitTarget(obs.Target)
		if pinnedStep(step) {
			return MetaDecision{
				Action:      ActionMetaReview,
				TargetFlow:  firstFlow(flow, in.TargetFlow),
				TargetStep:  firstStep(step, in.TargetStep),
				TargetLobes: in.CurrentLobes,
				Reason: "pinned step has an empty lobe slice; cite/filter are never " +
					"skippable — needs review",
				Confidence:   0.4,
				Queue:        queue,
				Observations: observations,
			}
		}
		return MetaDecision{
			Action:       ActionSkipStep,
			TargetFlow:   firstFlow(flow, in.TargetFlow),
			TargetStep:   firstStep(step, in.TargetStep),
			TargetLobes:  in.CurrentLobes,
			Reason:       obs.Detail,
			Confidence:   0.75,
			Queue:        queue,
			Observations: observations,
		}
	}

	if obs, ok := byKind["empty_step_context"]; ok {
		flow, step := splitTarget(obs.Target)
		return MetaDecision{
			Action:       ActionRetryStep,
			TargetFlow:   firstFlow(flow, in.TargetFlow),
			TargetStep:   firstStep(step, in.TargetStep),
			TargetLobes:  in.CurrentLobes,
			Reason:       obs.Detail,
			Confidence:   0.65,
			Queue:        queue,
			Observations: observations,
		}
	}

	return MetaDecision{
		Action:       ActionContinue,
		TargetFlow:   in.TargetFlow,
		TargetStep:   in.TargetStep,
		TargetLobes:  in.CurrentLobes,
		Reason:       "observations do not require regulation",
		Confidence:   0.9,
		Queue:        queue,
		Observations: observations,
	}
}

func narrow(lobes []string) []string {
	out := make([]string, 0, len(lobes))
	for _, l := range lobes {
		if _, trim := TrimmableLobes[l]; !trim {
			out = append(out, l)
		}
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// firstFlow returns &flow when flow != "", else the fallback pointer (Python:
// `flow or target_flow`).
func firstFlow(flow string, fallback *string) *string {
	if flow != "" {
		return strptr(flow)
	}
	return fallback
}

func firstStep(step string, fallback *string) *string {
	if step != "" {
		return strptr(step)
	}
	return fallback
}
