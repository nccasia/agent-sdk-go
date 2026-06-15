// Package memory ports the agent_sdk memory subsystem: the durable scoped store
// (Memory), the two-tier universal substrate (MemoryStore/MemoryEntry), the
// turn-scoped scratchpad, the recall/note tools, the prefetch index, and the
// semantic cache. Ported from agent_sdk/memory/.
package memory

import (
	"encoding/json"
	"regexp"
	"strings"
)

// EstTokens mirrors agent_sdk/skills.est_tokens (len(runes)//4).
func EstTokens(text string) int {
	n := len([]rune(text)) / 4
	if n < 0 {
		return 0
	}
	return n
}

// A line is salient if it carries decision-relevant signal: a number, a path/
// identifier, an ALLCAPS token, or a key:value. These are the needles a digest
// must keep. Mirrors _SALIENT_RE.
var salientRE = regexp.MustCompile(`\d|[A-Z]{3,}|[\w./-]+\.[a-zA-Z]{1,5}\b|/[\w./-]+|:\s*\S`)

// briefArgs renders args compactly for a digest label.
func briefArgs(args any, limit int) string {
	s := ""
	if b, err := json.Marshal(args); err == nil {
		s = string(b)
	} else {
		s = stringify(args)
	}
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) > limit {
		return string([]rune(s)[:limit]) + "…"
	}
	return s
}

func excerpt(text string, limit int) string {
	s := strings.Join(strings.Fields(text), " ")
	if len([]rune(s)) > limit {
		return string([]rune(s)[:limit]) + "…"
	}
	return s
}

// DeterministicDigest builds a free, deterministic dense digest of body for a
// kind entry: the head line plus the most salient lines, labeled by kind and
// (for tool results) the tool+args. Deterministic: same input → same digest.
func DeterministicDigest(kind string, meta map[string]any, body string, maxChars, maxSalient int) string {
	if maxChars <= 0 {
		maxChars = 240
	}
	if maxSalient <= 0 {
		maxSalient = 3
	}
	var lines []string
	for _, ln := range strings.Split(body, "\n") {
		ln = strings.TrimSpace(ln)
		if ln != "" {
			lines = append(lines, ln)
		}
	}
	head := ""
	if len(lines) > 0 {
		head = lines[0]
	}
	var salient []string
	for _, ln := range linesAfterFirst(lines) {
		if salientRE.MatchString(ln) {
			salient = append(salient, ln)
		}
		if len(salient) >= maxSalient {
			break
		}
	}
	var gistInput string
	if len(salient) > 0 {
		gistInput = strings.Join(append([]string{head}, salient...), " · ")
	} else {
		gistInput = head
	}
	gist := excerpt(gistInput, maxChars)
	label := kind
	if tool, ok := meta["tool"]; ok && tool != nil {
		args := meta["args"]
		if args == nil {
			args = meta["input"]
		}
		if args == nil {
			args = map[string]any{}
		}
		label = stringify(tool) + "(" + briefArgs(args, 48) + ")"
	}
	if gist != "" {
		return "[" + kind + "] " + label + " → " + gist
	}
	return "[" + kind + "] " + label
}

func linesAfterFirst(lines []string) []string {
	if len(lines) <= 1 {
		return nil
	}
	return lines[1:]
}

// CompressionRatio is digest tokens / body tokens — small is dense. 1.0 if the
// body is empty.
func CompressionRatio(digest, body string) float64 {
	b := EstTokens(body)
	if b == 0 {
		b = 1
	}
	return round4(float64(EstTokens(digest)) / float64(b))
}

func round4(v float64) float64 {
	if v < 0 {
		return -round4(-v)
	}
	return float64(int64(v*1e4+0.5)) / 1e4
}

func itoaInt(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		p--
		b[p] = '-'
	}
	return string(b[p:])
}

func stringify(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	default:
		if b, err := json.Marshal(x); err == nil {
			return string(b)
		}
		return ""
	}
}
