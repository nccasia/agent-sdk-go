// Package feature holds the B1 (perception) deterministic feature extraction —
// free path recognition over the turn's signals. Ported from the recognition
// half of agent_sdk/network/activation.py.
package feature

import (
	"github.com/mezon/agent-sdk-go/agent_sdk/core/signal"
	"github.com/mezon/agent-sdk-go/agent_sdk/core/spec"
)

// RecognizePaths scores every named path from the turn's free signals. Scores
// are PATH PRIORS (clamped to [0,1] and rounded to 4 places), not a routing
// decision — each turn resolves its own.
func RecognizePaths(ctx map[string]any, paths []spec.Path) map[string]float64 {
	out := make(map[string]float64, len(paths))
	for _, p := range paths {
		s := 0.0
		if p.Recognizer != nil {
			s = p.Recognizer(ctx)
		}
		if s < 0.0 {
			s = 0.0
		} else if s > 1.0 {
			s = 1.0
		}
		out[p.Name] = signal.Round4(s)
	}
	return out
}
