// Establish — native, deterministic fact offload.
//
// Relying on the model to call “note“ for every fact is unreliable (it
// skips some). Establish makes memorization native: after a turn it scans
// the user's message for **fact-shaped** statements (bullet items, and
// value-bearing sentences — a date, a time, an @owner, a metric, a
// decision) and offloads each to durable memory. So "the user told me X"
// reliably becomes a recallable fact, no model cooperation required. Pure
// and deterministic; deduped by content. Ported from
// agent_sdk/memory/establish.py.
package memory

import (
	"crypto/sha1"
	"encoding/hex"
	"regexp"
	"strings"
)

var (
	estBullet = regexp.MustCompile(`(?m)^\s*[-*•]\s+(.{6,240}?)\s*$`)
	estValue  = regexp.MustCompile(`\d{4}-\d{2}-\d{2}|\b\d{1,2}:\d{2}\b|@\w+|\b\d+(?:\.\d+)?\s?(?:ms|%)\b|\b(?:Mon|Tue|Wed|Thu|Fri|Sat|Sun)\w*\b`)
	estCue    = regexp.MustCompile(`(?i)\b(decided|agreed|owner|deadline|scheduled|rollout|cutover|sla|must|policy|rule|hotline|requirement|trigger|window)\b`)
	estSplit  = regexp.MustCompile(`[.!?]\s+|\n`)
	estValTok = regexp.MustCompile(`\d{4}-\d{2}-\d{2}|\b\d{1,2}:\d{2}\b|@\w+|\b\d+(?:\.\d+)?\s?(?:ms|%)?\b|\b(?:mon|tue|wed|thu|fri|sat|sun)\w*\b`)
)

// FactKey derives a topic key from a fact — the fact with its VALUES
// stripped, so "nova rollout Mon 17:00" and "nova rollout Wed 12:00" share
// a key (the fact updated, not duplicated).
func FactKey(fact string) string {
	topic := estValTok.ReplaceAllString(strings.ToLower(fact), "")
	topic = strings.Join(strings.Fields(topic), " ")
	topic = strings.Trim(topic, " -:")
	if topic == "" {
		topic = strings.ToLower(fact)
	}
	sum := sha1.Sum([]byte(topic))
	return "est-" + hex.EncodeToString(sum[:])[:12]
}

// SalientFacts returns the fact-shaped statements in text worth
// remembering — bullets first, then value/cue-bearing sentences.
// Deterministic, deduped, length-bounded.
func SalientFacts(text string) []string {
	const maxFacts = 16
	out := []string{}
	seen := map[string]struct{}{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		s = strings.Trim(s, "-*•")
		s = strings.TrimSpace(s)
		k := strings.ToLower(s)
		if len(s) < 8 || len(s) > 240 {
			return
		}
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		out = append(out, s)
	}
	for _, m := range estBullet.FindAllStringSubmatch(text, -1) {
		if len(m) > 1 {
			add(m[1])
			if len(out) >= maxFacts {
				return out
			}
		}
	}
	for _, sent := range estSplit.Split(text, -1) {
		if estValue.MatchString(sent) || estCue.MatchString(sent) {
			add(sent)
			if len(out) >= maxFacts {
				break
			}
		}
	}
	return out
}
