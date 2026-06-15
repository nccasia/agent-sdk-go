package react

import (
	"regexp"
	"strings"
)

// DocWorkspace is the heavy-document capability for PreAct: it offloads a
// document out of the prompt and exposes it by structure and by slice (outline /
// grep / read_section), never whole. Pure, deterministic, in-process.
//
// Ported from agent_sdk/react/docworkspace.py.
type DocWorkspace struct {
	docs map[string]*doc
}

type docSection struct {
	id      string
	heading string
	level   int
	start   int // char offset of the body (after the heading line)
	end     int
}

type doc struct {
	text     string
	sections []docSection
	parts    map[string]string // section_id -> transformed output
}

// NewDocWorkspace builds an empty workspace.
func NewDocWorkspace() *DocWorkspace {
	return &DocWorkspace{docs: map[string]*doc{}}
}

var headingRE = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
var slugRE = regexp.MustCompile(`[^a-z0-9]+`)

func slug(heading string, idx int) string {
	s := strings.Trim(slugRE.ReplaceAllString(strings.ToLower(heading), "-"), "-")
	if s == "" {
		return "sec-" + pad2(idx)
	}
	out := pad2(idx) + "-" + s
	if len([]rune(out)) > 48 {
		out = string([]rune(out)[:48])
	}
	return out
}

func pad2(i int) string {
	s := itoaInt(i)
	if len(s) < 2 {
		return "0" + s
	}
	return s
}

// parseSections splits a Markdown document into sections on ATX headings.
func parseSections(text string) []docSection {
	type head struct {
		off     int
		level   int
		heading string
	}
	var heads []head
	off := 0
	for _, ln := range splitKeepEnds(text) {
		trimmed := strings.Trim(ln, "\n")
		if m := headingRE.FindStringSubmatch(trimmed); m != nil {
			heads = append(heads, head{off: off, level: len(m[1]), heading: strings.TrimSpace(m[2])})
		}
		off += len(ln)
	}
	out := make([]docSection, 0, len(heads))
	for i, h := range heads {
		bodyStart := h.off + len(h.heading) + h.level + 2
		end := len(text)
		if i+1 < len(heads) {
			end = heads[i+1].off
		}
		if bodyStart > end {
			bodyStart = end
		}
		out = append(out, docSection{
			id:      slug(h.heading, i),
			heading: h.heading,
			level:   h.level,
			start:   bodyStart,
			end:     end,
		})
	}
	return out
}

func splitKeepEnds(text string) []string {
	var out []string
	start := 0
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			out = append(out, text[start:i+1])
			start = i + 1
		}
	}
	if start < len(text) {
		out = append(out, text[start:])
	}
	return out
}

// Offload stores the document body, returning a summary with its outline.
func (w *DocWorkspace) Offload(docID, text string) map[string]any {
	sections := parseSections(text)
	w.docs[docID] = &doc{text: text, sections: sections, parts: map[string]string{}}
	return map[string]any{
		"doc_id":      docID,
		"total_chars": len([]rune(text)),
		"sections":    len(sections),
		"outline":     w.Outline(docID),
	}
}

// Outline returns the section index (id, heading, level, char span).
func (w *DocWorkspace) Outline(docID string) []map[string]any {
	d, ok := w.docs[docID]
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(d.sections))
	for _, s := range d.sections {
		out = append(out, map[string]any{
			"id": s.id, "heading": s.heading, "level": s.level, "chars": s.end - s.start,
		})
	}
	return out
}

// Grep returns matching lines (plus their section), not the document.
func (w *DocWorkspace) Grep(docID, pattern string, maxMatches int) []map[string]any {
	d, ok := w.docs[docID]
	if !ok {
		return nil
	}
	rx, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil
	}
	if maxMatches <= 0 {
		maxMatches = 50
	}
	var out []map[string]any
	for _, s := range d.sections {
		body := d.text[s.start:s.end]
		for _, line := range strings.Split(body, "\n") {
			if rx.MatchString(line) {
				out = append(out, map[string]any{
					"section_id": s.id, "heading": s.heading, "line": trim200(strings.TrimSpace(line)),
				})
				if len(out) >= maxMatches {
					return out
				}
			}
		}
	}
	return out
}

func trim200(s string) string {
	if len([]rune(s)) > 200 {
		return string([]rune(s)[:200])
	}
	return s
}

// ReadSection returns one section's text (ok=false if unknown).
func (w *DocWorkspace) ReadSection(docID, sectionID string) (string, bool) {
	d, ok := w.docs[docID]
	if !ok {
		return "", false
	}
	for _, s := range d.sections {
		if s.id == sectionID {
			return strings.TrimSpace(d.text[s.start:s.end]), true
		}
	}
	return "", false
}

// WritePart stores a transformed part back (ok=false if section unknown).
func (w *DocWorkspace) WritePart(docID, sectionID, content string) bool {
	d, ok := w.docs[docID]
	if !ok {
		return false
	}
	found := false
	for _, s := range d.sections {
		if s.id == sectionID {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	d.parts[sectionID] = content
	return true
}

// Assemble concatenates written parts in document order — the long-form result.
func (w *DocWorkspace) Assemble(docID string) string {
	d, ok := w.docs[docID]
	if !ok {
		return ""
	}
	var parts []string
	for _, s := range d.sections {
		if p, ok := d.parts[s.id]; ok {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, "\n\n")
}

// DocWorkspaceToJSON is the loss-free snapshot (text + parts; sections re-derived).
func (w *DocWorkspace) ToJSON() map[string]any {
	out := map[string]any{}
	for id, d := range w.docs {
		parts := map[string]any{}
		for k, v := range d.parts {
			parts[k] = v
		}
		out[id] = map[string]any{"text": d.text, "parts": parts}
	}
	return out
}

// DocWorkspaceFromJSON rebuilds a workspace from ToJSON output.
func DocWorkspaceFromJSON(data map[string]any) *DocWorkspace {
	ws := NewDocWorkspace()
	for id, raw := range data {
		dm, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		text, _ := dm["text"].(string)
		parts := map[string]string{}
		if pm, ok := dm["parts"].(map[string]any); ok {
			for k, v := range pm {
				parts[k], _ = v.(string)
			}
		}
		ws.docs[id] = &doc{text: text, sections: parseSections(text), parts: parts}
	}
	return ws
}
