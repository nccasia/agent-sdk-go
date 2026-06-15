// Ported from the recognize_paths portion of
// agent-sdk/tests/test_blocks_smoke.py.
package feature

import (
	"testing"

	"github.com/mezon/agent-sdk-go/agent_sdk/core/spec"
)

func TestRecognizePathsClampsAndRounds(t *testing.T) {
	research := spec.Path{
		Name: "research",
		Recognizer: func(ctx map[string]any) float64 {
			if b, _ := ctx["complex"].(bool); b {
				return 1.0
			}
			return 0.0
		},
		Threshold: 0.5,
	}
	// over-range recognizer is clamped to 1.0
	hot := spec.Path{Name: "hot", Recognizer: func(map[string]any) float64 { return 5.0 }}
	cold := spec.Path{Name: "cold", Recognizer: func(map[string]any) float64 { return -3.0 }}

	scores := RecognizePaths(map[string]any{"complex": true}, []spec.Path{research, hot, cold})
	if scores["research"] != 1.0 {
		t.Errorf("research = %v", scores["research"])
	}
	if scores["hot"] != 1.0 {
		t.Errorf("hot (clamped high) = %v", scores["hot"])
	}
	if scores["cold"] != 0.0 {
		t.Errorf("cold (clamped low) = %v", scores["cold"])
	}
}

func TestRecognizePathsNilRecognizer(t *testing.T) {
	p := spec.Path{Name: "p"}
	scores := RecognizePaths(map[string]any{}, []spec.Path{p})
	if scores["p"] != 0.0 {
		t.Errorf("nil recognizer = %v", scores["p"])
	}
}
