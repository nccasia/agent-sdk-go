// Ported from agent_sdk/skills/packs.py — skill packs & registry, the skill
// logic layer.
//
// SkillPack is the runtime shape of a skill (the ported contract the engine
// consumes); SkillRegistry is the per-turn view of the skills a bot has, with
// DB rows overlaying builtin/plugin packs by slug. StageMatches /
// PolicySkillSlugs / MergeExtraSkillSlugs resolve which skills are active for a
// turn/stage.
package skills

import (
	"encoding/json"
	"strings"
)

// SkillPack is the runtime shape of a skill.
type SkillPack struct {
	// ID — slug, referenced from BotPolicy.capabilities.skills.
	ID            string
	Stages        []string
	Instructions  string
	RequiredTools []string
	Name          string
	Description   string
	// Injection — "eager" | "on_demand".
	Injection string
	// Files — reference files (layer 3): bundle-relative path → markdown.
	Files map[string]string
	// Checklist — declarative onboarding/wizard checklist.
	Checklist []map[string]any
	// ContextVars — custom per-skill workspace state while ACTIVE.
	ContextVars []map[string]any
	// SourceDir — folder this skill was loaded from, or "" for code/DB skills.
	SourceDir string
}

// AllContextVars returns the skill's context vars, including the legacy
// Checklist as a "type: checklist" var so both surface uniformly.
func (p *SkillPack) AllContextVars() []map[string]any {
	out := make([]map[string]any, 0, len(p.ContextVars)+1)
	hasChecklistVar := false
	for _, v := range p.ContextVars {
		cp := make(map[string]any, len(v))
		for k, val := range v {
			cp[k] = val
		}
		out = append(out, cp)
		if t, _ := v["type"].(string); t == "checklist" {
			hasChecklistVar = true
		}
	}
	if len(p.Checklist) > 0 && !hasChecklistVar {
		items := make([]any, len(p.Checklist))
		for i, c := range p.Checklist {
			items[i] = c
		}
		head := map[string]any{
			"key":   "checklist",
			"type":  "checklist",
			"title": "Checklist",
			"items": items,
		}
		out = append([]map[string]any{head}, out...)
	}
	return out
}

// KBLookupSkill is the builtin KB-lookup-with-citations skill.
var KBLookupSkill = &SkillPack{
	ID:            "kb_lookup",
	Name:          "Information lookup",
	Description:   "Look up facts in the bot's knowledge bases and answer with citations.",
	Stages:        []string{"simple_answer", "research", "synthesize"},
	RequiredTools: []string{"kg.schema", "kg.query", "kg.read"},
	Injection:     "eager",
	Instructions: `SKILL: knowledge-graph lookup with citations
- The knowledge base is a graph; answer factual questions from it.
- kg.schema once to see what's there, then kg.query for each fact (try several phrasings; filter by attr for exact values), then kg.read the best hits for full context.
- Prefer a short grounded answer over a broad speculative answer.
- Every factual claim must be supported by a graph node (cite its ref).
- If the graph does not answer the question, say you cannot confirm it from the knowledge base.`,
}

// SkillRegistry is the per-turn view of the skills available to a bot. DB rows
// (loaded by the cli) override builtin fallbacks by slug.
type SkillRegistry struct {
	byID map[string]*SkillPack
}

// NewSkillRegistry builds a registry over builtin packs plus the given packs
// (later packs override builtins / earlier packs by slug).
func NewSkillRegistry(packs []*SkillPack) *SkillRegistry {
	r := &SkillRegistry{byID: map[string]*SkillPack{}}
	r.byID[KBLookupSkill.ID] = KBLookupSkill
	for _, p := range packs {
		if p != nil {
			r.byID[p.ID] = p
		}
	}
	return r
}

// SkillRegistryFromRows builds a registry from DB-shaped rows (slug + fields).
func SkillRegistryFromRows(rows []map[string]any) *SkillRegistry {
	var packs []*SkillPack
	for _, row := range rows {
		if row == nil {
			continue
		}
		slug, _ := row["slug"].(string)
		if slug == "" {
			continue
		}
		packs = append(packs, &SkillPack{
			ID:            slug,
			Name:          strOr(row["name"], slug),
			Description:   strOr(row["description"], ""),
			Stages:        strSlice(row["stages"]),
			Instructions:  strOr(row["instructions"], ""),
			RequiredTools: strSlice(row["required_tools"]),
			Injection:     strOr(row["injection"], "on_demand"),
			Files:         filesFromRow(row["files"]),
			Checklist:     dictSliceFromRow(row["checklist"]),
			ContextVars:   dictSliceFromRow(row["context_vars"]),
		})
	}
	return NewSkillRegistry(packs)
}

// Get returns the pack for a slug, or nil.
func (r *SkillRegistry) Get(slug string) *SkillPack {
	return r.byID[slug]
}

// ActiveForPolicy returns the packs the policy selects, in slug order.
func (r *SkillRegistry) ActiveForPolicy(policy map[string]any) []*SkillPack {
	var out []*SkillPack
	for _, slug := range PolicySkillSlugs(policy) {
		if p := r.byID[slug]; p != nil {
			out = append(out, p)
		}
	}
	return out
}

// ActiveForStage filters ActiveForPolicy to those targeting stageID.
func (r *SkillRegistry) ActiveForStage(policy map[string]any, stageID string) []*SkillPack {
	var out []*SkillPack
	for _, p := range r.ActiveForPolicy(policy) {
		if StageMatches(stageID, p.Stages) {
			out = append(out, p)
		}
	}
	return out
}

// StageMatches reports whether production stage stageID is one a skill targets.
// Production stage ids are flow-namespaced (qna:synthesize); a skill declares
// the LOGICAL step name (synthesize) so it activates on every flow's step of
// that name. An exact full-id match still works.
func StageMatches(stageID string, stages []string) bool {
	for _, s := range stages {
		if s == stageID {
			return true
		}
	}
	suffix := stageID
	if i := strings.LastIndex(stageID, ":"); i >= 0 {
		suffix = stageID[i+1:]
	}
	if suffix == stageID {
		return false
	}
	for _, s := range stages {
		if s == suffix {
			return true
		}
	}
	return false
}

// PolicySkillSlugs returns the slugs the policy selects — capabilities.skills,
// with legacy aliases. memory_enabled:false drops context_management.
func PolicySkillSlugs(policy map[string]any) []string {
	caps, ok := policy["capabilities"].(map[string]any)
	if !ok {
		return nil
	}
	raw := caps["skills"]
	if raw == nil {
		raw = caps["tool_packs"]
	}
	list, ok := raw.([]any)
	if !ok {
		// Tolerate a []string too.
		if ss, ok := raw.([]string); ok {
			out := append([]string(nil), ss...)
			return dropContextManagement(policy, out)
		}
		return nil
	}
	out := make([]string, 0, len(list))
	for _, s := range list {
		out = append(out, toStr(s))
	}
	return dropContextManagement(policy, out)
}

func dropContextManagement(policy map[string]any, slugs []string) []string {
	memEnabled := true
	if v, ok := policy["memory_enabled"]; ok {
		memEnabled = truthy(v)
	}
	if memEnabled {
		return slugs
	}
	out := slugs[:0]
	for _, s := range slugs {
		if s != "context_management" {
			out = append(out, s)
		}
	}
	return out
}

// MergeExtraSkillSlugs unions per-turn skill slugs into a COPY of the policy's
// capabilities.skills — never mutating the input. Order-preserving dedupe; the
// same policy back when extra is empty.
func MergeExtraSkillSlugs(policy map[string]any, extra []string) map[string]any {
	var extraSlugs []string
	for _, s := range extra {
		if strings.TrimSpace(s) != "" {
			extraSlugs = append(extraSlugs, s)
		}
	}
	if len(extraSlugs) == 0 {
		return policy
	}
	merged := dedupe(append(PolicySkillSlugs(policy), extraSlugs...))
	caps := map[string]any{}
	if c, ok := policy["capabilities"].(map[string]any); ok {
		for k, v := range c {
			caps[k] = v
		}
	}
	mergedAny := make([]any, len(merged))
	for i, s := range merged {
		mergedAny[i] = s
	}
	caps["skills"] = mergedAny
	out := map[string]any{}
	for k, v := range policy {
		out[k] = v
	}
	out["capabilities"] = caps
	return out
}

func dedupe(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// ── row coercion helpers ─────────────────────────────────────────────────────

func toStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if v == nil {
		return ""
	}
	b, _ := json.Marshal(v)
	return strings.Trim(string(b), `"`)
}

func strOr(v any, def string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return def
}

func strSlice(v any) []string {
	switch t := v.(type) {
	case []string:
		return append([]string(nil), t...)
	case []any:
		out := make([]string, 0, len(t))
		for _, s := range t {
			out = append(out, toStr(s))
		}
		return out
	}
	return nil
}

func filesFromRow(v any) map[string]string {
	if s, ok := v.(string); ok {
		var m map[string]any
		if json.Unmarshal([]byte(s), &m) == nil {
			v = m
		} else {
			return map[string]string{}
		}
	}
	m, ok := v.(map[string]any)
	if !ok {
		return map[string]string{}
	}
	out := map[string]string{}
	for k, val := range m {
		out[k] = toStr(val)
	}
	return out
}

func dictSliceFromRow(v any) []map[string]any {
	if s, ok := v.(string); ok {
		var arr []any
		if json.Unmarshal([]byte(s), &arr) == nil {
			v = arr
		} else {
			return nil
		}
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []map[string]any
	for _, it := range arr {
		if d, ok := it.(map[string]any); ok {
			out = append(out, d)
		}
	}
	return out
}

func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case nil:
		return false
	case string:
		return t != ""
	case int:
		return t != 0
	case float64:
		return t != 0
	}
	return true
}
