// Ported from agent_sdk/skills/prompt.py — how a skill's information enters the
// reasoning context.
//
// BuildPromptBlock renders the per-stage skill section: eager skills inline their
// full instructions; on-demand skills contribute a one-line index entry plus the
// (deliberately pushy) ActivateSkill directive. With skill_strategy == "adaptive"
// the on-demand index is ranked by relevance and trimmed.
package skills

import (
	"math"
	"sort"
	"strings"

	"github.com/nccasia/agent-sdk-go/agent_sdk/core/attention"
)

// onDemandDirective is deliberately pushy AND lifecycle-teaching: a soft
// phrasing makes the model skip activation; without the search→section guidance
// it blind-activates then reads whole files. One directive, both lessons.
const onDemandDirective = "IMPORTANT: if a skill matches the request, you MUST call `ActivateSkill(slug)` " +
	"once to load it, then follow its steps — do not work from the summary. If it " +
	"has reference files, use `skill.search(query)` to find the relevant section, " +
	"then `skill.read(file, section=…)` — never read whole files."

// skillMinKeep is the high-recall floor: never trim below this many top-ranked
// on_demand skills.
const skillMinKeep = 3

// RankingRow is one on_demand skill's scoring record for the inspector.
type RankingRow struct {
	Label      string
	L1         float64
	L2         float64
	Activation float64
	Kept       bool
}

// PromptOptions carries the optional inputs to BuildPromptBlock.
type PromptOptions struct {
	Query       string
	QVec        []float64
	EmbedOne    func(string) []float64
	RankingOut  *[]RankingRow
	ActiveSlugs []string // nil ⇒ no reason-narrowing
	SkillsInUse []string
	hasActive   bool
}

// WithActiveSlugs marks the reason-narrowing active-slug set (use even for an
// empty set to signal narrowing is in effect).
func (o PromptOptions) WithActiveSlugs(slugs []string) PromptOptions {
	o.ActiveSlugs = slugs
	o.hasActive = true
	return o
}

// BuildPromptBlock renders the per-stage skill section of the system prompt.
func BuildPromptBlock(registry *SkillRegistry, policy map[string]any, stageID string, opts PromptOptions) string {
	packs := registry.ActiveForStage(policy, stageID)

	if opts.hasActive || opts.ActiveSlugs != nil {
		keep := map[string]struct{}{}
		for _, s := range opts.ActiveSlugs {
			keep[s] = struct{}{}
		}
		var scoped []*SkillPack
		for _, p := range packs {
			if _, ok := keep[p.ID]; ok {
				scoped = append(scoped, p)
				continue
			}
			if _, ok := keep[p.Name]; ok && p.Name != "" {
				scoped = append(scoped, p)
			}
		}
		if len(scoped) > 0 {
			packs = scoped
		}
	}

	inUse := map[string]struct{}{}
	for _, s := range opts.SkillsInUse {
		inUse[s] = struct{}{}
	}

	var eager []string
	var onDemand []onDemandEntry
	for _, pack := range packs {
		if _, ok := inUse[pack.ID]; ok {
			continue
		}
		if _, ok := inUse[pack.Name]; ok && pack.Name != "" {
			continue
		}
		if pack.Injection == "on_demand" {
			label := pack.Name
			if label == "" {
				label = pack.ID
			}
			desc := pack.Description
			if desc == "" {
				desc = "(no description)"
			}
			onDemand = append(onDemand, onDemandEntry{label, desc})
		} else {
			eager = append(eager, pack.Instructions)
		}
	}

	adaptive := strOrAny(policy["skill_strategy"], "static") == "adaptive"
	if adaptive && opts.Query != "" && len(onDemand) > 0 {
		kept := rankOnDemand(onDemand2pairs(onDemand), opts, policy)
		var trimmed []onDemandEntry
		for _, p := range kept {
			trimmed = append(trimmed, onDemandEntry{p[0], p[1]})
		}
		onDemand = trimmed
	} else if opts.RankingOut != nil {
		for _, e := range onDemand {
			*opts.RankingOut = append(*opts.RankingOut, RankingRow{Label: e.label, Kept: true})
		}
	}

	index := make([]string, 0, len(onDemand))
	for _, e := range onDemand {
		index = append(index, "- "+e.label+": "+e.desc)
	}
	parts := append([]string(nil), eager...)
	if len(index) > 0 {
		parts = append(parts, "Available skills:\n"+strings.Join(index, "\n")+"\n"+onDemandDirective)
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "\n\n")
}

// onDemandEntry is the (label, desc) record for an on-demand skill in the index.
type onDemandEntry struct{ label, desc string }

func onDemand2pairs(in []onDemandEntry) [][2]string {
	out := make([][2]string, len(in))
	for i, e := range in {
		out[i] = [2]string{e.label, e.desc}
	}
	return out
}

// rankOnDemand ranks (label, desc) entries by relevance and returns the kept
// ones in original registry order. High-recall: keep ≥ floor, always ≥ top-K.
func rankOnDemand(onDemand [][2]string, opts PromptOptions, policy map[string]any) [][2]string {
	floor := floatOr(policy["skill_min_activation"], 0.2)
	weights := attention.MergeWeights(floatMap(policy["skill_weights"]))

	type scored struct {
		idx         int
		label       string
		desc        string
		l1, l2, act float64
	}
	all := make([]scored, len(onDemand))
	for i, e := range onDemand {
		var textVec []float64
		if opts.EmbedOne != nil {
			textVec = opts.EmbedOne(e[0] + " " + e[1])
		}
		r := attention.ScoreText(opts.Query, opts.QVec, e[0]+" "+e[1], textVec, weights, 0.0)
		all[i] = scored{i, e[0], e[1], r.L1, r.L2, r.Activation}
	}
	byRank := append([]scored(nil), all...)
	sort.SliceStable(byRank, func(a, b int) bool { return byRank[a].act > byRank[b].act })
	keep := map[int]struct{}{}
	for rank, s := range byRank {
		if s.act >= floor || rank < skillMinKeep {
			keep[s.idx] = struct{}{}
		}
	}
	if opts.RankingOut != nil {
		for _, s := range all {
			_, kept := keep[s.idx]
			*opts.RankingOut = append(*opts.RankingOut, RankingRow{
				Label:      s.label,
				L1:         round3(s.l1),
				L2:         round3(s.l2),
				Activation: round3(s.act),
				Kept:       kept,
			})
		}
	}
	var out [][2]string
	for i, e := range onDemand {
		if _, ok := keep[i]; ok {
			out = append(out, e)
		}
	}
	return out
}

// ResolveSkillInstructions is builtin-only resolution — kept for callers without
// a DB-backed registry.
func ResolveSkillInstructions(policy map[string]any, stageID string) string {
	return BuildPromptBlock(NewSkillRegistry(nil), policy, stageID, PromptOptions{})
}

func strOrAny(v any, def string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return def
}

func floatOr(v any, def float64) float64 {
	switch t := v.(type) {
	case float64:
		if t == 0 {
			return def
		}
		return t
	case int:
		if t == 0 {
			return def
		}
		return float64(t)
	}
	return def
}

func floatMap(v any) map[string]float64 {
	m, ok := v.(map[string]float64)
	if ok {
		return m
	}
	if mm, ok := v.(map[string]any); ok {
		out := map[string]float64{}
		for k, val := range mm {
			if f, ok := val.(float64); ok {
				out[k] = f
			}
		}
		return out
	}
	return nil
}

func round3(x float64) float64 {
	return math.Round(x*1000) / 1000
}
