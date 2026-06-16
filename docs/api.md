# agent-sdk-go — public API

The Go public surface, package by package. It is at 100% parity with the Python
[`agent-sdk`](https://github.com/nccasia/agent-sdk) `__all__` ledger (116 exports — see
[`../PARITY.md`](../PARITY.md)). The conceptual model is identical to the Python reference; this
document is the Go binding. Import paths are under `github.com/nccasia/agent-sdk-go/agent_sdk/…`.

---

## 1. Quickstart

```go
import (
	"github.com/nccasia/agent-sdk-go/agent_sdk/agent"
	"github.com/nccasia/agent-sdk-go/agent_sdk/clients"
	"github.com/nccasia/agent-sdk-go/agent_sdk/tools"
)

search := tools.Tool("search",
	func(_ context.Context, in map[string]any) (any, error) { return "…", nil },
	tools.Desc("Search the knowledge base."),
	tools.Param("query", "string", true, nil),
)

a := agent.MustPreactAgent(agent.Config{
	Client:       clients.NewAnthropicClient("claude-opus-4-8"),
	Instructions: "You are a helpful research assistant.",
	Tools:        []any{search},
})

res, _ := a.Query(ctx, "What changed in v2?")          // one-shot → *result.AgentResult
for ev := range a.Act(ctx, "…").Iter() { _ = ev }      // streaming → typed events
```

A runnable offline version is [`examples/quickstart`](../examples/quickstart).

## 2. `PreactAgent` (`agent_sdk/agent`)

- `agent.NewPreactAgent(cfg Config) (*PreactAgent, error)` / `agent.MustPreactAgent(cfg Config) *PreactAgent`
- `Config` — `Client` (a `clients.LlmCall` or a model-name string), `Instructions`, `Tools`,
  `Lobes`/`Stages`/`Flows` (default to the built-in PreAct network when nil), `Skills`, `Plugins`,
  `Metacognition`, `Session`, `Memory`, `MCPServers`, `RequireCitations`, `Weights`, `Budgets`,
  `TZ`, `Lang`, … (see the struct doc-comment for the full set).

### Methods
- `Query(ctx, input) (*result.AgentResult, error)` · `QueryWithSession(ctx, input, sess)`
- `Act(ctx, input) *events.AgentStream` · `ActWithSession(ctx, input, sess)`
- `Inspect(query) result.ActivationSnapshot` · `InspectWithState(query, state)` — no-LLM routing probe
- `Spec() *spec.Spec` — the serializable config; rebuild with `agent.FromSpec(spec, client, tools…)`

## 3. Results (`agent_sdk/result`)

`AgentResult` — `Text`, `Status` (`"answered"`/`"refused"`), `Usage`, `Trace`, `Citations`,
`MemoryUpdates`, `Optimizations`, `Refusal`. `Usage` is the per-turn token + cost roll-up;
`ActivationSnapshot` is what `Inspect` returns.

## 4. Streaming & typed events (`agent_sdk/events`)

`agent.Act(ctx, q)` returns an `*AgentStream`; range over `.Iter()`. The event union:
`RunStart`, `PathResolved`, `StageStart`, `TextDelta`, `ToolCall`, `ToolResult`, `CitationFound`,
`MetaAction`, `StageEnd`, `Final`.

## 5. Building blocks

- **Lobes** (context axis) — `agent_sdk/lobes`, `activable.Activable`/`Layer`.
- **Stages** (execution units) — `engine.Stage`, `engine.NewStage` (builder), `engine.StageRegistry`.
- **Flows** (intent pipelines) — `flows.Flow`, `flows.NewFlow`; defaults via `preact.Flows`/`Lobes`/`Stages`.
- **Skills** (progressively disclosed) — `skills.Skill`, `skills.SkillRegistry`, `skills.SkillPack`.
- **Tools** — `tools.Tool(name, fn, opts…)` with `tools.Desc` / `tools.Param`; `FunctionToolRuntime`,
  `CompositeToolRuntime`. MCP via `mcp.MCPServerSpec` / `MCPToolRuntime` and `Config.MCPServers`.
- **Signals** — `compile_signal`/`eval_signal` equivalents in `agent_sdk/expression`.

## 6. Session & memory (`agent_sdk/session`, `memory`, `stores/*`)

- `session.Session`, `SessionState`, `Turn`; `SessionState` JSON-snapshots the whole turn state.
- Stores: `stores/memory` (default), `stores/redis`, `stores/sqlite` (pure-Go `modernc.org/sqlite`,
  one JSON blob per id). All implement the `SessionStore` Load/Save/Append/Compact contract.
- **Stateless serving** — `serve.AgentWorker` pool + queue/sink: any worker serves any session by id
  from the snapshot; `agent.FromSpec` rebuilds the (immutable) config.
- **Memory** — `memory.Memory`, `MemoryItem`, `Scratchpad`, `SemanticCache`; universal memory is on
  by default (the `memory` tool).

## 7. LLM clients (`agent_sdk/clients`)

`BaseClient`, `MakeClient` (string-shorthand resolver), `AnthropicClient`, `OpenAIClient`,
`MiniMaxClient` (markup tool-call recovery), `MixedClient` (route per stage/provider), `FakeClient`
(deterministic, no network). Messages/blocks: `Message`, `TextBlock`, `ToolUseBlock`, `ProviderUsage`.

## 8. Metacognition (`agent_sdk/metacognition`)

`Metacognition` supervises every turn (`monitor → regulate`): adjust the lobe slice, retry, or skip
a step — but never a pinned output-contract step (`filter`; `cite` when the RAG plugin is on).

## 9. Plugins (`agent_sdk/plugins`)

`Plugin`, `PluginRegistry` (`NewPluginRegistry`, register/override/enable/disable), `BuiltinRegistry`,
`DefaultCapabilityPlugins`, `AgentSetup`. Built-ins: `NewSafetyPlugin` (on), `NewFormatPlugin` (on),
`NewRagPlugin`, `NewTaskPlugin`, `NewMetacognitionPlugin`, `NewPluginSupportTriage`. Task primitives:
`Todo`, `TodoRail`, `TodosToolRuntime`.

## 10. Probe · inspect · benchmark (`agent_sdk/probe`, `bench`, `report`, `viewer`)

- `probe.Probe(ctx, agent, query, opts…) (*Record, error)` — one real turn, full trace (path, flow,
  per-stage `system_prompt`/`system_segments`, tools, usage). Captured by default (no env flag).
- `bench.Harness` / `Scenario` / `ScenarioResult` / `Report` — routing & behavior grading.
- `report.RenderHTML`/`WriteHTML` and `viewer.RenderHTML`/`Write` — single-file inspectable HTML.
- The benchmark runner (`benchmarks/`, `cmd/bench`) composes a `Verdict {Status, Gates}` per bench
  and writes an inspectable report per bench by default (`benchmarks/results/`).

---

For the conceptual deep dives (architecture, intent & paths, context funnel, memory, plugins,
metacognition), see the Python reference's
[`docs/concepts`](https://github.com/nccasia/agent-sdk/tree/main/docs/concepts) — the model is
identical. For how the Go surface maps to Python, see [`porting.md`](./porting.md).
