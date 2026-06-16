# BLOCKED — toolchain / dep deviations

Items that the port would do differently given a different environment, but
that the project's current state forces into a fallback. Each entry records
the deviation, the blocking reason, and the future-change path.

## rung 11 — modernc.org/sqlite unavailable on Go 1.19

- **Deviation**: `agent_sdk/stores/sqlite` ships a stdlib-only, file-based
  JSON-blob store (one JSON blob per id) instead of the modernc.org/sqlite
  pure-Go SQLite driver.
- **Block**: the project is on Go 1.19.1; `modernc.org/sqlite` requires
  Go 1.21+. There is no C compiler set up in the build environment either,
  so the CGO sqlite driver is also unavailable.
- **Contract preserved**: one JSON blob per id, Load / Save / Append /
  Compact surface identical, in-memory + file-DSN semantics.
- **Path forward**: bump the toolchain to Go 1.21+ and swap the
  implementation to `modernc.org/sqlite` (the public API does not change).
