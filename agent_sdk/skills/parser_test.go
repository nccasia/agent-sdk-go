// Ported from agent_sdk/tests/test_skill_parser.py — markdown navigation
// primitives used by the layered (chunk → file → skill) reading path.
package skills

import (
	"strings"
	"testing"
)

func TestEstTokens(t *testing.T) {
	if got := EstTokens(""); got != 0 {
		t.Errorf("empty text: got %d, want 0", got)
	}
	// 4 chars per token.
	if got := EstTokens("abcd"); got != 1 {
		t.Errorf("4 chars: got %d, want 1", got)
	}
	if got := EstTokens("abcde"); got != 1 {
		t.Errorf("5 chars: got %d, want 1 (floor)", got)
	}
	if got := EstTokens(strings.Repeat("x", 40)); got != 10 {
		t.Errorf("40 chars: got %d, want 10", got)
	}
}

func TestSlugifyHeading(t *testing.T) {
	cases := map[string]string{
		"Hello World":   "hello-world",
		"hello-world":   "hello-world",
		"Hello  World!": "hello-world",
		"-leading":      "leading",
		"":              "section",
		"---":           "section",
		"héllo":         "hllo", // non-ASCII dropped (NFKD approximation)
	}
	for in, want := range cases {
		if got := slugifyHeading(in); got != want {
			t.Errorf("slugifyHeading(%q): got %q, want %q", in, got, want)
		}
	}
}

func TestSplitSectionsDedupesHeadingIDs(t *testing.T) {
	md := "## A\nbody 1\n## A\nbody 2\n## A\nbody 3\n"
	secs := SplitSections(md)
	if len(secs) != 3 {
		t.Fatalf("got %d sections, want 3", len(secs))
	}
	if secs[0].ID != "a" {
		t.Errorf("first id: got %q, want a", secs[0].ID)
	}
	if secs[1].ID != "a-2" {
		t.Errorf("second id: got %q, want a-2", secs[1].ID)
	}
	if secs[2].ID != "a-3" {
		t.Errorf("third id: got %q, want a-3", secs[2].ID)
	}
}

func TestSplitSectionsIntroSection(t *testing.T) {
	md := "lead paragraph\n\n## H1\nbody\n"
	secs := SplitSections(md)
	if len(secs) != 2 {
		t.Fatalf("got %d sections, want 2", len(secs))
	}
	if secs[0].Heading != "intro" {
		t.Errorf("intro heading: got %q, want intro", secs[0].Heading)
	}
	if !strings.Contains(secs[0].Content, "lead paragraph") {
		t.Errorf("intro content missing lead paragraph: %q", secs[0].Content)
	}
}

func TestFileTocListsSections(t *testing.T) {
	md := "## One\nalpha alpha\n## Two\nbeta beta\n"
	toc := FileToc(md)
	if !strings.Contains(toc, "Table of contents") {
		t.Errorf("missing header: %q", toc)
	}
	if !strings.Contains(toc, "[one]") || !strings.Contains(toc, "One") {
		t.Errorf("missing first entry: %q", toc)
	}
	if !strings.Contains(toc, "[two]") || !strings.Contains(toc, "Two") {
		t.Errorf("missing second entry: %q", toc)
	}
	if !strings.Contains(toc, "tokens)") {
		t.Errorf("missing token estimate: %q", toc)
	}
}

func TestFilePurposePrefersFrontmatter(t *testing.T) {
	md := "---\ndescription: My purpose\n---\n# Heading\nbody"
	if got := FilePurpose(md); got != "My purpose" {
		t.Errorf("frontmatter desc: got %q, want My purpose", got)
	}
}

func TestFilePurposeFallsBackToHeading(t *testing.T) {
	md := "## Just a heading\nbody"
	if got := FilePurpose(md); got != "Just a heading" {
		t.Errorf("heading fallback: got %q, want Just a heading", got)
	}
}

func TestFilePurposeFallsBackToFirstLine(t *testing.T) {
	md := "first line of body\nmore body"
	if got := FilePurpose(md); got != "first line of body" {
		t.Errorf("first-line fallback: got %q", got)
	}
}

func TestSplitFrontmatterHappyPath(t *testing.T) {
	md := "---\nname: Code review\ndescription: Review code\n---\n# Body\n"
	front, body := SplitFrontmatter(md)
	if front["name"] != "Code review" {
		t.Errorf("name: got %q", front["name"])
	}
	if front["description"] != "Review code" {
		t.Errorf("description: got %q", front["description"])
	}
	if !strings.HasPrefix(strings.TrimSpace(body), "# Body") {
		t.Errorf("body prefix wrong: %q", body)
	}
}

func TestSplitFrontmatterNoFrontmatter(t *testing.T) {
	front, body := SplitFrontmatter("just a body\n")
	if len(front) != 0 {
		t.Errorf("expected empty front, got %v", front)
	}
	if !strings.Contains(body, "just a body") {
		t.Errorf("body not returned as-is: %q", body)
	}
}

func TestSearchBundleFindsHits(t *testing.T) {
	pack := &SkillPack{
		ID:    "advisor",
		Name:  "Advisor",
		Files: map[string]string{"rules.md": "## Reservation\nReserve up to two semesters."},
	}
	hits := SearchBundle([]*SkillPack{pack}, "reservation semesters", 5)
	if len(hits) == 0 {
		t.Fatalf("expected at least one hit, got 0")
	}
	if hits[0].File != "rules.md" {
		t.Errorf("hit file: got %q, want rules.md", hits[0].File)
	}
	if hits[0].Section != "reservation" {
		t.Errorf("hit section: got %q, want reservation", hits[0].Section)
	}
}

func TestSearchBundleRespectsTopK(t *testing.T) {
	pack := &SkillPack{
		ID: "x",
		Files: map[string]string{
			"a.md": "## A\nalpha",
			"b.md": "## B\nalpha",
			"c.md": "## C\nalpha",
		},
	}
	hits := SearchBundle([]*SkillPack{pack}, "alpha", 2)
	if len(hits) != 2 {
		t.Errorf("topK=2: got %d hits, want 2", len(hits))
	}
}
