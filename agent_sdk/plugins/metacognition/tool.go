package metacognition

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/mezon/agent-sdk-go/agent_sdk/flows"
	"github.com/mezon/agent-sdk-go/agent_sdk/memory"
)

// TurnKey is the goroutine-local key for the active turn (mirrors
// the Python “_TURN = ContextVar(...)“). Tests set it via
// “WithTurn“.
type TurnKey struct{}

// turnTLS holds the goroutine-local turn state (a *lobeOutputs + a
// *Scratchpad pair).
var turnTLS sync.Map // map[TurnKey]any → map[TurnKey]*turnState

type turnState struct {
	LobeOutputs map[string]any
	Scratchpad  *memory.Scratchpad
}

// WithTurn runs fn with the given turn state installed as the
// goroutine-local current turn. Mirrors Python “_TURN.set(turn)“.
func WithTurn(state any, fn func() string) string {
	key := TurnKey{}
	prev, had := turnTLS.Load(key)
	turnTLS.Store(key, state)
	defer func() {
		if had {
			turnTLS.Store(key, prev)
		} else {
			turnTLS.Delete(key)
		}
	}()
	return fn()
}

// currentTurn returns the goroutine-local turn or nil.
func currentTurn() *turnState {
	v, ok := turnTLS.Load(TurnKey{})
	if !ok {
		return nil
	}
	if t, ok := v.(*turnState); ok {
		return t
	}
	return nil
}

// PinnedSkills is the canonical pinned skill-slug set the meta
// control tool refuses to strip.
var PinnedSkills = map[string]struct{}{"cite": {}, "filter": {}}

// MetaControlToolRuntime is the one `meta_control` tool the
// metacognition plugin mounts.
type MetaControlToolRuntime struct{}

// NewMetaControlToolRuntime builds a runtime.
func NewMetaControlToolRuntime() *MetaControlToolRuntime { return &MetaControlToolRuntime{} }

// GetToolSpecs returns the single `meta_control` tool spec.
func (rt *MetaControlToolRuntime) GetToolSpecs() []map[string]any {
	return []map[string]any{
		{
			"name": "meta_control",
			"input_schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{
						"type": "string",
						"enum": []string{"use_skills", "bias_flow", "regulate"},
					},
					"slugs":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"path":    map[string]any{"type": "string"},
					"request": map[string]any{"type": "string", "enum": []string{"retry", "skip", "expand"}},
					"step":    map[string]any{"type": "string"},
				},
				"required": []string{"action"},
			},
			"description": "Per-turn meta control: pick skills, bias the next turn's flow, or regulate a step.",
		},
	}
}

// CallTool dispatches the action.
func (rt *MetaControlToolRuntime) CallTool(ctx context.Context, name string, inp map[string]any, _ []map[string]any, _ map[string]struct{}) (string, error) {
	if name != "meta_control" {
		return fmt.Sprintf("Error: unknown tool %q.", name), nil
	}
	turn := currentTurn()
	if turn == nil {
		return "no active turn", nil
	}
	action, _ := inp["action"].(string)
	switch action {
	case "use_skills":
		slugs, _ := inp["slugs"].([]any)
		clean := []string{}
		dropped := []string{}
		for _, s := range slugs {
			str, ok := s.(string)
			if !ok {
				continue
			}
			if _, pinned := PinnedSkills[str]; pinned {
				dropped = append(dropped, str)
				continue
			}
			clean = append(clean, str)
		}
		if len(slugs) > 0 && len(clean) == 0 {
			return "Error: refuse to strip all skills (pinned guards survived).", nil
		}
		if turn.LobeOutputs == nil {
			turn.LobeOutputs = map[string]any{}
		}
		turn.LobeOutputs["skills_in_use"] = clean
		parts := []string{"ok"}
		if len(clean) > 0 {
			parts = append(parts, "skills="+strings.Join(clean, ","))
		}
		if len(dropped) > 0 {
			parts = append(parts, "pinned="+strings.Join(dropped, ","))
		}
		return strings.Join(parts, " "), nil
	case "bias_flow":
		path, _ := inp["path"].(string)
		if path == "" {
			return "Error: `path` required.", nil
		}
		if turn.LobeOutputs == nil {
			turn.LobeOutputs = map[string]any{}
		}
		turn.LobeOutputs["meta_flow_bias"] = path
		return "ok (applies to NEXT turn)", nil
	case "regulate":
		request, _ := inp["request"].(string)
		step, _ := inp["step"].(string)
		if request == "skip" {
			if step == "cite" || step == "filter" {
				return "Refused: cannot skip a pinned step.", nil
			}
		}
		if turn.Scratchpad == nil {
			turn.Scratchpad = memory.NewScratchpad()
		}
		turn.Scratchpad.Set("meta_regulate_request", map[string]any{"request": request, "step": step})
		return "ok (recorded)", nil
	default:
		// fan_out is intentionally rejected here (delegation moved
		// to the planning plugin).
		return fmt.Sprintf("Error: unknown action %q.", action), nil
	}
}

// BiasFlagKey is the ctx key the recognizer reads for the recorded
// next-turn flow bias.
func BiasFlagKey(path string) string { return "meta_bias_to_" + path }

// RecognizerWithBias returns the signal fn for the `meta` flow: a
// conservative rewrite / rethink cue, plus a deterministic 1.0 when
// the recorded bias toward `meta` is on.
func RecognizerWithBias() flows.SignalFn {
	return func(ctx map[string]any) float64 {
		if v, ok := ctx[BiasFlagKey("meta")].(bool); ok && v {
			return 1.0
		}
		q, _ := ctx["query"].(string)
		if q == "" {
			return 0
		}
		q = strings.ToLower(strings.TrimSpace(q))
		if metaCue.MatchString(q) {
			return 0.85
		}
		return 0
	}
}

// BiasFlag returns the context key the engine uses to mark a
// recorded next-turn bias toward `path`. Mirrors the Python
// `path.bias_flag(path)`.
func BiasFlag(path string) string { return BiasFlagKey(path) }

var metaCue = regexp.MustCompile(`(?i)\b(` +
	`rethink|reconsider|think again|step back|reframe|take a step back|` +
	`reflect on your approach|change (your )?approach|different angle|` +
	`are you sure|double[- ]check` +
	`)\b`)
