package memory

import (
	"math"
	"regexp"
	"strings"
	"testing"
)

// conceptConcepts mirrors benchmarks/_shared/embed.py CONCEPTS — a deterministic
// concept embedder so the semantic-recall seam is reproducible without a model.
var conceptConcepts = [][]string{
	{"deadline", "duedate", "cutoff", "due", "eta", "deliver"},
	{"schedule", "scheduled", "rollout", "launch", "planned", "timing", "ship"},
	{"cost", "price", "budget", "payment", "invoice", "billing", "charge"},
	{"incident", "outage", "failure", "downtime", "breach", "alert"},
	{"owner", "assignee", "responsible", "oncall", "lead", "maintainer"},
	{"location", "venue", "room", "place", "address", "where"},
	{"preference", "prefers", "likes", "wants", "setting", "config"},
	{"security", "auth", "permission", "access", "credential", "token"},
	{"performance", "latency", "slow", "throughput", "speed", "perf"},
	{"data", "database", "table", "record", "query", "index", "schema"},
}

var conceptWordRE = regexp.MustCompile(`[a-z0-9]+`)

func conceptTokenOf() map[string]int {
	m := map[string]int{}
	for i, syns := range conceptConcepts {
		for _, tok := range syns {
			m[tok] = i
		}
	}
	return m
}

func conceptEmbed(text string) []float64 {
	tokenOf := conceptTokenOf()
	dim := len(conceptConcepts)
	vec := make([]float64, dim)
	for _, tok := range conceptWordRE.FindAllString(strings.ToLower(text), -1) {
		if i, ok := tokenOf[tok]; ok {
			vec[i] += 1.0
		}
	}
	n := 0.0
	for _, v := range vec {
		n += v * v
	}
	n = math.Sqrt(n)
	if n == 0 {
		return vec
	}
	for i := range vec {
		vec[i] /= n
	}
	return vec
}

func seedSemantic(store *MemoryStore) string {
	target := store.Remember("fact", "the rollout is scheduled for Friday", RememberOpts{Key: "target"})
	store.Remember("note", "when will the the the meeting notes", RememberOpts{Key: "decoy"})
	for i := 0; i < 50; i++ {
		store.Remember("note", "the invoice payment cost budget item "+itoa(i), RememberOpts{Key: "money" + itoa(i)})
	}
	return target
}

func TestSemanticRecallFindsLexicallyDisjointMatch(t *testing.T) {
	store := NewMemoryStore(WithEmbed(conceptEmbed))
	target := seedSemantic(store)
	top := store.Recall(RecallOpts{Query: "when will the launch happen", K: 1})
	if len(top) == 0 || top[0].Handle != target {
		t.Fatalf("semantic recall should find disjoint match, got %v", top)
	}
}

func TestLexicalOnlyMissesTheSynonymTarget(t *testing.T) {
	store := NewMemoryStore()
	target := seedSemantic(store)
	top := store.Recall(RecallOpts{Query: "when will the launch happen", K: 1})
	if len(top) > 0 && top[0].Handle == target {
		t.Fatal("lexical recall should NOT rank the semantic target first")
	}
}

func TestEmbedNoneIsLexicalByteIdentical(t *testing.T) {
	a := NewMemoryStore()
	b := NewMemoryStore(WithEmbed(nil))
	for _, s := range []*MemoryStore{a, b} {
		s.Remember("fact", "the deadline is 2026-07-15", RememberOpts{Key: "d"})
	}
	qa := handles(a.Recall(RecallOpts{Query: "deadline"}))
	qb := handles(b.Recall(RecallOpts{Query: "deadline"}))
	if strings.Join(qa, ",") != strings.Join(qb, ",") {
		t.Fatalf("embed=nil must change nothing: %v vs %v", qa, qb)
	}
}

func handles(es []*MemoryEntry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Handle
	}
	return out
}
