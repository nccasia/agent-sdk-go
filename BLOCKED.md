# BLOCKED — toolchain / dep deviations

Items that the port would do differently given a different environment, but
that the project's current state forces into a fallback. Each entry records
the deviation, the blocking reason, and the future-change path.

_None — all prior deviations resolved._

## resolved

### rung 11 — modernc.org/sqlite (was: unavailable on Go 1.19)

- **Was**: `agent_sdk/stores/sqlite` shipped a stdlib-only JSON-blob fallback
  because the project was pinned to Go 1.19 and `modernc.org/sqlite` needs 1.21+.
- **Resolved**: toolchain bumped (`go.mod` → go 1.21+), store swapped to the
  pure-Go `modernc.org/sqlite` driver with the Python `sessions(id, state)`
  schema (one JSON blob per id). Public API unchanged; tests green.
