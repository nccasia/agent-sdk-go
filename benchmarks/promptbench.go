package benchmarks

import (
	"context"
	"fmt"
	"regexp"
	"sort"

	"github.com/nccasia/agent-sdk-go/agent_sdk/agent"
	"github.com/nccasia/agent-sdk-go/agent_sdk/clients"
	"github.com/nccasia/agent-sdk-go/agent_sdk/cognition"
	"github.com/nccasia/agent-sdk-go/agent_sdk/expression"
	"github.com/nccasia/agent-sdk-go/agent_sdk/plugins/format"
	"github.com/nccasia/agent-sdk-go/agent_sdk/plugins/safety"
	"github.com/nccasia/agent-sdk-go/agent_sdk/probe"
)

// promptbench — the gate for the SDK's PROMPTS: are they well-structured AND
// well-written? FREE / deterministic — no provider. Two free tiers:
//
//  1. structure — the composed system prompt is well-layered: a stable
//     instruction prefix leads and the turn-volatile sections form a contiguous
//     tail (the cache-prefix boundary); identity appears once and first; the
//     <env>/datetime tail is last; no section/persona/conversation duplication.
//     Read off each probe stage's system_segments (no LLM).
//  2. quality — a rule-based lint of the SDK's authored prompt constants against
//     the prompt-engineering best practices: one role only, no double negatives,
//     an explicit output/action directive, no ALL-CAPS shouting, bounded length.
//     Emits a per-prompt quality score (quality_avg).
//
// The judge tier (run.py --live) is provider-gated; this Go port covers the two
// deterministic free tiers, which gate READY without a provider. Ported from
// benchmarks/promptbench/run.py.
//
// Deviation: the Python _authored_prompts collects whatever the surface ships
// and "a missing one is skipped (the surface evolves)". The Go port has not
// ported the rag cite SYSTEM_PROMPT nor the planning _PLAN_PROMPT, so those two
// quality rows are absent here (10 prompts vs Python's 12); every collected
// prompt is clean, so the verdict status (READY) matches the baseline.

// pbScenarios — one probed turn each; the same four the Python bench probes.
var pbScenarios = [][2]string{
	{"qna", "what is the capital of France?"},
	{"research", "compare React and Vue in depth and cite sources"},
	{"clarify", "what about that one?"},
	{"relational", "hello there!"},
}

// pbSingleton — the named sources that must appear at most once per stage.
var pbSingleton = map[string]struct{}{
	"instructions": {}, "memory_directive": {}, "stage_prompt": {}, "tools": {},
	"skills": {}, "grounding": {}, "datetime": {}, "respond": {},
}

// pbStab mirrors the Python _STAB ranking (stable < slow < turn < volatile).
var pbStab = map[string]int{"stable": 0, "slow": 1, "turn": 2, "volatile": 3}

var (
	pbPersonaRe   = regexp.MustCompile(`(?i)\byou are\b`)
	pbDoubleNegRe = regexp.MustCompile(`(?i)\b(never|not|cannot|don't|do not)\b[^.\n]*\b(without|unless|un\w+)\b`)
	pbDirectiveRe = regexp.MustCompile(`(?i)\b(output|respond|return|answer|write|rewrite|classify|verify|summari|produce|reply|drop|keep|use|apply|plan)\b`)
	pbAllcapsRe   = regexp.MustCompile(`\b[A-Z]{4,}\b`)
)

// pbAuthoredPrompts collects the shipped prompt constants under evaluation. A
// missing one is skipped (the surface evolves) — mirrors _authored_prompts.
// Ordered to match the Python collection order.
func pbAuthoredPrompts() [][2]string {
	add := func(out [][2]string, name, text string) [][2]string {
		if text != "" {
			return append(out, [2]string{name, text})
		}
		return out
	}
	var out [][2]string
	out = add(out, "synthesize.SYSTEM", cognition.SynthesizeSystemPrompt)
	out = add(out, "synthesize.SIMPLE", cognition.SynthesizeSimpleSystemPrompt)
	out = add(out, "respond.SYSTEM", expression.RespondSystemPrompt)
	// cite.SYSTEM — not ported to Go (rag cite carries no SYSTEM_PROMPT constant).
	out = add(out, "filter.SYSTEM", safety.FilterSystemPrompt)
	out = add(out, "format.SYSTEM", format.FormatSystemPrompt)
	out = add(out, "classify.SYSTEM", cognition.ClassifySystemPrompt)
	out = add(out, "condense.SYSTEM", cognition.CondenseSystemPrompt)
	out = add(out, "plan.SYSTEM", cognition.PlanSystemPrompt)
	out = add(out, "research.SYSTEM", cognition.ResearchSystemPrompt)
	out = add(out, "memory_directive", agent.MemoryDirective)
	// plan_prompt — not ported to Go (planning plugin _PLAN_PROMPT absent).
	return out
}

// pbProbeRecords runs every promptbench scenario through probe.Probe and
// returns the raw records (reused by the structure tier and the viewer probe
// closure). Offline-deterministic via FakeClient.
func pbProbeRecords(ctx context.Context) ([]*probe.Record, error) {
	var out []*probe.Record
	for _, sc := range pbScenarios {
		a := agent.MustPreactAgent(agent.Config{
			Client:       clients.NewFakeClient([]any{"ok", "ok", "ok", "ok", "ok", "ok", "ok", "ok"}, nil),
			Instructions: "You are a helpful research assistant.",
		})
		rec, err := probe.Probe(ctx, a, sc[1], probe.WithLabel(sc[0]))
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

// RunPromptBenchProbes runs a small (3) representative subset of the promptbench
// scenarios through probe.Probe and returns the records, so the viewer
// inspection shows a real path/flow + each stage's system_prompt/segments (the
// same kind of segments the structure tier lints). Offline-deterministic via
// FakeClient when model=="". Kept to 3 (a qna, a deep research, a clarify
// follow-up) so cmd/bench stays fast.
func RunPromptBenchProbes(ctx context.Context, _ string) ([]*probe.Record, error) {
	recs, err := pbProbeRecords(ctx)
	if err != nil {
		return nil, err
	}
	if len(recs) > 3 {
		recs = recs[:3]
	}
	return recs, nil
}

// pbProbeStages drives one probed turn per scenario and flattens to
// (label, stage) pairs — the structure tier reads off these.
func pbProbeStages(ctx context.Context) ([][2]any, error) {
	recs, err := pbProbeRecords(ctx)
	if err != nil {
		return nil, err
	}
	var out [][2]any
	for i, rec := range recs {
		sc := pbScenarios[i]
		for _, st := range rec.Stages {
			stage, _ := st["stage"].(string)
			out = append(out, [2]any{sc[0] + "/" + stage, st})
		}
	}
	return out, nil
}

// pbSegments coerces a stage's system_segments to a slice of maps (the probe
// emits []map[string]any; tolerate the JSON-able []any shape too).
func pbSegments(st map[string]any) []map[string]any {
	if segs, ok := st["system_segments"].([]map[string]any); ok {
		return segs
	}
	if raw, ok := st["system_segments"].([]any); ok {
		out := make([]map[string]any, 0, len(raw))
		for _, e := range raw {
			if m, ok := e.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

func pbSegInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	}
	return 0
}

// pbRunStructure — the layering property over every probed stage's segments.
func pbRunStructure(stages [][2]any) *ModePayload {
	order := []string{
		"identity.once", "identity.single_persona", "dedup.source_unique",
		"ordering.identity_first", "ordering.env_last", "ordering.volatile_tail",
		"convo.not_duplicated", "coverage.valid_offsets",
	}
	rules := map[string][]string{}
	for _, id := range order {
		rules[id] = []string{}
	}
	nSegs := 0
	for _, pair := range stages {
		label := pair[0].(string)
		st := pair[1].(map[string]any)
		segs := pbSegments(st)
		text, _ := st["system_prompt"].(string)
		sources := make([]string, len(segs))
		for i, s := range segs {
			sources[i], _ = s["source"].(string)
		}
		nSegs += len(segs)

		if pbCount(sources, "instructions") > 1 {
			rules["identity.once"] = append(rules["identity.once"], label)
		}
		if len(pbPersonaRe.FindAllString(text, -1)) > 1 {
			rules["identity.single_persona"] = append(rules["identity.single_persona"], label)
		}
		named := []string{}
		for _, s := range sources {
			if _, ok := pbSingleton[s]; ok {
				named = append(named, s)
			}
		}
		if !pbUnique(named) {
			rules["dedup.source_unique"] = append(rules["dedup.source_unique"], label)
		}
		if pbContains(sources, "instructions") && (len(sources) == 0 || sources[0] != "instructions") {
			rules["ordering.identity_first"] = append(rules["ordering.identity_first"], label)
		}
		if pbContains(sources, "datetime") && (len(sources) == 0 || sources[len(sources)-1] != "datetime") {
			rules["ordering.env_last"] = append(rules["ordering.env_last"], label)
		}
		tail := false
		for _, s := range segs {
			stab, _ := s["stability"].(string)
			rank, ok := pbStab[stab]
			if !ok {
				rank = 1
			}
			if rank >= 2 {
				tail = true
			} else if tail {
				rules["ordering.volatile_tail"] = append(rules["ordering.volatile_tail"], label)
				break
			}
		}
		if pbContains(sources, "conversation") {
			rules["convo.not_duplicated"] = append(rules["convo.not_duplicated"], label)
		}
		last := 0
		for _, s := range segs {
			start, end := pbSegInt(s["start"]), pbSegInt(s["end"])
			if start < last || end > len(text) || start >= end {
				rules["coverage.valid_offsets"] = append(rules["coverage.valid_offsets"], label)
				break
			}
			last = end
		}
	}
	checks := make([]Check, 0, len(order))
	for _, id := range order {
		bad := rules[id]
		detail := "clean"
		if len(bad) > 0 {
			shown := bad
			if len(shown) > 4 {
				shown = shown[:4]
			}
			detail = fmt.Sprintf("%d bad: %v", len(bad), shown)
		}
		checks = append(checks, ck(id, len(bad) == 0, detail))
	}
	return NewPayload(checks, map[string]any{"stages": len(stages), "segments": nSegs})
}

// pbLint returns the quality rules a prompt VIOLATES (empty = clean).
func pbLint(text string) []string {
	bad := []string{}
	if len(pbPersonaRe.FindAllString(text, -1)) > 1 {
		bad = append(bad, "multi_role")
	}
	if pbDoubleNegRe.MatchString(text) {
		bad = append(bad, "double_negative")
	}
	if !pbDirectiveRe.MatchString(text) {
		bad = append(bad, "no_directive")
	}
	if len(pbDistinct(pbAllcapsRe.FindAllString(text, -1))) > 6 {
		bad = append(bad, "allcaps_shouting")
	}
	if len(text) > 3000 {
		bad = append(bad, "too_long")
	}
	return bad
}

// pbRunQuality — the rule-based lint of the authored prompt constants.
func pbRunQuality(prompts [][2]string) *ModePayload {
	checks := make([]Check, 0, len(prompts))
	scoreSum := 0.0
	for _, p := range prompts {
		bad := pbLint(p[1])
		scoreSum += 1 - float64(len(bad))/5
		detail := "clean"
		if len(bad) > 0 {
			detail = fmt.Sprintf("violates %v", bad)
		}
		checks = append(checks, ck("quality."+p[0], len(bad) == 0, detail))
	}
	avg := 0.0
	if len(prompts) > 0 {
		avg = round3(scoreSum / float64(len(prompts)))
	}
	return NewPayload(checks, map[string]any{"prompts": len(prompts), "quality_avg": avg})
}

// RunPromptBench composes the promptbench verdict (deterministic free floor:
// structure + quality). Ported from benchmarks/promptbench/run.py (the --live
// judge tier is provider-gated and not part of the free floor).
func RunPromptBench(ctx context.Context, _ string) (Verdict, error) {
	stages, err := pbProbeStages(ctx)
	if err != nil {
		return Verdict{}, err
	}
	payloads := map[string]*ModePayload{
		"structure": pbRunStructure(stages),
		"quality":   pbRunQuality(pbAuthoredPrompts()),
	}
	return ComposeVerdict(payloads, map[string][]string{"quality": {"quality_avg"}}), nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func pbCount(xs []string, x string) int {
	n := 0
	for _, v := range xs {
		if v == x {
			n++
		}
	}
	return n
}

func pbContains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func pbUnique(xs []string) bool {
	seen := map[string]struct{}{}
	for _, x := range xs {
		if _, ok := seen[x]; ok {
			return false
		}
		seen[x] = struct{}{}
	}
	return true
}

func pbDistinct(xs []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, x := range xs {
		out[x] = struct{}{}
	}
	return out
}

var _ = sort.Strings
