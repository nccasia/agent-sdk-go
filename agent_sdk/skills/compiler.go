// Ported from agent_sdk/skills/compiler.py — compile a skill into a
// budget-bounded "surface": the must-know core plus chunk refs, lazily built.
//
// A real SOP bundle is too big to inline on every activation. The compiler turns
// it into a CompiledSkill: a dense surface (the core, within a token budget)
// that REFERENCES deeper chunks by id — so ActivateSkill returns the compact
// core and the model pulls a chunk back with skill.read only when a step needs
// it. Domain-pure: no I/O, no caching. compile_skill takes an injected LlmCall;
// a nil llm (or any error) degrades to a deterministic surface so it never fails
// a turn.
package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// DefaultBudget is the default surface token budget.
const DefaultBudget = 600

const compilePromptTmpl = "You are compiling a Standard Operating Procedure into a COMPACT surface for an AI " +
	"agent that will follow it. Write the must-know core — the steps and decisions to apply " +
	"every time this skill is used — in AT MOST %d tokens. Do NOT inline reference " +
	"detail; instead point to chunk ids in square brackets like [file#section] where the " +
	"agent can skill.read the detail when a step needs it. Output only the surface text."

// LlmFunc is the injectable LLM seam the compiler calls — the narrow shape of
// the SDK LlmCall (stage + system + messages + caps), returning the surface
// text. Returning an error degrades to the deterministic surface.
type LlmFunc func(ctx context.Context, req LlmRequest) (string, error)

// LlmRequest mirrors the kwargs the Python compiler passes to llm().
type LlmRequest struct {
	Stage       string
	System      string
	Messages    []map[string]any
	MaxTokens   int
	Temperature float64
}

// SkillChunk is one addressable piece of a skill bundle (a section of SKILL.md
// or a file).
type SkillChunk struct {
	ID         string // "<file>#<section_id>" — directly skill.read-able
	SourceFile string
	Heading    string
	Tokens     int
	Gist       string
}

// ToJSON renders the chunk as a JSON-safe map.
func (c SkillChunk) ToJSON() map[string]any {
	return map[string]any{
		"id":          c.ID,
		"source_file": c.SourceFile,
		"heading":     c.Heading,
		"tokens":      c.Tokens,
		"gist":        c.Gist,
	}
}

// CompiledSkill is a skill's compiled surface + the chunk index it references.
type CompiledSkill struct {
	Slug         string
	ContentHash  string
	BudgetTokens int
	Surface      string
	Chunks       []SkillChunk
	BuiltBy      string // "llm" | "deterministic"
}

// ToJSON renders the compiled skill as a JSON-safe map.
func (c CompiledSkill) ToJSON() map[string]any {
	chunks := make([]map[string]any, len(c.Chunks))
	for i, ch := range c.Chunks {
		chunks[i] = ch.ToJSON()
	}
	return map[string]any{
		"slug":          c.Slug,
		"content_hash":  c.ContentHash,
		"budget_tokens": c.BudgetTokens,
		"surface":       c.Surface,
		"chunks":        chunks,
		"built_by":      c.BuiltBy,
	}
}

// CompiledSkillFromJSON rebuilds a CompiledSkill from a decoded JSON map.
func CompiledSkillFromJSON(d map[string]any) CompiledSkill {
	budget := DefaultBudget
	if b, ok := asInt(d["budget_tokens"]); ok {
		budget = b
	}
	var chunks []SkillChunk
	if arr, ok := d["chunks"].([]any); ok {
		for _, it := range arr {
			c, ok := it.(map[string]any)
			if !ok {
				continue
			}
			tok, _ := asInt(c["tokens"])
			chunks = append(chunks, SkillChunk{
				ID:         asStr(c["id"]),
				SourceFile: asStr(c["source_file"]),
				Heading:    asStr(c["heading"]),
				Tokens:     tok,
				Gist:       asStr(c["gist"]),
			})
		}
	}
	builtBy := asStr(d["built_by"])
	if builtBy == "" {
		builtBy = "deterministic"
	}
	return CompiledSkill{
		Slug:         asStr(d["slug"]),
		ContentHash:  asStr(d["content_hash"]),
		BudgetTokens: budget,
		Surface:      asStr(d["surface"]),
		Chunks:       chunks,
		BuiltBy:      builtBy,
	}
}

// ContentHash is a stable 16-hex hash of a skill's content (body + files) — the
// cache key. Changes iff the SOP text changes.
func ContentHash(pack *SkillPack) string {
	h := sha256.New()
	h.Write([]byte(pack.Instructions))
	keys := make([]string, 0, len(pack.Files))
	for k := range pack.Files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		h.Write([]byte{0})
		h.Write([]byte(k))
		h.Write([]byte(pack.Files[k]))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// ChunkSkill splits the whole bundle (SKILL.md body + every file) into
// addressable chunks.
func ChunkSkill(pack *SkillPack) []SkillChunk {
	var out []SkillChunk
	for _, src := range orderedSources(pack) {
		for _, sec := range SplitSections(src.content) {
			body := strings.TrimSpace(sec.Content)
			gist := sec.Heading
			if body != "" {
				first := body
				if i := strings.IndexByte(first, '\n'); i >= 0 {
					first = first[:i]
				}
				if len(first) > 120 {
					first = first[:120]
				}
				gist = first
			}
			out = append(out, SkillChunk{
				ID:         src.name + "#" + sec.ID,
				SourceFile: src.name,
				Heading:    sec.Heading,
				Tokens:     EstTokens(sec.Content),
				Gist:       gist,
			})
		}
	}
	return out
}

func chunkIndex(chunks []SkillChunk) string {
	lines := make([]string, len(chunks))
	for i, c := range chunks {
		lines[i] = fmt.Sprintf("- [%s] %s (~%d tok)", c.ID, c.Heading, c.Tokens)
	}
	return strings.Join(lines, "\n")
}

// DeterministicSurface is the no-LLM surface (the fallback): description + the
// chunk index to skill.read.
func DeterministicSurface(pack *SkillPack, chunks []SkillChunk, budgetTokens int) string {
	head := pack.Description
	if head == "" {
		head = pack.Name
	}
	if head == "" {
		head = pack.ID
	}
	if head == "" {
		head = "skill"
	}
	head = strings.TrimSpace(head)
	return head + "\n\nThis skill's content — read a chunk with " +
		"skill.read(chunk='<id>') when a step needs it:\n" + chunkIndex(chunks)
}

// CompileSkill compiles pack into a CompiledSkill. A bundle whose BODY is within
// budgetTokens is its own surface (no LLM). A larger one gets an LLM-written core
// that references chunk ids; on a nil llm or any failure it falls back to
// DeterministicSurface.
func CompileSkill(ctx context.Context, pack *SkillPack, llm LlmFunc, budgetTokens int) CompiledSkill {
	chash := ContentHash(pack)
	chunks := ChunkSkill(pack)
	body := pack.Instructions
	if body == "" {
		name := pack.Name
		if name == "" {
			name = pack.ID
		}
		body = "SKILL: " + name
	}

	// Gate on the BODY size, not the whole bundle. When the body fits the
	// budget, the body IS the surface (+ a file list).
	if EstTokens(body) <= budgetTokens {
		surface := strings.TrimSpace(body)
		if len(pack.Files) > 0 {
			keys := make([]string, 0, len(pack.Files))
			for k := range pack.Files {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			surface += "\n\nReference files (skill.read a section when a step needs it): " +
				strings.Join(keys, ", ")
		}
		return CompiledSkill{pack.ID, chash, budgetTokens, surface, chunks, "deterministic"}
	}

	surface := ""
	if llm != nil {
		idx := chunkIndex(chunks)
		name := pack.Name
		if name == "" {
			name = pack.ID
		}
		user := fmt.Sprintf("SKILL: %s\n%s\n\nBODY:\n%s\n\nCHUNKS (reference by id):\n%s",
			name, pack.Description, body, idx)
		maxTokens := budgetTokens * 3
		if maxTokens < 2048 {
			maxTokens = 2048
		}
		txt, err := llm(ctx, LlmRequest{
			Stage:       "skill.compile",
			System:      fmt.Sprintf(compilePromptTmpl, budgetTokens),
			Messages:    []map[string]any{{"role": "user", "content": user}},
			MaxTokens:   maxTokens,
			Temperature: 0.0,
		})
		if err == nil {
			surface = strings.TrimSpace(txt)
		}
	}

	if surface == "" {
		return CompiledSkill{pack.ID, chash, budgetTokens,
			DeterministicSurface(pack, chunks, budgetTokens), chunks, "deterministic"}
	}

	if EstTokens(surface) > budgetTokens { // safety net — keep within budget
		cut := budgetTokens * 4
		if cut > len(surface) {
			cut = len(surface)
		}
		surface = strings.TrimRight(surface[:cut], " \t\n") + "\n…(truncated — skill.read for detail)"
	}
	return CompiledSkill{pack.ID, chash, budgetTokens, surface, chunks, "llm"}
}

func asInt(v any) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case int64:
		return int(t), true
	case float64:
		return int(t), true
	}
	return 0, false
}

func asStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
