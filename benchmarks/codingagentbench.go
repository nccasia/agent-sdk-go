package benchmarks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/nccasia/agent-sdk-go/agent_sdk/probe"
	codingagent "github.com/nccasia/agent-sdk-go/examples/codingagent"
)

// codingagentbench — the codebase-understanding stress test for the SDK. The
// flagship task pushes every component together: intent routing, a four-stage
// flow (survey → plan → investigate → document), long agentic loops with Funnel
// ReAct, memory-backed findings aggregation, and file writing — one request
// fanned out into a real ARCHITECTURE.md.
//
// Two tiers (mirrors benchmarks/coding-agent-bench/run.py):
//   - FREE replay tier (mode "replay") — the scripted understand model over a
//     small temp fixture exercises the WHOLE pipeline (loop, guards, repo map,
//     glob, memory) with NO provider call, then scores the same 10 checks the
//     live tier scores. Deterministic; the CI floor.
//   - LIVE tier (mode "understand") — the same flow against a real provider over
//     a large repo. Provider-gated: missing without a model.
//
// The bench is LIVE: without a provider the "understand" mode is missing
// evidence, so the composed verdict is UNMEASURED (no evidence is never READY) —
// mirroring run.py's refusal to compose the live verdict without a provider
// token (exit 2). The free replay mode still runs and gates as a real pass.

// codingAgentBenchTask is the flagship request — verbatim from run.py's
// UNDERSTAND_TASK.
const codingAgentBenchTask = "Explore this codebase and write an architecture document (ARCHITECTURE.md) " +
	"introducing the system."

// codingAgentBenchCheckIDs is the static check-id surface in run.py:score()
// order — 5 correctness + 3 efficiency + 2 accuracy. Asserted for cross-language
// parity independent of the provider.
func codingAgentBenchCheckIDs() []string {
	return []string{
		// correctness
		"routed", "used_tools", "wrote_doc", "answered", "not_truncated",
		// efficiency (size-normalized)
		"hops_bounded", "tokens_bounded", "no_redundant_writes",
		// accuracy
		"paths_grounded", "covers_subsystems",
	}
}

// RunCodingAgentBench composes the coding-agent-bench verdict. The FREE replay
// mode always runs (deterministic, no provider) and gates as a real pass. The
// LIVE "understand" mode is provider-gated: missing without a model, so the
// composed verdict is UNMEASURED at the deterministic floor.
func RunCodingAgentBench(ctx context.Context, model string) (Verdict, error) {
	replay, err := codingAgentBenchReplay(ctx)
	if err != nil {
		return Verdict{}, err
	}
	var understand *ModePayload
	if model != "" {
		p, err := codingAgentBenchLive(ctx, model)
		if err != nil {
			return Verdict{}, err
		}
		understand = p
	}
	payloads := map[string]*ModePayload{
		"replay":     replay,
		"understand": understand, // nil ⇒ missing without a provider ⇒ UNMEASURED
	}
	record := map[string][]string{
		"replay":     {"hops", "input_tokens", "path_exist_ratio", "anchor_coverage"},
		"understand": {"hops", "input_tokens", "path_exist_ratio", "anchor_coverage"},
	}
	return ComposeVerdict(payloads, record), nil
}

// RunCodingAgentBenchProbes captures one inspectable understand trace for the
// viewer. With a real model it drives the actual understand pipeline over the
// SDK tree; offline (model=="") it builds the SAME coding agent over a small
// temp fixture (calculator.py + test) driven by the scripted understand client,
// and runs the flagship task through probe.Probe — so the inspection renders the
// real understand path/flow + the four executed stages (survey → plan →
// investigate → document). Adds a trace only — the live verdict (Run) stays
// UNMEASURED without a provider. Mirrors run.py's understand probe feeding
// write_viewer.
func RunCodingAgentBenchProbes(ctx context.Context, model string) ([]*probe.Record, error) {
	if model != "" {
		target, err := filepath.Abs(filepath.Join("..", "agent_sdk"))
		if err != nil {
			return nil, err
		}
		rec, err := codingAgentBenchProbe(ctx, target, model)
		if err != nil {
			return nil, err
		}
		return []*probe.Record{rec}, nil
	}
	// Offline: the scripted understand client over a small temp fixture (mirrors
	// the FREE replay tier) so the trace is fully deterministic with no provider.
	dir, err := os.MkdirTemp("", "cab-probe-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "calculator.py"), []byte(codingagent.CalculatorPy), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "test_calculator.py"), []byte(codingagent.TestCalculatorPy), 0o644); err != nil {
		return nil, err
	}
	rec, err := codingAgentBenchProbe(ctx, dir, codingagent.MakeUnderstandClient())
	if err != nil {
		return nil, err
	}
	return []*probe.Record{rec}, nil
}

// codingAgentBenchProbe builds the coding agent over target and runs the
// flagship understand task through probe.Probe. Pure given (target, client).
func codingAgentBenchProbe(ctx context.Context, target string, client any) (*probe.Record, error) {
	ag := codingagent.BuildCodingAgent(target, client)
	return probe.Probe(ctx, ag, codingAgentBenchTask, probe.WithLabel("understand"))
}

// codingAgentBenchLive drives the understand pipeline against a real provider
// over the SDK package itself. It is only reached when a model is configured;
// without one the live mode is missing (the verdict is UNMEASURED).
func codingAgentBenchLive(ctx context.Context, model string) (*ModePayload, error) {
	// Understand a real, large repo: the SDK package tree. The agent builds its
	// client from the model string (as the other live benches do).
	target, err := filepath.Abs(filepath.Join("..", "agent_sdk"))
	if err != nil {
		return nil, err
	}
	nfiles := codingAgentBenchCountFiles(target)
	return codingAgentBenchRun(ctx, target, model, nfiles)
}

// codingAgentBenchReplay is the FREE tier: the scripted understand model over a
// small temp fixture (calculator.py + test_calculator.py), exercising the whole
// pipeline with no provider call, scored against the same 10 checks. Mirrors
// run.py:_run_replay + score(nfiles=2).
func codingAgentBenchReplay(ctx context.Context) (*ModePayload, error) {
	dir, err := os.MkdirTemp("", "cab-replay-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "calculator.py"), []byte(codingagent.CalculatorPy), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "test_calculator.py"), []byte(codingagent.TestCalculatorPy), 0o644); err != nil {
		return nil, err
	}
	return codingAgentBenchRun(ctx, dir, codingagent.MakeUnderstandClient(), 2)
}

// codingAgentBenchRun builds the coding agent over target, routes + probes the
// understand task, and scores the run. Pure given (target, client, nfiles) —
// reused by the replay and live tiers. Mirrors run.py:score().
func codingAgentBenchRun(ctx context.Context, target string, client any, nfiles int) (*ModePayload, error) {
	agent := codingagent.BuildCodingAgent(target, client)
	routed := agent.Inspect(codingAgentBenchTask).Path.Name
	rec, err := probe.Probe(ctx, agent, codingAgentBenchTask)
	if err != nil {
		return nil, err
	}
	checks := codingAgentBenchScore(rec, target, routed, nfiles)
	return NewPayload(checks, codingAgentBenchMetrics(rec, target, nfiles)), nil
}

// codingAgentBenchScore builds the 10 score() checks from a probe record.
// Mirrors run.py:score().
func codingAgentBenchScore(rec *probe.Record, target, routed string, nfiles int) []Check {
	docPath := filepath.Join(target, "ARCHITECTURE.md")
	docBytes, statErr := os.ReadFile(docPath)
	wrote := statErr == nil
	docText := ""
	if wrote {
		docText = string(docBytes)
	}

	totalHops := len(rec.LlmCalls)
	inputTokens := codingAgentBenchInt(rec.Usage["input_tokens"])

	// writes: each Write tool call. A write whose output does not start with
	// "Wrote " was blocked (a guard); a repeated file_path is redundant.
	seen := map[string]struct{}{}
	redundant, blocked, execWrites := 0, 0, 0
	for _, tc := range rec.ToolCalls {
		if name, _ := tc["name"].(string); name != "Write" {
			continue
		}
		out, _ := tc["output"].(string)
		if !strings.HasPrefix(out, "Wrote ") {
			blocked++
			continue
		}
		execWrites++
		inp, _ := tc["input"].(map[string]any)
		p, _ := inp["file_path"].(string)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			redundant++
		} else {
			seen[p] = struct{}{}
		}
	}

	// Size-normalized budgets: a 30-file and a 300-file repo can't share a
	// ceiling.
	maxHops := nfiles / 3
	if maxHops < 60 {
		maxHops = 60
	}
	maxTokens := nfiles * 4000
	if maxTokens < 400000 {
		maxTokens = 400000
	}

	acc := codingAgentBenchAccuracy(docText, target)

	truncatedFinal := 0
	for _, m := range rec.MetaActions {
		if a, _ := m["action"].(string); a == "truncated_final" {
			truncatedFinal++
		}
	}

	docLines := 0
	if wrote {
		docLines = len(strings.Split(strings.TrimRight(docText, "\n"), "\n"))
	}
	wroteDetail := "MISSING"
	if wrote {
		wroteDetail = fmt.Sprintf("written (%d lines)", docLines)
	}
	missingDetail := ""
	if len(acc.MissingPaths) > 0 {
		shown := acc.MissingPaths
		if len(shown) > 6 {
			shown = shown[:6]
		}
		missingDetail = fmt.Sprintf(", missing=%v", shown)
	}

	return []Check{
		// correctness
		ck("routed", routed == "understand", fmt.Sprintf("path=%q", routed)),
		ck("used_tools", len(rec.ToolCalls) > 0, fmt.Sprintf("tool_calls=%d", len(rec.ToolCalls))),
		ck("wrote_doc", wrote, wroteDetail),
		ck("answered", rec.Status == "answered", fmt.Sprintf("status=%q", rec.Status)),
		ck("not_truncated", truncatedFinal == 0, fmt.Sprintf("truncated_final=%d", truncatedFinal)),
		// efficiency (size-normalized)
		ck("hops_bounded", totalHops <= maxHops, fmt.Sprintf("hops=%d (<=%d)", totalHops, maxHops)),
		ck("tokens_bounded", inputTokens <= maxTokens, fmt.Sprintf("in_tok=%d (<=%d)", inputTokens, maxTokens)),
		ck("no_redundant_writes", redundant == 0,
			fmt.Sprintf("redundant=%d (exec=%d->%d distinct, blocked=%d)", redundant, execWrites, len(seen), blocked)),
		// accuracy
		ck("paths_grounded", acc.PathExistRatio >= 0.85,
			fmt.Sprintf("exist_ratio=%v of %d refs%s", acc.PathExistRatio, acc.RefCount, missingDetail)),
		ck("covers_subsystems", acc.AnchorCoverage >= 0.6,
			fmt.Sprintf("anchor_cov=%v (%d/%d)", acc.AnchorCoverage, len(acc.AnchorsPresent), len(acc.Anchors))),
	}
}

// codingAgentBenchMetrics surfaces the headline metrics run.py records (without
// gating).
func codingAgentBenchMetrics(rec *probe.Record, target string, nfiles int) map[string]any {
	acc := codingAgentBenchAccuracy(codingAgentBenchReadDoc(target), target)
	redundant := 0
	seen := map[string]struct{}{}
	for _, tc := range rec.ToolCalls {
		if name, _ := tc["name"].(string); name != "Write" {
			continue
		}
		out, _ := tc["output"].(string)
		if !strings.HasPrefix(out, "Wrote ") {
			continue
		}
		inp, _ := tc["input"].(map[string]any)
		p, _ := inp["file_path"].(string)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			redundant++
		} else {
			seen[p] = struct{}{}
		}
	}
	return map[string]any{
		"hops":             len(rec.LlmCalls),
		"input_tokens":     codingAgentBenchInt(rec.Usage["input_tokens"]),
		"cost":             rec.Usage["estimated_cost"],
		"redundant_writes": redundant,
		"path_exist_ratio": acc.PathExistRatio,
		"anchor_coverage":  acc.AnchorCoverage,
	}
}

func codingAgentBenchReadDoc(target string) string {
	b, err := os.ReadFile(filepath.Join(target, "ARCHITECTURE.md"))
	if err != nil {
		return ""
	}
	return string(b)
}

// ── accuracy: the doc must be grounded in the real tree ───────────────────────

// codingAgentBenchSkip is run.py's _SKIP set.
var codingAgentBenchSkip = map[string]struct{}{
	".git": {}, "__pycache__": {}, ".venv": {}, "venv": {}, "node_modules": {}, "dist": {}, "build": {},
}

// codingAgentBenchPathRE / codingAgentBenchCodeSfx mirror run.py's _PATH_RE and
// _CODE_SFX.
var codingAgentBenchPathRE = regexp.MustCompile(`[A-Za-z0-9_][A-Za-z0-9_./-]*\.[A-Za-z0-9]{1,6}`)

var codingAgentBenchCodeSfx = []string{
	".py", ".js", ".ts", ".go", ".rs", ".md", ".toml", ".json", ".yaml", ".yml", ".sh",
}

// codingAgentBenchAccuracyResult mirrors run.py:accuracy_metrics' return dict.
type codingAgentBenchAccuracyResult struct {
	RefCount       int
	MissingPaths   []string
	PathExistRatio float64
	Anchors        []string
	AnchorsPresent []string
	AnchorCoverage float64
}

// codingAgentBenchAnchors returns the real subsystems — top-level package dirs +
// prominent root modules (the first 14). Mirrors run.py:_anchors.
func codingAgentBenchAnchors(target string) []string {
	entries, err := os.ReadDir(target)
	if err != nil {
		return []string{}
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	out := []string{}
	for _, name := range names {
		if _, skip := codingAgentBenchSkip[name]; skip || strings.HasPrefix(name, ".") {
			continue
		}
		info, err := os.Stat(filepath.Join(target, name))
		if err != nil {
			continue
		}
		if info.IsDir() {
			out = append(out, name)
		} else if strings.HasSuffix(name, ".py") && name != "__init__.py" {
			out = append(out, strings.TrimSuffix(name, ".py"))
		}
	}
	if len(out) > 14 {
		out = out[:14]
	}
	return out
}

// codingAgentBenchAccuracy resolves the doc's file references against the real
// tree and measures subsystem coverage. Mirrors run.py:accuracy_metrics.
func codingAgentBenchAccuracy(docText, target string) codingAgentBenchAccuracyResult {
	refs := []string{}
	seenRef := map[string]struct{}{}
	for _, m := range codingAgentBenchPathRE.FindAllString(docText, -1) {
		if _, ok := seenRef[m]; ok {
			continue
		}
		seenRef[m] = struct{}{}
		hasSfx := false
		for _, sfx := range codingAgentBenchCodeSfx {
			if strings.HasSuffix(m, sfx) {
				hasSfx = true
				break
			}
		}
		if hasSfx {
			refs = append(refs, m)
		}
	}
	missing := []string{}
	for _, r := range refs {
		if _, err := os.Stat(filepath.Join(target, strings.TrimPrefix(r, "/"))); err != nil {
			missing = append(missing, r)
		}
	}
	n := len(refs)
	pathRatio := 1.0
	if n > 0 {
		pathRatio = round3(float64(n-len(missing)) / float64(n))
	}
	anchors := codingAgentBenchAnchors(target)
	low := strings.ToLower(docText)
	present := []string{}
	for _, a := range anchors {
		if strings.Contains(low, strings.ToLower(a)) {
			present = append(present, a)
		}
	}
	cov := 1.0
	if len(anchors) > 0 {
		cov = round3(float64(len(present)) / float64(len(anchors)))
	}
	return codingAgentBenchAccuracyResult{
		RefCount: n, MissingPaths: missing, PathExistRatio: pathRatio,
		Anchors: anchors, AnchorsPresent: present, AnchorCoverage: cov,
	}
}

// codingAgentBenchCountFiles mirrors `sum(len(f) for _, _, f in os.walk(target))`.
func codingAgentBenchCountFiles(target string) int {
	n := 0
	_ = filepath.WalkDir(target, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

func codingAgentBenchInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	}
	return 0
}
