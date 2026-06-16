package codingagent

import (
	"github.com/nccasia/agent-sdk-go/agent_sdk/core/signal"
	"github.com/nccasia/agent-sdk-go/agent_sdk/core/spec"
	"github.com/nccasia/agent-sdk-go/agent_sdk/flows"
)

// Per-stage tool slices (Claude Code's canonical tool names). Mirror
// flows/stages/_slices.py.
var (
	readTools   = []string{"LS", "Glob", "Grep", "Read", "Bash"}
	editTools   = []string{"Read", "Write", "Edit", "Glob", "Grep", "Bash"}
	verifyTools = []string{"Bash", "Read", "Edit", "Grep"}
	noteTools   = []string{"LS", "Glob", "Grep", "Read", "memory", "Bash"}
	docTools    = []string{"memory", "Read", "Glob", "Write", "Bash"}
)

// ReadonlyStages — read-only stages must not write files. Consumed by the
// DocWriteGuard the CodingPlugin installs. Mirrors flows/stages/__init__.py.
var ReadonlyStages = []string{"explore", "survey", "investigate", "answer", "plan"}

func hopsPtr(n int) *int { return &n }
func tokPtr(n int) *int  { return &n }

// CodingStages returns every stage, in registration order (also the
// codebase-understanding tail: survey → investigate → document). Mirrors
// coding_agent.flows.stages.coding_stages.
func CodingStages() []flows.FlowStep {
	return []flows.FlowStep{
		flows.NewFlowStep(flows.FlowStep{
			Name: "explore", Lobes: []string{"triage", "explore"}, Loop: "agentic", Tools: readTools,
			Description: "Navigate + read the codebase to ground the work.", Hops: hopsPtr(50),
		}),
		flows.NewFlowStep(flows.FlowStep{
			Name: "plan", Lobes: []string{"plan"}, Loop: "single",
			Description: "Decompose a multi-step change into ordered steps.",
		}),
		flows.NewFlowStep(flows.FlowStep{
			Name: "implement", Lobes: []string{"implement"}, Loop: "agentic", Tools: editTools,
			Description: "Make the change on disk.", Hops: hopsPtr(80),
		}),
		flows.NewFlowStep(flows.FlowStep{
			Name: "verify", Lobes: []string{"verify"}, Loop: "agentic", Tools: verifyTools,
			Description: "Run the tests and fix failures.", Hops: hopsPtr(40),
		}),
		flows.NewFlowStep(flows.FlowStep{
			Name: "answer", Lobes: []string{"triage", "explore", "summarize"}, Loop: "agentic", Tools: readTools,
			Description: "Deeply explore, then answer a question about the code.", Hops: hopsPtr(80),
		}),
		flows.NewFlowStep(flows.FlowStep{
			Name: "summarize", Lobes: []string{"summarize"}, Loop: "single",
			Description: "Report what changed (files + test result).",
		}),
		flows.NewFlowStep(flows.FlowStep{
			Name: "survey", Lobes: []string{"triage", "surveyor"}, Loop: "agentic", Tools: readTools,
			Description: "Map the repository structure top-down.", Hops: hopsPtr(40),
		}),
		flows.NewFlowStep(flows.FlowStep{
			Name: "investigate", Lobes: []string{"explore"}, Loop: "agentic", Tools: noteTools,
			Description: "Follow the plan: read each subsystem, save findings to memory.", Hops: hopsPtr(80),
		}),
		flows.NewFlowStep(flows.FlowStep{
			Name: "document", Lobes: []string{"documenter"}, Loop: "agentic", Tools: docTools,
			Description: "Aggregate findings + write the architecture document.",
			Hops:        hopsPtr(50), MaxTokens: tokPtr(8000),
		}),
	}
}

// CodingStagesAny is CodingStages as []any (for cfg.Stages / setup.AddStage).
func CodingStagesAny() []any {
	ss := CodingStages()
	out := make([]any, len(ss))
	for i := range ss {
		st := ss[i]
		out[i] = &st
	}
	return out
}

// notQuestion gates the change flows so a long question never mis-routes.
var notQuestion = map[string]any{"not": map[string]any{"flag": "is_question"}}

type flowDef struct {
	id        string
	useWhen   string
	stages    []string
	threshold float64
	signal    map[string]any
}

func codingFlowDefs() []flowDef {
	return []flowDef{
		{
			id: "feature", useWhen: "a multi-step change: a feature, refactor, or new code",
			stages: []string{"explore", "plan", "implement", "verify", "summarize"}, threshold: 0.5,
			signal: map[string]any{"all": []any{
				notQuestion,
				map[string]any{"any": []any{
					map[string]any{"lexical": []any{"add", "implement", "create", "build", "feature",
						"refactor", "support", "introduce", "rewrite", "migrate"}},
					map[string]any{"min_words": 18},
				}},
			}},
		},
		{
			id: "quick_fix", useWhen: "a small bug fix",
			stages: []string{"explore", "implement", "verify", "summarize"}, threshold: 0.5,
			signal: map[string]any{"all": []any{
				notQuestion,
				map[string]any{"lexical": []any{"fix", "bug", "broken", "error", "fails", "failing",
					"crash", "typo", "incorrect", "regression"}},
			}},
		},
		{
			id: "understand", useWhen: "understand a whole system + write an architecture doc",
			stages: []string{"survey", "plan", "investigate", "document"}, threshold: 0.55,
			signal: map[string]any{"any": []any{
				map[string]any{"lexical": []any{"architecture", "overview", "document the", "introduce the",
					"map the codebase", "system design", "how the system",
					"whole codebase", "entire codebase", "the codebase and write"}},
				map[string]any{"all": []any{
					map[string]any{"lexical": []any{"understand"}},
					map[string]any{"lexical": []any{"codebase", "system", "architecture", "repo", "project"}},
				}},
			}},
		},
		{
			id: "question", useWhen: "a question about the code (no change)",
			stages: []string{"answer"}, threshold: 0.4,
			signal: map[string]any{"any": []any{
				map[string]any{"flag": "is_question"},
				map[string]any{"lexical": []any{"how", "what", "why", "explain", "trace", "describe",
					"where", "which", "does", "summarize"}},
				map[string]any{"const": 0.3},
			}},
		},
	}
}

// CodingFlows returns the coding agent's flows (the intent axis). Mirrors
// coding_agent.flows.coding_flows.
func CodingFlows() []flows.Flow {
	defs := codingFlowDefs()
	out := make([]flows.Flow, 0, len(defs))
	for _, d := range defs {
		out = append(out, flows.NewFlow(d.id,
			flows.FlowUseWhen(d.useWhen),
			flows.FlowStages(d.stages...),
			flows.FlowThreshold(d.threshold),
			flows.FlowGrounds(false),
			flows.FlowSignalExpr(d.signal),
		))
	}
	return out
}

// codingPaths derives one spec.Path per flow with the flow's OWN compiled signal
// as the recognizer and the flow's stage-lobes as the bias members — the
// faithful derivation (Python _derive_path_specs / Flow.to_path_spec). Added via
// the plugin so it overrides the agent's default flow-derived paths.
func codingPaths() []spec.Path {
	stageLobes := map[string][]string{}
	for _, st := range CodingStages() {
		stageLobes[st.Name] = st.Lobes
	}
	out := []spec.Path{}
	for _, d := range codingFlowDefs() {
		members := []string{}
		seen := map[string]struct{}{}
		for _, sid := range d.stages {
			for _, lb := range stageLobes[sid] {
				if _, ok := seen[lb]; !ok {
					seen[lb] = struct{}{}
					members = append(members, lb)
				}
			}
		}
		bias := map[string]float64{}
		for _, m := range members {
			bias[m] = 1.0
		}
		sig, err := signal.Compile(d.signal)
		if err != nil {
			panic(err)
		}
		out = append(out, spec.Path{
			Name:       d.id,
			Members:    members,
			Bias:       bias,
			Threshold:  d.threshold,
			Grounds:    false,
			Recognizer: func(ctx map[string]any) float64 { return sig(ctx) },
		})
	}
	return out
}
