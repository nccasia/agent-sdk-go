# Documentation

**agent-sdk-go** is a Go port of the Python [`agent-sdk` (PreAct)](https://github.com/nccasia/agent-sdk).

- [`api.md`](./api.md) — the Go public surface, package by package (100% parity with `agent_sdk.__all__`).
- [`porting.md`](./porting.md) — how the Go port maps to the Python reference, the package map, naming
  conventions, dependency policy, and conformance.
- [`../PARITY.md`](../PARITY.md) — the `agent_sdk.__all__` → Go export ledger (the parity gate).
- [`UPSTREAM.md`](./UPSTREAM.md) — the upstream-sync ledger: which Python commits the port has absorbed
  (with `scripts/check-upstream.sh` to list unported ones).
- [`../CHANGELOG.md`](../CHANGELOG.md) · [`../CONTRIBUTING.md`](../CONTRIBUTING.md)

## The conceptual model

PreAct's model — lobes (context) × stages/flows (time), with metacognition supervising and a
context funnel — is identical to the Python reference. Read it there (numbered in reading order):

- [01 · architecture (the OX/OY plane)](https://github.com/nccasia/agent-sdk/blob/main/docs/concepts/01-architecture.md)
- [02 · intent & paths](https://github.com/nccasia/agent-sdk/blob/main/docs/concepts/02-intent-and-paths.md)
- [03 · reply flow](https://github.com/nccasia/agent-sdk/blob/main/docs/concepts/03-reply-flow.md)
- [04 · context management (the funnel)](https://github.com/nccasia/agent-sdk/blob/main/docs/concepts/04-react-context-management.md)
- [06 · universal memory](https://github.com/nccasia/agent-sdk/blob/main/docs/concepts/06-universal-memory.md)
- [10 · plugins (core vs. extensions)](https://github.com/nccasia/agent-sdk/blob/main/docs/concepts/10-plugins.md)
- [11 · metacognition](https://github.com/nccasia/agent-sdk/blob/main/docs/concepts/11-metacognition.md)
- [16 · stateless serving](https://github.com/nccasia/agent-sdk/blob/main/docs/concepts/16-stateless-serving.md)

## Runnable references

- [`../examples/quickstart`](../examples/quickstart) — the README quickstart, offline.
- [`../examples/codingagent`](../examples/codingagent) — multi-stage coding agent over a real FS.
- [`../examples/subagents-analytics`](../examples/subagents-analytics) — plan → fan-out → fan-in SQL agent.
