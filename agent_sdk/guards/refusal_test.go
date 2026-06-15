package guards

import "testing"

func head(items []GoldenItem, vecs []Vec) *GoldenHead {
	return NewGoldenHead(items, vecs, "m")
}

func TestMatchRefusalKeywordTopicRegex(t *testing.T) {
	rules := []RefusalRule{
		{RuleType: "keyword", Pattern: "secret|password", Reason: "no secrets"},
		{RuleType: "topic", Pattern: "salary", Reason: "no HR"},
		{RuleType: "regex", Pattern: `\bssn\b`, Reason: "no PII"},
	}
	if got := MatchRefusal("what is the SECRET key", rules); got != "no secrets" {
		t.Fatalf("keyword: %q", got)
	}
	if got := MatchRefusal("tell me the salary range", rules); got != "no HR" {
		t.Fatalf("topic: %q", got)
	}
	if got := MatchRefusal("my ssn please", rules); got != "no PII" {
		t.Fatalf("regex: %q", got)
	}
	if got := MatchRefusal("how do I reset my account", rules); got != "" {
		t.Fatalf("expected miss, got %q", got)
	}
}

func TestGoldenHitAboveAndBelowThreshold(t *testing.T) {
	h := head(
		[]GoldenItem{{CaseID: "g1", Query: "passing grade?", ExpectedBehavior: "5.0 on a 10-point scale."}},
		[]Vec{{1.0, 0.0}},
	)
	res := GoldenHit(Vec{1.0, 0.0}, h, 0.86)
	if res == nil || res.Status != "answered" {
		t.Fatalf("expected answered hit, got %+v", res)
	}
	if res.Text != "5.0 on a 10-point scale." {
		t.Fatalf("text = %q", res.Text)
	}
	if res.Citations[0].SourceRef != "golden://g1" {
		t.Fatalf("source_ref = %q", res.Citations[0].SourceRef)
	}
	if GoldenHit(Vec{0.0, 1.0}, h, 0.86) != nil {
		t.Fatal("orthogonal query should miss")
	}
}

func TestGoldenHitEmptyHeadIsNone(t *testing.T) {
	empty := &GoldenHead{Items: nil, Embeddings: nil, EmbeddingModelID: "m"}
	if GoldenHit(Vec{1.0, 0.0}, empty, 0.5) != nil {
		t.Fatal("empty head should miss")
	}
}

func TestGateRefusalShortCircuits(t *testing.T) {
	gate := MakePreTurnGate(GateOptions{
		RefusalRules: []RefusalRule{{RuleType: "keyword", Pattern: "secret", Reason: "no secrets"}},
	})
	res := gate("tell me the secret", nil)
	if res == nil || res.Status != "refused" || res.Text != "no secrets" {
		t.Fatalf("expected refused, got %+v", res)
	}
	if gate("a normal question", nil) != nil {
		t.Fatal("normal question should pass")
	}
}

func TestGateGoldenBeatsSemanticRefusal(t *testing.T) {
	h := head([]GoldenItem{{CaseID: "g1", Query: "hi", ExpectedBehavior: "Hello there."}}, []Vec{{1.0, 0.0}})
	rules := []RefusalRule{{RuleType: "semantic", Reason: "blocked", QueryExamples: []string{"hi"}}}
	sem := MakeSemanticRefusal(rules, func(string) Vec { return Vec{1.0, 0.0} }, 0.5, nil)
	gate := MakePreTurnGate(GateOptions{
		GoldenHead:      h,
		Embed:           func(string) Vec { return Vec{1.0, 0.0} },
		GoldenThreshold: 0.86,
		SemanticRefusal: sem,
	})
	res := gate("hi", nil)
	if res == nil || res.Status != "answered" || res.Text != "Hello there." {
		t.Fatalf("golden must beat semantic refusal, got %+v", res)
	}
}

func TestGateSemanticRefusalFiresWhenNoGolden(t *testing.T) {
	rules := []RefusalRule{{RuleType: "semantic", Reason: "out of scope", QueryExamples: []string{"politics"}}}
	sem := MakeSemanticRefusal(rules, func(string) Vec { return Vec{1.0, 0.0} }, 0.5, nil)
	gate := MakePreTurnGate(GateOptions{
		Embed:           func(string) Vec { return Vec{1.0, 0.0} },
		SemanticRefusal: sem,
	})
	res := gate("anything", nil)
	if res == nil || res.Status != "refused" || res.Text != "out of scope" {
		t.Fatalf("expected refused, got %+v", res)
	}
}

func TestGateDisabledIsNoop(t *testing.T) {
	gate := MakePreTurnGate(GateOptions{
		RefusalRules:       []RefusalRule{{RuleType: "keyword", Pattern: "secret", Reason: "x"}},
		RefusalEnforcement: "disabled",
	})
	if gate("tell me the secret", nil) != nil {
		t.Fatal("disabled gate should be a no-op")
	}
}

func TestSemanticRefusalExcludesNotInDocsTag(t *testing.T) {
	rules := []RefusalRule{{RuleType: "semantic", Reason: "x", QueryExamples: []string{"q"}, Tags: []string{"not-in-docs"}}}
	if MakeSemanticRefusal(rules, func(string) Vec { return Vec{1.0, 0.0} }, 0, nil) != nil {
		t.Fatal("not-in-docs rule should be excluded → nil semantic refusal")
	}
}

func TestGoldenHeadRoundtripCacheBundle(t *testing.T) {
	h := head([]GoldenItem{{CaseID: "g1", Query: "q", ExpectedBehavior: "a", Criteria: []string{"c"}, Tags: []string{"t"}}}, []Vec{{1.0, 0.0}})
	bundle := h.ToCacheBundle()
	entries, _ := bundle["entries"].([]map[string]any)
	rebuilt := GoldenHeadFromCached(entries, bundle["model_id"].(string))
	if rebuilt == nil || rebuilt.Size() != 1 {
		t.Fatalf("rebuilt = %+v", rebuilt)
	}
	if rebuilt.Items[0].ExpectedBehavior != "a" {
		t.Fatalf("expected_behavior = %q", rebuilt.Items[0].ExpectedBehavior)
	}
}
