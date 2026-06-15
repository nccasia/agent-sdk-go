// Ported from agent-sdk/tests/test_signals.py — declarative signal grammar.
package signal

import (
	"errors"
	"math"
	"testing"
)

func mustCompile(t *testing.T, expr any) Signal {
	t.Helper()
	s, err := Compile(expr)
	if err != nil {
		t.Fatalf("Compile(%#v) failed: %v", expr, err)
	}
	return s
}

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestConstAndScalar(t *testing.T) {
	if got := Eval(map[string]any{"const": 0.7}, Context{}); got != 0.7 {
		t.Errorf("const 0.7 = %v", got)
	}
	if got := Eval(1.0, Context{}); got != 1.0 {
		t.Errorf("1.0 = %v", got)
	}
	if got := Eval(0, Context{}); got != 0.0 {
		t.Errorf("0 = %v", got)
	}
	if got := Eval(true, Context{}); got != 1.0 {
		t.Errorf("true = %v", got)
	}
	if got := Eval(nil, Context{}); got != 0.0 {
		t.Errorf("nil = %v", got)
	}
}

func TestConstClamps(t *testing.T) {
	if got := Eval(map[string]any{"const": 5.0}, Context{}); got != 1.0 {
		t.Errorf("const 5.0 = %v", got)
	}
	if got := Eval(-3, Context{}); got != 0.0 {
		t.Errorf("-3 = %v", got)
	}
}

func TestFlag(t *testing.T) {
	s := mustCompile(t, map[string]any{"flag": "is_question"})
	if got := s(Context{"is_question": true}); got != 1.0 {
		t.Errorf("flag true = %v", got)
	}
	if got := s(Context{"is_question": false}); got != 0.0 {
		t.Errorf("flag false = %v", got)
	}
	if got := s(Context{}); got != 0.0 {
		t.Errorf("flag missing = %v", got)
	}
}

func TestLexicalCaseInsensitive(t *testing.T) {
	s := mustCompile(t, map[string]any{"lexical": []any{"compare", "vs"}})
	if got := s(Context{"query": "Compare A and B"}); got != 1.0 {
		t.Errorf("compare = %v", got)
	}
	if got := s(Context{"query": "A vs B"}); got != 1.0 {
		t.Errorf("vs = %v", got)
	}
	if got := s(Context{"query": "hello there"}); got != 0.0 {
		t.Errorf("none = %v", got)
	}
}

func TestMinWordsUsesContextOrQuery(t *testing.T) {
	s := mustCompile(t, map[string]any{"min_words": 4})
	if got := s(Context{"query": "one two three four"}); got != 1.0 {
		t.Errorf("4 words = %v", got)
	}
	if got := s(Context{"query": "one two"}); got != 0.0 {
		t.Errorf("2 words = %v", got)
	}
	if got := s(Context{"word_count": 9}); got != 1.0 {
		t.Errorf("word_count 9 = %v", got)
	}
}

func TestRegex(t *testing.T) {
	s := mustCompile(t, map[string]any{"regex": `\?$`})
	if got := s(Context{"query": "what is this?"}); got != 1.0 {
		t.Errorf("question = %v", got)
	}
	if got := s(Context{"query": "a statement"}); got != 0.0 {
		t.Errorf("statement = %v", got)
	}
}

func TestAllIsMin(t *testing.T) {
	s := mustCompile(t, map[string]any{"all": []any{
		map[string]any{"const": 0.4}, map[string]any{"const": 0.9},
	}})
	if got := s(Context{}); got != 0.4 {
		t.Errorf("all = %v", got)
	}
}

func TestAnyIsMax(t *testing.T) {
	s := mustCompile(t, map[string]any{"any": []any{
		map[string]any{"const": 0.4}, map[string]any{"const": 0.9},
	}})
	if got := s(Context{}); got != 0.9 {
		t.Errorf("any = %v", got)
	}
}

func TestNot(t *testing.T) {
	s := mustCompile(t, map[string]any{"not": map[string]any{"flag": "x"}})
	if got := s(Context{"x": true}); got != 0.0 {
		t.Errorf("not true = %v", got)
	}
	if got := s(Context{"x": false}); got != 1.0 {
		t.Errorf("not false = %v", got)
	}
}

func TestScale(t *testing.T) {
	s := mustCompile(t, map[string]any{"scale": []any{map[string]any{"const": 1.0}, 0.6}})
	if got := s(Context{}); !approx(got, 0.6) {
		t.Errorf("scale = %v", got)
	}
}

func TestSumClamped(t *testing.T) {
	s := mustCompile(t, map[string]any{"sum": []any{
		map[string]any{"const": 0.7}, map[string]any{"const": 0.8},
	}})
	if got := s(Context{}); got != 1.0 {
		t.Errorf("sum = %v", got)
	}
}

func TestNestedComposition(t *testing.T) {
	expr := map[string]any{"all": []any{
		map[string]any{"flag": "is_question"},
		map[string]any{"any": []any{
			map[string]any{"lexical": []any{"x"}},
			map[string]any{"min_words": 3},
		}},
	}}
	s := mustCompile(t, expr)
	if got := s(Context{"is_question": true, "query": "a b c"}); got != 1.0 {
		t.Errorf("nested 1 = %v", got)
	}
	if got := s(Context{"is_question": true, "query": "short"}); got != 0.0 {
		t.Errorf("nested 2 = %v", got)
	}
	if got := s(Context{"is_question": false, "query": "a b c"}); got != 0.0 {
		t.Errorf("nested 3 = %v", got)
	}
}

func TestUnknownOperatorRaises(t *testing.T) {
	_, err := Compile(map[string]any{"frobnicate": 1})
	var se *SignalError
	if !errors.As(err, &se) {
		t.Errorf("expected SignalError, got %v", err)
	}
}

func TestMalformedRaises(t *testing.T) {
	if _, err := Compile(map[string]any{"a": 1, "b": 2}); err == nil {
		t.Error("expected error for two-key dict")
	}
	if _, err := Compile(map[string]any{"scale": []any{1.0}}); err == nil {
		t.Error("expected error for scale with one element")
	}
}

func BenchmarkEvalNested(b *testing.B) {
	expr := map[string]any{"all": []any{
		map[string]any{"flag": "is_question"},
		map[string]any{"any": []any{
			map[string]any{"lexical": []any{"compare", "vs"}},
			map[string]any{"min_words": 3},
		}},
	}}
	s, err := Compile(expr)
	if err != nil {
		b.Fatal(err)
	}
	ctx := Context{"is_question": true, "query": "compare a and b"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s(ctx)
	}
}
