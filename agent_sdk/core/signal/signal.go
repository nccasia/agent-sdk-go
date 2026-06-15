// Package signal implements declarative signal expressions — the
// host-language-free activation grammar ported from agent_sdk/signals.py.
//
// A signal is a small JSON-shaped expression evaluated against a context map to
// a float in [0, 1]. Keeping activation declarative (rather than a host-language
// closure) is what lets the deterministic core serialize and port
// byte-identically across runtimes.
//
// Grammar:
//
//	{"const": 1.0}                  constant
//	{"flag": "is_question"}         context[flag] truthy -> 1.0 else 0.0
//	{"lexical": ["compare", "vs"]}  any term present in the query -> 1.0
//	{"min_words": 8}                query word count >= n -> 1.0
//	{"regex": "\\?$"}               query matches -> 1.0
//	{"all": [<expr>, ...]}          min() of children (AND)
//	{"any": [<expr>, ...]}          max() of children (OR)
//	{"not": <expr>}                 1 - child
//	{"scale": [<expr>, 0.6]}        child * weight
//	{"sum": [<expr>, ...]}          clamped sum
//
// Compile turns an expression into a pure func(Context) float64; Eval evaluates
// one in place. Both are free and deterministic — no clock, no I/O, no LLM. A
// bare number compiles to a constant; nil to 0.0.
package signal

import (
	"fmt"
	"math"
	"regexp"
	"strings"
)

// Round4 rounds to 4 decimal places using banker's rounding (round-half-to-
// even), matching Python's round(x, 4). The deterministic core emits floats
// through this so traces are byte-stable across runtimes.
func Round4(x float64) float64 {
	scaled := x * 1e4
	rounded := math.RoundToEven(scaled)
	return rounded / 1e4
}

// Context is the evaluation substrate — a plain map mirroring the Python dict.
type Context = map[string]any

// Signal is a compiled pure activation function.
type Signal func(ctx Context) float64

// SignalError is a malformed signal expression.
type SignalError struct{ msg string }

func (e *SignalError) Error() string { return e.msg }

func errf(format string, args ...any) *SignalError {
	return &SignalError{msg: fmt.Sprintf(format, args...)}
}

func clamp(x float64) float64 {
	if x < 0.0 {
		return 0.0
	}
	if x > 1.0 {
		return 1.0
	}
	return x
}

func query(ctx Context) string {
	if v, ok := ctx["query"]; ok && v != nil {
		if s, ok := v.(string); ok {
			return s
		}
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func wordCount(ctx Context) int {
	if wc, ok := ctx["word_count"]; ok {
		switch n := wc.(type) {
		case int:
			return n
		case int64:
			return int(n)
		}
	}
	return len(strings.Fields(query(ctx)))
}

// asFloat coerces a numeric-ish value to float64, reporting success.
func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case bool:
		if n {
			return 1.0, true
		}
		return 0.0, true
	}
	return 0, false
}

// Compile compiles a declarative signal expression to a pure func(Context) float64.
//
// It returns a SignalError for unknown operators or malformed shapes so a bad
// spec fails at load time, not silently at activation time.
func Compile(expr any) (Signal, error) {
	// Bare scalars and nil — the common "always on" / "dark" cases.
	if expr == nil {
		return func(Context) float64 { return 0.0 }, nil
	}
	switch v := expr.(type) {
	case bool:
		bv := 0.0
		if v {
			bv = 1.0
		}
		return func(Context) float64 { return bv }, nil
	case int:
		nv := clamp(float64(v))
		return func(Context) float64 { return nv }, nil
	case int64:
		nv := clamp(float64(v))
		return func(Context) float64 { return nv }, nil
	case float32:
		nv := clamp(float64(v))
		return func(Context) float64 { return nv }, nil
	case float64:
		nv := clamp(v)
		return func(Context) float64 { return nv }, nil
	}

	m, ok := expr.(map[string]any)
	if !ok || len(m) != 1 {
		return nil, errf("signal expression must be a scalar or a single-key dict, got %#v", expr)
	}

	var op string
	var arg any
	for k, v := range m {
		op, arg = k, v
	}

	switch op {
	case "const":
		f, ok := asFloat(arg)
		if !ok {
			return nil, errf("const expects a number, got %#v", arg)
		}
		v := clamp(f)
		return func(Context) float64 { return v }, nil

	case "flag":
		key := fmt.Sprintf("%v", arg)
		return func(ctx Context) float64 {
			if truthy(ctx[key]) {
				return 1.0
			}
			return 0.0
		}, nil

	case "lexical":
		terms := toLowerTerms(arg)
		return func(ctx Context) float64 {
			q := strings.ToLower(query(ctx))
			for _, t := range terms {
				if strings.Contains(q, t) {
					return 1.0
				}
			}
			return 0.0
		}, nil

	case "min_words":
		f, ok := asFloat(arg)
		if !ok {
			return nil, errf("min_words expects an int, got %#v", arg)
		}
		n := int(f)
		return func(ctx Context) float64 {
			if wordCount(ctx) >= n {
				return 1.0
			}
			return 0.0
		}, nil

	case "regex":
		pat, err := regexp.Compile(fmt.Sprintf("%v", arg))
		if err != nil {
			return nil, errf("regex %q: %v", arg, err)
		}
		return func(ctx Context) float64 {
			if pat.MatchString(query(ctx)) {
				return 1.0
			}
			return 0.0
		}, nil

	case "all":
		children, err := compileChildren(arg)
		if err != nil {
			return nil, err
		}
		if len(children) == 0 {
			return func(Context) float64 { return 1.0 }, nil // vacuous AND
		}
		return func(ctx Context) float64 {
			m := children[0](ctx)
			for _, c := range children[1:] {
				if v := c(ctx); v < m {
					m = v
				}
			}
			return m
		}, nil

	case "any":
		children, err := compileChildren(arg)
		if err != nil {
			return nil, err
		}
		if len(children) == 0 {
			return func(Context) float64 { return 0.0 }, nil // vacuous OR
		}
		return func(ctx Context) float64 {
			m := children[0](ctx)
			for _, c := range children[1:] {
				if v := c(ctx); v > m {
					m = v
				}
			}
			return m
		}, nil

	case "not":
		child, err := Compile(arg)
		if err != nil {
			return nil, err
		}
		return func(ctx Context) float64 { return clamp(1.0 - child(ctx)) }, nil

	case "scale":
		pair, ok := arg.([]any)
		if !ok || len(pair) != 2 {
			return nil, errf("scale expects [<expr>, weight]")
		}
		child, err := Compile(pair[0])
		if err != nil {
			return nil, err
		}
		weight, ok := asFloat(pair[1])
		if !ok {
			return nil, errf("scale expects [<expr>, weight]")
		}
		return func(ctx Context) float64 { return clamp(child(ctx) * weight) }, nil

	case "sum":
		children, err := compileChildren(arg)
		if err != nil {
			return nil, err
		}
		return func(ctx Context) float64 {
			s := 0.0
			for _, c := range children {
				s += c(ctx)
			}
			return clamp(s)
		}, nil
	}

	return nil, errf("unknown signal operator %q", op)
}

// Eval evaluates a signal expression against a context (compile + call).
//
// It panics on a malformed expression to mirror the in-place Python eval_signal
// raising. Callers wanting the error should use Compile.
func Eval(expr any, ctx Context) float64 {
	s, err := Compile(expr)
	if err != nil {
		panic(err)
	}
	return s(ctx)
}

func compileChildren(arg any) ([]Signal, error) {
	if arg == nil {
		return nil, nil
	}
	list, ok := arg.([]any)
	if !ok {
		return nil, errf("expected a list of child expressions, got %#v", arg)
	}
	out := make([]Signal, 0, len(list))
	for _, c := range list {
		s, err := Compile(c)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func toLowerTerms(arg any) []string {
	if arg == nil {
		return nil
	}
	list, ok := arg.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, t := range list {
		out = append(out, strings.ToLower(fmt.Sprintf("%v", t)))
	}
	return out
}

// truthy mirrors Python's truthiness for the values a context map carries.
func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	case float32:
		return x != 0
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	}
	return true
}
