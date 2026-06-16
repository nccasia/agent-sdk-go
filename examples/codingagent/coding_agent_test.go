package codingagent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const understandTask = "Explore this codebase and write an architecture document (ARCHITECTURE.md) " +
	"introducing the system."

// repo writes the calculator sandbox into a fresh temp dir and returns it.
func repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "calculator.py"), []byte(CalculatorPy), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "test_calculator.py"), []byte(TestCalculatorPy), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRoutingIsDeterministic(t *testing.T) {
	r := repo(t)
	a := BuildCodingAgent(r, MakeFakeClient())
	if got := a.Inspect(understandTask).Path.Name; got != "understand" {
		t.Fatalf("understand task routed to %q, want understand", got)
	}
	if got := a.Inspect("add a multiply function and a test").Path.Name; got != "feature" {
		t.Fatalf("feature task routed to %q, want feature", got)
	}
	if got := a.Inspect("fix the broken subtract").Path.Name; got != "quick_fix" {
		t.Fatalf("fix task routed to %q, want quick_fix", got)
	}
	if got := a.Inspect("what does the subtract function return").Path.Name; got != "question" {
		t.Fatalf("question routed to %q, want question", got)
	}
}

func TestUnderstandPipelineWritesArchitecture(t *testing.T) {
	r := repo(t)
	a := BuildCodingAgent(r, MakeUnderstandClient())
	if _, err := a.Query(context.Background(), understandTask); err != nil {
		t.Fatal(err)
	}
	if name, _ := a.LastTrace().Path["name"].(string); name != "understand" {
		t.Fatalf("trace path name = %q, want understand", name)
	}
	if _, err := os.Stat(filepath.Join(r, "ARCHITECTURE.md")); err != nil {
		t.Fatalf("ARCHITECTURE.md not written: %v", err)
	}
}

func TestGlobMatchesRootLevelFiles(t *testing.T) {
	r := repo(t)
	tools := toolMap(r)
	g := tools["Glob"]
	if out := invoke(t, g, map[string]any{"pattern": "**/*.py"}); !strings.Contains(out, "calculator.py") {
		t.Fatalf("**/*.py did not match calculator.py: %q", out)
	}
	if out := invoke(t, g, map[string]any{"pattern": "**/calculator.py"}); !strings.Contains(out, "calculator.py") {
		t.Fatalf("**/calculator.py did not match calculator.py: %q", out)
	}
	if err := os.Mkdir(filepath.Join(r, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r, "pkg", "mod.py"), []byte("x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if nested := invoke(t, g, map[string]any{"pattern": "**/mod.py"}); !strings.Contains(nested, "pkg/mod.py") {
		t.Fatalf("**/mod.py did not match pkg/mod.py: %q", nested)
	}
	if out := invoke(t, g, map[string]any{"pattern": "**/*.rs"}); out != "(no files match)" {
		t.Fatalf("**/*.rs over-matched: %q", out)
	}
}

func TestSelfCorrectingPathErrors(t *testing.T) {
	r := repo(t)
	tools := toolMap(r)
	out := invoke(t, tools["Read"], map[string]any{"file_path": "calculater.py"}) // typo
	if !strings.Contains(out, "Error: not a file") || !strings.Contains(out, "did you mean: calculator.py") {
		t.Fatalf("Read typo did not self-correct: %q", out)
	}
	out2 := invoke(t, tools["LS"], map[string]any{"path": "src"}) // nonexistent dir
	if !strings.Contains(out2, "Error: not a directory") ||
		(!strings.Contains(out2, "did you mean") && !strings.Contains(out2, "contains:")) {
		t.Fatalf("LS nonexistent dir did not self-correct: %q", out2)
	}
}

func TestRepoMapIsDeterministicAndGrounded(t *testing.T) {
	r := repo(t)
	m1 := BuildRepoMap(r)
	m2 := BuildRepoMap(r)
	if m1 != m2 {
		t.Fatal("repo map not deterministic")
	}
	if !strings.Contains(m1, "calculator.py") || !strings.Contains(m1, "test_calculator.py") {
		t.Fatalf("repo map missing files: %q", m1)
	}
	if !strings.Contains(m1, "add") {
		t.Fatalf("repo map missing top-level def add: %q", m1)
	}
}

func TestFeatureFlowResolvesFullPipeline(t *testing.T) {
	r := repo(t)
	a := BuildCodingAgent(r, MakeFakeClient())
	snap := a.Inspect("add a multiply function to calculator.py and a test")
	wantFlow := []string{"explore", "plan", "implement", "verify", "summarize"}
	if !equalStrs(snap.Flow, wantFlow) {
		t.Fatalf("flow = %v, want %v", snap.Flow, wantFlow)
	}
	activated := map[string]bool{}
	for _, lb := range snap.Lobes {
		if a, _ := lb["activated"].(bool); a {
			if id, _ := lb["id"].(string); id != "" {
				activated[id] = true
			}
		}
	}
	for _, want := range []string{"triage", "explore", "plan", "implement", "verify", "summarize"} {
		if !activated[want] {
			t.Fatalf("lobe %q not activated; activated=%v", want, activated)
		}
	}
}

func TestAgentEditsRealFilesAndTestsPass(t *testing.T) {
	r := repo(t)
	a := BuildCodingAgent(r, MakeFakeClient())
	result, err := a.Query(context.Background(), "add a multiply function to calculator.py and a test for it")
	if err != nil {
		t.Fatal(err)
	}

	src, err := os.ReadFile(filepath.Join(r, "calculator.py"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "def multiply(a, b):") {
		t.Fatalf("multiply not added:\n%s", src)
	}
	if !strings.Contains(string(src), "def add(a, b):") {
		t.Fatalf("existing add() not preserved:\n%s", src)
	}

	if _, err := os.Stat(filepath.Join(r, "test_multiply.py")); err != nil {
		t.Fatalf("test_multiply.py not created: %v", err)
	}

	// the agent ran the real suite and it passes
	if _, err := exec.LookPath("python3"); err == nil {
		cmd := exec.Command("bash", "-c", PytestCmd)
		cmd.Dir = r
		cmd.Env = append(os.Environ(), "PYTHONPATH="+r)
		if out, perr := cmd.CombinedOutput(); perr != nil {
			t.Fatalf("suite failed: %v\n%s", perr, out)
		}
	}

	if !strings.Contains(strings.ToLower(result.Text), "multiply") {
		t.Fatalf("result text omits multiply: %q", result.Text)
	}
	if result.Status != "answered" {
		t.Fatalf("status = %q, want answered", result.Status)
	}
}

func TestTraceRecordsEveryStage(t *testing.T) {
	r := repo(t)
	a := BuildCodingAgent(r, MakeFakeClient())
	if _, err := a.Query(context.Background(), "add a multiply function and a test"); err != nil {
		t.Fatal(err)
	}
	var stages []string
	for _, s := range a.LastTrace().FlowStages {
		if st, _ := s["stage"].(string); st != "" {
			stages = append(stages, st)
		}
	}
	want := []string{"explore", "plan", "implement", "verify", "summarize"}
	if !equalStrs(stages, want) {
		t.Fatalf("stages = %v, want %v", stages, want)
	}
	// explore ran tool calls (real reads)
	var explore map[string]any
	for _, s := range a.LastTrace().FlowStages {
		if st, _ := s["stage"].(string); st == "explore" {
			explore = s
			break
		}
	}
	if explore == nil {
		t.Fatal("no explore stage in trace")
	}
	if !hasToolUseStep(explore) {
		t.Fatalf("explore had no tool_use step: %v", explore)
	}
}

func TestNotesCarryForwardAcrossStages(t *testing.T) {
	r := repo(t)
	client := MakeFakeClient()
	a := BuildCodingAgent(r, client)
	if _, err := a.Query(context.Background(), "add a multiply function and a test"); err != nil {
		t.Fatal(err)
	}
	var verifyCalls []string
	for _, c := range client.Calls {
		if c.Stage == "verify" {
			verifyCalls = append(verifyCalls, asStr(c.System))
		}
	}
	if len(verifyCalls) == 0 {
		t.Fatal("no verify stage calls recorded")
	}
	if !strings.Contains(verifyCalls[0], "[plan]") {
		t.Fatalf("verify system missing [plan] note:\n%s", verifyCalls[0])
	}
	if !strings.Contains(verifyCalls[0], "[explore]") {
		t.Fatalf("verify system missing [explore] note:\n%s", verifyCalls[0])
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func toolMap(root string) map[string]interface {
	Invoke(context.Context, map[string]any) (any, error)
} {
	out := map[string]interface {
		Invoke(context.Context, map[string]any) (any, error)
	}{}
	for _, td := range CodingTools(root) {
		out[td.Name] = td
	}
	return out
}

func invoke(t *testing.T, tool interface {
	Invoke(context.Context, map[string]any) (any, error)
}, in map[string]any) string {
	t.Helper()
	res, err := tool.Invoke(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := res.(string)
	return s
}

func equalStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func hasToolUseStep(stage map[string]any) bool {
	steps, ok := stage["steps"].([]map[string]any)
	if !ok {
		if anySteps, ok2 := stage["steps"].([]any); ok2 {
			for _, s := range anySteps {
				if m, ok3 := s.(map[string]any); ok3 {
					if k, _ := m["kind"].(string); k == "tool_use" {
						return true
					}
				}
			}
		}
		return false
	}
	for _, s := range steps {
		if k, _ := s["kind"].(string); k == "tool_use" {
			return true
		}
	}
	return false
}

func asStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
