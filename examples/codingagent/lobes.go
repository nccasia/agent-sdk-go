package codingagent

import (
	"github.com/nccasia/agent-sdk-go/agent_sdk/core/spec"
	"github.com/nccasia/agent-sdk-go/agent_sdk/lobes"
)

func always(map[string]any) float64 { return 1.0 }
func dark(map[string]any) float64   { return 0.0 }

// CodingLobes returns the production-shaped coding lobes (the OY context axis).
// Mirrors coding_agent.lobes.coding_lobes. triage/explore/summarize are
// always-on; the rest are lit by their flow's lobe bias.
func CodingLobes() []lobes.Lobe {
	return []lobes.Lobe{
		{
			ID: "triage", Name: "Triage",
			Description: "Classify the request: a question, a quick fix, or a feature.",
			UseWhen:     "every coding turn",
			Layer:       spec.LayerCognition, Behavior: "select",
			SystemPrompt: "You are a careful senior software engineer working in a real repository. " +
				"First understand exactly what is being asked: is it a question about the " +
				"code, a small fix, or a multi-step change? Match your effort to the task.",
			Activation: always,
		},
		{
			ID: "explore", Name: "Explore",
			Description: "Read the relevant code before proposing or making changes.",
			UseWhen:     "before answering or editing",
			Layer:       spec.LayerCognition, Behavior: "gather", Order: 1,
			SystemPrompt: "Before you answer or edit, ground yourself in the ACTUAL code — even in a " +
				"large repo. Navigate efficiently: LS for layout, Glob (e.g. " +
				"`**/*.py`, `apps/**/*.ts`) to find files by name, Grep to find symbols/" +
				"usages, then Read (with offset/limit for big files) to read the exact " +
				"lines. Follow imports and references to build a precise mental model. " +
				"Never guess a file's contents — read it. Don't read the whole repo; reach " +
				"the few files that matter.",
			Activation: always,
		},
		{
			ID: "plan", Name: "Plan",
			Description: "Decompose a multi-step change into concrete, ordered steps.",
			UseWhen:     "a feature or refactor that needs more than one edit",
			Layer:       spec.LayerCognition, Behavior: "decompose", Order: 2,
			SystemPrompt: "Lay out the few concrete steps this change needs (which files, which " +
				"functions, which tests). Keep it minimal — the smallest change that " +
				"correctly does the job. Save the plan to memory (action=remember, " +
				"scope=conversation, key=plan) so you can track progress across many steps " +
				"without losing the thread.",
			Activation: dark, // lit by the feature flow's lobe bias
		},
		{
			ID: "implement", Name: "Implement",
			Description: "Write minimal, correct code that matches the surrounding style.",
			UseWhen:     "making the change",
			Layer:       spec.LayerCognition, Behavior: "compose", Order: 3,
			SystemPrompt: "Make the change with Edit (exact string match — Read first so the match is " +
				"exact) or Write for new files. Match the existing code's style, naming, and " +
				"idioms. Change as little as possible. Add or update tests for what you " +
				"changed. Do not leave the tree broken. As you complete plan steps, update " +
				"the plan in memory.",
			Activation: dark,
		},
		{
			ID: "verify", Name: "Verify",
			Description: "Run the real test suite / build and report the result honestly.",
			UseWhen:     "after making a change",
			Layer:       spec.LayerExpression, Behavior: "verify", Order: 8,
			SystemPrompt: "Run the project's tests (or the most relevant subset) with Bash and " +
				"read the output. If anything fails, fix it. Report pass/fail truthfully — " +
				"never claim success you did not observe.",
			Activation: dark,
		},
		{
			ID: "summarize", Name: "Summarize",
			Description: "State concisely what changed (files touched) and the test result.",
			UseWhen:     "producing the final reply",
			Layer:       spec.LayerExpression, Behavior: "compose", Order: 9,
			SystemPrompt: "Summarize for a reviewer: what you changed, which files, and the test " +
				"result. Be concrete and brief. If you could not complete the task, say so " +
				"plainly and explain what is blocking.",
			Activation: always,
		},
		{
			ID: "surveyor", Name: "Surveyor",
			Description: "Map a large codebase's structure breadth-first before diving in.",
			UseWhen:     "understanding a whole system",
			Layer:       spec.LayerCognition, Behavior: "gather", Order: 1,
			SystemPrompt: "Map the repository top-down before diving deep. Use LS on the root " +
				"and key directories, Glob for the dominant file types and entry points " +
				"(README, pyproject/package.json, __init__/main/index), and Grep for the " +
				"high-level wiring. Build a mental table of contents — the subsystems, " +
				"where each lives, and how they connect — so the plan can target them.",
			Activation: dark,
		},
		{
			ID: "documenter", Name: "Documenter",
			Description: "Aggregate findings into a clear architecture document.",
			UseWhen:     "writing the architecture overview",
			Layer:       spec.LayerExpression, Behavior: "compose", Order: 9,
			SystemPrompt: "Recall ALL findings you saved to memory (action=recall, scope=conversation, " +
				"with a query like 'finding') and synthesize them into a single, well-" +
				"structured Markdown architecture document. Then WRITE it to disk in ONE " +
				"call: either Write(file_path='ARCHITECTURE.md', content=<the FULL document " +
				"text>) — always include the complete `content` — or a `cat > ARCHITECTURE.md " +
				"<<'EOF' … EOF` heredoc via Bash. The document must include: a one-paragraph " +
				"overview, a subsystem-by-subsystem breakdown (what each does + its key " +
				"files), how the pieces fit together (the data/control flow), and the main " +
				"entry points. Cite concrete file paths. Be accurate — only state what you " +
				"actually read; do not invent APIs or line numbers.",
			Activation: dark,
		},
	}
}

// CodingLobeSpecs compiles the coding lobes to spec.Lobe form.
func CodingLobeSpecs() []spec.Lobe {
	ls := CodingLobes()
	out := make([]spec.Lobe, len(ls))
	for i, l := range ls {
		out[i] = l.Spec()
	}
	return out
}
