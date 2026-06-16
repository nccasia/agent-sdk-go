// Make every Go bench inspectable by default: cmd/bench writes a viewer HTML per
// bench (verdict + modes + captured probe traces) so the inspection panel shows
// turn/path/flow/steps + the composed system prompt & per-step payload. The Go
// engine already records messages/system_prompt/system_segments on every flow
// stage (no AGENT_CORE_CAPTURE_PROMPTS needed — capture is on by default); the
// only missing wire is emitting the viewer with the records. Sequential — shared
// `benchmarks` package + per-agent `git add -A` means no parallelism.
export const meta = {
  name: 'bench-inspectable',
  description: 'Wire cmd/bench to emit an inspectable viewer HTML per bench by default, feeding each bench its captured probe traces.',
  phases: [
    { title: 'Core wiring' },
    { title: 'Bench probes' },
    { title: 'Verify + serve' },
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
  `Python reference (source of truth) at ${PY}: study how benchmarks/*/run.py build the viewer — \`from agent_sdk.viewer import write_viewer\` and \`write_viewer(path, probes, label=…, verdict=…, modes=…)\`, and how _shared/report.py:emit_report wraps it. Mirror that shape.`,
  `Keep the WHOLE module green: \`go test ./... -race\` exit 0, \`gofmt -l .\` empty, \`go vet ./...\` clean. Do NOT regress any rung; \`go run ./cmd/bench\` free-gate stays green / exit 0; \`go run ./cmd/parity\` stays 116/116.`,
  `Commit ONLY when every check is green: \`git add -A && git commit\` with a descriptive subject + a \`Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>\` trailer. NEVER commit red. NEVER \`git add\` a .env (gitignored). If you cannot reach green, leave the tree and report. Return the VERDICT object.`,
].join('\n')

const STAGES = [
  {
    phase: 'Core wiring',
    unit: 'core-html-emit',
    prompt: [
      `Wire the benchmark runner to emit an inspectable viewer HTML per bench by DEFAULT.`,
      `Study the existing Go surface: ${REPO}/benchmarks/registry.go (Bench{Name,Tier,Run,ExpectStatus}, Registry, FreeGate, DefaultRegistry), ${REPO}/agent_sdk/viewer/viewer.go (Write(path, []*probe.Record, ...Option), WithLabel/WithVerdict/WithModes, ToRecord), ${REPO}/agent_sdk/probe/probe.go (Probe, Record), and ${REPO}/cmd/bench/main.go.`,
      `1. Add an optional field to Bench: \`Probe func(ctx context.Context, model string) ([]*probe.Record, error)\` (nil ⇒ no traces, report still shows the verdict). Import the probe package.`,
      `2. Add \`func (r *Registry) WriteReports(ctx context.Context, model, dir string) ([]string, error)\`: for each bench in All(), compute its Verdict via Run(ctx, model); if Probe!=nil capture its records; then viewer.Write(filepath.Join(dir, name+".html"), records, viewer.WithLabel(name+" · "+status), viewer.WithVerdict(<verdict as the viewer expects>), viewer.WithModes(<modes>)). Convert benchmarks.Verdict to whatever shape viewer.WithVerdict/overview consume (match how the viewer's ToRecord/overview reads status/gates/reasons — check viewer_test.go for the expected shape; a map[string]any with keys status/gates/reasons/metrics is the Python contract). Also write an index.html linking every per-bench report with its verdict badge. Return the written paths.`,
      `3. cmd/bench: by DEFAULT also write the reports to \`benchmarks/results\` (already gitignored) after the free-gate, printing the index path. Add flags \`-html <dir>\` (override dir) and \`-no-html\` (skip). Keep the existing free-gate text output + exit code.`,
      `4. Add a test (benchmarks) asserting WriteReports writes one HTML per bench + an index, and that a bench whose Probe returns a record yields HTML containing the trace (path/flow/stage markers).`,
      `Note: the Go engine already records messages/system_prompt/system_segments on each flow stage, so a captured probe.Record renders a populated inspection — no env var required.`,
    ].join('\n'),
  },
  {
    phase: 'Bench probes',
    unit: 'probes-routing',
    prompt: [
      `Implement the Bench.Probe closure for the LLM-routing free benches: flowbench, promptbench, corgictionbech.`,
      `These already drive PreactAgents through representative scenarios inside their Run path (they call probe.Probe for their checks). For each, set Probe to a closure that runs a small, representative set of that bench's scenarios through probe.Probe (reuse the same agent/scenario builders Run uses) and returns the []*probe.Record — offline-deterministic via FakeClient when model=="". Register the closure in DefaultRegistry (Bench{... Probe: RunXBenchProbes}).`,
      `Each record must carry a real path/flow and ≥1 stage so the viewer inspection shows turn/path/flow/steps + the stage system_prompt/segments. Keep the set small (1-3 scenarios) so cmd/bench stays fast.`,
    ].join('\n'),
  },
  {
    phase: 'Bench probes',
    unit: 'probes-live',
    prompt: [
      `Implement the Bench.Probe closure for the live benches: agentbench, taskbench, extensionbench, skillbench, codingagentbench, delegationbench.`,
      `Their full behavior needs a provider, but the report must be inspectable OFFLINE. So Probe(ctx, model) should: when model!="" use the real model; when model=="" build the bench's representative agent with a FakeClient and run 1-2 representative turns through probe.Probe, returning records with a real path/flow + stages. Register each closure in DefaultRegistry. Do not change the verdict/Run logic (live verdicts stay UNMEASURED without a provider) — Probe only adds inspectable traces.`,
    ].join('\n'),
  },
  {
    phase: 'Bench probes',
    unit: 'probes-plumbing',
    prompt: [
      `Implement the Bench.Probe closure for the plumbing free benches: statelessbench, toolbench, attentionbench.`,
      `statelessbench/toolbench exercise plumbing (snapshot/store/serve, tool spec/select) more than LLM turns — still produce a representative probe.Probe trace of the bench's agent on a representative query (FakeClient) so the inspection is populated, not empty. attentionbench already drives an attention agent; capture 1-2 of its scenarios. Register each closure in DefaultRegistry.`,
    ].join('\n'),
  },
]

const journal = []
for (const s of STAGES) {
  phase(s.phase)
  const v = await agent([s.prompt, '', COMMON].join('\n'), { schema: VERDICT, label: s.unit, phase: s.phase })
  journal.push({ ...(v || { unit: s.unit, checksPass: false }) })
  log(`${s.unit}: ${v && v.checksPass ? 'green' : 'NOT green'} ${v && v.committed ? '(committed)' : ''}`)
}

phase('Verify + serve')
const verify = await agent(
  [
    `Final verification that every bench is inspectable by default. From ${REPO} (\`cd ${REPO} && ${ENV}\`):`,
    `1. \`go run ./cmd/bench\` — confirm it writes benchmarks/results/<bench>.html for ALL 12 benches + index.html, free-gate still green/exit 0.`,
    `2. For 3 benches (one routing, one live, one plumbing), open the generated HTML and confirm the inspection is POPULATED: a turn with a real path/flow, ≥1 step, and a non-empty System prompt panel (NOT "0 steps"/"No captured prompts"). Grep the HTML for the embedded record JSON (path/flow/stages/system_segments) to prove it.`,
    `3. \`go test ./... -race\` green, \`gofmt -l .\` empty, \`go vet ./...\` clean, \`go run ./cmd/parity\` 116/116.`,
    `Fix the smallest thing if any check is red and commit. Report VERDICT (unit="verify-inspectable") with notes listing the per-bench step/prompt counts you observed.`,
    COMMON,
  ].join('\n'),
  { schema: VERDICT, label: 'verify-inspectable', phase: 'Verify + serve' },
)
journal.push({ ...(verify || { unit: 'verify-inspectable', checksPass: false }) })

return { journal }
