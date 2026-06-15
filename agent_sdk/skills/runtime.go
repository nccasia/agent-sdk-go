// Ported from agent_sdk/skills/runtime.py — the skill-activation tool runtime,
// the model's hands on an on-demand skill.
//
// On-demand skills (RFC 0013 progressive disclosure) surface only a one-line
// index in the prompt; the model must activate one to load its body. This
// runtime exposes the three tools that make that real:
//
//   - ActivateSkill(slug) — load the skill's instructions (or a ToC when the body
//     is large), pin its workspace state, and mark it in use for the turn.
//   - skill.read(slug, file=…, section=…) — read one reference file, its ToC when
//     large, or a single section — layered reading, never a whole-bundle dump.
//   - skill.search(query, slug=…) — keyword-search every section of the active
//     skills' bundles and get back where the answer lives.
package skills

import (
	"context"
	"sort"
	"strings"
)

// Tool names.
const (
	ACTIVATE = "ActivateSkill"
	READ     = "skill.read"
	SEARCH   = "skill.search"
)

// TurnOutputs is the seam the runtime uses to record an activation on the live
// turn (so the skill_active lobe drives it and the engine can persist it). It
// mirrors current_turn().lobe_outputs in Python.
type TurnOutputs interface {
	// LobeOutputs returns the live per-turn output map (mutated in place).
	LobeOutputs() map[string]any
}

// CurrentTurn is the per-turn seam: the engine sets it to return the active turn
// (or nil) for the duration of a turn. Defaults to nil — a runtime-only caller
// (no engine turn) records nothing, matching the Python None-turn behavior.
var CurrentTurn func() TurnOutputs

// SkillToolRuntime exposes ActivateSkill / skill.read / skill.search over the
// on-demand skills in a registry.
type SkillToolRuntime struct {
	Registry     *SkillRegistry
	Slugs        []string // the on-demand skills this runtime serves
	Activated    []string // slugs activated this turn (for inspection)
	Llm          LlmFunc
	BudgetTokens int
	SurfaceMode  string // "llm" | "deterministic" | "off"
	Cache        *SurfaceCache
}

// RuntimeOption configures a SkillToolRuntime.
type RuntimeOption func(*SkillToolRuntime)

// WithLlm injects the compile LLM.
func WithLlm(fn LlmFunc) RuntimeOption { return func(r *SkillToolRuntime) { r.Llm = fn } }

// WithBudgetTokens sets the surface budget.
func WithBudgetTokens(n int) RuntimeOption { return func(r *SkillToolRuntime) { r.BudgetTokens = n } }

// WithSurfaceMode sets "llm" | "deterministic" | "off".
func WithSurfaceMode(m string) RuntimeOption { return func(r *SkillToolRuntime) { r.SurfaceMode = m } }

// WithCache injects a SurfaceCache (default: a fresh persisting one).
func WithCache(c *SurfaceCache) RuntimeOption { return func(r *SkillToolRuntime) { r.Cache = c } }

// NewSkillToolRuntime builds a runtime over the on-demand slugs in registry.
func NewSkillToolRuntime(registry *SkillRegistry, slugs []string, opts ...RuntimeOption) *SkillToolRuntime {
	r := &SkillToolRuntime{
		Registry:     registry,
		Slugs:        append([]string(nil), slugs...),
		BudgetTokens: DefaultBudget,
		SurfaceMode:  "deterministic",
	}
	for _, o := range opts {
		o(r)
	}
	if r.Cache == nil {
		r.Cache = NewSurfaceCache(true)
	}
	return r
}

// GetToolSpecs returns provider-compatible specs, or nil when no slug is served.
func (r *SkillToolRuntime) GetToolSpecs() []map[string]any {
	if len(r.Slugs) == 0 {
		return nil
	}
	slugEnum := make([]any, len(r.Slugs))
	for i, s := range r.Slugs {
		slugEnum[i] = s
	}
	return []map[string]any{
		{
			"name": ACTIVATE,
			"description": "Load a skill's instructions before doing its task. Activate once, " +
				"then follow the steps; don't work from the summary.",
			"input_schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"slug": map[string]any{"type": "string", "enum": slugEnum},
				},
				"required": []any{"slug"},
			},
		},
		{
			"name": READ,
			"description": "Read one referenced chunk of an activated skill: pass chunk='<id>' " +
				"(the [file#section] ids the surface points to), or file=…/section=…. " +
				"Read only the chunk a step needs — not whole files.",
			"input_schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"slug":    map[string]any{"type": "string", "enum": slugEnum},
					"chunk":   map[string]any{"type": "string"},
					"file":    map[string]any{"type": "string"},
					"section": map[string]any{"type": "string"},
				},
				"required": []any{"slug"},
			},
		},
		{
			"name": SEARCH,
			"description": "Find where an answer lives in a skill's files (returns file + section " +
				"+ snippet). Use this first on a bundle, then skill.read that section.",
			"input_schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
					"slug":  map[string]any{"type": "string", "enum": slugEnum},
				},
				"required": []any{"query"},
			},
		},
	}
}

// CallTool dispatches one skill tool call.
func (r *SkillToolRuntime) CallTool(ctx context.Context, name string, inp map[string]any, retrievedChunks []map[string]any, alreadyRead map[string]struct{}) (string, error) {
	switch name {
	case ACTIVATE:
		return r.activate(ctx, asStr(inp["slug"])), nil
	case READ:
		return r.read(asStr(inp["slug"]), inp["file"], inp["section"], inp["chunk"]), nil
	case SEARCH:
		return r.search(asStr(inp["query"]), inp["slug"]), nil
	}
	return "Error: unknown tool '" + name + "'.", nil
}

func (r *SkillToolRuntime) serves(slug string) bool {
	for _, s := range r.Slugs {
		if s == slug {
			return true
		}
	}
	return false
}

func (r *SkillToolRuntime) markInUse(slug string) {
	r.Activated = append(r.Activated, slug)
	if CurrentTurn == nil {
		return
	}
	turn := CurrentTurn()
	if turn == nil {
		return
	}
	outputs := turn.LobeOutputs()
	if outputs == nil {
		return
	}
	existing, _ := outputs["skills_in_use"].([]string)
	for _, s := range existing {
		if s == slug {
			return
		}
	}
	outputs["skills_in_use"] = append(existing, slug)
}

func (r *SkillToolRuntime) packs() []*SkillPack {
	var out []*SkillPack
	for _, s := range r.Slugs {
		if p := r.Registry.Get(s); p != nil {
			out = append(out, p)
		}
	}
	return out
}

func (r *SkillToolRuntime) activate(ctx context.Context, slug string) string {
	pack := r.Registry.Get(slug)
	if pack == nil || !r.serves(slug) {
		return "Error: unknown skill '" + slug + "'. Available: " + strings.Join(r.Slugs, ", ") + "."
	}
	r.markInUse(slug)
	var surface string
	if r.SurfaceMode == "off" {
		surface = r.rawSurface(pack)
	} else {
		compiled, ok := r.Cache.Get(pack)
		if !ok {
			var llm LlmFunc
			if r.SurfaceMode == "llm" {
				llm = r.Llm
			}
			compiled = CompileSkill(ctx, pack, llm, r.BudgetTokens)
			r.Cache.Put(pack, compiled)
		}
		surface = compiled.Surface
	}
	parts := []string{surface}
	if cv := RenderContextVarsBlock(pack); cv != "" {
		parts = append(parts, cv)
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "\n\n")
}

func (r *SkillToolRuntime) rawSurface(pack *SkillPack) string {
	body := pack.Instructions
	if body == "" {
		name := pack.Name
		if name == "" {
			name = pack.ID
		}
		body = "SKILL: " + name
	}
	var lead string
	if EstTokens(body) > FullFileTokens {
		lead = "Skill '" + pack.ID + "' (large — read sections as needed):\n" + FileToc(body)
	} else {
		lead = body
	}
	if len(pack.Files) > 0 {
		keys := sortedKeys(pack.Files)
		lead += "\n\nReference files: " + strings.Join(keys, ", ")
	}
	return lead
}

func (r *SkillToolRuntime) read(slug string, file, section, chunk any) string {
	pack := r.Registry.Get(slug)
	if pack == nil || !r.serves(slug) {
		return "Error: unknown skill '" + slug + "'. Available: " + strings.Join(r.Slugs, ", ") + "."
	}
	fileS := strings.TrimSpace(asStr(file))
	sectionS := strings.TrimSpace(asStr(section))
	chunkS := strings.TrimSpace(asStr(chunk))
	// A surface references chunks as "<file>#<section>"; accept that form.
	if chunkS != "" && fileS == "" && sectionS == "" {
		if i := strings.Index(chunkS, "#"); i >= 0 {
			fileS = chunkS[:i]
			sectionS = chunkS[i+1:]
		} else {
			sectionS = chunkS
		}
	}
	var content, label string
	if fileS == "" || fileS == "SKILL.md" {
		content = pack.Instructions
		label = "SKILL.md"
	} else {
		c, ok := pack.Files[fileS]
		if !ok {
			avail := strings.Join(sortedKeys(pack.Files), ", ")
			if avail == "" {
				avail = "(none)"
			}
			return "Error: skill '" + slug + "' has no file '" + fileS + "'. Files: " + avail + "."
		}
		content = c
		label = fileS
	}
	sections := SplitSections(content)
	if sectionS != "" {
		norm := slugifyHeading(sectionS)
		for _, s := range sections {
			if s.ID == sectionS ||
				strings.EqualFold(strings.TrimSpace(s.Heading), sectionS) ||
				s.ID == norm ||
				slugifyHeading(s.Heading) == norm {
				return s.Content
			}
		}
		return "Error: no section '" + sectionS + "' in " + label + ". " + FileToc(content)
	}
	if EstTokens(content) > FullFileTokens {
		return label + " is large — request one section with skill.read(section=…):\n" + FileToc(content)
	}
	return content
}

func (r *SkillToolRuntime) search(query string, slug any) string {
	if strings.TrimSpace(query) == "" {
		return "Error: search needs a query."
	}
	packs := r.packs()
	if s := asStr(slug); s != "" {
		var f []*SkillPack
		for _, p := range packs {
			if p.ID == s {
				f = append(f, p)
			}
		}
		packs = f
	}
	hits := SearchBundle(packs, query, 5)
	if len(hits) == 0 {
		return "(no matches for '" + query + "')"
	}
	lines := make([]string, len(hits))
	for i, h := range hits {
		lines[i] = "- " + h.Skill + " · " + h.File + " · [" + h.Section + "] " + h.Heading + ": " + h.Snippet
	}
	return strings.Join(lines, "\n")
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
