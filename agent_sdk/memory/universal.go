package memory

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/nccasia/agent-sdk-go/agent_sdk/core/attention"
	"github.com/nccasia/agent-sdk-go/agent_sdk/react"
)

// FlashScope is the turn-scoped (in-RAM, dropped at turn end) tier.
const FlashScope = "turn"

// LongTermScopes are the durable tiers.
var LongTermScopes = []string{"conversation", "channel", "user", "bot"}

// KindUtility is the thought-steering weight per kind (utility in CDS).
var KindUtility = map[string]float64{
	"decision":    1.4,
	"plan":        1.4,
	"obligation":  1.3,
	"sub_goal":    1.2,
	"fact":        1.2,
	"note":        1.1,
	"hypothesis":  1.0,
	"context":     1.0,
	"artifact":    0.9,
	"tool_result": 0.9,
	"temp_file":   0.8,
}

const defaultLargeBodyChars = 2000
const snapshotMaxEntries = 256
const snapshotMaxTokens = 16000

// Embed encodes a text to a vector for SEMANTIC recall (L2). nil ⇒ lexical-only.
type Embed func(text string) []float64

// Summarizer builds a digest (kind, meta, body) -> digest (sync).
type Summarizer func(kind string, meta map[string]any, body string) string

// MemoryEntry is the one universal entry type: a dense digest (the gist) plus an
// offloaded body (the detail, re-fetchable by handle), valued by CDS.
type MemoryEntry struct {
	Handle     string // mem://<kind>/<scope>/<key>
	Kind       string
	Scope      string
	Digest     string
	Body       string
	Utility    float64
	Relevance  float64
	CDS        float64
	Tier       int
	Pinned     bool
	Recency    float64
	Tokens     int
	Source     string
	Meta       map[string]any
	CreatedSeq int
	Offloaded  bool
}

// IsFlash reports whether the entry lives in the flash (turn) tier.
func (e *MemoryEntry) IsFlash() bool { return e.Scope == FlashScope }

// ToJSON is the prompt-facing view.
func (e *MemoryEntry) ToJSON() map[string]any {
	return map[string]any{
		"handle": e.Handle, "kind": e.Kind, "scope": e.Scope, "digest": e.Digest,
		"tokens": e.Tokens, "utility": e.Utility, "cds": round4(e.CDS), "tier": e.Tier,
		"pinned": e.Pinned, "source": e.Source, "meta": e.Meta, "offloaded": e.Offloaded,
	}
}

func (e *MemoryEntry) snapshot() map[string]any {
	return map[string]any{
		"handle": e.Handle, "kind": e.Kind, "scope": e.Scope, "digest": e.Digest,
		"body": e.Body, "utility": e.Utility, "relevance": e.Relevance, "cds": e.CDS,
		"tier": e.Tier, "pinned": e.Pinned, "recency": e.Recency, "tokens": e.Tokens,
		"source": e.Source, "meta": e.Meta, "created_seq": e.CreatedSeq, "offloaded": e.Offloaded,
	}
}

func restoreEntry(d map[string]any) *MemoryEntry {
	meta, _ := d["meta"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
	}
	return &MemoryEntry{
		Handle:     asString(d["handle"]),
		Kind:       asString(d["kind"]),
		Scope:      asString(d["scope"]),
		Digest:     asString(d["digest"]),
		Body:       asString(d["body"]),
		Utility:    asFloat(d["utility"], 1.0),
		Relevance:  asFloat(d["relevance"], 0.0),
		CDS:        asFloat(d["cds"], 0.0),
		Tier:       asInt(d["tier"], 0),
		Pinned:     asBool(d["pinned"]),
		Recency:    asFloat(d["recency"], 0.0),
		Tokens:     asInt(d["tokens"], 0),
		Source:     asString(d["source"]),
		Meta:       meta,
		CreatedSeq: asInt(d["created_seq"], 0),
		Offloaded:  asBool(d["offloaded"]),
	}
}

var memSlugRE = regexp.MustCompile(`[^a-z0-9]+`)

// MemoryStore is the two-tier universal memory. Pure/in-process; large bodies
// route to a DocWorkspace.
type MemoryStore struct {
	flash      map[string]*MemoryEntry
	long       map[string]*MemoryEntry
	docs       *react.DocWorkspace
	summarizer Summarizer
	digestMax  int
	large      int
	costUnit   float64
	embed      Embed
	seq        int
}

// StoreOption configures a MemoryStore.
type StoreOption func(*MemoryStore)

// WithSummarizer overrides the digest builder.
func WithSummarizer(s Summarizer) StoreOption { return func(m *MemoryStore) { m.summarizer = s } }

// WithDigestMaxChars sets the digest truncation cap.
func WithDigestMaxChars(n int) StoreOption { return func(m *MemoryStore) { m.digestMax = n } }

// WithLargeBodyChars sets the offload threshold.
func WithLargeBodyChars(n int) StoreOption { return func(m *MemoryStore) { m.large = n } }

// WithCDSCostUnit calibrates the size penalty in CDS.
func WithCDSCostUnit(f float64) StoreOption { return func(m *MemoryStore) { m.costUnit = f } }

// WithEmbed sets the optional embedder for semantic recall (L2).
func WithEmbed(e Embed) StoreOption { return func(m *MemoryStore) { m.embed = e } }

// WithDocWorkspace injects a DocWorkspace.
func WithDocWorkspace(d *react.DocWorkspace) StoreOption {
	return func(m *MemoryStore) { m.docs = d }
}

// NewMemoryStore builds a two-tier universal memory store.
func NewMemoryStore(opts ...StoreOption) *MemoryStore {
	m := &MemoryStore{
		flash:     map[string]*MemoryEntry{},
		long:      map[string]*MemoryEntry{},
		docs:      react.NewDocWorkspace(),
		digestMax: 240,
		large:     defaultLargeBodyChars,
		costUnit:  40.0,
	}
	for _, o := range opts {
		o(m)
	}
	if m.costUnit == 0 {
		m.costUnit = 40.0
	}
	return m
}

func (m *MemoryStore) digest(kind string, meta map[string]any, body string) string {
	if m.summarizer != nil {
		return m.summarizer(kind, meta, body)
	}
	return DeterministicDigest(kind, meta, body, m.digestMax, 3)
}

func (m *MemoryStore) qVec(query string) []float64 {
	if m.embed != nil && query != "" {
		return m.embed(query)
	}
	return nil
}

// RememberOpts configures a write.
type RememberOpts struct {
	Scope  string
	Key    string
	Digest *string
	Meta   map[string]any
	Pinned bool
	Source string
}

// Remember stores an entry; returns its handle. Large bodies offload to the
// DocWorkspace.
func (m *MemoryStore) Remember(kind string, content any, opts RememberOpts) string {
	m.seq++
	meta := map[string]any{}
	for k, v := range opts.Meta {
		meta[k] = v
	}
	scope := opts.Scope
	if scope == "" {
		scope = FlashScope
	}
	body := stringify(content)
	key := opts.Key
	if key == "" {
		key = m.autoKey(kind, meta)
	}
	handle := fmt.Sprintf("mem://%s/%s/%s", kind, scope, key)
	offloaded := len(body) >= m.large
	if offloaded {
		m.docs.Offload(handle, body)
	}
	var dg string
	if opts.Digest != nil {
		dg = *opts.Digest
	} else {
		dg = m.digest(kind, meta, body)
	}
	dg = m.truncate(dg)
	utility := KindUtility[kind]
	if utility == 0 {
		utility = 1.0
	}
	entry := &MemoryEntry{
		Handle:     handle,
		Kind:       kind,
		Scope:      scope,
		Digest:     dg,
		Body:       body,
		Utility:    utility,
		Tokens:     EstTokens(body),
		Pinned:     opts.Pinned,
		Recency:    float64(m.seq),
		Source:     opts.Source,
		Meta:       meta,
		CreatedSeq: m.seq,
		Offloaded:  offloaded,
	}
	m.bucket(scope)[handle] = entry
	return handle
}

func (m *MemoryStore) autoKey(kind string, meta map[string]any) string {
	if k, ok := meta["key"]; ok && k != nil {
		s := strings.Trim(memSlugRE.ReplaceAllString(strings.ToLower(stringify(k)), "-"), "-")
		if len([]rune(s)) > 48 {
			s = string([]rune(s)[:48])
		}
		return s
	}
	if t, ok := meta["tool"]; ok && t != nil {
		s := strings.Trim(memSlugRE.ReplaceAllString(strings.ToLower(stringify(t)), "-"), "-")
		if len([]rune(s)) > 32 {
			s = string([]rune(s)[:32])
		}
		return fmt.Sprintf("%s-%04d", s, m.seq)
	}
	return fmt.Sprintf("%06d", m.seq)
}

func (m *MemoryStore) truncate(s string) string {
	if len([]rune(s)) <= m.digestMax {
		return s
	}
	return string([]rune(s)[:m.digestMax]) + "…"
}

func (m *MemoryStore) bucket(scope string) map[string]*MemoryEntry {
	if scope == FlashScope {
		return m.flash
	}
	return m.long
}

// Get returns the entry for handle (flash first), or nil.
func (m *MemoryStore) Get(handle string) *MemoryEntry {
	if e, ok := m.flash[handle]; ok {
		return e
	}
	if e, ok := m.long[handle]; ok {
		return e
	}
	return nil
}

// Read returns the full body (the detail re-enters context). "" if unknown.
func (m *MemoryStore) Read(handle string) string {
	if e := m.Get(handle); e != nil {
		return e.Body
	}
	return ""
}

// Grep returns matching lines (not the whole body).
func (m *MemoryStore) Grep(handle, pattern string, maxMatches int) []map[string]any {
	e := m.Get(handle)
	if e == nil {
		return nil
	}
	if maxMatches <= 0 {
		maxMatches = 50
	}
	if e.Offloaded {
		return m.docs.Grep(handle, pattern, maxMatches)
	}
	rx, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil
	}
	var out []map[string]any
	for _, line := range strings.Split(e.Body, "\n") {
		if rx.MatchString(line) {
			s := strings.TrimSpace(line)
			if len([]rune(s)) > 200 {
				s = string([]rune(s)[:200])
			}
			out = append(out, map[string]any{"line": s})
			if len(out) >= maxMatches {
				break
			}
		}
	}
	return out
}

// ReadSection returns one bounded slice of an offloaded body. "" if unknown.
func (m *MemoryStore) ReadSection(handle, section string) string {
	e := m.Get(handle)
	if e == nil || !e.Offloaded {
		return ""
	}
	s, ok := m.docs.ReadSection(handle, section)
	if !ok {
		return ""
	}
	return s
}

// Outline returns the section index of an offloaded body.
func (m *MemoryStore) Outline(handle string) []map[string]any {
	e := m.Get(handle)
	if e == nil || !e.Offloaded {
		return nil
	}
	return m.docs.Outline(handle)
}

// RecallOpts configures Recall.
type RecallOpts struct {
	Query  string
	Handle string
	Kind   string
	Scope  string
	Full   bool
	K      int
}

// Recall is the universal read: search/list the digest index across both tiers,
// scored by relevance to Query (newest-first if none). For a handle, use Get/Read.
func (m *MemoryStore) Recall(opts RecallOpts) []*MemoryEntry {
	k := opts.K
	if k == 0 {
		k = 8
	}
	var entries []*MemoryEntry
	for _, e := range m.long {
		entries = append(entries, e)
	}
	for _, e := range m.flash {
		entries = append(entries, e)
	}
	if opts.Kind != "" {
		entries = filter(entries, func(e *MemoryEntry) bool { return e.Kind == opts.Kind })
	}
	if opts.Scope != "" {
		entries = filter(entries, func(e *MemoryEntry) bool { return e.Scope == opts.Scope })
	}
	if opts.Query != "" {
		qVec := m.qVec(opts.Query)
		for _, e := range entries {
			var tv []float64
			if m.embed != nil {
				tv = m.embed(e.Kind + " " + e.Digest)
			}
			e.Relevance = attention.ScoreText(opts.Query, qVec, e.Kind+" "+e.Digest, tv, nil, 0.0).Activation
		}
		sort.SliceStable(entries, func(i, j int) bool {
			if entries[i].Relevance != entries[j].Relevance {
				return entries[i].Relevance > entries[j].Relevance
			}
			return entries[i].Recency > entries[j].Recency
		})
	} else {
		sort.SliceStable(entries, func(i, j int) bool { return entries[i].Recency > entries[j].Recency })
	}
	if k < len(entries) {
		entries = entries[:k]
	}
	return entries
}

// Tier scores entries by CDS vs query and assigns tiers (1 inject · 2
// digest+handle · 3 offload), greedy under budgetTokens. Pinned floor to Tier 1.
// Mutates and returns the entries.
func (m *MemoryStore) Tier(entries []*MemoryEntry, query string, budgetTokens int, injectThreshold, hintThreshold float64) []*MemoryEntry {
	if injectThreshold == 0 {
		injectThreshold = 0.30
	}
	if hintThreshold == 0 {
		hintThreshold = 0.12
	}
	qVec := m.qVec(query)
	for _, e := range entries {
		var tv []float64
		if m.embed != nil {
			tv = m.embed(e.Kind + " " + e.Digest)
		}
		e.Relevance = attention.ScoreText(query, qVec, e.Kind+" "+e.Digest, tv, nil, 0.0).Activation
		toks := e.Tokens
		if toks == 0 {
			toks = EstTokens(e.Body)
		}
		cost := float64(toks) / m.costUnit
		if cost < 1.0 {
			cost = 1.0
		}
		e.CDS = (max0(e.Relevance) * max0(e.Utility)) / cost
		e.Tier = 0
	}
	used := 0
	for _, e := range entries {
		if e.Pinned {
			e.Tier = 1
			used += e.Tokens
		}
	}
	var rest []*MemoryEntry
	for _, e := range entries {
		if e.Tier == 0 {
			rest = append(rest, e)
		}
	}
	sort.SliceStable(rest, func(i, j int) bool {
		if rest[i].CDS != rest[j].CDS {
			return rest[i].CDS > rest[j].CDS
		}
		return rest[i].Recency > rest[j].Recency
	})
	for _, e := range rest {
		if e.CDS >= injectThreshold && used+e.Tokens <= budgetTokens {
			e.Tier = 1
			used += e.Tokens
		} else if e.CDS >= hintThreshold {
			e.Tier = 2
		} else {
			e.Tier = 3
		}
	}
	return entries
}

// Forget removes a handle from either tier; returns whether it existed.
func (m *MemoryStore) Forget(handle string) bool {
	if _, ok := m.flash[handle]; ok {
		delete(m.flash, handle)
		return true
	}
	if _, ok := m.long[handle]; ok {
		delete(m.long, handle)
		return true
	}
	return false
}

// ResetFlash drops the turn's working memory. Long-term persists.
func (m *MemoryStore) ResetFlash() { m.flash = map[string]*MemoryEntry{} }

// Reset drops ALL state (flash + long-term + offloaded bodies).
func (m *MemoryStore) Reset() {
	m.flash = map[string]*MemoryEntry{}
	m.long = map[string]*MemoryEntry{}
	m.docs = react.NewDocWorkspace()
	m.seq = 0
}

// Seq returns the current sequence counter.
func (m *MemoryStore) Seq() int { return m.seq }

// SnapshotOpts bounds a snapshot.
type SnapshotOpts struct {
	MaxEntries int
	MaxTokens  int
}

func (o SnapshotOpts) entries() int {
	if o.MaxEntries <= 0 {
		return snapshotMaxEntries
	}
	return o.MaxEntries
}

func (o SnapshotOpts) tokens() int {
	if o.MaxTokens <= 0 {
		return snapshotMaxTokens
	}
	return o.MaxTokens
}

// DumpLong is the durable tier as a bounded list of loss-free entry snapshots.
// Flash is NOT included. Pinned entries always survive; the rest are kept
// highest-CDS-then-newest until a cap is hit.
func (m *MemoryStore) DumpLong(opts SnapshotOpts) []map[string]any {
	var pinned, rest []*MemoryEntry
	for _, e := range m.long {
		if e.Pinned {
			pinned = append(pinned, e)
		} else {
			rest = append(rest, e)
		}
	}
	sort.SliceStable(pinned, func(i, j int) bool { return pinned[i].Recency > pinned[j].Recency })
	sort.SliceStable(rest, func(i, j int) bool {
		if rest[i].CDS != rest[j].CDS {
			return rest[i].CDS > rest[j].CDS
		}
		return rest[i].Recency > rest[j].Recency
	})
	maxEntries := opts.entries()
	maxTokens := opts.tokens()
	var out []map[string]any
	used := 0
	for _, e := range pinned {
		used += e.Tokens
	}
	for _, e := range pinned {
		out = append(out, e.snapshot())
	}
	for _, e := range rest {
		if len(out) >= maxEntries {
			break
		}
		if used+e.Tokens > maxTokens {
			continue
		}
		out = append(out, e.snapshot())
		used += e.Tokens
	}
	return out
}

// LoadLong restores durable entries (replacing by handle), advancing seq so new
// writes never collide with restored ones.
func (m *MemoryStore) LoadLong(entries []map[string]any) {
	for _, d := range entries {
		e := restoreEntry(d)
		m.long[e.Handle] = e
		if e.CreatedSeq > m.seq {
			m.seq = e.CreatedSeq
		}
	}
}

// Snapshot is a full session snapshot blob.
type Snapshot struct {
	Seq  int              `json:"seq"`
	Long []map[string]any `json:"long"`
	Docs map[string]any   `json:"docs"`
}

// ToJSON is a full session snapshot: durable tier + the offloaded bodies it
// references. Flash is dropped (turn-scratch). Pair with MemoryStoreFromJSON.
func (m *MemoryStore) ToJSON(opts SnapshotOpts) Snapshot {
	long := m.DumpLong(opts)
	keptOffloaded := map[string]struct{}{}
	for _, e := range long {
		if b, _ := e["offloaded"].(bool); b {
			keptOffloaded[asString(e["handle"])] = struct{}{}
		}
	}
	allDocs := m.docs.ToJSON()
	docs := map[string]any{}
	for h, v := range allDocs {
		if _, ok := keptOffloaded[h]; ok {
			docs[h] = v
		}
	}
	return Snapshot{Seq: m.seq, Long: long, Docs: docs}
}

// MemoryStoreFromJSON rebuilds a store from ToJSON. Flash starts empty;
// long-term + offloaded bodies are restored.
func MemoryStoreFromJSON(data Snapshot, embed Embed, opts ...StoreOption) *MemoryStore {
	allOpts := append([]StoreOption{WithEmbed(embed)}, opts...)
	store := NewMemoryStore(allOpts...)
	store.Restore(data)
	return store
}

// Restore resets this store IN PLACE and loads a snapshot (pairs with ToJSON).
func (m *MemoryStore) Restore(data Snapshot) {
	m.Reset()
	m.docs = react.DocWorkspaceFromJSON(data.Docs)
	m.LoadLong(data.Long)
	if data.Seq > m.seq {
		m.seq = data.Seq
	}
}

// Promote writes a flash entry back to long-term, consolidating against an
// existing entry with the same key. Returns the new long-term handle, or "".
func (m *MemoryStore) Promote(handle, scope, key string) string {
	e := m.Get(handle)
	if e == nil {
		return ""
	}
	if scope == "" {
		scope = "conversation"
	}
	if key == "" {
		if k, ok := e.Meta["key"]; ok {
			key = stringify(k)
		}
	}
	source := e.Source
	if source == "" {
		source = handle
	}
	dg := e.Digest
	return m.Remember(e.Kind, e.Body, RememberOpts{
		Scope: scope, Key: key, Digest: &dg, Meta: e.Meta, Pinned: e.Pinned, Source: source,
	})
}

// CompactionSummarizer returns a sync summarize(name, input, raw) -> digest for
// the funnel: it offloads the spent tool body to flash memory and returns a
// dense digest that NAMES the handle, so the compacted result is re-fetchable.
func (m *MemoryStore) CompactionSummarizer() func(name string, inp any, raw string) string {
	return func(name string, inp any, raw string) string {
		handle := m.Remember("tool_result", raw, RememberOpts{
			Scope:  FlashScope,
			Meta:   map[string]any{"tool": name, "args": inp},
			Source: name,
		})
		digest := m.Get(handle).Digest
		return fmt.Sprintf("%s %s · read('%s') for full", react.SpentMarker, digest, handle)
	}
}

// RenderIndexOpts configures RenderIndex.
type RenderIndexOpts struct {
	Query        string
	BudgetTokens int
	MaxPerKind   int
	Kinds        []string
}

// RenderIndex is the memory MENU — one line per entry, grouped by kind,
// newest-first, capped per kind and by a token budget; overflow is announced as
// a count. Injected each turn as "## Memory".
func (m *MemoryStore) RenderIndex(opts RenderIndexOpts) string {
	budget := opts.BudgetTokens
	if budget == 0 {
		budget = 600
	}
	maxPerKind := opts.MaxPerKind
	if maxPerKind == 0 {
		maxPerKind = 6
	}
	var entries []*MemoryEntry
	if opts.Query != "" {
		entries = m.Recall(RecallOpts{Query: opts.Query, K: 10000})
	} else {
		entries = m.Entries("")
		sort.SliceStable(entries, func(i, j int) bool { return entries[i].Recency > entries[j].Recency })
	}
	kindSet := map[string]struct{}{}
	for _, k := range opts.Kinds {
		kindSet[k] = struct{}{}
	}
	var kindOrder []string
	byKind := map[string][]*MemoryEntry{}
	for _, e := range entries {
		if len(opts.Kinds) > 0 {
			if _, ok := kindSet[e.Kind]; !ok {
				continue
			}
		}
		if _, seen := byKind[e.Kind]; !seen {
			kindOrder = append(kindOrder, e.Kind)
		}
		byKind[e.Kind] = append(byKind[e.Kind], e)
	}
	header := "## Memory — recall(handle) to expand a digest, recall(query=…) to search"
	lines := []string{header}
	used := EstTokens(header)
	dropped := 0
	for _, kind := range kindOrder {
		es := byKind[kind]
		shown := es
		if len(shown) > maxPerKind {
			shown = shown[:maxPerKind]
		}
		for _, e := range shown {
			line := "- [" + kind + "] " + e.Handle + " — " + e.Digest
			t := EstTokens(line)
			if used+t > budget {
				dropped++
				continue
			}
			lines = append(lines, line)
			used += t
		}
		if extra := len(es) - maxPerKind; extra > 0 {
			dropped += extra
		}
	}
	if dropped > 0 {
		lines = append(lines, fmt.Sprintf("- (+%d more — recall(query=…) to find them)", dropped))
	}
	return strings.Join(lines, "\n")
}

// Stats reports per-tier counts and token totals.
func (m *MemoryStore) Stats() map[string]int {
	flashTokens, longTokens := 0, 0
	for _, e := range m.flash {
		flashTokens += e.Tokens
	}
	for _, e := range m.long {
		longTokens += e.Tokens
	}
	return map[string]int{
		"flash": len(m.flash), "long_term": len(m.long),
		"flash_tokens": flashTokens, "long_term_tokens": longTokens,
	}
}

// Entries returns all entries (optionally filtered by scope).
func (m *MemoryStore) Entries(scope string) []*MemoryEntry {
	var out []*MemoryEntry
	for _, e := range m.long {
		if scope == "" || e.Scope == scope {
			out = append(out, e)
		}
	}
	for _, e := range m.flash {
		if scope == "" || e.Scope == scope {
			out = append(out, e)
		}
	}
	return out
}

func filter(es []*MemoryEntry, pred func(*MemoryEntry) bool) []*MemoryEntry {
	var out []*MemoryEntry
	for _, e := range es {
		if pred(e) {
			out = append(out, e)
		}
	}
	return out
}

func max0(x float64) float64 {
	if x < 0 {
		return 0
	}
	return x
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asFloat(v any, def float64) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	}
	return def
}

func asInt(v any, def int) int {
	switch x := v.(type) {
	case int:
		return x
	case float64:
		return int(x)
	}
	return def
}

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}
