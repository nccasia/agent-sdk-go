// Package react: enriched conversation context — the engine's living working
// memory of a turn's conversation, distilled and persisted across turns so
// context management has more to reason with than the raw transcript.
//
// The richest, highest-density context is not the history — it is a small
// curated distillation of *where the conversation is*. This profile maintains
// that distillation:
//
//   - intent        — qna / task / heavy_doc / channel / config (drives path, tools, budgets)
//   - entities      — the salient topics in play (sharpens relevance for nodes AND tools)
//   - artifacts     — manifest of offloaded files/docs available to read (the workspace map)
//   - obligations   — open sub-questions / todos not yet answered (nothing silently dropped)
//   - facts         — established constraints/answers (the value-aware KEEP anchors)
//   - recent_tools  — tools used lately (a utility prior — used last turn ⇒ likely relevant)
//
// It exposes three views over that one state:
//
//   - Signals   — deterministic flags for the lobe network's signal ctx
//     (never an LLM judging the pipeline — these are free signals).
//   - Render    — a compact, high-density context node the model reads.
//   - KeepAnchors — the ids/keys to PIN full (facts + artifact map) so the
//     funnel never demotes them.
//
// Pure and deterministic. Intent/tool-family recognition is keyword-first
// (cheap); an LLM touch can refine it upstream, but the profile itself never
// calls a model. Flags reflect the CURRENT turn; persistent fields (facts,
// artifacts, obligations) accumulate and the caller decays/clears them as
// the conversation drifts.
//
// Ported from agent_sdk/react/conversation.py.
package react

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// intentCues maps an intent name to the (cue tokens, tool families) it needs.
// The model uses these to recognize the intent from the user's own words
// (the vocabulary the model uses), not the tool names — closing the lexical
// gap that makes name-matching drop fetch_messages/tasks.list.
var intentCues = map[string]struct {
	cues   map[string]struct{}
	family string
}{
	"channel": {
		cues:   tokensVNEN("said say message messages chat channel discussed conversation noi tin nhan kenh thread"),
		family: "channel",
	},
	"task": {
		cues:   tokensVNEN("remind reminder schedule task todo tasks cron daily nhac lich hen nhiem"),
		family: "tasks",
	},
	"heavy_doc": {
		cues:   tokensVNEN("convert document markdown html rewrite summarize paste pasted doc file tai lieu chuyen"),
		family: "workspace",
	},
	"config": {
		cues:   tokensVNEN("connect configure setup settings onboarding admin cau hinh ket noi cai"),
		family: "admin",
	},
}

var familyTools = map[string][]string{
	"channel":   {},
	"tasks":     {"tasks.list", "tasks.create", "todos.update"},
	"workspace": {"Glob", "Grep", "Read", "Write"},
	"admin":     {"admin.overview", "admin.connect_mello"},
	"kb":        {"kg.schema", "kg.query", "kg.read"},
}

// stopWords is the set of short or function words we drop before
// tokenizing a query for entities / intent cues.
var stopWords = func() map[string]struct{} {
	s := map[string]struct{}{}
	for _, w := range strings.Fields(
		"the a an of to in on for and or is are what who how when where why can you we i me my do does this that it please help " +
			"minh cho la gi co khong va cac mot nay ban giup") {
		s[w] = struct{}{}
	}
	return s
}()

// wordRE matches runs of word characters; the Python equivalent uses
// “re.UNICODE“ so we use the Unicode word class.
var wordRE = regexp.MustCompile(`[\p{L}\p{N}_]+`)

// tokensVNEN splits a space-delimited list of words into a set.
func tokensVNEN(list string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, w := range strings.Fields(list) {
		out[w] = struct{}{}
	}
	return out
}

// ConversationProfile is the living state of a turn's conversation: intent,
// entities, artifacts, obligations, facts, recent_tools, needs_tools.
type ConversationProfile struct {
	Intent      string
	Entities    map[string]struct{}
	Artifacts   map[string]int
	Obligations []string
	Facts       map[string]string
	RecentTools []string
	NeedsTools  []string
}

// NewConversationProfile builds an empty profile with the default
// dataclass-style defaults (intent "qna"; empty collections; needs ["kb"]).
func NewConversationProfile() *ConversationProfile {
	return &ConversationProfile{
		Intent:      "qna",
		Entities:    map[string]struct{}{},
		Artifacts:   map[string]int{},
		Obligations: []string{},
		Facts:       map[string]string{},
		RecentTools: []string{},
		NeedsTools:  []string{},
	}
}

// Update folds one turn's signals into the living state. Deterministic.
func (p *ConversationProfile) Update(
	query string,
	toolsUsed []string,
	facts map[string]string,
	artifacts map[string]int,
	addObligations []string,
	resolvedObligations []string,
	maxEntities int,
	maxRecentTools int,
) *ConversationProfile {
	if maxEntities <= 0 {
		maxEntities = 40
	}
	if maxRecentTools <= 0 {
		maxRecentTools = 12
	}
	if query != "" {
		qt := tokensOf(query)
		for t := range qt {
			p.Entities[t] = struct{}{}
		}
		if len(p.Entities) > maxEntities {
			// keep last (newest) maxEntities — drop oldest by deterministic order
			keys := sortedKeys(p.Entities)
			keep := keys[len(keys)-maxEntities:]
			p.Entities = map[string]struct{}{}
			for _, k := range keep {
				p.Entities[k] = struct{}{}
			}
		}
		intent, needs := p.recognize(qt)
		p.Intent = intent
		p.NeedsTools = needs
	}
	if len(toolsUsed) > 0 {
		p.RecentTools = append(p.RecentTools, toolsUsed...)
		if len(p.RecentTools) > maxRecentTools {
			p.RecentTools = p.RecentTools[len(p.RecentTools)-maxRecentTools:]
		}
	}
	if len(facts) > 0 {
		for k, v := range facts {
			p.Facts[k] = v
		}
	}
	if len(artifacts) > 0 {
		for k, v := range artifacts {
			p.Artifacts[k] = v
		}
	}
	for _, o := range addObligations {
		if !contains(p.Obligations, o) {
			p.Obligations = append(p.Obligations, o)
		}
	}
	if len(resolvedObligations) > 0 {
		out := p.Obligations[:0]
		for _, o := range p.Obligations {
			if !contains(resolvedObligations, o) {
				out = append(out, o)
			}
		}
		p.Obligations = out
	}
	return p
}

// recognize picks an intent + the tool families this turn needs, given the
// query tokens + the current artifact map. Heavy-doc is sticky once an
// artifact is offloaded.
func (p *ConversationProfile) recognize(qt map[string]struct{}) (string, []string) {
	scored := map[string]int{}
	for name, cfg := range intentCues {
		overlap := 0
		for t := range qt {
			if _, ok := cfg.cues[t]; ok {
				overlap++
			}
		}
		scored[name] = overlap
	}
	best := "qna"
	bestScore := 0
	for name, s := range scored {
		if s > bestScore {
			best = name
			bestScore = s
		}
	}
	intent := best
	if bestScore == 0 {
		intent = "qna"
	}
	if len(p.Artifacts) > 0 {
		intent = "heavy_doc"
	}
	families := []string{}
	seen := map[string]struct{}{}
	for _, cfg := range intentCues {
		overlap := false
		for t := range qt {
			if _, ok := cfg.cues[t]; ok {
				overlap = true
				break
			}
		}
		if overlap {
			if _, ok := seen[cfg.family]; !ok {
				seen[cfg.family] = struct{}{}
				families = append(families, cfg.family)
			}
		}
	}
	if len(p.Artifacts) > 0 {
		if _, ok := seen["workspace"]; !ok {
			seen["workspace"] = struct{}{}
			families = append(families, "workspace")
		}
	}
	if _, ok := seen["kb"]; !ok {
		seen["kb"] = struct{}{}
		families = append(families, "kb")
	}
	return intent, families
}

// Signals returns free deterministic flags for the lobe-network signal ctx.
func (p *ConversationProfile) Signals() map[string]float64 {
	fams := map[string]struct{}{}
	for _, f := range p.NeedsTools {
		fams[f] = struct{}{}
	}
	return map[string]float64{
		"intent_qna":          boolFloat(p.Intent == "qna"),
		"intent_task":         boolFloat(p.Intent == "task"),
		"intent_channel":      boolFloat(p.Intent == "channel"),
		"intent_heavy_doc":    boolFloat(p.Intent == "heavy_doc"),
		"intent_config":       boolFloat(p.Intent == "config"),
		"needs_kb":            boolFloat(hasKey(fams, "kb")),
		"needs_channel_tools": boolFloat(hasKey(fams, "channel")),
		"needs_tasks_tools":   boolFloat(hasKey(fams, "tasks")),
		"needs_offload":       boolFloat(len(p.Artifacts) > 0),
		"has_obligations":     boolFloat(len(p.Obligations) > 0),
		"has_anchors":         boolFloat(len(p.Facts) > 0),
	}
}

// KeepTools returns the concrete tools to KEEP exposed this turn
// (intent-driven, not lexical): every tool in a needed family + recently-used
// tools (utility prior).
func (p *ConversationProfile) KeepTools() map[string]struct{} {
	out := map[string]struct{}{}
	for _, t := range p.RecentTools {
		out[t] = struct{}{}
	}
	for _, fam := range p.NeedsTools {
		for _, t := range familyTools[fam] {
			out[t] = struct{}{}
		}
	}
	return out
}

// KeepAnchors returns the ids/keys to PIN full so the funnel never demotes
// them: established facts (constraints/answers) + the offloaded-artifact map.
func (p *ConversationProfile) KeepAnchors() map[string]struct{} {
	out := map[string]struct{}{}
	for k := range p.Facts {
		out[k] = struct{}{}
	}
	for k := range p.Artifacts {
		out[k] = struct{}{}
	}
	return out
}

// Render produces a compact, high-density context node the model reads —
// "where we are".
func (p *ConversationProfile) Render() string {
	lines := []string{
		"## Conversation state",
		"intent: " + p.Intent,
	}
	if len(p.Entities) > 0 {
		keys := sortedKeys(p.Entities)
		if len(keys) > 8 {
			keys = keys[:8]
		}
		lines = append(lines, "about: "+strings.Join(keys, ", "))
	}
	if len(p.Facts) > 0 {
		lines = append(lines, "established:")
		keys := sortedStringKeys(p.Facts)
		if len(keys) > 8 {
			keys = keys[:8]
		}
		for _, k := range keys {
			lines = append(lines, "  - "+k+": "+p.Facts[k])
		}
	}
	if len(p.Obligations) > 0 {
		lines = append(lines, "open (not yet answered):")
		limit := len(p.Obligations)
		if limit > 8 {
			limit = 8
		}
		for _, o := range p.Obligations[:limit] {
			lines = append(lines, "  - "+o)
		}
	}
	if len(p.Artifacts) > 0 {
		lines = append(lines, fmtInt(len(p.Artifacts))+" offloaded files (read via the workspace tools)")
	}
	return strings.Join(lines, "\n")
}

// ── helpers ──────────────────────────────────────────────────────────────────

// tokensOf normalizes text: NFC + lower, drops stop words and tokens shorter
// than 3 runes.
func tokensOf(text string) map[string]struct{} {
	lower := strings.ToLower(text)
	out := map[string]struct{}{}
	for _, m := range wordRE.FindAllString(lower, -1) {
		if len(m) < 3 {
			continue
		}
		if _, stop := stopWords[m]; stop {
			continue
		}
		// Skip purely-digit tokens (per the Python _tokens behavior).
		allDigits := true
		for _, r := range m {
			if !unicode.IsDigit(r) {
				allDigits = false
				break
			}
		}
		if allDigits {
			continue
		}
		out[m] = struct{}{}
	}
	return out
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func hasKey(m map[string]struct{}, k string) bool {
	_, ok := m[k]
	return ok
}

func boolFloat(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedStringKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func fmtInt(n int) string {
	// keep simple: Go's strconv is fine; this avoids an import for one call
	digits := "0123456789"
	if n == 0 {
		return "0"
	}
	out := ""
	for n > 0 {
		out = string(digits[n%10]) + out
		n /= 10
	}
	return out
}
