// Package metacognition hosts the MetacognitionPlugin — an opt-in
// plugin that mirrors the per-turn thinking state to the model
// (meta_context), adds the Navigator brief (nav_brief), exposes the
// meta_control tool (MetaControlToolRuntime), and provides a meta
// flow biased by next-turn flow signals.
package metacognition

import (
	"sort"
	"strings"

	"github.com/mezon/agent-sdk-go/agent_sdk/core/spec"
	"github.com/mezon/agent-sdk-go/agent_sdk/lobes"
)

// MetaContextLOBE mirrors the per-turn thinking state.
var MetaContextLOBE = lobes.Lobe{
	ID:           "meta_context",
	Name:         "meta_context",
	Description:  "Mirrors the per-turn thinking state (active path, stage, skills in use, observations, next-turn bias) to the model.",
	UseWhen:      "The metacognition plugin is active.",
	How:          "Renders a 'How you are thinking' block when any meta_observations or skills_in_use state is present.",
	Layer:        4,
	Behavior:     "recall",
	Order:        1,
	BuildContext: true,
	Threshold:    0.0, // always-on while the plugin is present
	Activation:   metaContextActivation,
}

func metaContextActivation(ctx map[string]any) float64 {
	// Always on while the plugin is mounted.
	_ = ctx
	return 1.0
}

// NavBriefLOBE is the Navigator brief lobe (a one-line navigational
// hint the metacognition surface adds to a stage's prompt).
var NavBriefLOBE = lobes.Lobe{
	ID:           "nav_brief",
	Name:         "nav_brief",
	Description:  "A one-line navigational hint from the metacognition controller.",
	UseWhen:      "The metacognition controller flagged a next-step hint.",
	How:          "Emits a one-line brief if lobe_outputs contains a nav_brief entry.",
	Layer:        4,
	Behavior:     "rewrite",
	Order:        2,
	BuildContext: true,
	Threshold:    0.0,
	Activation:   navBriefActivation,
}

func navBriefActivation(ctx map[string]any) float64 {
	if v, ok := ctx["nav_brief"]; ok && v != nil && v != "" {
		return 1.0
	}
	if outs, ok := ctx["lobe_outputs"].(map[string]any); ok {
		if v, ok := outs["nav_brief"]; ok && v != nil && v != "" {
			return 1.0
		}
	}
	return 0
}

// MetaContextLobeSpec compiles MetaContextLOBE to its internal form.
func MetaContextLobeSpec() spec.Lobe { return MetaContextLOBE.Spec() }

// NavBriefLobeSpec compiles NavBriefLOBE to its internal form.
func NavBriefLobeSpec() spec.Lobe { return NavBriefLOBE.Spec() }

// MetaContextLobe is the explicit Go mirror of the Python
// MetaContextLobe class. `prompt(ctx) -> [PromptContribution]`.
type MetaContextLobe struct{}

// Prompt renders the meta block. Mirrors `MetaContextLobe.prompt`.
func (MetaContextLobe) Prompt(ctx any) []MetaRender {
	return metaContextRender(ctx)
}

// MetaRender is the rich render output.
type MetaRender struct {
	Text   string
	Source string
}

// MetaContextRenderString returns the rendered text.
func MetaContextRenderString(ctx any) string {
	parts := []string{}
	for _, c := range metaContextRender(ctx) {
		parts = append(parts, c.Text)
	}
	return strings.Join(parts, "")
}

func metaContextRender(ctx any) []MetaRender {
	if ctx == nil {
		return nil
	}
	// Pull a small set of fields; tolerate missing.
	activePath := stringField(ctx, "active_path")
	stageID := stringField(ctx, "stage_id")
	activeLobes := stringSetField(ctx, "active_lobes")
	outs := mapField(ctx, "lobe_outputs")
	if len(activePath) == 0 && len(stageID) == 0 && len(outs) == 0 {
		return nil
	}
	skillsInUse, _ := outs["skills_in_use"].([]any)
	obs, _ := outs["meta_observations"].([]any)
	bias, _ := outs["meta_flow_bias"].(string)
	if len(skillsInUse) == 0 && len(obs) == 0 && bias == "" && activePath == "" && stageID == "" {
		_ = activeLobes
		return nil
	}
	var b strings.Builder
	b.WriteString("## How you are thinking\n")
	if activePath != "" {
		b.WriteString("- Path (recognized intent): ")
		b.WriteString(activePath)
		b.WriteString("\n")
	}
	if stageID != "" {
		b.WriteString("- Current step: ")
		b.WriteString(stageID)
		b.WriteString("\n")
	}
	if len(skillsInUse) > 0 {
		slugs := []string{}
		for _, s := range skillsInUse {
			if str, ok := s.(string); ok {
				slugs = append(slugs, str)
			}
		}
		sort.Strings(slugs)
		if len(slugs) > 0 {
			b.WriteString("- Skills in use: ")
			b.WriteString(strings.Join(slugs, ", "))
			b.WriteString("\n")
		}
	}
	if len(obs) > 0 {
		for _, o := range obs {
			row, ok := o.(map[string]any)
			if !ok {
				continue
			}
			kind, _ := row["kind"].(string)
			target, _ := row["target"].(string)
			b.WriteString("- observation: ")
			b.WriteString(kind)
			if target != "" {
				b.WriteString(" @ ")
				b.WriteString(target)
			}
			b.WriteString("\n")
		}
	}
	if bias != "" {
		b.WriteString("- next-turn flow bias: ")
		b.WriteString(bias)
		b.WriteString(" (applies to your NEXT turn)\n")
	}
	return []MetaRender{{Text: b.String(), Source: "meta_context"}}
}

func stringField(v any, field string) string {
	if v == nil {
		return ""
	}
	if m, ok := v.(map[string]any); ok {
		s, _ := m[field].(string)
		return s
	}
	return structStringField(v, field)
}

func stringSetField(v any, field string) map[string]struct{} {
	out := map[string]struct{}{}
	if v == nil {
		return out
	}
	var raw any
	if m, ok := v.(map[string]any); ok {
		raw = m[field]
	} else {
		raw = structReadField(v, field)
	}
	switch x := raw.(type) {
	case map[string]struct{}:
		for k := range x {
			out[k] = struct{}{}
		}
	case []string:
		for _, k := range x {
			out[k] = struct{}{}
		}
	case []any:
		for _, e := range x {
			if s, ok := e.(string); ok {
				out[s] = struct{}{}
			}
		}
	}
	return out
}

func mapField(v any, field string) map[string]any {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		out, _ := m[field].(map[string]any)
		return out
	}
	raw := structReadField(v, field)
	out, _ := raw.(map[string]any)
	return out
}
