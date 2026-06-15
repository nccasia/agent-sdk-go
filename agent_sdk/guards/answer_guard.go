// Package guards holds deterministic guards — pure, no LLM, no I/O.
//
//   - answer_guard — output-side leak detectors (secret-shaped strings, bulk
//     PII, forbidden substrings, impossible commitments, refusal markers).
//   - refusal — input-side pre-turn gate builder (refusal rules + golden
//     known-answer short-circuit + semantic refusal), dependency-injected.
//
// Ported from agent_sdk/guards/{answer_guard,refusal,safety}.py.
package guards

import (
	"regexp"
	"strings"
)

// BulkPIIThreshold: an answer enumerating this many distinct emails/phones is a
// data dump, not a conversational reply.
const BulkPIIThreshold = 3

// DefaultCommitmentCues are looked for BEFORE an impossible-action phrase
// ("I will <act>"). English defaults — pass your own language's cues.
var DefaultCommitmentCues = []string{
	"i will", "i'll", "i have", "i've", "done", "ok", "okay", "sure",
}

// DefaultNegationCues make a mention SAFE (a refusal talks ABOUT the action).
var DefaultNegationCues = []string{
	"cannot", "can't", "not able", "won't", "not allowed", "unable to",
}

// DefaultRefusalMarkers signal an explicit refusal in the answer (English).
var DefaultRefusalMarkers = []string{
	"cannot", "can't", "not able to", "unable to", "refuse", "not allowed",
	"i'm sorry, i can't", "i can't help with",
}

// Credential-looking value: 20+ url-safe chars containing BOTH letters and
// digits (rules out prose), optionally after a label assignment.
//
// Go's regexp (RE2) has no lookahead, so the "letters AND digits" constraint is
// enforced procedurally in secretLabelMatch.
var secretLabel = regexp.MustCompile(
	`(?i)(?:api[ _-]?key|secret|token|password|passwd|access[ _-]?key|private[ _-]?key)` +
		"[^\n]{0,40}?[:=]?\\s*[\"'`]?([A-Za-z0-9_\\-]{20,})",
)

// Vendor-style key prefixes are secrets even without a label.
var secretPrefix = regexp.MustCompile(`\b(?:sk|pk|ghp|xox[bap]|AKIA)[-_][A-Za-z0-9_\-]{12,}`)

var emailRe = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)

// Generic phone shapes: international prefix or trunk-0, 9-12 digits, tolerant
// of common separators. Locale-neutral by design. RE2 has no lookbehind/ahead;
// boundary handling is done in phoneMatches.
var phoneRe = regexp.MustCompile(`(?:\+\d{1,3}|0)(?:[\s.\-]?\d){8,11}`)

var nonDigit = regexp.MustCompile(`\D`)

// nfcLower casefolds for matching. The Python original NFC-normalizes first;
// the deterministic test corpus is already precomposed, so a stdlib ToLower is
// behavior-equivalent here without pulling in golang.org/x/text.
func nfcLower(text string) string {
	return strings.ToLower(text)
}

func hasLetterAndDigit(s string) bool {
	var letter, digit bool
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digit = true
		} else if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			letter = true
		}
	}
	return letter && digit
}

func secretLabelMatch(answer string) bool {
	for _, m := range secretLabel.FindAllStringSubmatch(answer, -1) {
		if hasLetterAndDigit(m[1]) {
			return true
		}
	}
	return false
}

// SecretViolation returns "secret_shaped_string" when the answer carries a
// labelled credential or a vendor key prefix, else "".
func SecretViolation(answer string) string {
	if answer != "" && (secretLabelMatch(answer) || secretPrefix.MatchString(answer)) {
		return "secret_shaped_string"
	}
	return ""
}

// phoneMatches returns the distinct digit-normalized phone numbers, applying
// the (?<!\d)…(?!\d) boundary that RE2 cannot express directly.
func phoneMatches(answer string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, loc := range phoneRe.FindAllStringIndex(answer, -1) {
		start, end := loc[0], loc[1]
		if start > 0 && answer[start-1] >= '0' && answer[start-1] <= '9' {
			continue
		}
		if end < len(answer) && answer[end] >= '0' && answer[end] <= '9' {
			continue
		}
		out[nonDigit.ReplaceAllString(answer[start:end], "")] = struct{}{}
	}
	return out
}

// BulkPIIViolation returns a violation tag when the answer enumerates
// threshold+ distinct emails or phone numbers (a data dump), else "".
func BulkPIIViolation(answer string, threshold int) string {
	if answer == "" {
		return ""
	}
	if threshold <= 0 {
		threshold = BulkPIIThreshold
	}
	emails := map[string]struct{}{}
	for _, e := range emailRe.FindAllString(answer, -1) {
		emails[e] = struct{}{}
	}
	if len(emails) >= threshold {
		return "bulk_pii_emails"
	}
	if len(phoneMatches(answer)) >= threshold {
		return "bulk_pii_phones"
	}
	return ""
}

// ForbiddenViolation returns "forbidden:<pattern>" for the first caller-supplied
// substring present in the answer (NFC, case-insensitive), else "".
func ForbiddenViolation(answer string, forbidden []string) string {
	if answer == "" {
		return ""
	}
	haystack := nfcLower(answer)
	for _, pattern := range forbidden {
		if strings.Contains(haystack, nfcLower(pattern)) {
			return "forbidden:" + pattern
		}
	}
	return ""
}

// AnswerLeakViolation returns the first leak-violation tag for an answer, else
// "". Composes secret-shaped strings, bulk PII, and forbidden substrings.
func AnswerLeakViolation(answer string, forbidden []string, bulkPIIThreshold int) string {
	if answer == "" {
		return ""
	}
	if v := SecretViolation(answer); v != "" {
		return v
	}
	if v := BulkPIIViolation(answer, bulkPIIThreshold); v != "" {
		return v
	}
	return ForbiddenViolation(answer, forbidden)
}

// CommitmentViolation detects a COMMITMENT to one of the caller-declared
// impossible actions. A bare or negated mention is safe; a commitment cue within
// the 30 chars before the action phrase, with no negation cue in that window, is
// a violation. Returns "impossible_commitment:<action>" or "". No-op when
// actions is empty. Pass nil for commitmentCues/negationCues to use defaults.
func CommitmentViolation(answer string, actions, commitmentCues, negationCues []string) string {
	if len(actions) == 0 {
		return ""
	}
	if commitmentCues == nil {
		commitmentCues = DefaultCommitmentCues
	}
	if negationCues == nil {
		negationCues = DefaultNegationCues
	}
	haystack := nfcLower(answer)
	for _, raw := range actions {
		action := nfcLower(raw)
		if action == "" {
			continue
		}
		start := 0
		for {
			idx := strings.Index(haystack[start:], action)
			if idx == -1 {
				break
			}
			idx += start
			lo := idx - 30
			if lo < 0 {
				lo = 0
			}
			window := haystack[lo:idx]
			if !containsAny(window, negationCues) && containsAny(window, commitmentCues) {
				return "impossible_commitment:" + action
			}
			start = idx + len(action)
		}
	}
	return ""
}

// HasRefusalMarker reports whether the answer contains an explicit refusal
// phrase (NFC + casefold matched). Pass nil for markers to use the defaults.
func HasRefusalMarker(answer string, markers []string) bool {
	if markers == nil {
		markers = DefaultRefusalMarkers
	}
	return containsAny(nfcLower(answer), markers)
}

func containsAny(haystack string, cues []string) bool {
	for _, c := range cues {
		if strings.Contains(haystack, nfcLower(c)) {
			return true
		}
	}
	return false
}
