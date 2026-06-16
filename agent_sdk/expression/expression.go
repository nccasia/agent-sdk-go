// Package expression is the reply-flow domain (B5). Core owns respond — the
// terminal response lobe that frames the next reply as a continuation of the
// conversation. Ported from agent_sdk/expression/lobes/respond.py.
package expression

import (
	"github.com/nccasia/agent-sdk-go/agent_sdk/core/spec"
	"github.com/nccasia/agent-sdk-go/agent_sdk/lobes"
)

// RespondSystemPrompt frames the terminal stage as a conversation continuation.
// Byte-identical to the Python SYSTEM_PROMPT.
const RespondSystemPrompt = "Write the next reply to the user's latest message, continuing this conversation. Use the " +
	"notes gathered this turn and the conversation so far (in the messages). Continue naturally — " +
	"do not restart, re-introduce yourself, or re-greet. Be concrete and direct."

// Respond is the B5 response lobe: render the next reply as a continuation.
// Pinned (always renders within its stage); contributes prompt only — no tools,
// no retrieval. The ground-or-refuse contract stays with cite/filter.
var Respond = lobes.Lobe{
	ID: "respond", Name: "Respond", Behavior: "compose",
	Layer: spec.LayerExpression, Order: 3, Pinned: true,
	Description:  "Render the next reply, continuing the conversation from the gathered notes.",
	UseWhen:      "the terminal response stage — composing the reply to the user's latest message",
	How:          "a single pass that frames the reply as a continuation using the turn's notes + transcript",
	SystemPrompt: RespondSystemPrompt,
	Activation:   func(map[string]any) float64 { return 1.0 },
}

// Lobes is the expression domain's lobe set.
var Lobes = []lobes.Lobe{Respond}
