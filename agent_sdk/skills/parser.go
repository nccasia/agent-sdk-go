// Package skills ports agent_sdk/skills/ — turning a Standard Operating
// Procedure (a folder indexed by SKILL.md) into agent behavior via progressive
// disclosure.
//
// This file ports parser.py: deterministic markdown navigation (RFC 0013
// layered skills). Split a bundle into sections, render a table of contents for
// a large file, estimate tokens, and keyword-search across every section of
// every file — so the model reads a large bundle progressively (index → ToC →
// one section) instead of dumping files. Pure functions, no I/O.
package skills

import (
	"regexp"
	"sort"
	"strings"
)

// FullFileTokens — a file at or below this estimated size is returned whole;
// above it, a bare file read returns the ToC and the model requests a section.
const FullFileTokens = 1500

// Section is one #/##/### heading span of a markdown bundle.
type Section struct {
	ID        string
	Heading   string
	Content   string
	LineCount int
}

// EstTokens estimates the token count of text (chars / 4, floored at 0).
func EstTokens(text string) int {
	n := len(text) / 4
	if n < 0 {
		return 0
	}
	return n
}

var headingRe = regexp.MustCompile(`(?m)^(#{1,3})[ \t]+(.+?)[ \t]*$`)

var nonAlnumRe = regexp.MustCompile(`[^a-z0-9]+`)

// slugifyHeading lowercases, strips non-ASCII, and dash-joins a heading.
func slugifyHeading(heading string) string {
	ascii := make([]rune, 0, len(heading))
	for _, r := range strings.ToLower(heading) {
		if r < 128 {
			ascii = append(ascii, r)
		}
		// Non-ASCII runes are dropped (the NFKD ascii-ignore path in Python
		// keeps decomposable accents' base letters; for the deterministic core
		// we approximate by dropping non-ASCII, which the chunk-id tolerant
		// match in the runtime compensates for).
	}
	slug := nonAlnumRe.ReplaceAllString(string(ascii), "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "section"
	}
	return slug
}

// SplitSections splits markdown by #/##/### headings. Content before the first
// heading becomes a synthetic "intro" section. Section ids are slugified
// headings, deduped with -2, -3 … suffixes.
func SplitSections(markdown string) []Section {
	text := markdown
	matches := headingRe.FindAllStringSubmatchIndex(text, -1)
	sections := make([]Section, 0)
	seen := map[string]int{}

	add := func(heading, content string) {
		base := slugifyHeading(heading)
		seen[base]++
		sid := base
		if seen[base] != 1 {
			sid = base + "-" + itoa(seen[base])
		}
		sections = append(sections, Section{
			ID:        sid,
			Heading:   heading,
			Content:   strings.Trim(content, "\n"),
			LineCount: strings.Count(content, "\n") + 1,
		})
	}

	if len(matches) == 0 {
		if strings.TrimSpace(text) != "" {
			add("intro", text)
		}
		return sections
	}

	firstStart := matches[0][0]
	if strings.TrimSpace(text[:firstStart]) != "" {
		add("intro", text[:firstStart])
	}
	for i, m := range matches {
		// m[2:4] is the heading marker group; m[4:6] is the heading text group.
		heading := text[m[4]:m[5]]
		start := m[0]
		end := len(text)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		add(heading, text[start:end])
	}
	return sections
}

// FileToc renders a table of contents for a large file: section ids, headings,
// sizes.
func FileToc(content string) string {
	lines := []string{"Table of contents (request one section at a time):"}
	for _, sec := range SplitSections(content) {
		lines = append(lines, "- ["+sec.ID+"] "+sec.Heading+" (~"+itoa(EstTokens(sec.Content))+" tokens)")
	}
	return strings.Join(lines, "\n")
}

// FilePurpose returns a one-line purpose for the layer-2 file index: the
// frontmatter description, else the first heading, else the first non-empty
// line.
func FilePurpose(content string) string {
	text := strings.TrimLeft(content, " \t\n\r")
	if strings.HasPrefix(text, "---") {
		lines := strings.Split(text, "\n")
		if len(lines) > 30 {
			lines = lines[:30]
		}
		for _, line := range lines[1:] {
			if strings.TrimSpace(line) == "---" {
				break
			}
			if strings.HasPrefix(strings.ToLower(line), "description:") {
				return strings.TrimSpace(line[strings.Index(line, ":")+1:])
			}
		}
	}
	if m := headingRe.FindStringSubmatch(text); m != nil {
		return m[2]
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) != "" {
			t := strings.TrimSpace(line)
			if len(t) > 120 {
				t = t[:120]
			}
			return t
		}
	}
	return "(empty)"
}

// SplitFrontmatter splits a leading --- … --- frontmatter block from the body.
// Each "key: value" line becomes a string field (lower-cased key); everything
// after the closing --- is the body. No frontmatter ⇒ ({}, text).
func SplitFrontmatter(text string) (map[string]string, string) {
	if !strings.HasPrefix(strings.TrimLeft(text, " \t\n\r"), "---") {
		return map[string]string{}, text
	}
	stripped := strings.TrimLeft(text, "\n")
	lines := strings.Split(stripped, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return map[string]string{}, text
	}
	fields := map[string]string{}
	bodyStart := -1
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "---" {
			bodyStart = i + 1
			break
		}
		if idx := strings.Index(line, ":"); idx >= 0 {
			key := strings.ToLower(strings.TrimSpace(line[:idx]))
			val := strings.TrimSpace(line[idx+1:])
			if key != "" {
				fields[key] = val
			}
		}
	}
	if bodyStart < 0 {
		return map[string]string{}, text
	}
	body := strings.Trim(strings.Join(lines[bodyStart:], "\n"), "\n")
	return fields, body
}

var wordSplitRe = regexp.MustCompile(`\W+`)

// nfcLower is the search-normalization seam (NFC + lower). Go has no stdlib NFC;
// lowercasing suffices for ASCII and is byte-stable for the multibyte hay/term
// comparison the search relies on.
func nfcLower(s string) string {
	return strings.ToLower(s)
}

// Hit is one search_bundle result.
type Hit struct {
	Skill   string
	File    string
	Section string
	Heading string
	Score   int
	Snippet string
}

// SearchBundle keyword-searches every section of every file in the given skills.
// Deterministic token-overlap scoring — the fast path through very large
// bundles.
func SearchBundle(packs []*SkillPack, query string, topK int) []Hit {
	var terms []string
	for _, t := range wordSplitRe.Split(nfcLower(query), -1) {
		if len(t) > 1 {
			terms = append(terms, t)
		}
	}
	if len(terms) == 0 {
		return nil
	}
	var hits []Hit
	for _, pack := range packs {
		sources := orderedSources(pack)
		for _, src := range sources {
			path, content := src.name, src.content
			for _, sec := range SplitSections(content) {
				hay := nfcLower(sec.Heading + "\n" + sec.Content)
				score := 0
				for _, t := range terms {
					score += strings.Count(hay, t)
				}
				if score <= 0 {
					continue
				}
				snippet := ""
				for _, line := range strings.Split(sec.Content, "\n") {
					low := nfcLower(line)
					for _, t := range terms {
						if strings.Contains(low, t) {
							snippet = strings.TrimSpace(line)
							if len(snippet) > 200 {
								snippet = snippet[:200]
							}
							break
						}
					}
					if snippet != "" {
						break
					}
				}
				name := pack.Name
				if name == "" {
					name = pack.ID
				}
				hits = append(hits, Hit{
					Skill:   name,
					File:    path,
					Section: sec.ID,
					Heading: sec.Heading,
					Score:   score,
					Snippet: snippet,
				})
			}
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if hits[i].File != hits[j].File {
			return hits[i].File < hits[j].File
		}
		return hits[i].Section < hits[j].Section
	})
	if topK > 0 && len(hits) > topK {
		hits = hits[:topK]
	}
	return hits
}

type namedSource struct {
	name    string
	content string
}

// orderedSources yields SKILL.md first, then files sorted by path — a
// deterministic iteration order that matches the dict insertion + sort the
// callers depend on (search ties break on file/section).
func orderedSources(pack *SkillPack) []namedSource {
	out := []namedSource{{"SKILL.md", pack.Instructions}}
	keys := make([]string, 0, len(pack.Files))
	for k := range pack.Files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, namedSource{k, pack.Files[k]})
	}
	return out
}

// itoa is a tiny strconv.Itoa alias kept local to avoid importing strconv in
// hot string-building paths repeatedly.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
