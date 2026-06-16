// Package cognition is the deliberate-behavior domain (B4) — the reasoning
// work. Each lobe is LLM-driven at runtime (the prompts live here) and its
// activation signal is deterministic + free (delegated to lobes.SignalFor, the
// OY perception seam shared with the production network). Ported from
// agent_sdk/cognition/lobes/{classify,condense,plan,research,scope_check,synthesize}.py.
package cognition

import (
	"github.com/nccasia/agent-sdk-go/agent_sdk/core/spec"
	"github.com/nccasia/agent-sdk-go/agent_sdk/lobes"
)

// System prompts — the answer-composition / routing copy each lobe owns. Kept
// byte-identical to the Python SYSTEM_PROMPT constants.
const (
	ClassifySystemPrompt = `You are a query complexity classifier. Classify the user query as either "simple" or "complex".

Rules:
- "simple": A single knowledge lookup can answer the question. Factual questions, definitional queries, direct KB lookups.
- "complex": Requires multi-step reasoning, comparing information from multiple sources, investigating multiple aspects, or synthesis of several pieces of information.

Respond with ONLY the word "simple" or "complex".`

	CondenseSystemPrompt = `You rewrite a user's latest chat message into a standalone search query.

The user is in an ongoing conversation. Their latest message may reference earlier turns with pronouns or demonstratives (e.g. "this course", "khóa này", "môn đó", "it"). Using the conversation provided, rewrite the latest message so it is fully self-contained: resolve every pronoun/reference to the explicit subject it refers to.

Rules:
- Keep the SAME language as the latest message.
- Preserve the user's intent exactly — do not answer the question, do not add new asks.
- If the message is already self-contained, return it unchanged.
- Output ONLY the rewritten query, nothing else.`

	PlanSystemPrompt = `You are a research planning agent. Given the user's query, break it down into 2-5 distinct research aspects.
Each aspect should be a self-contained sub-question that can be investigated independently.

Respond with a JSON object with an "aspects" key, each aspect having:
- "id": a short slug
- "question": the specific question to investigate

Example:
{"aspects": [{"id": "policy_details", "question": "What are the specific PTO accrual rates?"}]}
`

	ResearchSystemPrompt = `You are a research sub-agent. Your job is to gather factual information about a specific aspect of a question.

The knowledge base is a KNOWLEDGE GRAPH. You have three tools:
- kg.schema: See the graph structure — node kinds + counts, attributes per kind, relation types, the document list (each with a TOC node). Call once to plan.
- kg.query: Search AND filter the graph. Pass several phrasings in ` + "`queries`" + ` (OR) when unsure of wording. Facts come back as VALUES (a node's url/email/number attribute), not snippets. Narrow with kind/attr/level/parent or an XPath-style ` + "`path`" + `.
- kg.read: Read a node by its stable ref — full text + breadcrumb (document › section › …) + semantic neighbors. Use AFTER query to read back the context chain.

IMPORTANT: query previews are abbreviated. Always kg.read the best hits to get full context before forming claims.

Strategy:
1. kg.schema once to see what's there, then kg.query for each fact (try several phrasings; filter by attr for exact values like a URL/email/count)
2. kg.read the top 2-3 hits in full (with breadcrumb + neighbors) before forming any claims
3. If a query yields poor results, try alternative phrasings or filter by kind/path
4. Only make claims directly supported by the graph — never extrapolate

When done, return a JSON memo:
{"aspect_id": "...", "claims": [{"text": "...", "supporting_chunk_ids": ["..."]}], "unresolved": []}

Mark a claim as unresolved if evidence is ambiguous or contradictory. An empty claims list with clear unresolved items is better than hallucinated claims.`

	ScopeCheckSystemPrompt = `You are a scope classifier for an assistant that only answers questions within its configured domain.
Classify the user's question:
- IN_SCOPE: it is about the assistant's domain.
- OUT_OF_SCOPE: it is unrelated to that domain (off-topic small talk, a different subject, etc.).
Reply with exactly one word: IN_SCOPE or OUT_OF_SCOPE.`

	ScopeCheckDefaultRefusal = `I can only help with questions within my area. Please ask me something in that scope.`

	SynthesizeSystemPrompt = `Synthesize the research memos below into a coherent, well-structured answer.
Merge overlapping claims, note contradictions, and clearly distinguish verified from unverified information.
Output ONLY the synthesized answer in clean markdown. Do not reveal tool use or internal reasoning.

RULES:
- Only include claims that have supporting_chunk_ids — drop unsupported claims entirely.
- If the question contains a false premise, explicitly state it and provide the correct information from the evidence.
- If all memos have empty claims, refuse to answer rather than guessing.
- Prefer a concise, correct answer over a comprehensive but speculative one.
- Do NOT use markdown tables. Present comparisons and structured data as short bullet lists instead.`

	SynthesizeSimpleSystemPrompt = `Answer the user's question using ONLY information found through the tools available to you.
Do not make up information. If the tools return no relevant results, explicitly say you cannot find the answer.
When you used a source, cite it using [chunk_id](source_ref) notation.

Strategy:
- Search for relevant material, then read the FULL content of the most promising results before answering — search results are often abbreviated snippets.
- For a specific named section or item, look it up directly rather than loading an entire document.
- If the first search misses, try a different query or a different search tool before giving up.

Critical rules:
- NEVER fabricate facts, numbers, dates, or names. Every claim must be supported by retrieved text.
- If the question contains a false premise (asks about something that doesn't exist or states something incorrect), say so and correct the premise using retrieved evidence.
- If searches return no relevant results, refuse to answer rather than guess.
- Prefer precision over recall: a short correct answer beats a long speculative one.
- Do NOT use markdown tables. Present comparisons and structured data as short bullet lists instead.`
)

// sig adapts lobes.SignalFor to the spec.SignalFn the authoring struct expects.
func sig(id string) spec.SignalFn {
	fn := lobes.SignalFor(id)
	return func(ctx map[string]any) map[string]float64 { return fn(ctx) }
}

// Classify is the B4 route lobe: the LLM router with a deterministic
// simple-shape skip (the inhibitory simple_shape signal lives in lobes).
var Classify = lobes.Lobe{
	ID: "classify", Name: "Classify", Behavior: "route",
	Layer: spec.LayerCognition, Order: 2, Prior: 0.0,
	Description:  "Route the turn: simple direct answer vs. multi-step research.",
	UseWhen:      "every answer-producing turn",
	SystemPrompt: ClassifySystemPrompt,
	Writes:       []string{"route"},
	Excites:      map[string]float64{"plan": 1.0, "synthesize": 1.0},
	Signals:      sig("classify"),
}

// Condense is the B4 rewrite lobe: anaphoric follow-ups become standalone
// retrieval queries. anaphora/short_query each clear the 0.5 threshold alone.
var Condense = lobes.Lobe{
	ID: "condense", Name: "Condense", Behavior: "rewrite",
	Layer: spec.LayerCognition, Order: 0, Prior: 0.0,
	Description:   "Rewrite an anaphoric follow-up into a standalone retrieval query.",
	UseWhen:       "a follow-up that references earlier turns",
	SystemPrompt:  CondenseSystemPrompt,
	Writes:        []string{"retrieval_query"},
	SignalWeights: map[string]float64{"anaphora": 0.6, "short_query": 0.6, "has_history": 0.0},
	Signals:       sig("condense"),
}

// Plan is the B4 decompose lobe: the complex-path entry point. Threshold 1.5
// encodes the conjunction (classify edge AND route="complex").
var Plan = lobes.Lobe{
	ID: "plan", Name: "Plan", Behavior: "decompose",
	Layer: spec.LayerCognition, Order: 3, Prior: 0.0, Threshold: 1.5,
	Description:  "Decompose a complex query into 2-5 research aspects.",
	UseWhen:      "a multi-step question that needs a plan",
	SystemPrompt: PlanSystemPrompt,
	Writes:       []string{"aspect"},
	Excites:      map[string]float64{"research": 1.0},
	Signals:      sig("plan"),
}

// Research is the B4 fanout lobe: per-aspect retrieval sub-agents. Edge-driven
// (runs iff plan ran); raw chunks stay confined to its receptive field.
var Research = lobes.Lobe{
	ID: "research", Name: "Research", Behavior: "fanout",
	Layer: spec.LayerCognition, Order: 4, Prior: 0.0,
	Description:  "Gather evidence per aspect from retrieval tools before answering.",
	UseWhen:      "the question needs external facts",
	SystemPrompt: ResearchSystemPrompt,
	Writes:       []string{"memo"},
	Excites:      map[string]float64{"synthesize": 1.0},
	Signals:      sig("research"),
}

// ScopeCheck is the B4 gate lobe: an LLM scope check that refuses out-of-domain
// questions before any work. Inert unless policy sets scope_gate.
var ScopeCheck = lobes.Lobe{
	ID: "scope_check", Name: "Scope Check", Behavior: "gate",
	Layer: spec.LayerCognition, Order: 1, Prior: 0.0,
	Description:  "Refuse out-of-scope queries up front, per the bot's scope policy.",
	UseWhen:      "the bot enables scope gating (policy.scope_gate)",
	SystemPrompt: ScopeCheckSystemPrompt,
	Writes:       []string{"scope_verdict"},
	Signals:      sig("scope_check"),
}

// Synthesize is the B4 compose lobe: the answer composer. Pinned on answer
// paths — any turn that reaches the network is answer-producing.
var Synthesize = lobes.Lobe{
	ID: "synthesize", Name: "Synthesize", Behavior: "compose",
	Layer: spec.LayerCognition, Order: 5, Prior: 1.0, Pinned: true,
	Description:  "Compose the grounded answer from research memos.",
	UseWhen:      "producing the answer",
	SystemPrompt: SynthesizeSystemPrompt,
	Writes:       []string{"draft_answer"},
	Signals:      sig("synthesize"),
}

// Lobes is the cognition domain's lobe set, in declaration order.
var Lobes = []lobes.Lobe{Condense, ScopeCheck, Classify, Plan, Research, Synthesize}

// FormatMemos renders the memo digest the synthesis call sees — aspect headers
// + claim bullets. memos are anything carrying AspectID + Claims (each Text).
// Mirrors synthesize.format_memos.
func FormatMemos(memos []Memo) string {
	out := ""
	for i, m := range memos {
		if i > 0 {
			out += "\n"
		}
		out += "## " + m.AspectID
		for _, c := range m.Claims {
			out += "\n- " + c.Text
		}
	}
	return out
}

// Memo is the per-aspect research result (the synthesize input shape).
type Memo struct {
	AspectID string
	Claims   []Claim
}

// Claim is one supported assertion within a memo.
type Claim struct {
	Text               string
	SupportingChunkIDs []string
}
