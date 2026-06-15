---
id: 14-benchmarks
title: All 16 benchmarks + verdict/ratchet + free-gate
group: benchmarks
inputs:
  - agent_sdk/agent/
  - agent_sdk/bench/
  - agent_sdk/plugins/
outputs:
  - benchmarks/
  - cmd/bench/main.go
checks:
  - id: bench-tests
    cmd: go test ./benchmarks/...
    description: bench unit + Test<Name>Bench_Ready tests pass
  - id: free-gate
    cmd: go run ./cmd/bench
    description: free benches READY (exit 0)
mode: task
---
# Rung 14 — benchmarks

Port the benchmark/optimize ecosystem.

- `benchmarks/_shared/{verdict,improve,provider}.py` → `benchmarks` (verdict contract `{Status, Gates}`, `composeVerdict`/`Mode` (1 Python check = 1 Go Gate `mode.check`; any missing mode → UNMEASURED), `Ratchet` deterministic keep/revert, provider load = free/live split).
- Free (6): attentionbench, flowbench, corgictionbech, statelessbench, promptbench, toolbench(free tier). Live (6): agentbench, taskbench, extensionbench, skillbench, coding-agent-bench, delegationbench — deterministic tiers always run; `--live` tiers register only with a provider.
- `cmd/bench` registers all benches; exits non-zero unless every free bench READY. Port `ladder.sh`/`snapshot.py` → a Go sweep + `ci-free-gates` equivalent.

Each bench mirrors the Python `run.py` check ids exactly. Acceptance = cross-language parity: diff each Go `Gate.Name=="mode.check"` Pass vs Python `run.py` `ok`, and `Verdict.Status`. score_relevance floats gated by a 1e-6 golden fixture.

PARITY.md: (bench types power the gates; no `__all__` entries).
