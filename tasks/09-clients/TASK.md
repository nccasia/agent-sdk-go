---
id: 09-clients
title: LLM clients (anthropic, openai, minimax, mixed, fake)
group: domains
inputs:
  - agent_sdk/contracts/
outputs:
  - agent_sdk/clients/
checks:
  - id: clients-tests
    cmd: go test ./agent_sdk/clients/...
    description: client tests pass
mode: task
---
# Rung 09 — clients

Python → Go (`agent_sdk/clients/` → `clients`):
- `base.py` (`make_client`), `anthropic_client.py`, `openai_client.py`, `minimax_client.py`, `mixed.py` (MixedClient per-stage routing), `fake.py` (deterministic FakeClient — scripted replies for offline tests), `messages.py` (ProviderUsage).
- Pragmatic deps: real `net/http` clients (or official Go SDKs behind the `LlmCall` interface). MiniMax via the Anthropic-compatible endpoint (`ANTHROPIC_BASE_URL`). Streaming supported.

Translate: `tests/test_clients.py` (FakeClient scripting, MixedClient routing, shorthand). Keep network calls behind the interface so tests stay offline (FakeClient).

PARITY.md: (clients are constructors, not in `__all__` directly — but FakeClient/MixedClient power many later tests).
