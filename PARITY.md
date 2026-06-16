# API parity ledger — `agent_sdk.__all__` (116 exports)

The 100%-API-parity contract. Each line maps a Python public export to its Go
equivalent. A rung checks `[x]` an export once it exists in Go with a matching
signature and a passing test. `cmd/parity` parses this file and exits non-zero
while any box is unchecked (the production-ready gateway gate).

Format: `- [ ] PythonName → go/import/path.GoName` (rungs flip `[ ]`→`[x]`).

## Façade (rung 01–10)
- [x] PreactAgent → agent.PreactAgent
- [x] Engine → engine.Engine (runnable façade engine)
- [x] Activable → activable.Activable
- [x] Layer → activable.Layer
- [x] Stage → engine.Stage
- [x] StageRegistry → engine.StageRegistry
- [x] stage → engine.NewStage (builder)
- [x] Flow → flows.Flow
- [x] flow → flows.NewFlow
- [x] Flows → preact.Flows
- [x] Lobes → preact.Lobes
- [x] Stages → preact.Stages
- [x] Skill → skills.Skill
- [x] Session → session.Session
- [x] SessionState → session.SessionState
- [x] Turn → session.Turn
- [x] Memory → memory.Memory
- [x] MemoryItem → memory.MemoryItem
- [x] Scratchpad → memory.Scratchpad
- [x] SemanticCache → memory.SemanticCache

## Shared context (rung 05–06)
- [x] AgentContext → context.AgentContext
- [x] Scope → context.Scope
- [x] Evidence → context.Evidence
- [x] current_context → context.Current
- [x] bind_context → context.Bind
- [x] Metacognition → metacognition.Metacognition

## Tools / signals (rung 01,07)
- [x] tool → tools.Tool (builder)
- [x] Tool → tools.ToolDef
- [x] FunctionToolRuntime → tools.FunctionToolRuntime
- [x] MCPToolRuntime → mcp.MCPToolRuntime
- [x] MCPServerSpec → mcp.ServerSpec
- [x] MCPError → mcp.Error
- [x] compile_signal → core/signal.Compile
- [x] eval_signal → core/signal.Eval

## Benchmark / probe / report (rung 13)
- [x] Harness → bench.Harness
- [x] Scenario → bench.Scenario
- [x] ScenarioResult → bench.ScenarioResult
- [x] Report → bench.Report
- [x] probe → probe.Probe
- [x] ProbeRecord → probe.Record
- [x] DocWriteGuard → react.DocWriteGuard
- [x] DocGroundingGuard → react.DocGroundingGuard
- [x] render_html → report.RenderHTML
- [x] write_html → report.WriteHTML
- [x] render_viewer_html → viewer.RenderHTML
- [x] write_viewer → viewer.Write
- [x] to_viewer_record → viewer.ToRecord

## Results + events (rung 03)
- [x] AgentResult → result.AgentResult
- [x] AgentStream → result.AgentStream
- [x] Trace → result.Trace
- [x] Usage → result.Usage
- [x] Refusal → result.Refusal
- [x] MemoryUpdate → result.MemoryUpdate
- [x] Optimization → result.Optimization
- [x] ActivationSnapshot → result.ActivationSnapshot
- [x] Citation → contracts.Citation
- [x] RunStart → events.RunStart
- [x] PathResolved → events.PathResolved
- [x] StageStart → events.StageStart
- [x] TextDelta → events.TextDelta
- [x] ToolCall → events.ToolCall
- [x] ToolResult → events.ToolResult
- [x] CitationFound → events.CitationFound
- [x] MetaAction → events.MetaAction
- [x] StageEnd → events.StageEnd
- [x] Final → events.Final

## Framework primitives — _blocks (rung 01–08)
- [x] Lobe → lobes.Lobe
- [x] LobeSpec → core/spec.Lobe
- [x] LobeRegistry → lobes.Registry
- [x] PathSpec → core/spec.Path
- [x] Blackboard → core/attention.Blackboard
- [x] ContextNode → core/attention.ContextNode
- [x] build_attention → core/attention.Build
- [x] propagate → core/activate.Propagate
- [x] recognize_paths → core/feature.RecognizePaths
- [x] resolve_path → core/activate.ResolvePath
- [x] validate_network → core/spec.ValidateNetwork
- [x] tool_loop → engine.ToolLoop
- [x] LlmCall → contracts.LlmCall
- [x] LobeServices → contracts.LobeServices
- [x] TurnContext → contracts.TurnContext
- [x] ToolRuntime → contracts.ToolRuntime
- [x] CompositeToolRuntime → tools.CompositeToolRuntime
- [x] SkillRegistry → skills.Registry
- [x] SkillPack → skills.Pack
- [x] build_skill_prompt_block → skills.BuildPromptBlock
- [x] Claim → contracts.Claim
- [x] Memo → contracts.Memo
- [x] FinalEnvelope → contracts.FinalEnvelope
- [x] PINNED_LOBES → core/spec.PinnedLobes

## LLM clients (rung 09)
- [x] BaseClient → clients.BaseClient
- [x] make_client → clients.MakeClient
- [x] AnthropicClient → clients.AnthropicClient
- [x] MiniMaxClient → clients.MiniMaxClient
- [x] OpenAIClient → clients.OpenAIClient
- [x] MixedClient → clients.MixedClient
- [x] FakeClient → clients.FakeClient
- [x] Message → clients.Message
- [x] ProviderUsage → clients.ProviderUsage
- [x] TextBlock → clients.TextBlock
- [x] ToolUseBlock → clients.ToolUseBlock

## Plugins (rung 12)
- [x] Plugin → agent.Plugin
- [x] AgentSetup → agent.AgentSetup
- [x] PluginRegistry → agent.PluginRegistry
- [x] builtin_registry → plugins.BuiltinRegistry
- [x] default_capability_plugins → plugins.DefaultCapabilityPlugins
- [x] capability_lobes → plugins.CapabilityLobes
- [x] SafetyPlugin → plugins.NewSafetyPlugin
- [x] FormatPlugin → plugins.NewFormatPlugin
- [x] TaskPlugin → plugins.NewTaskPlugin
- [x] RagPlugin → plugins.NewRagPlugin
- [x] MetacognitionPlugin → plugins.NewMetacognitionPlugin
- [x] PluginSupportTriage → plugins.NewPluginSupportTriage
- [x] Todo → tasks.Todo
- [x] TodoRail → tasks.TodoRail
- [x] TodosToolRuntime → tasks.TodosToolRuntime

## Benchmarks / verdict / ratchet (rung 14)
Bench types power the gates (no `__all__` entries in the Python `_shared`).
These rows track bench-port progress only; `cmd/parity` stops at this heading and
does NOT count them toward the 116-export gate.
- [x] compose_verdict → benchmarks.ComposeVerdict (+ Verdict{Status, Gates})
- [x] _payload → benchmarks.NewPayload (ModePayload{Checks, AllPass, Metrics})
- [x] verdict_summary → benchmarks.VerdictSummary (Summary{Status, GatesPass, GatesTotal})
- [x] delta_gate → benchmarks.DeltaGate (deterministic keep/revert ratchet)
- [x] load_provider → benchmarks.LoadProvider / LoadProviderFrom (free/live split)
- [x] bench registry + free-gate → benchmarks.Registry / FreeGate, cmd/bench
- [x] attentionbench → benchmarks.RunAttentionBench (select/recall/grounding/reply/determinism)
- [x] corgictionbech → benchmarks.RunCorgictionBench (monitor/regulate/pinned/channel/plugin_surface/plan_compile)
- [x] toolbench (free) → benchmarks.RunToolBench (spec/select/composite)
- [ ] flowbench → benchmarks (needs engine.inspect-with-state for clarify routing)
- [ ] statelessbench → benchmarks (needs agent_from_spec + AgentWorker pool semantics)
- [ ] promptbench → benchmarks (needs trace system_segments + authored prompt constants)
- [ ] live benches (agent/task/extension/skill/coding-agent/delegation) → benchmarks
