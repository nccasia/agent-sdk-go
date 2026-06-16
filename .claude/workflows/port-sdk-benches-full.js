// Full rung-14 completion: port the SDK surface the remaining benches need, then
// all remaining benches, to Python verdict parity. Everything is SEQUENTIAL —
// each unit lives in shared Go packages and every agent runs `git add -A &&
// git commit`, so parallel agents would race the git index and shared-package
// compiles. One agent per unit; each keeps the whole suite green and commits
// before the next starts.
export const meta = {
  name: 'port-sdk-benches-full',
  description: 'Port the prerequisite SDK surface (agent_from_spec, inspect-with-state, system_segments) then all remaining benches to verdict parity.',
  phases: [
    { title: 'Prereqs' },
    { title: 'Verify prereqs' },
    { title: 'Free benches' },
    { title: 'Live benches' },
    { title: 'Free-gate' },
  ],
}

const REPO = (args && args.repo) || '/d/agent-sdk-go'
const PY = (args && args.python) || '/d/agent-sdk'
const ENV = 'export PATH="$HOME/go-sdk/go/bin:$PATH" GOPATH="$HOME/go" GOFLAGS=-mod=mod'

const VERDICT = {
  type: 'object',
  additionalProperties: false,
  required: ['unit', 'checksPass', 'committed', 'notes'],
  properties: {
    unit: { type: 'string' },
    checksPass: { type: 'boolean' },
    committed: { type: 'boolean' },
    notes: { type: 'string' },
  },
}

const COMMON = [
  `Run every Go command from the module root with the toolchain on PATH: prefix with \`cd ${REPO} && ${ENV} && …\` (module github.com/mezon/agent-sdk-go).`,
  `The Python reference SDK is at ${PY} — it is the source of truth; where Go and Python disagree, Python wins.`,
  `STRICT TDD: write the Go test(s) translating the Python behavior FIRST (red), then implement to green.`,
  `Keep the WHOLE module green: \`go test ./... -race\` must stay exit 0, \`gofmt -l .\` empty, \`go vet ./...\` clean. Do NOT regress any existing rung.`,
  `Commit ONLY when every check is green: \`git add -A && git commit\` with a descriptive subject + a \`Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>\` trailer. NEVER commit red. NEVER \`git add\` a .env (it is gitignored — leave it). If you cannot reach green, leave the tree as-is and report what blocks. Return the VERDICT object.`,
].join('\n')

// ---- Phase 1: prerequisite SDK surface (sequential) ----
const PREREQS = [
  {
    unit: 'agent_from_spec',
    prompt: [
      `Port \`agent_from_spec\` — the rebuilder that reconstructs a working PreactAgent from a spec (the inverse of PreactAgent.Spec()/spec.ToJSON()).`,
      `Python: ${PY}/agent_sdk/spec.py — \`PreactSpec\` (fields incl. weights/budgets/flow_lobe_weights/flow_layer_budgets/pinned_lobes/require_citations/tz/lang) and \`agent_from_spec(spec, *, client, tools=None, **overrides)\` at line ~181 (folds named aliases into weights/budgets; builds lobes/stages/flows/skills via make_stage/make_flow/Skill).`,
      `Go today: ${REPO}/agent_sdk/core/spec/spec.go has Spec + ToJSON/ToJSONStr/FromJSON; ${REPO}/agent_sdk/agent/agent.go:487 has \`(*PreactAgent).Spec()\`; ${REPO}/agent_sdk/agent/assemble.go:296 has \`stagesFromSpec\`. There is NO full rebuilder yet.`,
      `Implement an exported rebuilder in the agent package, e.g. \`func FromSpec(s *spec.Spec, client clients.Client, tools ...) (*PreactAgent, error)\`, reusing stagesFromSpec/assemble helpers. The round-trip MUST hold: build agent A, then FromSpec(A.Spec()) ⇒ an agent whose Spec().ToJSON() deep-equals A.Spec().ToJSON(). Add a round-trip test. Match the Python field-folding (named aliases → weights/budgets).`,
    ].join('\n'),
  },
  {
    unit: 'inspect-with-state',
    prompt: [
      `Add state-aware inspection so a follow-up turn routes using prior SessionState (needed by flowbench clarify-followup).`,
      `Python: ${PY}/agent_sdk/engine.py:1982 \`def inspect(self, query: str, state: SessionState | None = None)\` — when state is given, routing/path recognition sees the prior turn context.`,
      `Go today: ${REPO}/agent_sdk/agent/engine.go:102 \`(*Engine).Inspect(query string)\` and ${REPO}/agent_sdk/agent/assemble.go:667 \`(*PreactAgent).Inspect(input string)\` — neither accepts state.`,
      `Add an exported state-aware variant (e.g. \`(*Engine).InspectWithState(query string, state session.SessionState)\` and the matching PreactAgent method), keeping the existing \`Inspect(query)\` as the empty-state call so no caller breaks. Add a test showing routing differs when prior state implies a clarify follow-up vs a cold query.`,
    ].join('\n'),
  },
  {
    unit: 'system_segments',
    prompt: [
      `Surface segmented system-prompt composition on the probe trace (needed by promptbench).`,
      `Python: the engine composes the system prompt as ordered segments {source, stability, start, end} and the probe trace stages carry \`system_segments\`. Find the Python segment composition in ${PY}/agent_sdk/engine.py (search "system_segments" / segment / stability) and the probe emission.`,
      `Go today: ${REPO}/agent_sdk/probe/probe.go stages are []map[string]any and carry system_prompt but NOT system_segments; ${REPO}/agent_sdk/viewer/viewer.go already passes through a "system_segments" key (currently always empty). There is no SystemSegment type in the engine yet — you may need to add segmented composition (Source/Stability/Start/End offsets over the composed system string) and thread it onto each probe stage as "system_segments".`,
      `Implement the minimum faithful version: compose the system prompt as segments with the same sources/stability ordering as Python, attach them to probe trace stages, and have the viewer passthrough show them. Add a test asserting a stage's system_segments cover the system_prompt with correct ordering (identity first, volatile/env last) and non-overlapping offsets.`,
    ].join('\n'),
  },
]

// ---- Phase 3/4: benches (sequential) ----
const FREE = [
  { unit: 'statelessbench', note: 'THE ci-free-gates.sh free bench. Modes snapshot/store/isolation/spec/schema. Now uses FromSpec (agent_from_spec) + SessionStoreSQL (modernc) + AgentWorker pool. Deterministic via FakeClient ⇒ READY.' },
  { unit: 'flowbench', note: 'Modes routing/tiers/states/grounding/coverage/determinism/subject/execution, one scenario per flow. Uses InspectWithState for clarify-followup routing. Deterministic ⇒ READY.' },
  { unit: 'promptbench', note: 'Free tiers structure (identity/dedup/ordering/coverage over system_segments) + quality (5 rule checks). Live judge tier provider-gated. Deterministic ⇒ READY.' },
]
const LIVE = [
  { unit: 'agentbench', note: 'mission.* + hard.* checks; UNMEASURED without provider.' },
  { unit: 'taskbench', note: 'one mode per capability; answered/answer_correct/bounded; UNMEASURED without provider.' },
  { unit: 'extensionbench', note: 'plugin.* + mcp.* plug/unplug; UNMEASURED without provider.' },
  { unit: 'skillbench', note: 'lint/parse/mapping/activation/follow/funnel; precision/recall ≥0.8; UNMEASURED without provider.' },
  { unit: 'codingagentbench', note: 'coding-agent-bench: FREE replay tier (mode understand) gated + live tier provider-gated.' },
  { unit: 'delegationbench', note: 'payload live (None⇒UNMEASURED without provider); decision/exec/fanin; precision≥0.8 recall≥0.7.' },
]

function benchPrompt(b, tier) {
  const pyDir = b.unit === 'codingagentbench' ? 'coding-agent-bench' : b.unit
  return [
    `Port/complete the "${b.unit}" benchmark (${tier} tier) to verdict parity with Python.`,
    COMMON,
    ``,
    `Python source of truth: ${PY}/benchmarks/${pyDir}/ — read run.py, verdicts/*.json, METHOD.md, scoring.py. Reproduce the SAME modes + check-ids; compose via the shared ComposeVerdict (${REPO}/benchmarks/verdict.go). Study registry.go (Bench{Name,Tier,Run,ExpectStatus}, DefaultRegistry, FreeGate) and an existing bench (corgictionbench.go) for the RunXBench style.`,
    `Notes: ${b.note}`,
    `Implement benchmarks/${b.unit}.go + benchmarks/${b.unit}_test.go (Test<Name>Bench_Ready asserting the deterministic-floor verdict status matches the Python verdicts/ baseline, plus a check-id parity assertion), and register it in registry.go DefaultRegistry with the confirmed Tier+ExpectStatus. Namespace helpers by the bench prefix.`,
    `${tier === 'Free' ? 'Your free bench must now be gated by the free-gate (go run ./cmd/bench stays green).' : 'Your live bench must be UNMEASURED without a provider and must NOT break the free-gate.'}`,
    `Checks to green before commit: \`go test ./benchmarks/... -race\`, \`go run ./cmd/bench\`, \`gofmt -l .\`, \`go vet ./...\`.`,
  ].join('\n')
}

const journal = []

phase('Prereqs')
for (const p of PREREQS) {
  const v = await agent([p.prompt, '', COMMON].join('\n'), { schema: VERDICT, label: 'prereq ' + p.unit, phase: 'Prereqs' })
  journal.push({ phase: 'prereq', ...(v || { unit: p.unit, checksPass: false }) })
  log(`prereq ${p.unit}: ${v && v.checksPass ? 'green' : 'NOT green'} ${v && v.committed ? '(committed)' : ''}`)
}

phase('Verify prereqs')
const pv = await agent(
  [`Verify the prerequisite ports did not regress the SDK. From ${REPO} (\`cd ${REPO} && ${ENV}\`): run \`go test ./... -race\`, \`gofmt -l .\`, \`go vet ./...\`. If anything is red, apply the smallest fix in the owning package and commit it. Report VERDICT (unit="verify-prereqs").`, COMMON].join('\n'),
  { schema: VERDICT, label: 'verify-prereqs', phase: 'Verify prereqs' },
)
journal.push({ phase: 'verify', ...(pv || { unit: 'verify-prereqs', checksPass: false }) })

phase('Free benches')
for (const b of FREE) {
  const v = await agent(benchPrompt(b, 'Free'), { schema: VERDICT, label: 'free ' + b.unit, phase: 'Free benches' })
  journal.push({ phase: 'free', ...(v || { unit: b.unit, checksPass: false }) })
  log(`free ${b.unit}: ${v && v.checksPass ? 'green' : 'NOT green'} ${v && v.committed ? '(committed)' : ''}`)
}

phase('Live benches')
for (const b of LIVE) {
  const v = await agent(benchPrompt(b, 'Live'), { schema: VERDICT, label: 'live ' + b.unit, phase: 'Live benches' })
  journal.push({ phase: 'live', ...(v || { unit: b.unit, checksPass: false }) })
  log(`live ${b.unit}: ${v && v.checksPass ? 'green' : 'NOT green'} ${v && v.committed ? '(committed)' : ''}`)
}

phase('Free-gate')
const gate = await agent(
  [
    `Final free-gate verification for ${REPO}. Prefix commands with \`cd ${REPO} && ${ENV} && …\`.`,
    `1. \`go run ./cmd/bench\`: every FREE bench (statelessbench, flowbench, promptbench, toolbench, corgictionbech, attentionbench, codingagentbench) appears and the free-gate prints green / exits 0; live benches are UNMEASURED (not gated).`,
    `2. \`go test ./... -race\` green, \`gofmt -l .\` empty, \`go vet ./...\` clean, \`go run ./cmd/parity\` still 116/116.`,
    `3. Cross-language sanity for 2-3 free benches: Go Gate Pass set vs Python run.py "ok" for the same check-ids; Verdict.Status matches the Python verdicts/ baseline.`,
    `Apply the smallest fix for any red and commit it. Update README/PARITY bench rows to reflect what is now ported. Report VERDICT (unit="free-gate").`,
    COMMON,
  ].join('\n'),
  { schema: VERDICT, label: 'free-gate', phase: 'Free-gate' },
)

return { journal, gate }
