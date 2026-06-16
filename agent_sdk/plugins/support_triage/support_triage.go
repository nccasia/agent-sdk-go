// Package support_triage hosts PluginSupportTriage — a worked example
// plugin carrying every capacity kind (lobe, stage, flow, path,
// skill, tool) used by the extensionbench parity tests.
package support_triage

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/mezon/agent-sdk-go/agent_sdk/agent"
	"github.com/mezon/agent-sdk-go/agent_sdk/core/spec"
	"github.com/mezon/agent-sdk-go/agent_sdk/flows"
	"github.com/mezon/agent-sdk-go/agent_sdk/lobes"
)

// Lobe is the canonical triage lobe (a simple recall lobe that
// surfaces the triage skill in the prompt).
var TriageLobe = lobes.Lobe{
	ID:           "triage",
	Name:         "triage",
	Description:  "Surfaces the triage_policy skill and any open ticket context.",
	UseWhen:      "The triage flow is active.",
	How:          "Renders a one-line triage context block.",
	Layer:        4,
	Behavior:     "recall",
	Order:        0,
	BuildContext: true,
	Threshold:    0.0,
	Activation:   triageActivation,
}

func triageActivation(ctx map[string]any) float64 {
	if v, ok := ctx["active_path"].(string); ok && v == "triage" {
		return 1.0
	}
	return 0
}

// TriageLobeSpec compiles TriageLobe to its internal spec.Lobe form.
func TriageLobeSpec() spec.Lobe { return TriageLobe.Spec() }

// TriageRecognizer is the path's recognizer. Conservative: matches
// "this incident is urgent, escalate ticket N, the service is down"
// and similar operational cues.
func TriageRecognizer(ctx map[string]any) float64 {
	if fired, ok := ctx["fired_prompt"].(bool); ok && fired {
		return 1.0
	}
	q, _ := ctx["query"].(string)
	if q == "" {
		return 0
	}
	q = strings.ToLower(q)
	if triageCue.MatchString(q) {
		return 0.9
	}
	return 0
}

var triageCue = regexp.MustCompile(`(?i)\b(` +
	`incident|escalat|sev[0-9]|p[0-9]|outage|service (is )?down|` +
	`urgent|critical|on[- ]call|page[d]?|alert|ticket \d+|` +
	`production (is )?down|deployment (is )?broken` +
	`)\b`)

// LookupTicketToolRuntime is the single `lookup_ticket` tool the
// support_triage plugin mounts. Pure stub: returns the input echoed
// back as the "ticket row". The real implementation would hit a
// ticketing backend.
type LookupTicketToolRuntime struct{}

// NewLookupTicketToolRuntime builds a runtime.
func NewLookupTicketToolRuntime() *LookupTicketToolRuntime { return &LookupTicketToolRuntime{} }

// GetToolSpecs returns the single `lookup_ticket` tool spec.
func (rt *LookupTicketToolRuntime) GetToolSpecs() []map[string]any {
	return []map[string]any{
		{
			"name": "lookup_ticket",
			"input_schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "string"},
				},
				"required": []string{"id"},
			},
			"description": "Look up a ticket by id.",
		},
	}
}

// CallTool dispatches the action.
func (rt *LookupTicketToolRuntime) CallTool(ctx context.Context, name string, inp map[string]any, _ []map[string]any, _ map[string]struct{}) (string, error) {
	if name != "lookup_ticket" {
		return fmt.Sprintf("Error: unknown tool %q.", name), nil
	}
	id, _ := inp["id"].(string)
	if id == "" {
		return "Error: `id` required.", nil
	}
	return fmt.Sprintf(`{"id": %q, "status": "open", "severity": "high"}`, id), nil
}

// TriageAgentSetup is the type alias the plugin uses when installing.
type TriageAgentSetup = agent.AgentSetup

// PluginSupportTriage is the worked-example plugin.
type PluginSupportTriage struct{}

// NewPluginSupportTriage builds a PluginSupportTriage.
func NewPluginSupportTriage() *PluginSupportTriage { return &PluginSupportTriage{} }

// Name returns the plugin name.
func (p *PluginSupportTriage) Name() string { return "support_triage" }

// Lobes returns the lobes the plugin contributes.
func (p *PluginSupportTriage) Lobes() []spec.Lobe {
	return []spec.Lobe{TriageLobe.Spec()}
}

// TriageSkill is the procedural knowledge the plugin contributes.
type TriageSkill struct{ id string }

func (s TriageSkill) ID() string { return s.id }

// TriageStage is the single triage stage.
type TriageStage struct{ id string }

func (s TriageStage) ID() string { return s.id }

// Install wires the plugin into the AgentSetup.
func (p *PluginSupportTriage) Install(setup *TriageAgentSetup) {
	setup.AddLobe(TriageLobe.Spec())
	setup.AddStage(triageStage())
	setup.AddFlow(triageFlow())
	setup.AddPath(triagePath())
	setup.AddSkill(TriageSkill{id: "triage_policy"})
	setup.AddToolRuntime(NewLookupTicketToolRuntime())
}

func triageStage() flows.FlowStep {
	return flows.NewFlowStep(flows.FlowStep{
		Name:        "triage",
		Description: "Triage the incident (look up the ticket, set severity).",
		Loop:        "single",
		Tools:       []string{"lookup_ticket"},
		Lobes:       []string{"triage"},
	})
}

func triageFlow() flows.Flow {
	return flows.NewFlow("triage",
		flows.FlowUseWhen("an incident that needs escalation / triage"),
		flows.FlowStages("triage"),
		flows.FlowGrounds(false),
		flows.FlowThreshold(0.5),
		flows.FlowSignalFn(TriageRecognizer),
	)
}

func triagePath() spec.Path {
	return spec.Path{
		Name:       "triage",
		Members:    []string{"triage"},
		Recognizer: TriageRecognizer,
		Threshold:  0.5,
	}
}
