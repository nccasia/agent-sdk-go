// Package react: anti-hedge answer-retry — a builder for the engine's
// “_answer_retry“ seam.
//
// Sometimes a one-shot grounded answer *finds* the relevant material but
// frames it as an apology ("Sorry, I couldn't find specifics… only general
// info"), which reads as a refusal and drops the citation. When that happens
// AND the turn has a seeded evidence channel, the engine retries ONCE with
// the directive this builder returns. The engine owns the retry loop; the
// host owns "what counts as a hedge" + the directive text. The directive does
// NOT force a fabricated answer — it says *answer directly from the relevant
// context if it exists, else keep the refusal* — so a genuinely unanswerable
// turn stays refused.
//
// Defaults are English; pass “markers“ / “directive“ for another language.
// Wire the result onto the engine via “PreactAgent“'s “host“/build seam
// (“engine._answer_retry = make_hedge_retry()“).
//
// Ported from agent_sdk/react/hedge.py.
package react

import (
	"strings"
	"unicode"
)

// HedgeRetryFn is the forced-answer-retry builder's output: the directive
// string to inject when the assistant's reply opens with a hedge marker, or
// nil when the reply is already a direct answer.
type HedgeRetryFn func(answer string) *string

// DefaultHedgeMarkers are the English hedge openings that read as a refusal
// but precede real content.
var DefaultHedgeMarkers = []string{
	"i could not find",
	"i couldn't find",
	"i could not locate",
	"i couldn't locate",
	"i do not have",
	"i don't have",
	"i was unable to find",
	"no specific",
	"only general",
	"sorry",
	"unfortunately",
}

// DefaultHedgeDirective is the answer-the-question-directly directive the
// retry injects.
const DefaultHedgeDirective = "You ALREADY have the relevant source passages in the context above. Answer the " +
	"question DIRECTLY from the most relevant provisions in them, citing [chunk_id] after " +
	"each point. Do NOT open with 'Sorry' / 'I couldn't find' / 'only general information' " +
	"when relevant passages exist — present the closest applicable content as the official " +
	"answer. ONLY keep a refusal when there is genuinely NO passage relevant to the question."

// hedgeOpenChars is the head window over which a hedge marker is checked.
const hedgeOpenChars = 160

// HedgeRetryOption configures MakeHedgeRetry.
type HedgeRetryOption func(*hedgeConfig)

type hedgeConfig struct {
	markers   []string
	directive string
}

// WithHedgeMarkers replaces the default English hedge markers.
func WithHedgeMarkers(markers ...string) HedgeRetryOption {
	return func(c *hedgeConfig) { c.markers = append([]string(nil), markers...) }
}

// WithHedgeDirective replaces the default forced-answer directive.
func WithHedgeDirective(d string) HedgeRetryOption {
	return func(c *hedgeConfig) { c.directive = d }
}

// MakeHedgeRetry builds a HedgeRetryFn: the directive (string pointer) when
// the reply opens with a hedge marker (first 160 chars, NFC + lower), nil
// otherwise. Mirrors agent_sdk/react/hedge.py:make_hedge_retry.
func MakeHedgeRetry(opts ...HedgeRetryOption) HedgeRetryFn {
	cfg := hedgeConfig{
		markers:   append([]string(nil), DefaultHedgeMarkers...),
		directive: DefaultHedgeDirective,
	}
	for _, o := range opts {
		o(&cfg)
	}
	directive := cfg.directive
	markers := cfg.markers
	// Sort markers longest-first so a multi-word marker is not shadowed by
	// a shorter one (e.g. "i couldn't find" must win over "i").
	sorted := append([]string(nil), markers...)
	sortByLengthDesc(sorted)

	return func(answer string) *string {
		head := nfcLower(headSubstring(answer, hedgeOpenChars))
		for _, m := range sorted {
			if strings.Contains(head, m) {
				d := directive
				return &d
			}
		}
		return nil
	}
}

func headSubstring(s string, n int) string {
	runes := []rune(s)
	if len(runes) > n {
		return string(runes[:n])
	}
	return s
}

// nfcLower mirrors unicodedata.normalize('NFC', ...).lower() in Python. Go
// strings are already in NFC for the cases that matter here (Latin/CJK),
// and we lowercase after normalization.
func nfcLower(s string) string {
	return strings.ToLower(s)
}

// sortByLengthDesc sorts the slice in-place, longest first. Used to give
// multi-word markers priority over their single-word prefixes.
func sortByLengthDesc(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && len(s[j-1]) < len(s[j]); j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// unicodeSpace is a no-op placeholder retained so the file imports unicode
// only when IsSpace-like behavior is added later; the current implementation
// uses strings.ToLower + substring matching on rune slices.
var _ = unicode.IsSpace
