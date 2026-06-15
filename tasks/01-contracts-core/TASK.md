---
id: 01-contracts-core
title: Core spec, signals, contracts, activable, activation + attention
group: core
inputs:
  - go.mod
  - PARITY.md
outputs:
  - agent_sdk/core/spec/
  - agent_sdk/core/signal/
  - agent_sdk/core/activate/
  - agent_sdk/core/attention/
  - agent_sdk/contracts/
  - agent_sdk/activable/
checks:
  - id: core-tests
    cmd: go test ./agent_sdk/core/... ./agent_sdk/contracts/... ./agent_sdk/activable/...
    description: ported core tests pass
  - id: vet
    cmd: go vet ./agent_sdk/core/... ./agent_sdk/contracts/... ./agent_sdk/activable/...
    description: vet clean
mode: task
---
# Rung 01 — contracts + core

Port the deterministic core (pure functions of `(spec, context)` — no I/O, no LLM,
no clock, no randomness; sorted iteration; round emitted floats).

Python → Go:
- `agent_sdk/spec.py` → `core/spec` (Spec/Lobe/Stage/Flow/Path, `ValidateNetwork`, `PinnedLobes={cite,filter}`, to_json/from_json, SPEC_VERSION).
- `agent_sdk/signals.py` → `core/signal` (`Compile`/`Eval`: const/flag/ctx/lexical/min_words/regex/all/any/not/scale/sum → float in [0,1]; banker's rounding `Round4`).
- `agent_sdk/network/activation.py` → `core/activate` (`Propagate`, `merge_lobe_weights`, `ResolvePath`/`RecognizePaths` trace).
- `agent_sdk/network/context_builder.py` → `core/attention` (`Build`/`build_attention`, `ScoreText`==`score_relevance` L1+L2, `Blackboard`, `ContextNode`, `DefaultNodeWeights`).
- `agent_sdk/contracts/{llm,memo,pins,services,tools,turn}.py` → `contracts` (LlmCall, LobeServices, TurnContext, ToolRuntime, Claim/Memo/FinalEnvelope/Citation, PINNED_LOBES).
- `agent_sdk/activable.py` → `activable` (Activable, Layer + 6 layers).

Translate these Python test files to Go `*_test.go` (red first):
`tests/test_spec.py`, `tests/test_signals.py`, `tests/test_blocks_smoke.py`, `tests/test_capability_resolver.py`.

Numeric parity: `ScoreText` must reproduce Python `score_relevance` (text-only: rel=1.0, disjoint=0.0). Gate floats at 1e-6 vs a golden fixture exported from Python.

PARITY.md exports to check off: compile_signal, eval_signal, propagate, resolve_path, recognize_paths, validate_network, build_attention, Blackboard, ContextNode, LobeSpec, PathSpec, LlmCall, LobeServices, TurnContext, ToolRuntime, Claim, Memo, FinalEnvelope, Citation, PINNED_LOBES, Activable, Layer.
