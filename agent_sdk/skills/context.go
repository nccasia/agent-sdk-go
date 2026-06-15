// Ported from agent_sdk/skills/context.py — the live workspace state a skill
// carries while active.
//
// A skill's context_vars (checklist / todos / notes / var) are its durable
// working memory. These renderers turn them into the authoritative pinned block
// the model sees — surfaced by the skill_active lobe (next-turn) and the
// ActivateSkill tool result (the on-demand commitment moment).
package skills

import (
	"fmt"
	"strings"
)

// RenderContextVar renders one context var as the authoritative live workspace
// block. checklist/todos render as a numbered status list; other types as a
// "title: value" line.
func RenderContextVar(skillID string, v map[string]any) string {
	key := firstNonEmpty(asStr(v["key"]), asStr(v["title"]), "var")
	title := firstNonEmpty(asStr(v["title"]), key)
	vtype := firstNonEmpty(asStr(v["type"]), "var")

	if vtype == "checklist" || vtype == "todos" {
		lines := []string{fmt.Sprintf("### Skill %s · %s", skillID, title)}
		items, _ := v["items"].([]any)
		for i, it := range items {
			label := "item"
			status := "todo"
			if d, ok := it.(map[string]any); ok {
				label = firstNonEmpty(asStr(d["title"]), asStr(d["ask"]), asStr(d["key"]), "item")
				status = firstNonEmpty(asStr(d["status"]), "todo")
			} else {
				label = fmt.Sprintf("%v", it)
			}
			lines = append(lines, fmt.Sprintf("  %d. [%s] %s", i+1, status, label))
		}
		lines = append(lines, fmt.Sprintf(
			"Advance the next open item, then persist progress under "+
				"`skill:%s:%s` via todos.update / memory.", skillID, key))
		return strings.Join(lines, "\n")
	}

	val := v["value"]
	var body string
	if val != nil && fmt.Sprintf("%v", val) != "" && truthy(val) {
		body = fmt.Sprintf("%s: %v", title, val)
	} else {
		body = fmt.Sprintf("%s (empty) — track it under `skill:%s:%s` via the memory tool", title, skillID, key)
	}
	return "### Skill " + skillID + " · " + body
}

// RenderContextVarsBlock returns the full pinned context-vars block for a skill
// pack, or "" if it declares none.
func RenderContextVarsBlock(pack *SkillPack) string {
	vars := pack.AllContextVars()
	rendered := make([]string, 0, len(vars))
	sid := pack.ID
	if sid == "" {
		sid = "skill"
	}
	for _, v := range vars {
		rendered = append(rendered, RenderContextVar(sid, v))
	}
	if len(rendered) == 0 {
		return ""
	}
	return "Live workspace state for this skill (authoritative — recomputed " +
		"every turn):\n" + strings.Join(rendered, "\n")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
