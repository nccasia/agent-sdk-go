// Resume the TDD port from rung 09 onward. Rungs 01-08 are already committed
// (see git log); the parity ledger is 77/93, build + tests + gofmt are clean.
//
// This driver is the same keep/revert ratchet the original port-sdk.js uses,
// narrowed to the remaining seven rungs plus the production-ready gateway.
// `MAX_WAVES = 3` per rung — after that, BLOCKED.md is written and we move on
// rather than letting a red rung poison the later ones.
export const meta = {
  name: 'port-sdk-resume',
  description: 'Resume the TDD port from rung 09 onward. Same keep/revert ratchet as port-sdk.js but narrowed to the remaining 7 rungs + the production-ready gateway.',
  phases: [
    { title: 'Ladder (09-15)' },
    { title: 'Gateway' },
  ],
}

const RUNGS = [
  '09-clients',
  '10-agent-facade',
  '11-serve-stateless',
  '12-plugins',
  '13-inspection-probe-bench-viewer',
  '14-benchmarks',
  '15-examples',
]

const MAX_WAVES = 3

// Absolute paths — spawned agents start at the session root, not the module dir.
const REPO = (args && args.repo) || '/Users/minh/Documents/mezon-agent-sdk/agent-sdk-go'
const PY = (args && args.python) || '/Users/minh/Documents/mezon-agent-sdk/agent-sdk'

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
  required: ['gofmtClean', 'vetClean', 'testsGreen', 'parity', 'benchesReady', 'examplesGreen'],
  properties: {
    gofmtClean: { type: 'boolean' },
    vetClean: { type: 'boolean' },
    testsGreen: { type: 'boolean' },
    parity: { type: 'string' },
    benchesReady: { type: 'boolean' },
    examplesGreen: { type: 'boolean' },
  },
}

function rungPrompt(rung, wave) {
  return [
    `You are porting the Python agent_sdk to Go. Work ONLY on ladder rung "${rung}" (wave ${wave}/${MAX_WAVES}).`,
    `Run EVERY command from the Go module root: prefix shell commands with \`cd ${REPO} && …\` (the module is github.com/mezon/agent-sdk-go).`,
    `The Python reference SDK is at ${PY} — read its modules and tests there.`,
    ``,
    `1. Read ${REPO}/tasks/${rung}/TASK.md — it lists the Python modules + the exact Python test files to translate, the public exports to expose (and check off in PARITY.md), the dep/deviation notes, and the "checks" commands.`,
    `2. STRICT TDD: for each named Python test file, write the equivalent Go *_test.go FIRST (translate test names: test_tight_adjust → TestTightAdjust; preserve assertions). Run them RED.`,
    `3. Implement ONLY this rung's Go package(s) until every "checks" command in the TASK.md exits 0. Do not modify other rungs' packages.`,
    `4. Keep \`gofmt -l .\` empty and \`go vet ./...\` clean for the packages you touched. Add testing.B micro-benchmarks for any hot path.`,
    `5. Check off the exports you implemented in PARITY.md ([ ] → [x]).`,
    `6. If — and only if — every check exits 0: \`git add -A && git commit\` with subject \`feat(${rung}): port <subsystem>\` and a \`Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>\` trailer.`,
    `7. If a check still fails after your best effort this wave, leave the tree as-is (do NOT commit a red rung) and report what's blocking.`,
    `Return the VERDICT object.`,
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
      `Rung ${rung} did not reach green after ${MAX_WAVES} waves. From ${REPO}: write a short tasks/${rung}/BLOCKED.md describing the failing checks and the root cause, then \`cd ${REPO} && git restore --staged . && git checkout -- . && git clean -fd agent_sdk benchmarks examples\` to discard uncommitted partial work for this rung while keeping prior rungs' commits and the new BLOCKED.md (commit BLOCKED.md first). Report VERDICT with committed=false.`,
      { schema: VERDICT, label: rung + ' revert', phase: 'Rung ' + rung },
    )
    log(`${rung}: BLOCKED after ${MAX_WAVES} waves — recorded and reverted; continuing.`)
  }
  journal.push({ rung, ...(verdict || { checksPass: false }) })
}

phase('Gateway')
log('Running the production-ready gateway: gofmt, vet, full test suite, parity, free benches, examples.')
const gateway = await agent(
  [
    `Run the production-ready parity gateway for the Go module at ${REPO}. Prefix every command with \`cd ${REPO} && …\` and report each result:`,
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
