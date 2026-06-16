// Rung-14 completion: bring the Go benchmark registry to full parity with the
// Python benchmarks/ suite. The first rung-14 pass only registered 3 benches
// (attention/corgiction/tool) and mis-tiered toolbench; this driver ports the
// remaining deterministic-floor (free) benches + the live-only benches, mirrors
// each bench's verdict from its Python run.py, and gates the free ones.
//
// Sequential by design: every bench lives in the one `benchmarks` Go package and
// registers into the shared DefaultRegistry, so the package must stay compiling
// between benches — one agent per bench, each greens `go test ./benchmarks/...`
// + `go run ./cmd/bench` and commits before the next starts.
export const meta = {
  name: 'port-sdk-benches',
  description: 'Complete rung-14: port all remaining benchmark suites to full Python verdict parity and gate the free tier.',
  phases: [
    { title: 'Benches' },
    { title: 'Free-gate' },
  ],
}

const REPO = (args && args.repo) || '/d/agent-sdk-go'
const PY = (args && args.python) || '/d/agent-sdk'
const ENV = 'export PATH="$HOME/go-sdk/go/bin:$PATH" GOPATH="$HOME/go" GOFLAGS=-mod=mod'

// Per the Python source of truth (benchmarks/*/run.py + verdicts/ + ci-free-gates.sh):
// free benches have a deterministic FakeClient floor (no provider) and must be
// gated; live-only benches return UNMEASURED without a provider and are NOT gated.
// Each entry: {name, tier, status, note}. `status` is the EXPECTED deterministic
// verdict status (the agent must confirm it against the Python verdicts/ dir).
const BENCHES = [
  { name: 'statelessbench', tier: 'Free', status: 'READY',
    note: 'THE ci-free-gates.sh bench. Modes: snapshot, store (SessionStoreSQL via modernc.org/sqlite), isolation (AgentWorker pool), spec, schema. Fully deterministic via FakeClient.' },
  { name: 'flowbench', tier: 'Free', status: 'READY',
    note: 'Modes: routing, tiers, states, grounding, coverage, determinism, subject, execution — one scenario per flow. Deterministic floor.' },
  { name: 'promptbench', tier: 'Free', status: 'READY',
    note: 'Free tiers: structure (identity/dedup/ordering/coverage) + quality (5 rule-based checks). The live `judge` tier is provider-gated — register it live, UNMEASURED without provider.' },
  { name: 'toolbench', tier: 'Free', status: 'READY',
    note: 'FIX existing mis-tier (currently Live). Free modes: spec, select, composite. The live `loop` tier is provider-gated. So toolbench is Free with a provider-gated live tier.' },
  { name: 'corgictionbech', tier: 'Free', status: 'READY',
    note: 'Already present — VERIFY only. Free modes monitor/regulate/pinned/channel/plugin_surface/plan_compile; live tier provider-gated.' },
  { name: 'attentionbench', tier: 'Free', status: 'VERIFY',
    note: 'Already present — VERIFY the ExpectStatus against benchmarks/attentionbench/verdicts/*.json (the first pass guessed NOT_READY; confirm or correct). Modes select/recall/grounding/reply/determinism.' },
  { name: 'codingagentbench', tier: 'Free', status: 'READY',
    note: 'coding-agent-bench has a FREE replay tier (mode `understand`: routed/used_tools/wrote_doc/answered/bounded/grounded). Port the replay floor as Free; live tier provider-gated.' },
  { name: 'agentbench', tier: 'Live', status: 'UNMEASURED',
    note: 'LIVE-only. Single payload `agentbench` (mission.* + hard.* checks). Without provider → UNMEASURED (Python exits 2; Go: register Live, deterministic floor returns UNMEASURED).' },
  { name: 'taskbench', tier: 'Live', status: 'UNMEASURED',
    note: 'LIVE-only. One mode per capability; per-task answered/answer_correct/bounded. UNMEASURED without provider.' },
  { name: 'extensionbench', tier: 'Live', status: 'UNMEASURED',
    note: 'LIVE-only. Single payload `extension`: plugin.* + mcp.* plug/unplug checks. UNMEASURED without provider.' },
  { name: 'skillbench', tier: 'Live', status: 'UNMEASURED',
    note: 'LIVE-only. Modes lint/parse/mapping/activation/follow/funnel; precision/recall thresholds 0.8. UNMEASURED without provider.' },
  { name: 'delegationbench', tier: 'Live', status: 'UNMEASURED',
    note: 'LIVE-only. Payload `live` (None without provider → UNMEASURED). decision/exec/fanin checks; precision≥0.8 recall≥0.7 exec≥0.7 fidelity≥0.7.' },
]

const VERDICT = {
  type: 'object',
  additionalProperties: false,
  required: ['bench', 'checksPass', 'status', 'committed', 'notes'],
  properties: {
    bench: { type: 'string' },
    checksPass: { type: 'boolean' },
    status: { type: 'string' },
    committed: { type: 'boolean' },
    notes: { type: 'string' },
  },
}

function benchPrompt(b) {
  return [
    `You are completing the Go port of the Python agent_sdk benchmark suite. Port/verify ONLY the "${b.name}" benchmark this step.`,
    `Run every Go command from the module root with the toolchain on PATH: prefix with \`cd ${REPO} && ${ENV} && …\` (module github.com/mezon/agent-sdk-go).`,
    `Python source of truth: ${PY}/benchmarks/${b.name === 'codingagentbench' ? 'coding-agent-bench' : b.name}/ — read its run.py, verdicts/*.json, METHOD.md, scoring.py. Where Go and Python disagree, Python wins.`,
    ``,
    `Target tier: ${b.tier}. Expected deterministic (no-provider) verdict status: ${b.status}.`,
    `Notes: ${b.note}`,
    ``,
    `Study the EXISTING Go bench infrastructure first: ${REPO}/benchmarks/verdict.go (ComposeVerdict / Status / Gate), registry.go (Bench struct {Name,Tier,Run,ExpectStatus}, DefaultRegistry, Free/FreeGate), and an existing bench like corgictionbench.go for the RunXBench signature + helper style.`,
    `1. Implement benchmarks/${b.name}.go: a RunXBench that builds the SAME modes and check-ids as the Python run.py and composes a verdict via the shared ComposeVerdict. The deterministic floor must run with NO provider (FakeClient); a provider-gated tier (if any) registers separately as live and returns UNMEASURED without a provider. ${b.status === 'VERIFY' ? 'This bench already exists — only correct its ExpectStatus/tier if the Python verdicts/ disagree; otherwise leave the impl and just confirm.' : ''}`,
    `2. Add benchmarks/${b.name}_test.go with Test<Name>Bench_Ready asserting the deterministic floor verdict status == ${b.status === 'VERIFY' ? 'the value you confirmed from verdicts/' : '"' + b.status + '"'}, plus a check-id parity assertion (the Go Gate names match the Python check ids).`,
    `3. Register it in benchmarks/registry.go DefaultRegistry: \`r.Register(Bench{Name:"${b.name}", Tier:${b.tier}, Run:RunXBench, ExpectStatus:"<confirmed>"})\`. Namespace any new helper funcs by the bench prefix to avoid collisions in the shared package.`,
    `4. Make these exit 0 and keep the package compiling: \`go test ./benchmarks/...\`, \`go run ./cmd/bench\` (free-gate must stay green — ${b.tier === 'Free' ? 'your free bench is now gated' : 'your live bench must NOT break the free-gate (UNMEASURED is not gated)'}), \`gofmt -l .\` empty, \`go vet ./...\` clean.`,
    `5. If — and only if — all checks exit 0: \`git add -A && git commit\` subject \`feat(14-benchmarks): port ${b.name} to verdict parity\` with a \`Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>\` trailer.`,
    `If a check fails after your best effort, leave the tree as-is (do NOT commit red) and report what blocks. Return the VERDICT object.`,
  ].join('\n')
}

phase('Benches')
const journal = []
for (const b of BENCHES) {
  const v = await agent(benchPrompt(b), { schema: VERDICT, label: 'bench ' + b.name, phase: 'Benches' })
  journal.push({ bench: b.name, ...(v || { checksPass: false }) })
  if (v && v.checksPass) log(`${b.name}: ${v.status} — ${v.committed ? 'committed' : 'no commit'}`)
  else log(`${b.name}: NOT green (${v && v.notes || 'no verdict'}) — continuing; will revisit in free-gate.`)
}

phase('Free-gate')
const gate = await agent(
  [
    `Final rung-14 free-gate verification for the Go module at ${REPO}. Prefix every command with \`cd ${REPO} && ${ENV} && …\`.`,
    `1. \`go run ./cmd/bench\` — every FREE bench (statelessbench, flowbench, promptbench, toolbench, corgictionbech, attentionbench, codingagentbench) must appear and the free-gate must print green / exit 0. Live benches must be UNMEASURED (not gated).`,
    `2. \`go test ./benchmarks/... -race\`, \`gofmt -l .\` (empty), \`go vet ./...\` (clean).`,
    `3. Cross-language sanity: for 2-3 free benches, diff a Go Gate Pass set vs the Python run.py "ok" for the same check-ids and confirm Verdict.Status matches the Python verdicts/ baseline.`,
    `If anything is red, apply the smallest fix in the owning bench file, keep gofmt/vet clean, and commit it. Report the VERDICT object (bench="free-gate").`,
  ].join('\n'),
  { schema: VERDICT, label: 'free-gate', phase: 'Free-gate' },
)

return { journal, gate }
