package subagentsanalytics

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/mezon/agent-sdk-go/agent_sdk/clients"
)

// AnalystPrompt mirrors analytics.agent.ANALYST_PROMPT.
const AnalystPrompt = "You are a data analyst with a `sql` tool over a sales database " +
	"(table: sales(region, product, category, units, revenue, order_date)). " +
	"For a multi-part analytics question, plan it with the TodoWrite tool (one todo per " +
	"independent analysis, each with its own prompt + the sql tool). The independent analyses " +
	"fan out to a subagent each; you then combine their results into a short executive summary."

// AnalyticsResult is the outcome of one plan-driven fan-out run.
type AnalyticsResult struct {
	Text            string   // the executive summary
	Status          string   // "answered"
	Todos           []Todo   // the plan the model wrote with TodoWrite
	PlanStructure   string   // "fanout" (independent) or "sequential" (deps present)
	SubagentResults []string // each subagent's reported result row
}

// AnalyticsAgent plans a multi-part analytics question with TodoWrite, fans out
// one isolated subagent per independent todo (each running its own REAL SQL),
// then aggregates the results into an executive summary. It is the example's
// self-contained port of the SDK's plan → supervise → fanout → fanin flow
// (the Python PlanningPlugin), driven by a scripted FakeClient.
type AnalyticsAgent struct {
	client clients.LlmCall
	sqlrt  *SqlToolRuntime
	prompt string
}

// BuildAnalyticsAgent builds the analytics agent over the SQLite fixture.
// Mirrors analytics.agent.build_analytics_agent.
func BuildAnalyticsAgent(db *sql.DB, client clients.LlmCall) *AnalyticsAgent {
	return &AnalyticsAgent{
		client: client,
		sqlrt:  NewSqlToolRuntime(db),
		prompt: AnalystPrompt,
	}
}

// Query runs the plan → supervise → fanout → fanin pipeline for one question.
func (a *AnalyticsAgent) Query(ctx context.Context, question string) (*AnalyticsResult, error) {
	res := &AnalyticsResult{Status: "answered"}

	// 1. PLAN — the model writes a TodoWrite list, then stops the planning loop.
	todos, err := a.plan(ctx, question)
	if err != nil {
		return nil, err
	}
	res.Todos = todos

	// 2. SUPERVISE — deterministic: any todo with deps ⇒ sequential, else fanout.
	res.PlanStructure = supervise(todos)

	// 3. EXECUTE — one subagent per todo, isolated, each running its own SQL.
	subResults, err := a.fanout(ctx, todos)
	if err != nil {
		return nil, err
	}
	res.SubagentResults = subResults

	// 4. FANIN — aggregate every subagent's result into one combined answer.
	summary, err := a.fanin(ctx, subResults)
	if err != nil {
		return nil, err
	}
	res.Text = summary
	return res, nil
}

// plan drives the plan stage's agentic loop: the model calls TodoWrite, the tool
// records the list, the loop feeds the result back, and the model stops.
func (a *AnalyticsAgent) plan(ctx context.Context, question string) ([]Todo, error) {
	messages := []map[string]any{{"role": "user", "content": question}}
	var todos []Todo
	for hop := 0; hop < 10; hop++ {
		msg, err := a.call(ctx, "plan", a.prompt, messages)
		if err != nil {
			return nil, err
		}
		uses := msg.ToolUses()
		if len(uses) == 0 {
			break
		}
		messages = append(messages, assistantToolUse(msg))
		var blocks []any
		for _, tu := range uses {
			out := ""
			if tu.Name == "TodoWrite" {
				todos = parseTodos(tu.Input["todos"])
				out = fmt.Sprintf("Todos updated (0/%d done):\n%s", len(todos), renderTodos(todos))
			} else {
				out = fmt.Sprintf("Error: unknown tool %q.", tu.Name)
			}
			blocks = append(blocks, toolResult(tu.ID, out))
		}
		messages = append(messages, map[string]any{"role": "user", "content": blocks})
	}
	return todos, nil
}

// supervise picks the execution structure: sequential when any todo declares
// deps, else fanout (independent). Mirrors the plan_supervise lobe.
func supervise(todos []Todo) string {
	for _, t := range todos {
		if len(t.Deps) > 0 {
			return "sequential"
		}
	}
	return "fanout"
}

// fanout runs one isolated subagent per todo. Each subagent runs the execute
// stage scoped to its own prompt + sql tool, runs its REAL SQL, and reports its
// headline row.
func (a *AnalyticsAgent) fanout(ctx context.Context, todos []Todo) ([]string, error) {
	results := make([]string, 0, len(todos))
	for _, t := range todos {
		out, err := a.runSubagent(ctx, t)
		if err != nil {
			return nil, err
		}
		results = append(results, out)
	}
	return results, nil
}

// runSubagent runs one isolated execute-stage ReAct loop: the model calls sql,
// the tool runs the real query, the loop feeds the rows back, the model reports.
func (a *AnalyticsAgent) runSubagent(ctx context.Context, t Todo) (string, error) {
	// Each subagent's transcript starts fresh (isolation) with its own sub-task.
	messages := []map[string]any{{"role": "user", "content": t.Prompt}}
	final := ""
	for hop := 0; hop < 12; hop++ {
		msg, err := a.call(ctx, "execute", a.prompt, messages)
		if err != nil {
			return "", err
		}
		if txt := msg.Text(); txt != "" {
			final = txt
		}
		uses := msg.ToolUses()
		if len(uses) == 0 {
			break
		}
		messages = append(messages, assistantToolUse(msg))
		var blocks []any
		for _, tu := range uses {
			out := ""
			if tu.Name == "sql" {
				out, err = a.sqlrt.CallTool(ctx, "sql", tu.Input, nil, nil)
				if err != nil {
					return "", err
				}
			} else {
				out = fmt.Sprintf("Error: unknown tool %q.", tu.Name)
			}
			blocks = append(blocks, toolResult(tu.ID, out))
		}
		messages = append(messages, map[string]any{"role": "user", "content": blocks})
	}
	return final, nil
}

// fanin aggregates the subagent results into one executive summary. The results
// are rendered into the fan-in system prompt (the plan_results lobe), so the
// scripted model reads its findings from there.
func (a *AnalyticsAgent) fanin(ctx context.Context, subResults []string) (string, error) {
	system := a.prompt + "\n\nSubagent results:\n" + strings.Join(subResults, "\n")
	messages := []map[string]any{{"role": "user", "content": "Combine the results into an executive summary."}}
	msg, err := a.call(ctx, "fanin", system, messages)
	if err != nil {
		return "", err
	}
	return msg.Text(), nil
}

// call invokes the client for one stage and returns the Message.
func (a *AnalyticsAgent) call(ctx context.Context, stage, system string, messages []map[string]any) (clients.Message, error) {
	specs := a.sqlrt.GetToolSpecs()
	raw, err := a.client.Call(ctx, clients.Request{
		Stage:    stage,
		System:   system,
		Messages: messages,
		Tools:    specs,
	})
	if err != nil {
		return clients.Message{}, err
	}
	msg, ok := raw.(clients.Message)
	if !ok {
		return clients.Message{}, fmt.Errorf("client returned %T, want clients.Message", raw)
	}
	return msg, nil
}

// assistantToolUse renders the model's tool_use turn as a message map.
func assistantToolUse(msg clients.Message) map[string]any {
	var content []any
	if txt := msg.Text(); txt != "" {
		content = append(content, map[string]any{"type": "text", "text": txt})
	}
	for _, tu := range msg.ToolUses() {
		content = append(content, map[string]any{
			"type": "tool_use", "id": tu.ID, "name": tu.Name, "input": tu.Input,
		})
	}
	return map[string]any{"role": "assistant", "content": content}
}

// toolResult renders one tool_result content block.
func toolResult(id, output string) map[string]any {
	return map[string]any{"type": "tool_result", "tool_use_id": id, "content": output}
}

// parseTodos converts the loose TodoWrite input list into typed Todos.
func parseTodos(raw any) []Todo {
	list, ok := raw.([]any)
	if !ok {
		// the scripted plan passes []map[string]any directly
		if ml, ok2 := raw.([]map[string]any); ok2 {
			out := make([]Todo, 0, len(ml))
			for _, m := range ml {
				out = append(out, todoFromMap(m))
			}
			return out
		}
		return nil
	}
	out := make([]Todo, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, todoFromMap(m))
		}
	}
	return out
}

func todoFromMap(m map[string]any) Todo {
	t := Todo{
		Content:    asString(m["content"]),
		Status:     asString(m["status"]),
		ActiveForm: asString(m["activeForm"]),
		Prompt:     asString(m["prompt"]),
	}
	if t.Status == "" {
		t.Status = "pending"
	}
	if t.ActiveForm == "" {
		t.ActiveForm = t.Content
	}
	t.Tools = toStrSlice(m["tools"])
	t.Deps = toIntSlice(m["deps"])
	return t
}

func toStrSlice(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func toIntSlice(v any) []int {
	switch x := v.(type) {
	case []int:
		return x
	case []any:
		out := make([]int, 0, len(x))
		for _, e := range x {
			switch n := e.(type) {
			case int:
				out = append(out, n)
			case int64:
				out = append(out, int(n))
			case float64:
				out = append(out, int(n))
			}
		}
		return out
	}
	return nil
}

// renderTodos renders the todo list as a checklist block (mirrors the planning
// tool's render_todos so the plan-stage tool result matches the Python shape).
func renderTodos(todos []Todo) string {
	if len(todos) == 0 {
		return "(no todos yet)"
	}
	mark := map[string]string{"pending": " ", "in_progress": "~", "completed": "x"}
	var lines []string
	for i, t := range todos {
		status := t.Status
		if status == "" {
			status = "pending"
		}
		label := t.Content
		if status == "in_progress" {
			label = t.ActiveForm
		}
		suffix := ""
		if len(t.Tools) > 0 {
			suffix += "  · tools: " + strings.Join(t.Tools, ", ")
		}
		if len(t.Deps) > 0 {
			ds := make([]string, len(t.Deps))
			for j, d := range t.Deps {
				ds[j] = fmt.Sprintf("%d", d)
			}
			suffix += "  · needs " + strings.Join(ds, ", ")
		}
		lines = append(lines, fmt.Sprintf("%d. [%s] %s%s", i+1, mark[status], label, suffix))
	}
	return strings.Join(lines, "\n")
}
