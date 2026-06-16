// Package react: DocGroundingGuard — a tool-call filter that keeps a
// written document's file references grounded in reality.
//
// A capable model writing an architecture/summary doc will confidently cite
// paths that don't exist — a conventional layout it inferred rather than the
// real one (observed live: a doc naming “guards/docguard.py“ when the file
// is at “react/docguard.py“). That is the coding-agent analog of the RAG
// invariant "refuse rather than emit ungrounded claims": a documented path
// should resolve to a real file.
//
// This guard watches write calls for documents (“doc_suffixes“) and, when
// the content references paths that “exists“ reports as absent, steers the
// model to fix them (it has the deterministic repo map + the files it has
// read). It steers a bounded number of times per document so it cannot
// deadlock the stage — after that it lets the write through (a measurement
// gate still records the defect). It is generic: pass an “exists“ predicate
// and the read/write tool names.
//
// “record_only=true“ logs events without intercepting.
//
// Ported from agent_sdk/react/grounding.py.
package react

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// existsFn is the file-existence predicate the guard uses to verify cited
// paths (true ⇒ the path resolves to a real file).
type existsFn func(path string) bool

// DocGuardEvent is one telemetry record (stage, path, action).
type DocGroundingEvent struct {
	Stage   string   `json:"stage"`
	Path    string   `json:"path"`
	Action  string   `json:"action"`
	Missing []string `json:"missing,omitempty"`
}

// pathRE matches path-ish tokens: a dotted code/file path, optionally with
// directories. Mirrors “_PATH_RE“ in grounding.py.
var pathRE = regexp.MustCompile(`[A-Za-z0-9_][A-Za-z0-9_./-]*\.[A-Za-z0-9]{1,6}`)

// codeSuffixes is the set of file extensions treated as a real "code" file
// (the written doc is grounded against these).
var codeSuffixes = []string{
	".py", ".js", ".ts", ".tsx", ".go", ".rs", ".java", ".rb", ".md",
	".toml", ".json", ".yaml", ".yml", ".cfg", ".ini", ".sh",
}

// DocGroundingGuard is a tool-call filter that refuses a document citing
// nonexistent file paths.
type DocGroundingGuard struct {
	exists      existsFn
	readTools   map[string]struct{}
	writeTools  map[string]struct{}
	pathKeys    []string
	contentKey  string
	docSuffixes []string
	maxRefusals int
	maxReport   int
	recordOnly  bool
	read        map[string]struct{}
	refusals    map[string]int
	Events      []DocGroundingEvent
}

// DocGroundingOption configures a DocGroundingGuard built via NewDocGroundingGuard.
type DocGroundingOption func(*DocGroundingGuard)

// WithExists sets the file-existence predicate (true ⇒ path resolves).
func WithExists(fn func(path string) bool) DocGroundingOption {
	return func(g *DocGroundingGuard) { g.exists = fn }
}

// WithReadTools overrides the default read tool names ("Read").
func WithReadTools(names ...string) DocGroundingOption {
	return func(g *DocGroundingGuard) {
		g.readTools = make(map[string]struct{}, len(names))
		for _, n := range names {
			g.readTools[n] = struct{}{}
		}
	}
}

// WithWriteTools overrides the default write tool names ("Write").
func WithWriteTools(names ...string) DocGroundingOption {
	return func(g *DocGroundingGuard) {
		g.writeTools = make(map[string]struct{}, len(names))
		for _, n := range names {
			g.writeTools[n] = struct{}{}
		}
	}
}

// WithPathKeys overrides the default path key set
// ({"file_path","path","file","filename"}).
func WithPathKeys(keys ...string) DocGroundingOption {
	return func(g *DocGroundingGuard) { g.pathKeys = append([]string(nil), keys...) }
}

// WithContentKey overrides the default content key ("content").
func WithContentKey(k string) DocGroundingOption {
	return func(g *DocGroundingGuard) { g.contentKey = k }
}

// WithDocSuffixes overrides the default doc suffix set ((".md",)).
func WithDocSuffixes(suffixes ...string) DocGroundingOption {
	return func(g *DocGroundingGuard) { g.docSuffixes = append([]string(nil), suffixes...) }
}

// WithMaxRefusals overrides the per-document refusal cap (default 2).
func WithMaxRefusals(n int) DocGroundingOption {
	return func(g *DocGroundingGuard) { g.maxRefusals = n }
}

// WithMaxReport overrides the max missing-paths reported per refusal
// (default 10).
func WithMaxReport(n int) DocGroundingOption {
	return func(g *DocGroundingGuard) { g.maxReport = n }
}

// WithRecordOnly toggles measure-only mode (events recorded, no intercept).
func WithRecordOnly(b bool) DocGroundingOption {
	return func(g *DocGroundingGuard) { g.recordOnly = b }
}

// NewDocGroundingGuard builds a DocGroundingGuard with sensible defaults
// and the given options applied. The exists predicate is required; pass it
// via WithExists.
func NewDocGroundingGuard(opts ...DocGroundingOption) *DocGroundingGuard {
	g := &DocGroundingGuard{
		readTools:   map[string]struct{}{"Read": {}},
		writeTools:  map[string]struct{}{"Write": {}},
		pathKeys:    []string{"file_path", "path", "file", "filename"},
		contentKey:  "content",
		docSuffixes: []string{".md"},
		maxRefusals: 2,
		maxReport:   10,
		read:        map[string]struct{}{},
		refusals:    map[string]int{},
	}
	for _, o := range opts {
		o(g)
	}
	return g
}

// path extracts the target file path from a tool call's input, checking the
// configured path keys. Mirrors DocGroundingGuard._path.
func (g *DocGroundingGuard) path(inp map[string]any) string {
	for _, k := range g.pathKeys {
		if v, ok := inp[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// Check is the filter entry point: returns a steering/refusal message to
// surface to the model, or "" to allow the call. Mirrors
// DocGroundingGuard.__call__.
func (g *DocGroundingGuard) Check(stageID, name string, inp map[string]any) string {
	// Track reads so "grounded" can later mean read-and-exists if desired.
	if _, ok := g.readTools[name]; ok {
		if p := g.path(inp); p != "" {
			g.read[strings.TrimLeft(p, "/")] = struct{}{}
		}
		return ""
	}
	if _, ok := g.writeTools[name]; !ok {
		return ""
	}
	p := g.path(inp)
	if p == "" {
		return ""
	}
	// Only check document writes (configurable suffix set).
	if !hasSuffixAny(p, g.docSuffixes) {
		return ""
	}
	content := ""
	if v, ok := inp[g.contentKey].(string); ok {
		content = v
	}
	refs := findPathRefs(content)
	if len(refs) == 0 {
		return ""
	}
	missing := []string{}
	for _, r := range refs {
		if g.exists == nil || !g.exists(strings.TrimLeft(r, "/")) {
			missing = append(missing, r)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	g.Events = append(g.Events, DocGroundingEvent{
		Stage:   stageID,
		Path:    p,
		Action:  "ungrounded_refs",
		Missing: missing,
	})
	g.refusals[p]++
	if g.recordOnly || g.refusals[p] > g.maxRefusals {
		return "" // bounded — don't deadlock the stage; a gate still records it
	}
	limit := g.maxReport
	if limit > len(missing) {
		limit = len(missing)
	}
	shown := strings.Join(missing[:limit], ", ")
	more := ""
	if len(missing) > limit {
		more = fmt.Sprintf(" (+%d more)", len(missing)-limit)
	}
	return fmt.Sprintf(
		"Refused: %q references paths that do not exist: %s%s. "+
			"Correct them to real paths from the repository map / the files you've read "+
			"(or remove the claim), then write again.",
		p, shown, more)
}

// findPathRefs extracts path-ish tokens from the content, dedups, and keeps
// only those with a code/file extension.
func findPathRefs(content string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, m := range pathRE.FindAllString(content, -1) {
		if _, ok := seen[m]; ok {
			continue
		}
		if !hasSuffixAny(m, codeSuffixes) {
			continue
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

func hasSuffixAny(s string, suffixes []string) bool {
	for _, suf := range suffixes {
		if strings.HasSuffix(s, suf) {
			return true
		}
	}
	return false
}
