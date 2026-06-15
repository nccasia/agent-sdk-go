// Ported from agent_sdk/skills/loader.py — load a SKILL.md bundle from disk
// into a SkillPack.
//
// This is the code-first half of the skill story: a plugin owns its skills as
// SKILL.md files under a folder and loads them here at registration time. A
// bundle is one directory:
//
//	my_skill/
//	  SKILL.md            # frontmatter + markdown body
//	  reference/notes.md  # sibling text files become layer-3 reference files
//
// The Python loader parses real YAML (PyYAML). To keep the Go module dependency-
// free, this port hand-rolls a frontmatter parser covering the shapes a SKILL.md
// uses: scalar fields, block sequences (key:\n  - item) and block sequences of
// mappings (key:\n  - subkey: val\n    subkey: val).
package skills

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SkillLoadError is raised when a SKILL.md bundle cannot be parsed.
type SkillLoadError struct{ msg string }

func (e *SkillLoadError) Error() string { return e.msg }

func loadErr(msg string) *SkillLoadError { return &SkillLoadError{msg: msg} }

// IsSkillLoadError reports whether err is a SkillLoadError.
func IsSkillLoadError(err error) bool {
	var e *SkillLoadError
	return errors.As(err, &e)
}

var textSuffixes = map[string]struct{}{".md": {}, ".markdown": {}, ".txt": {}}

// ParseSkillMD splits a SKILL.md into (frontmatter, body). Frontmatter values
// are parsed into a map of string→any (string, []string, or []map[string]any).
func ParseSkillMD(text string) (map[string]any, string, error) {
	if !strings.HasPrefix(text, "---") {
		return nil, "", loadErr("SKILL.md must start with YAML frontmatter (--- … ---)")
	}
	// Find the closing fence after the opening one.
	lines := strings.Split(text, "\n")
	if strings.TrimSpace(lines[0]) != "---" {
		return nil, "", loadErr("SKILL.md must start with YAML frontmatter (--- … ---)")
	}
	close := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			close = i
			break
		}
	}
	if close < 0 {
		return nil, "", loadErr("SKILL.md must start with YAML frontmatter (--- … ---)")
	}
	front, err := parseFrontmatterYAML(lines[1:close])
	if err != nil {
		return nil, "", err
	}
	body := strings.Join(lines[close+1:], "\n")
	return front, body, nil
}

// parseFrontmatterYAML parses the minimal subset of YAML SKILL.md frontmatter
// uses.
func parseFrontmatterYAML(lines []string) (map[string]any, error) {
	out := map[string]any{}
	i := 0
	for i < len(lines) {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			i++
			continue
		}
		// Top-level key (no leading indent).
		if line[0] == ' ' || line[0] == '\t' {
			return nil, loadErr("invalid YAML frontmatter: unexpected indentation")
		}
		colon := strings.Index(line, ":")
		if colon < 0 {
			return nil, loadErr("invalid YAML frontmatter: expected 'key: value'")
		}
		key := strings.TrimSpace(line[:colon])
		val := strings.TrimSpace(line[colon+1:])
		if val != "" {
			out[key] = unquote(val)
			i++
			continue
		}
		// Block follows: gather indented lines.
		block := []string{}
		j := i + 1
		for j < len(lines) {
			l := lines[j]
			if strings.TrimSpace(l) == "" {
				block = append(block, l)
				j++
				continue
			}
			if l[0] != ' ' && l[0] != '\t' {
				break
			}
			block = append(block, l)
			j++
		}
		parsed, err := parseBlock(block)
		if err != nil {
			return nil, err
		}
		out[key] = parsed
		i = j
	}
	return out, nil
}

// parseBlock parses an indented block as either a []string (scalar sequence) or
// a []map[string]any (sequence of mappings).
func parseBlock(block []string) (any, error) {
	// Determine base indent and whether items are mappings.
	var items []string // raw lines for each "- " item, joined
	var current []string
	flush := func() {
		if len(current) > 0 {
			items = append(items, strings.Join(current, "\n"))
			current = nil
		}
	}
	for _, l := range block {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "- ") || t == "-" {
			flush()
			current = []string{strings.TrimSpace(strings.TrimPrefix(t, "-"))}
		} else {
			current = append(current, t)
		}
	}
	flush()

	// Decide scalar vs mapping: a mapping item contains a "key: value".
	mapping := false
	for _, it := range items {
		first := it
		if idx := strings.IndexByte(it, '\n'); idx >= 0 {
			first = it[:idx]
		}
		if c := strings.Index(first, ":"); c >= 0 {
			mapping = true
		}
		if strings.Contains(it, "\n") {
			mapping = true
		}
	}
	if !mapping {
		out := make([]string, 0, len(items))
		for _, it := range items {
			out = append(out, unquote(strings.TrimSpace(it)))
		}
		return out, nil
	}
	dicts := make([]map[string]any, 0, len(items))
	for _, it := range items {
		d := map[string]any{}
		for _, ln := range strings.Split(it, "\n") {
			ln = strings.TrimSpace(ln)
			if ln == "" {
				continue
			}
			c := strings.Index(ln, ":")
			if c < 0 {
				continue
			}
			d[strings.TrimSpace(ln[:c])] = unquote(strings.TrimSpace(ln[c+1:]))
		}
		dicts = append(dicts, d)
	}
	return dicts, nil
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

// LoadSkillPack parses <skillDir>/SKILL.md (+ sibling text reference files) into
// a SkillPack.
func LoadSkillPack(skillDir string) (*SkillPack, error) {
	skillMD := filepath.Join(skillDir, "SKILL.md")
	info, err := os.Stat(skillMD)
	if err != nil || info.IsDir() {
		return nil, loadErr("no SKILL.md in " + skillDir)
	}
	raw, err := os.ReadFile(skillMD)
	if err != nil {
		return nil, loadErr("no SKILL.md in " + skillDir)
	}
	front, body, err := ParseSkillMD(string(raw))
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(frontStr(front, "name"))
	description := strings.TrimSpace(frontStr(front, "description"))
	if name == "" || description == "" {
		return nil, loadErr("SKILL.md in " + skillDir + " must declare name and description")
	}

	files := map[string]string{}
	_ = filepath.Walk(skillDir, func(path string, fi os.FileInfo, werr error) error {
		if werr != nil || fi.IsDir() {
			return nil
		}
		if path == skillMD {
			return nil
		}
		if _, ok := textSuffixes[strings.ToLower(filepath.Ext(path))]; !ok {
			return nil
		}
		rel, rerr := filepath.Rel(skillDir, path)
		if rerr != nil {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		files[filepath.ToSlash(rel)] = string(data)
		return nil
	})

	stages := frontStrSlice(front, "stages")
	if len(stages) == 0 {
		stages = []string{"simple_answer"}
	}
	injection := strings.TrimSpace(frontStr(front, "injection"))
	if injection == "" {
		injection = "on_demand"
	}
	slug := strings.TrimSpace(frontStr(front, "slug"))
	if slug == "" {
		slug = filepath.Base(skillDir)
	}
	instructions := strings.TrimSpace(body)
	if instructions == "" {
		instructions = "SKILL: " + name
	}

	return &SkillPack{
		ID:            slug,
		Name:          name,
		Description:   description,
		Stages:        stages,
		Instructions:  instructions,
		RequiredTools: frontStrSlice(front, "required_tools"),
		Injection:     injection,
		Files:         files,
		Checklist:     frontDictSlice(front, "checklist"),
		ContextVars:   frontDictSlice(front, "context_vars"),
		SourceDir:     skillDir,
	}, nil
}

// LoadSkillPacks loads every <skillsRoot>/<slug>/SKILL.md bundle (immediate
// subdirs). Returns nil when the root does not exist.
func LoadSkillPacks(skillsRoot string) ([]*SkillPack, error) {
	info, err := os.Stat(skillsRoot)
	if err != nil || !info.IsDir() {
		return nil, nil
	}
	entries, err := os.ReadDir(skillsRoot)
	if err != nil {
		return nil, nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	var packs []*SkillPack
	for _, n := range names {
		child := filepath.Join(skillsRoot, n)
		fi, serr := os.Stat(child)
		if serr != nil || !fi.IsDir() {
			continue
		}
		if mi, merr := os.Stat(filepath.Join(child, "SKILL.md")); merr == nil && !mi.IsDir() {
			pack, perr := LoadSkillPack(child)
			if perr != nil {
				return nil, perr
			}
			packs = append(packs, pack)
		}
	}
	return packs, nil
}

func frontStr(front map[string]any, key string) string {
	if s, ok := front[key].(string); ok {
		return s
	}
	return ""
}

func frontStrSlice(front map[string]any, key string) []string {
	switch v := front[key].(type) {
	case []string:
		return append([]string(nil), v...)
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	}
	return nil
}

func frontDictSlice(front map[string]any, key string) []map[string]any {
	if v, ok := front[key].([]map[string]any); ok {
		return v
	}
	return nil
}
