# Upstream sync ledger

agent-sdk-go is a port of the Python [`agent-sdk`](https://github.com/nccasia/agent-sdk) (the
**source of truth**). This file tracks exactly which upstream commits the Go port has absorbed, so
the two stay in sync over time.

| | |
|---|---|
| **Upstream repo** | https://github.com/nccasia/agent-sdk |
| **Full-port baseline** | `59feb07` (the rung-by-rung port through rung 16 was built against this tree) |
| **Synced to** | `e52e033` (`Merge PR #5 — chore/kg-tool-vocab`) |

## How sync is tracked

- Every Go commit that ports an upstream change carries an **`Upstream: <full-sha>`** trailer in its
  message. That trailer is the machine-checkable record (see [`scripts/check-upstream.sh`](../scripts/check-upstream.sh)).
- The table below is the human-readable log of upstream commits **after the baseline**.
- Merge commits and changes with no Go-relevant content are marked `n/a`.

## Procedure

```bash
# 1. point at a local clone of the Python SDK and see what's new
PYTHON_SDK=../agent-sdk ./scripts/check-upstream.sh      # lists PORTED / PENDING / n/a

# 2. for each PENDING commit: read its diff, translate to Go (tests first),
#    keep the suite green, and commit with the trailer:
#    Upstream: <full upstream sha>

# 3. update this file: advance "Synced to" and add a row below
```

A change is **ported** when the suite stays green: `gofmt -l .` empty · `go vet ./...` clean ·
`go test ./... -race` · `go run ./cmd/parity` 116/116 · `go run ./cmd/bench` free-gate green.

## Ported commits (since baseline `59feb07`)

| Upstream | Summary | Status | Go commit |
|---|---|---|---|
| `dccf939` | refactor(rag): `kg.*` tool vocabulary in retrieval prompts/allowlists | ✅ ported | `1f7ec09` |
| `e52e033` | Merge PR #5 (chore/kg-tool-vocab) | n/a (merge) | — |

### Notes
- `dccf939` — ported the research sub-agent prompt (`cognition`), the conversation `kb` tool group
  (`react`), and the `KB_LOOKUP` skill pack (`skills`) to `kg.schema`/`kg.query`/`kg.read`. The
  upstream `flows/stages/synthesize.py` relearn-allowlist hunk has **no Go equivalent** — the
  `OnboardingSynthesize` stage is not ported — so that hunk is n/a.
