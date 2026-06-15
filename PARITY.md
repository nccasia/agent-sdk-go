# API parity ledger — `agent_sdk.__all__` (82 exports)

The 100%-API-parity contract. Each line maps a Python public export to its Go
equivalent. A rung checks `[x]` an export once it exists in Go with a matching
signature and a passing test. `cmd/parity` parses this file and exits non-zero
while any box is unchecked (the production-ready gateway gate).

Format: `- [ ] PythonName → go/import/path.GoName` (rungs flip `[ ]`→`[x]`).

## Façade (rung 01–10)
- [ ] PreactAgent → agent.PreactAgent
- [ ] Engine → engine.Engine
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
- [ ] Skill → skills.Skill
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
- [ ] Metacognition → metacognition.Metacognition

## Tools / signals (rung 01,07)
- [ ] tool → tools.Tool (builder)
- [ ] Tool → tools.ToolDef
- [ ] FunctionToolRuntime → tools.FunctionToolRuntime
- [ ] MCPToolRuntime → mcp.MCPToolRuntime
- [ ] MCPServerSpec → mcp.ServerSpec
- [ ] MCPError → mcp.Error
- [x] compile_signal → core/signal.Compile
- [x] eval_signal → core/signal.Eval

## Benchmark / probe / report (rung 13)
- [ ] Harness → bench.Harness
- [ ] Scenario → bench.Scenario
- [ ] ScenarioResult → bench.ScenarioResult
- [ ] Report → bench.Report
- [ ] probe → probe.Probe
- [ ] ProbeRecord → probe.Record
- [ ] DocWriteGuard → react.DocWriteGuard
- [ ] DocGroundingGuard → react.DocGroundingGuard
- [ ] render_html → report.RenderHTML
- [ ] write_html → report.WriteHTML
- [ ] render_viewer_html → viewer.RenderHTML
- [ ] write_viewer → viewer.Write
- [ ] to_viewer_record → viewer.ToRecord

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
- [ ] tool_loop → engine.ToolLoop
- [x] LlmCall → contracts.LlmCall
- [x] LobeServices → contracts.LobeServices
- [x] TurnContext → contracts.TurnContext
- [x] ToolRuntime → contracts.ToolRuntime
- [ ] CompositeToolRuntime → tools.CompositeToolRuntime
- [ ] SkillRegistry → skills.Registry
- [ ] SkillPack → skills.Pack
- [ ] build_skill_prompt_block → skills.BuildPromptBlock
- [x] Claim → contracts.Claim
- [x] Memo → contracts.Memo
- [x] FinalEnvelope → contracts.FinalEnvelope
- [x] PINNED_LOBES → core/spec.PinnedLobes
