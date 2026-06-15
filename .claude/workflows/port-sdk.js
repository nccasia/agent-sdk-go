export const meta = {
  name: 'port-sdk',
  description: 'TDD-port the Python agent_sdk to Go, rung by rung, with a deterministic keep/revert ratchet, ending in a 100%-parity production-ready gateway. Each rung translates the named Python tests to Go (red), implements to green, and commits only when every check exits 0.',
  phases: [
    { title: 'Bootstrap' },
    { title: 'Ladder' },
    { title: 'Gateway' },
  ],
}

// Execution order = numeric folder prefixes under tasks/ (ls-alphabetical).
// Hardcoded here (mirrors optimize-bench.js BENCHES[]) since workflow scripts
// have no fs access; each rung's agent reads its own tasks/<rung>/TASK.md.
const RUNGS = [
  '01-contracts-core',
  '02-lobes-network',
  '03-events-result-session',
  '04-engine',
  '05-memory',
  '06-metacognition-cognition',
  '07-tools-mcp',
  '08-skills',
  '09-clients',
  '10-agent-facade',
  '11-serve-stateless',
  '12-plugins',
  '13-inspection-probe-bench-viewer',
  '14-benchmarks',
  '15-examples',
]

const MAX_WAVES = 3 // per rung, before recording BLOCKED and moving on

// Per-rung structured return — the model reports; the CLI/exit-codes decide.
const VERDICT = {
  type: 'object',
  additionalProperties: false,
  required: ['rung', 'checksPass', 'failing', 'committed', 'notes'],
  properties: {
    rung: { type: 'string' },
    checksPass: { type: 'boolean', description: 'true iff every check cmd in the TASK.md exited 0' },
    failing: { type: 'array', items: { type: 'string' }, description: 'check ids that did not pass' },
    committed: { type: 'boolean', description: 'true iff a git commit was made this wave' },
    notes: { type: 'string', description: 'one-line status / blocker summary' },
  },
}

const GATEWAY = {
  type: 'object',
  additionalProperties: false,
  required: ['gofmtClean', 'vetClean', 'testsGreen', 'parity', 'benchesReady', 'examplesGreen'],
  properties: {
    gofmtClean: { type: 'boolean' },
    vetClean: { type: 'boolean' },
    testsGreen: { type: 'boolean' },
    parity: { type: 'string', description: 'N/82 from `go run ./cmd/parity`' },
    benchesReady: { type: 'boolean', description: 'go run ./cmd/bench exits 0 (free benches READY)' },
    examplesGreen: { type: 'boolean' },
  },
}

function rungPrompt(rung, wave) {
  return [
    `You are porting the Python agent_sdk to Go. Work ONLY on ladder rung "${rung}" (wave ${wave}/${MAX_WAVES}).`,
    `Repo: the Go module is the current working directory (github.com/mezon/agent-sdk-go); the Python reference is ../agent-sdk.`,
    ``,
    `1. Read tasks/${rung}/TASK.md — it lists the Python modules + the exact Python test files to translate, the public exports to expose (and check off in PARITY.md), the dep/deviation notes, and the "checks" commands.`,
    `2. STRICT TDD: for each named Python test file, write the equivalent Go *_test.go FIRST (translate test names: test_tight_adjust → TestTightAdjust; preserve assertions). Run them RED.`,
    `3. Implement ONLY this rung's Go package(s) until every "checks" command in the TASK.md exits 0. Do not modify other rungs' packages.`,
    `4. Keep \`gofmt -l .\` empty and \`go vet ./...\` clean for the packages you touched. Add testing.B micro-benchmarks for any hot path.`,
    `5. Check off the exports you implemented in PARITY.md ([ ] → [x]).`,
    `6. If — and only if — every check exits 0: \`git add -A && git commit\` with subject \`feat(${rung}): port <subsystem>\` and a \`Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>\` trailer.`,
    `7. If a check still fails after your best effort this wave, leave the tree as-is (do NOT commit a red rung) and report what's blocking.`,
    `Return the VERDICT object.`,
  ].join('\n')
}

// ── Bootstrap ────────────────────────────────────────────────────────────────
phase('Bootstrap')
log('Verifying rung 00 scaffold (module, cmd/parity, cmd/bench, ladder, PARITY.md)…')
const boot = await agent(
  'Verify the rung 00 scaffold of the Go agent-sdk-go module is in place and builds: run `go build ./...` (must succeed), `go run ./cmd/parity` (prints N/82), and confirm tasks/ contains the numbered TASK.md ladder and .claude/workflows/port-sdk.js exists. If go.mod is missing, run `go mod init github.com/mezon/agent-sdk-go`. Report a one-line status as VERDICT with rung="00-bootstrap".',
  { schema: VERDICT, label: 'bootstrap', phase: 'Bootstrap' },
)
if (boot && boot.checksPass === false) {
  log('Bootstrap not green — fix rung 00 before walking the ladder.')
}

// ── Ladder ───────────────────────────────────────────────────────────────────
const journal = []
for (const rung of RUNGS) {
  phase('Rung ' + rung)
  let verdict = null
  for (let wave = 1; wave <= MAX_WAVES; wave++) {
    verdict = await agent(rungPrompt(rung, wave), { schema: VERDICT, label: rung + ' w' + wave, phase: 'Rung ' + rung })
    if (verdict && verdict.checksPass) break
    if (wave < MAX_WAVES) log(`${rung}: wave ${wave} red (${(verdict && verdict.failing || []).join(', ')}) — retrying`)
  }
  if (!verdict || !verdict.checksPass) {
    // Deterministic revert: do not let a red rung poison later rungs.
    await agent(
      `Rung ${rung} did not reach green after ${MAX_WAVES} waves. Write a short tasks/${rung}/BLOCKED.md describing the failing checks and the root cause, then \`git restore .\` and \`git checkout -- .\` to discard any uncommitted partial work for this rung (keep prior rungs' commits intact). Report VERDICT with committed=false.`,
      { schema: VERDICT, label: rung + ' revert', phase: 'Rung ' + rung },
    )
    log(`${rung}: BLOCKED after ${MAX_WAVES} waves — recorded and reverted; continuing.`)
  }
  journal.push({ rung, ...(verdict || { checksPass: false }) })
}

// ── Gateway ──────────────────────────────────────────────────────────────────
phase('Gateway')
log('Running the production-ready gateway: gofmt, vet, full test suite, parity, free benches, examples.')
const gateway = await agent(
  [
    'Run the production-ready parity gateway for the Go agent-sdk-go module and report each result:',
    '- `gofmt -l .` (must print nothing) → gofmtClean',
    '- `go vet ./...` (clean) → vetClean',
    '- `go test ./... -race` (all green) → testsGreen',
    '- `go run ./cmd/parity` → parity ("N/82"), 100% required',
    '- `go run ./cmd/bench` (exit 0, free benches READY) → benchesReady',
    '- `go test ./examples/...` → examplesGreen',
    'Do not fix anything here — just measure and report the GATEWAY object honestly.',
  ].join('\n'),
  { schema: GATEWAY, label: 'gateway', phase: 'Gateway' },
)

return { journal, gateway }
