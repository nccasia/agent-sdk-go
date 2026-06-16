// Resume the TDD port from rung 14 onward. Rungs 00-13 are already committed
// (see git log); the parity ledger is 115/116, build + tests + gofmt are clean.
//
// This driver is the same keep/revert ratchet the original port-sdk.js uses,
// narrowed to the two remaining rungs (14 benchmarks, 15 examples) plus the
// production-ready converger gateway (rung 16). `MAX_WAVES = 3` per rung —
// after that, BLOCKED.md is written and we move on rather than letting a red
// rung poison the later ones.
export const meta = {
  name: 'port-sdk-resume',
  description: 'Resume the TDD port from rung 14 onward (benchmarks + examples) and converge the production-ready parity gateway.',
  phases: [
    { title: 'Ladder (14-15)' },
    { title: 'Gateway' },
  ],
}

const RUNGS = [
  '14-benchmarks',
  '15-examples',
]

const MAX_WAVES = 3
const GATE_WAVES = 5

// Absolute paths — spawned agents start at the session root, not the module dir.
const REPO = (args && args.repo) || '/d/agent-sdk-go'
const PY = (args && args.python) || '/d/agent-sdk'

// The Go toolchain is installed in the home dir (not on the default PATH), so
// every spawned shell must export it. modernc.org/sqlite needs the module-mode
// flag. Prefix EVERY Go command with this.
const ENV = 'export PATH="$HOME/go-sdk/go/bin:$PATH" GOPATH="$HOME/go" GOFLAGS=-mod=mod'

const VERDICT = {
  type: 'object',
  additionalProperties: false,
  required: ['rung', 'checksPass', 'failing', 'committed', 'notes'],
  properties: {
    rung: { type: 'string' },
    checksPass: { type: 'boolean' },
    failing: { items: { type: 'string' }, type: 'array' },
    committed: { type: 'boolean' },
    notes: { type: 'string' },
  },
}

const GATEWAY = {
  type: 'object',
  additionalProperties: false,
  required: ['gofmtClean', 'vetClean', 'testsGreen', 'parity', 'parityFull', 'benchesReady', 'examplesGreen', 'notes'],
  properties: {
    gofmtClean: { type: 'boolean' },
    vetClean: { type: 'boolean' },
    testsGreen: { type: 'boolean' },
    parity: { type: 'string' },
    parityFull: { type: 'boolean' },
    benchesReady: { type: 'boolean' },
    examplesGreen: { type: 'boolean' },
    notes: { type: 'string' },
  },
}

function rungPrompt(rung, wave) {
  return [
    `You are porting the Python agent_sdk to Go. Work ONLY on ladder rung "${rung}" (wave ${wave}/${MAX_WAVES}).`,
    `Run EVERY Go command from the module root with the toolchain on PATH: prefix shell commands with \`cd ${REPO} && ${ENV} && …\` (the module is github.com/mezon/agent-sdk-go; Go lives at $HOME/go-sdk/go/bin).`,
    `The Python reference SDK is at ${PY} — read its modules and tests there (this is the source of truth; where Go and Python disagree, Python wins).`,
    ``,
    `1. Read ${REPO}/tasks/${rung}/TASK.md — it lists the Python modules + the exact Python files/tests to translate, the outputs to create, the dep/deviation notes, and the "checks" commands.`,
    `2. STRICT TDD: for each named Python test, write the equivalent Go *_test.go FIRST (translate names: test_tight_adjust → TestTightAdjust; preserve assertions). Run them RED.`,
    `3. Implement ONLY this rung's Go package(s)/outputs until every "checks" command in the TASK.md exits 0. Do not modify other rungs' packages.`,
    `4. Keep \`gofmt -l .\` empty and \`go vet ./...\` clean for the packages you touched. Use modernc.org/sqlite (pure-Go) for any real SQLite, FakeClient for offline-determinism.`,
    `5. Check off any exports you implemented in PARITY.md ([ ] → [x]).`,
    `6. If — and only if — every check exits 0: \`git add -A && git commit\` with subject \`feat(${rung}): port <subsystem>\` and a \`Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>\` trailer.`,
    `7. If a check still fails after your best effort this wave, leave the tree as-is (do NOT commit a red rung) and report what's blocking.`,
    `Return the VERDICT object.`,
  ].join('\n')
}

function measurePrompt() {
  return [
    `Measure the production-ready parity gateway for the Go module at ${REPO}. Prefix every command with \`cd ${REPO} && ${ENV} && …\` and report each result honestly (do NOT fix anything in this step):`,
    '- `gofmt -l .` (must print nothing) → gofmtClean',
    '- `go vet ./...` (clean) → vetClean',
    '- `go test ./... -race` (all green) → testsGreen',
    '- `go run ./cmd/parity` → parity (the "N/116" line); parityFull=true only when N==116 (all exports present, including tool_loop → engine.ToolLoop)',
    '- `go run ./cmd/bench` (exit 0, every free bench READY) → benchesReady',
    '- `go test ./examples/...` → examplesGreen',
    'Put a one-line summary of the worst failure in notes. Return the GATEWAY object.',
  ].join('\n')
}

function fixPrompt(g, wave) {
  return [
    `Production-ready parity gateway — converger wave ${wave}/${GATE_WAVES} for the Go module at ${REPO}. Prefix every command with \`cd ${REPO} && ${ENV} && …\`. The Python reference is at ${PY} (source of truth).`,
    `Current measurement: gofmt=${g.gofmtClean} vet=${g.vetClean} tests=${g.testsGreen} parity=${g.parity} (full=${g.parityFull}) benches=${g.benchesReady} examples=${g.examplesGreen}. Notes: ${g.notes || ''}`,
    `Pick the HIGHEST-VALUE failing check, diagnose the root cause, and apply the SMALLEST fix in the owning package. Order of priority: tests → parity → benches → examples → vet → gofmt.`,
    `For parity<116: the only pending export is \`tool_loop → engine.ToolLoop\`. Translate the Python \`tool_loop\` (agent_sdk/engine.py: the agentic tool loop with the forced tool-free final hop) into an exported \`engine.ToolLoop\`, add a test, and flip its PARITY.md box.`,
    `Run the affected checks to confirm the fix, keep gofmt/vet clean, then \`git add -A && git commit\` with a descriptive subject + the \`Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>\` trailer. Report the VERDICT object (rung="16-gateway").`,
  ].join('\n')
}

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
    await agent(
      `Rung ${rung} did not reach green after ${MAX_WAVES} waves. From the module root (\`cd ${REPO} && ${ENV}\`): write a short tasks/${rung}/BLOCKED.md describing the failing checks and the root cause, commit ONLY that file, then \`git restore --staged . && git checkout -- . && git clean -fd agent_sdk benchmarks examples cmd\` to discard uncommitted partial work for this rung while keeping prior rungs' commits. Report VERDICT with committed=false.`,
      { schema: VERDICT, label: rung + ' revert', phase: 'Rung ' + rung },
    )
    log(`${rung}: BLOCKED after ${MAX_WAVES} waves — recorded and reverted; continuing.`)
  }
  journal.push({ rung, ...(verdict || { checksPass: false }) })
}

phase('Gateway')
log('Converging the production-ready gateway: gofmt, vet, full -race suite, parity 116/116, free benches, examples.')
let gateway = null
for (let wave = 1; wave <= GATE_WAVES; wave++) {
  gateway = await agent(measurePrompt(), { schema: GATEWAY, label: 'gateway measure w' + wave, phase: 'Gateway' })
  const halt = gateway && gateway.testsGreen && gateway.parityFull && gateway.benchesReady
  if (halt) {
    log(`Gateway halt conditions met on wave ${wave} (tests + parity 116/116 + benches).`)
    break
  }
  if (wave < GATE_WAVES) {
    log(`Gateway wave ${wave}: ${gateway && gateway.notes || 'failing checks'} — applying smallest fix.`)
    await agent(fixPrompt(gateway, wave), { schema: VERDICT, label: 'gateway fix w' + wave, phase: 'Gateway' })
  }
}

return { journal, gateway }
