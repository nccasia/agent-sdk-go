// Package react ports the PreAct reasoning-loop helpers. funnel.go is the
// per-hop observation tiering — the heart of PreAct.
//
// The largest source of hop-over-hop bloat is raw tool output. TierObservations
// keeps the most recent observation(s) FULL and demotes older, spent ones to a
// one-line hint while keeping them discoverable and re-fetchable. The
// tool_use ⇄ tool_result tool_use_id pairing is preserved so the next provider
// call never sees an orphaned tool_use. Pure and deterministic: returns a NEW
// message list, never mutates the input. Idempotent — re-tiering an already
// demoted observation is a no-op (the spent marker is the sentinel).
//
// Ported from agent_sdk/react/funnel.py.
package react

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/nccasia/agent-sdk-go/agent_sdk/core/attention"
)

// SpentMarker prefixes a demoted (funneled-out) observation hint. Re-tiering an
// observation whose content already starts with it is a no-op.
const SpentMarker = "[spent observation]"

// CompactionMarker prefixes the single rolling summary folded in by
// CompactObservations.
const CompactionMarker = "[compacted earlier tool results]"

// EstTokens mirrors agent_sdk/skills.est_tokens (len(runes)//4).
func EstTokens(text string) int {
	n := len([]rune(text)) / 4
	if n < 0 {
		return 0
	}
	return n
}

func stringifyContent(content any) string {
	switch c := content.(type) {
	case nil:
		return ""
	case string:
		return c
	case []any:
		var parts []string
		for _, block := range c {
			if bm, ok := block.(map[string]any); ok {
				if t, _ := bm["text"].(string); t != "" {
					parts = append(parts, t)
				}
			} else {
				parts = append(parts, toStr(block))
			}
		}
		return strings.Join(parts, "\n")
	default:
		return toStr(content)
	}
}

func toStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func briefArgs(inp any, limit int) string {
	b, err := json.Marshal(inp)
	s := ""
	if err == nil {
		s = string(b)
	} else {
		s = toStr(inp)
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

func defaultHint(name string, inp any, content string, hintMaxChars int) string {
	call := "tool call"
	if name != "" {
		call = name + "(" + briefArgs(inp, 60) + ")"
	}
	return SpentMarker + " " + call + " → " + excerpt(content, hintMaxChars) +
		" (re-run the tool or read the source to expand)"
}

type toolUseMeta struct {
	name  string
	input any
}

func toolUseIndex(messages []map[string]any) map[string]toolUseMeta {
	index := map[string]toolUseMeta{}
	for _, msg := range messages {
		if role, _ := msg["role"].(string); role != "assistant" {
			continue
		}
		content, ok := msg["content"].([]any)
		if !ok {
			continue
		}
		for _, b := range content {
			bm, ok := b.(map[string]any)
			if !ok {
				continue
			}
			if t, _ := bm["type"].(string); t != "tool_use" {
				continue
			}
			tid, _ := bm["id"].(string)
			if tid == "" {
				continue
			}
			name, _ := bm["name"].(string)
			index[tid] = toolUseMeta{name: name, input: bm["input"]}
		}
	}
	return index
}

func contentBlocks(msg map[string]any) []any {
	if c, ok := msg["content"].([]any); ok {
		return c
	}
	return nil
}

func isObservation(msg map[string]any) bool {
	if role, _ := msg["role"].(string); role != "user" {
		return false
	}
	for _, b := range contentBlocks(msg) {
		if bm, ok := b.(map[string]any); ok {
			if t, _ := bm["type"].(string); t == "tool_result" {
				return true
			}
		}
	}
	return false
}

func isErrorObs(msg map[string]any) bool {
	for _, b := range contentBlocks(msg) {
		bm, ok := b.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := bm["type"].(string); t != "tool_result" {
			continue
		}
		if e, _ := bm["is_error"].(bool); e {
			return true
		}
	}
	return false
}

func obsToolIDs(msg map[string]any) map[string]struct{} {
	out := map[string]struct{}{}
	for _, b := range contentBlocks(msg) {
		bm, ok := b.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := bm["type"].(string); t != "tool_result" {
			continue
		}
		tid, _ := bm["tool_use_id"].(string)
		out[tid] = struct{}{}
	}
	return out
}

// Summarize optionally builds a richer hint than the default truncation.
type Summarize func(name string, input any, raw string) string

// TierOpts configures TierObservations.
type TierOpts struct {
	Hop            int
	KeepLastFull   int
	HintMaxChars   int
	Summarize      Summarize
	KeepFullIDs    map[string]struct{}
	KeepErrorsFull bool
}

func (o TierOpts) hintMax() int {
	if o.HintMaxChars <= 0 {
		return 160
	}
	return o.HintMaxChars
}

// TierObservations funnels the message tail: keep the newest KeepLastFull
// observation message(s) full; demote older ones' tool_result content to a
// one-line hint (preserving tool_use_id). KeepFullIDs and (by default) error
// observations stay full regardless of age. Returns a new message list.
func TierObservations(messages []map[string]any, opts TierOpts) []map[string]any {
	var obsPositions []int
	for i, m := range messages {
		if isObservation(m) {
			obsPositions = append(obsPositions, i)
		}
	}

	keepPositions := map[int]struct{}{}
	if opts.KeepLastFull > 0 {
		start := len(obsPositions) - opts.KeepLastFull
		if start < 0 {
			start = 0
		}
		for _, p := range obsPositions[start:] {
			keepPositions[p] = struct{}{}
		}
	}
	for _, i := range obsPositions {
		ids := obsToolIDs(messages[i])
		for id := range opts.KeepFullIDs {
			if _, ok := ids[id]; ok {
				keepPositions[i] = struct{}{}
			}
		}
		if opts.KeepErrorsFull && isErrorObs(messages[i]) {
			keepPositions[i] = struct{}{}
		}
	}

	demote := map[int]struct{}{}
	for _, i := range obsPositions {
		if _, ok := keepPositions[i]; !ok {
			demote[i] = struct{}{}
		}
	}
	if len(demote) == 0 {
		return append([]map[string]any(nil), messages...)
	}
	index := toolUseIndex(messages)

	out := make([]map[string]any, 0, len(messages))
	for i, msg := range messages {
		if _, ok := demote[i]; !ok {
			out = append(out, msg)
			continue
		}
		var newContent []any
		for _, b := range contentBlocks(msg) {
			bm, ok := b.(map[string]any)
			if !ok {
				newContent = append(newContent, b)
				continue
			}
			if t, _ := bm["type"].(string); t != "tool_result" {
				newContent = append(newContent, b)
				continue
			}
			tid, _ := bm["tool_use_id"].(string)
			raw := stringifyContent(bm["content"])
			if strings.HasPrefix(raw, SpentMarker) {
				newContent = append(newContent, b)
				continue
			}
			meta := index[tid]
			var hint string
			if opts.Summarize != nil {
				hint = opts.Summarize(meta.name, meta.input, raw)
			} else {
				hint = defaultHint(meta.name, meta.input, raw, opts.hintMax())
			}
			demoted := copyBlock(bm)
			demoted["content"] = hint
			newContent = append(newContent, demoted)
		}
		nm := copyMsg(msg)
		nm["content"] = newContent
		out = append(out, nm)
	}
	return out
}

func copyBlock(b map[string]any) map[string]any {
	out := make(map[string]any, len(b))
	for k, v := range b {
		out[k] = v
	}
	return out
}

func copyMsg(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func obsIsSpent(msg map[string]any) bool {
	for _, b := range contentBlocks(msg) {
		bm, ok := b.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := bm["type"].(string); t != "tool_result" {
			continue
		}
		if strings.HasPrefix(stringifyContent(bm["content"]), SpentMarker) {
			return true
		}
	}
	return false
}

func mergeConsecutive(messages []map[string]any) []map[string]any {
	var out []map[string]any
	for _, m := range messages {
		var blocks []any
		if c, ok := m["content"].([]any); ok {
			blocks = c
		} else {
			blocks = []any{map[string]any{"type": "text", "text": toStr(m["content"])}}
		}
		if len(out) > 0 {
			lastRole, _ := out[len(out)-1]["role"].(string)
			role, _ := m["role"].(string)
			if lastRole == role {
				prev := out[len(out)-1]
				prevBlocks, _ := prev["content"].([]any)
				merged := copyMsg(prev)
				merged["content"] = append(append([]any{}, prevBlocks...), blocks...)
				out[len(out)-1] = merged
				continue
			}
		}
		nm := copyMsg(m)
		nm["content"] = append([]any{}, blocks...)
		out = append(out, nm)
	}
	return out
}

// CompactOpts configures CompactObservations.
type CompactOpts struct {
	KeepLastFull   int
	KeepFullIDs    map[string]struct{}
	KeepErrorsFull bool
	MaxSpent       int
	SummaryLines   int
	HintMaxChars   int
	Summarize      Summarize
}

func (o CompactOpts) summaryLines() int {
	if o.SummaryLines <= 0 {
		return 4
	}
	return o.SummaryLines
}

// CompactObservations bounds the tool-loop tail. After funneling, spent-hint
// pairs older than the most recent MaxSpent are eliminated (their tool_use +
// tool_result removed together) and folded into ONE bounded rolling summary.
func CompactObservations(messages []map[string]any, opts CompactOpts) []map[string]any {
	tiered := TierObservations(messages, TierOpts{
		KeepLastFull:   opts.KeepLastFull,
		KeepFullIDs:    opts.KeepFullIDs,
		KeepErrorsFull: opts.KeepErrorsFull,
		HintMaxChars:   opts.HintMaxChars,
		Summarize:      opts.Summarize,
	})
	var spent []int
	for i, m := range tiered {
		if isObservation(m) && obsIsSpent(m) {
			spent = append(spent, i)
		}
	}
	if len(spent) <= opts.MaxSpent {
		return tiered
	}
	cut := len(spent) - opts.MaxSpent
	excess := spent[:cut]
	retained := spent[cut:]

	excessTids := map[string]struct{}{}
	var digests []string
	for _, i := range excess {
		for _, b := range contentBlocks(tiered[i]) {
			bm, ok := b.(map[string]any)
			if !ok {
				continue
			}
			if t, _ := bm["type"].(string); t != "tool_result" {
				continue
			}
			tid, _ := bm["tool_use_id"].(string)
			excessTids[tid] = struct{}{}
			txt := strings.TrimSpace(strings.ReplaceAll(stringifyContent(bm["content"]), SpentMarker, ""))
			txt = strings.Join(strings.Fields(txt), " ")
			if len([]rune(txt)) > 120 {
				txt = string([]rune(txt)[:120])
			}
			digests = append(digests, txt)
		}
	}

	n := len(digests)
	kept := digests
	sl := opts.summaryLines()
	if len(kept) > sl {
		kept = kept[len(kept)-sl:]
	}
	var sb strings.Builder
	sb.WriteString(CompactionMarker + " ")
	sb.WriteString(itoaInt(n))
	sb.WriteString(" earlier tool results offloaded to memory (re-run the tool to retrieve any):\n")
	for idx, d := range kept {
		if idx > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("· " + d)
	}
	if n > len(kept) {
		sb.WriteString("\n· (+" + itoaInt(n-len(kept)) + " more earlier calls offloaded)")
	}
	summary := sb.String()

	attach := -1
	if len(retained) > 0 {
		attach = retained[0]
	}

	excessSet := map[int]struct{}{}
	for _, i := range excess {
		excessSet[i] = struct{}{}
	}

	var out []map[string]any
	attached := false
	for i, m := range tiered {
		if _, ok := excessSet[i]; ok {
			continue
		}
		if role, _ := m["role"].(string); role == "assistant" {
			if blocks, ok := m["content"].([]any); ok {
				var kept []any
				for _, b := range blocks {
					bm, isMap := b.(map[string]any)
					if isMap {
						if t, _ := bm["type"].(string); t == "tool_use" {
							id, _ := bm["id"].(string)
							if _, drop := excessTids[id]; drop {
								continue
							}
						}
					}
					kept = append(kept, b)
				}
				if len(kept) == 0 {
					continue
				}
				nm := copyMsg(m)
				nm["content"] = kept
				m = nm
			}
		}
		if i == attach && !attached {
			blocks, _ := m["content"].([]any)
			nm := copyMsg(m)
			nm["content"] = append([]any{map[string]any{"type": "text", "text": summary}}, blocks...)
			m = nm
			attached = true
		}
		out = append(out, m)
	}
	if !attached {
		for j := len(out) - 1; j >= 0; j-- {
			if isObservation(out[j]) {
				blocks, _ := out[j]["content"].([]any)
				nm := copyMsg(out[j])
				nm["content"] = append([]any{map[string]any{"type": "text", "text": summary}}, blocks...)
				out[j] = nm
				break
			}
		}
	}
	return mergeConsecutive(out)
}

// ObsTailTokens estimates tokens of all tool-result (observation) content in
// the tail — the signal the working-set budget gates on.
func ObsTailTokens(messages []map[string]any) int {
	total := 0
	for _, msg := range messages {
		if !isObservation(msg) {
			continue
		}
		for _, b := range contentBlocks(msg) {
			bm, ok := b.(map[string]any)
			if !ok {
				continue
			}
			if t, _ := bm["type"].(string); t != "tool_result" {
				continue
			}
			total += EstTokens(stringifyContent(bm["content"]))
		}
	}
	return total
}

// EmbedOne encodes one text to a vector (injected). nil ⇒ lexical-only scoring.
type EmbedOne func(text string) []float64

// ScoreObservations returns the tool_use_ids of the highest-CDS spent
// observations w.r.t. goal. CDS = relevance(goal, observation) / cost_norm.
// Returns the top keepTop ids to pin FULL via KeepFullIDs so a goal-critical-
// but-old observation survives a newer-but-off-goal one. embedOne nil ⇒ L1 only.
func ScoreObservations(messages []map[string]any, goal string, embedOne EmbedOne, weights map[string]float64, keepTop int) map[string]struct{} {
	if keepTop <= 0 {
		return map[string]struct{}{}
	}
	w := weights
	if w == nil {
		w = attention.DefaultNodeWeights
	}
	costUnit := w["cds_cost_unit"]
	if costUnit == 0 {
		costUnit = 40.0
	}
	index := toolUseIndex(messages)
	var qVec []float64
	if embedOne != nil && goal != "" {
		qVec = embedOne(goal)
	}

	type rec struct {
		cds   float64
		order int
		tid   string
	}
	var scored []rec
	order := 0
	for _, msg := range messages {
		if !isObservation(msg) {
			continue
		}
		for _, b := range contentBlocks(msg) {
			bm, ok := b.(map[string]any)
			if !ok {
				continue
			}
			if t, _ := bm["type"].(string); t != "tool_result" {
				continue
			}
			tid, _ := bm["tool_use_id"].(string)
			if tid == "" {
				continue
			}
			content := stringifyContent(bm["content"])
			meta := index[tid]
			text := meta.name + " " + briefArgs(meta.input, 60) + " " + content
			var textVec []float64
			if embedOne != nil {
				textVec = embedOne(text)
			}
			sc := attention.ScoreText(goal, qVec, text, textVec, w, 0.0)
			costNorm := float64(EstTokens(content)) / costUnit
			if costNorm < 1.0 {
				costNorm = 1.0
			}
			cds := sc.Activation / costNorm
			scored = append(scored, rec{cds: cds, order: order, tid: tid})
			order++
		}
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].cds != scored[j].cds {
			return scored[i].cds > scored[j].cds
		}
		return scored[i].order < scored[j].order
	})
	out := map[string]struct{}{}
	for _, r := range scored {
		if len(out) >= keepTop {
			break
		}
		out[r.tid] = struct{}{}
	}
	return out
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
