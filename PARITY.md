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
- [ ] Stage → engine.Stage
- [ ] StageRegistry → engine.StageRegistry
- [ ] stage → engine.Stage (builder)
- [ ] Flow → flows.Flow
- [ ] flow → flows.NewFlow
- [ ] Flows → preact.Flows
- [ ] Lobes → preact.Lobes
- [ ] Stages → preact.Stages
- [ ] Skill → skills.Skill
- [ ] Session → session.Session
- [ ] SessionState → session.SessionState
- [ ] Turn → session.Turn
- [ ] Memory → memory.Memory
- [ ] MemoryItem → memory.MemoryItem
- [ ] Scratchpad → memory.Scratchpad
- [ ] SemanticCache → memory.SemanticCache

## Shared context (rung 05–06)
- [ ] AgentContext → context.AgentContext
- [ ] Scope → context.Scope
- [ ] Evidence → context.Evidence
- [ ] current_context → context.Current
- [ ] bind_context → context.Bind
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
- [ ] AgentResult → result.AgentResult
- [ ] AgentStream → result.AgentStream
- [ ] Trace → result.Trace
- [ ] Usage → result.Usage
- [ ] Refusal → result.Refusal
- [ ] MemoryUpdate → result.MemoryUpdate
- [ ] Optimization → result.Optimization
- [ ] ActivationSnapshot → result.ActivationSnapshot
- [x] Citation → contracts.Citation
- [ ] RunStart → events.RunStart
- [ ] PathResolved → events.PathResolved
- [ ] StageStart → events.StageStart
- [ ] TextDelta → events.TextDelta
- [ ] ToolCall → events.ToolCall
- [ ] ToolResult → events.ToolResult
- [ ] CitationFound → events.CitationFound
- [ ] MetaAction → events.MetaAction
- [ ] StageEnd → events.StageEnd
- [ ] Final → events.Final

## Framework primitives — _blocks (rung 01–08)
- [ ] Lobe → lobes.Lobe
- [x] LobeSpec → core/spec.Lobe
- [ ] LobeRegistry → lobes.Registry
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
