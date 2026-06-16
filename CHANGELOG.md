# Changelog

All notable changes to **agent-sdk-go** are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project aims to follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html) from 1.0 onward.

agent-sdk-go is a Go port of the Python [agent-sdk](https://github.com/nccasia/agent-sdk); the
Python project is the source of truth for behavior. See [`docs/porting.md`](./docs/porting.md).

## [Unreleased]

### Added
- **Full PreAct engine in Go** — `PreactAgent` façade and the generic `Engine` kernel (lobes →
  stages → flows, deterministic activation/attention/flow resolution, the agentic tool loop with a
  forced tool-free final hop, metacognition `monitor → regulate`).
- **Public API at 100% parity** — all 116 `agent_sdk.__all__` exports are present in Go with
  matching signatures and tests (`go run ./cmd/parity`), including `engine.ToolLoop`.
- **Multi-provider clients** — `AnthropicClient`, `OpenAIClient`, `MiniMaxClient` (markup tool-call
  recovery), `MixedClient`, and a deterministic `FakeClient`, behind one `LlmCall` interface.
- **Sessions, memory & stateless serving** — `Session`/`SessionState`, scratchpad + semantic cache,
  in-memory / Redis / SQLite session stores (SQLite via pure-Go `modernc.org/sqlite`), and an
  `AgentWorker` pool for stateless multi-session serving.
- **Plugin/extension system** — `PluginRegistry` (register / override / enable / disable) plus the
  built-in plugins (Safety, Format, Task, Rag, Metacognition, …); an agent with no extra plugins is
  identical to the default network.
- **Skills, tools & MCP** — `@tool`-equivalent `tools.Tool`, progressive-disclosure skills, and MCP
  server mounting.
- **Inspection** — `probe.Probe` (one real turn, full trace), the `bench` harness, and the HTML
  `report`/`viewer` (per-turn path/flow/steps + composed system prompt & per-lobe provenance).
- **Benchmarks** — the verdict/ratchet/provider spine and all 12 bench suites behind a free-gate
  (`go run ./cmd/bench`); each free bench reproduces its Python source-of-truth verdict, and
  `cmd/bench` writes an inspectable viewer HTML per bench by default.
- **Runnable examples** — `examples/codingagent` and `examples/subagents-analytics`, both
  offline-deterministic via `FakeClient`.

### Changed
- **RAG tool vocabulary → `kg.*`** (synced from upstream `dccf939`): the research sub-agent prompt,
  the conversation `kb` tool group, and the `KB_LOOKUP` skill pack now use the knowledge-graph tools
  `kg.schema` / `kg.query` / `kg.read` instead of `semantic_search` / `keyword_search` / `read_chunk`
  / `kb.retrieve`.

### Upstream sync
- Synced to upstream `e52e033`. See [`docs/UPSTREAM.md`](./docs/UPSTREAM.md) for the per-commit
  ledger and `scripts/check-upstream.sh` to list unported upstream commits.

### Notes
- Built test-first as a rung-by-rung port (see `tasks/` and [`docs/porting.md`](./docs/porting.md)).
  The API may still shift before 1.0.
