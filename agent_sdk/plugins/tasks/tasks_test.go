// Tasks plugin — rail, todos tool, lobes, path, stages, plugin
// surface. Mirrors agent_sdk/plugins/tasks/tests/*.
package tasks

import (
	"context"
	"testing"
)

// TestRailDepsReadyAndTopoOrder mirrors test_rail_deps_ready_and_topo_order.
func TestRailDepsReadyAndTopoOrder(t *testing.T) {
	r := NewTodoRail()
	r.Add("A")
	r.Add("C", "t1") // depends on B
	r.Add("B", "t0") // depends on A
	ready := r.Ready()
	if len(ready) != 1 || ready[0].ID != "t0" {
		t.Fatalf("expected ready=[t0], got %v", ids(ready))
	}
	order := r.TopoOrder()
	got := []string{}
	for _, x := range order {
		got = append(got, x.Title)
	}
	want := []string{"A", "B", "C"}
	if !equalStrings(got, want) {
		t.Fatalf("expected topo=%v, got %v", want, got)
	}
}

// TestAsItemsCarriesSpecOverrides mirrors test_as_items_carries_spec_overrides.
func TestAsItemsCarriesSpecOverrides(t *testing.T) {
	r := NewTodoRail()
	r.AddWithSpec("query", map[string]any{"tools": []string{"db.query"}, "system_prompt": "SQL only"})
	items := r.AsItems()
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	it := items[0]
	if it["id"] != "t0" || it["input"] != "query" {
		t.Fatalf("expected id=t0 input=query, got %v", it)
	}
	tools, _ := it["tools"].([]string)
	if !equalStrings(tools, []string{"db.query"}) {
		t.Fatalf("expected tools=[db.query], got %v", it["tools"])
	}
	if it["system_prompt"] != "SQL only" {
		t.Fatalf("expected system_prompt=SQL only, got %v", it["system_prompt"])
	}
}

// TestOneToolWithActions mirrors test_one_tool_with_actions.
func TestOneToolWithActions(t *testing.T) {
	r := NewTodoRail()
	rt := NewTodosToolRuntimeWithRail(r)
	specs := rt.GetToolSpecs()
	if len(specs) != 1 || specs[0]["name"] != "todos" {
		t.Fatalf("expected one `todos` tool, got %v", specs)
	}
	props := specs[0]["input_schema"].(map[string]any)["properties"].(map[string]any)
	actionEnum := props["action"].(map[string]any)["enum"].([]string)
	have := map[string]bool{}
	for _, a := range actionEnum {
		have[a] = true
	}
	for _, want := range []string{"add", "add_many", "list", "done", "block", "request_human"} {
		if !have[want] {
			t.Fatalf("expected action %q in enum %v", want, actionEnum)
		}
	}
}

// TestAddThenAdvanceToComplete mirrors test_add_then_advance_to_complete.
func TestAddThenAdvanceToComplete(t *testing.T) {
	r := NewTodoRail()
	rt := NewTodosToolRuntimeWithRail(r)
	ctx := context.Background()
	if _, err := rt.CallTool(ctx, "todos", map[string]any{"action": "add", "title": "A"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.CallTool(ctx, "todos", map[string]any{"action": "add", "title": "B"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.CallTool(ctx, "todos", map[string]any{"action": "done", "result": "did A"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if r.ByID("t0").Status != StatusDone || r.ByID("t0").Result != "did A" {
		t.Fatalf("expected t0 done with 'did A', got %+v", r.ByID("t0"))
	}
	out, err := rt.CallTool(ctx, "todos", map[string]any{"action": "done", "id": "t1"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsComplete() {
		t.Fatalf("expected rail complete")
	}
	if out == "" || !contains(out, "complete") {
		t.Fatalf("expected 'complete' in output, got %q", out)
	}
}

// TestDependencyOrderEnforcedByTool mirrors test_dependency_order_enforced_by_tool.
func TestDependencyOrderEnforcedByTool(t *testing.T) {
	r := NewTodoRail()
	rt := NewTodosToolRuntimeWithRail(r)
	ctx := context.Background()
	_, _ = rt.CallTool(ctx, "todos", map[string]any{"action": "add", "title": "A"}, nil, nil)
	_, _ = rt.CallTool(ctx, "todos", map[string]any{"action": "add_many", "steps": []any{
		map[string]any{"title": "B", "deps": []string{"t0"}},
	}}, nil, nil)
	out, _ := rt.CallTool(ctx, "todos", map[string]any{"action": "done", "id": "t1"}, nil, nil)
	if !contains(out, "needs t0") {
		t.Fatalf("expected 'needs t0' in output, got %q", out)
	}
}

// TestRequestHumanBlocks mirrors test_request_human_blocks.
func TestRequestHumanBlocks(t *testing.T) {
	r := NewTodoRail()
	rt := NewTodosToolRuntimeWithRail(r)
	ctx := context.Background()
	_, _ = rt.CallTool(ctx, "todos", map[string]any{"action": "add", "title": "needs sign-off"}, nil, nil)
	out, _ := rt.CallTool(ctx, "todos", map[string]any{
		"action":   "request_human",
		"question": "approve?",
	}, nil, nil)
	if !contains(out, "Escalated") {
		t.Fatalf("expected 'Escalated' in output, got %q", out)
	}
	if r.ByID("t0").Status != StatusBlocked {
		t.Fatalf("expected t0 blocked, got %s", r.ByID("t0").Status)
	}
	if len(r.HumanAsks()) == 0 || r.HumanAsks()[0] != "approve?" {
		t.Fatalf("expected HumanAsks=[approve?], got %v", r.HumanAsks())
	}
}

// TestRecognizerFiredPromptIsCertain mirrors test_fired_prompt_is_certain.
func TestRecognizerFiredPromptIsCertain(t *testing.T) {
	if got := Recognize(map[string]any{"fired_prompt": true}); got != 1.0 {
		t.Fatalf("expected 1.0, got %v", got)
	}
}

// TestRecognizerAnalyticalCuesScoreAboveQna mirrors test_analytical_cues_score_above_qna.
func TestRecognizerAnalyticalCuesScoreAboveQna(t *testing.T) {
	for _, q := range []string{
		"compute the total revenue",
		"what are the top 3 products",
		"how many orders shipped",
		"list customers by spend",
		"average order value per region",
	} {
		if got := Recognize(map[string]any{"query": q}); got < 0.5 {
			t.Fatalf("expected score >= 0.5 for %q, got %v", q, got)
		}
	}
}

// TestRecognizerPlainQuestionsAndChitchatDoNotTrigger mirrors test_plain_questions_and_chitchat_do_not_trigger.
func TestRecognizerPlainQuestionsAndChitchatDoNotTrigger(t *testing.T) {
	for _, q := range []string{
		"what is the capital of France?",
		"who are you?",
		"hello there",
		"thanks!",
	} {
		if got := Recognize(map[string]any{"query": q}); got != 0.0 {
			t.Fatalf("expected 0.0 for %q, got %v", q, got)
		}
	}
}

// TestRendersRailWithDoneStatusAndDeps mirrors test_renders_rail_with_done_status_and_deps.
func TestRendersRailWithDoneStatusAndDeps(t *testing.T) {
	ctx := map[string]any{
		"scratchpad": map[string]any{
			"todos": []any{
				map[string]any{"id": "t0", "input": "fetch revenue"},
				map[string]any{"id": "t1", "input": "compute profit", "deps": []any{"t0"}},
			},
			"todos_results": []any{
				map[string]any{"label": "t0", "result": "200"},
			},
		},
	}
	out := TaskRailRenderString(ctx)
	if !contains(out, "[x] t0: fetch revenue") {
		t.Fatalf("expected '[x] t0: fetch revenue' in output, got:\n%s", out)
	}
	if !contains(out, "[ ] t1: compute profit (needs t0)") {
		t.Fatalf("expected '[ ] t1: compute profit (needs t0)' in output, got:\n%s", out)
	}
}

// TestEmptyRailContributesNothing mirrors test_empty_rail_contributes_nothing.
func TestEmptyRailContributesNothing(t *testing.T) {
	if got := TaskRailRenderString(map[string]any{}); got != "" {
		t.Fatalf("expected '' for empty ctx, got %q", got)
	}
	if got := TaskRailRenderString(map[string]any{"scratchpad": nil}); got != "" {
		t.Fatalf("expected '' for nil scratchpad, got %q", got)
	}
}

// TestPipelineShape mirrors test_pipeline_shape.
func TestPipelineShape(t *testing.T) {
	stages := TaskStages()
	byID := map[string]FlowStepLite{}
	for _, s := range stages {
		byID[s.Name] = FlowStepLite{Lobes: s.Lobes, Loop: s.Loop, Tools: s.Tools, FanoutKey: s.FanoutKey}
	}
	if _, ok := byID["plan"]; !ok {
		t.Fatalf("expected `plan` stage")
	}
	if _, ok := byID["execute"]; !ok {
		t.Fatalf("expected `execute` stage")
	}
	if _, ok := byID["deliver"]; !ok {
		t.Fatalf("expected `deliver` stage")
	}
	if byID["plan"].Loop != "agentic" || !equalStrings(byID["plan"].Tools, []string{"todos"}) {
		t.Fatalf("plan stage wrong: %+v", byID["plan"])
	}
	if byID["execute"].Loop != "map" || byID["execute"].FanoutKey != "todos" {
		t.Fatalf("execute stage wrong: %+v", byID["execute"])
	}
	if !containsSlice(byID["execute"].Lobes, "task_rail") || !containsSlice(byID["deliver"].Lobes, "task_rail") {
		t.Fatalf("expected task_rail in execute+deliver; got %v / %v", byID["execute"].Lobes, byID["deliver"].Lobes)
	}
	if byID["deliver"].Loop != "single" {
		t.Fatalf("deliver should be single, got %q", byID["deliver"].Loop)
	}
}

// FlowStepLite is a test projection of flows.FlowStep.
type FlowStepLite struct {
	Lobes     []string
	Loop      string
	Tools     []string
	FanoutKey string
}

func ids(todos []*Todo) []string {
	out := []string{}
	for _, t := range todos {
		out = append(out, t.ID)
	}
	return out
}

func equalStrings(a, b []string) bool {
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

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func containsSlice(s []string, x string) bool {
	for _, e := range s {
		if e == x {
			return true
		}
	}
	return false
}
