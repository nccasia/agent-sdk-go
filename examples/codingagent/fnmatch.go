package codingagent

import (
	"regexp"
	"strings"
	"sync"
)

// fnmatch mirrors Python's fnmatch.fnmatch: shell-style glob where `*` matches
// everything (including path separators), `?` matches one char, `[...]`
// matches a character set. Translation follows fnmatch.translate.
func fnmatch(name, pattern string) bool {
	rx := compileFnmatch(pattern)
	return rx.MatchString(name)
}

var (
	fnmatchCache = map[string]*regexp.Regexp{}
	fnmatchMu    sync.Mutex
)

func compileFnmatch(pattern string) *regexp.Regexp {
	fnmatchMu.Lock()
	defer fnmatchMu.Unlock()
	if rx, ok := fnmatchCache[pattern]; ok {
		return rx
	}
	rx := regexp.MustCompile(translateFnmatch(pattern))
	fnmatchCache[pattern] = rx
	return rx
}

// translateFnmatch converts an fnmatch pattern to an anchored Go regexp,
// following CPython's fnmatch.translate semantics (* → .*, ? → ., [..] sets).
func translateFnmatch(pat string) string {
	var b strings.Builder
	b.WriteString(`(?s)\A`)
	i, n := 0, len(pat)
	for i < n {
		c := pat[i]
		i++
		switch c {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		case '[':
			j := i
			if j < n && (pat[j] == '!' || pat[j] == ']') {
				j++
			}
			for j < n && pat[j] != ']' {
				j++
			}
			if j >= n {
				b.WriteString(`\[`)
			} else {
				stuff := pat[i:j]
				stuff = strings.ReplaceAll(stuff, `\`, `\\`)
				i = j + 1
				b.WriteByte('[')
				if len(stuff) > 0 && stuff[0] == '!' {
					b.WriteByte('^')
					stuff = stuff[1:]
				} else if len(stuff) > 0 && stuff[0] == '^' {
					b.WriteByte('\\')
				}
				b.WriteString(stuff)
				b.WriteByte(']')
			}
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString(`\z`)
	return b.String()
}
