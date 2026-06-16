package subagentsanalytics

import (
	"strings"

	"github.com/nccasia/agent-sdk-go/agent_sdk/clients"
)

// Todo is one planned analysis — a designed pipeline step with its own prompt +
// tools (the subset the spawned subagent runs with). Mirrors the dicts in
// analytics.fakes.TODOS.
type Todo struct {
	Content    string   `json:"content"`
	Status     string   `json:"status"`
	ActiveForm string   `json:"activeForm"`
	Prompt     string   `json:"prompt"`
	Tools      []string `json:"tools"`
	Deps       []int    `json:"deps,omitempty"`
}

// PlannedTodos are the 3 independent analyses the scripted model writes with
// TodoWrite. No deps ⇒ the supervisor picks fanout (one subagent per todo).
// Mirrors analytics.fakes.TODOS.
var PlannedTodos = []Todo{
	{
		Content:    "Find the top 3 products by total revenue",
		Status:     "pending",
		ActiveForm: "Finding the top products by revenue",
		Prompt:     "Run one SQL query to find the top 3 products by total revenue, then report them.",
		Tools:      []string{"sql"},
	},
	{
		Content:    "Compute total revenue by region",
		Status:     "pending",
		ActiveForm: "Computing revenue by region",
		Prompt:     "Run one SQL query to total revenue by region, then report it.",
		Tools:      []string{"sql"},
	},
	{
		Content:    "Compute the monthly revenue trend across 2024",
		Status:     "pending",
		ActiveForm: "Computing the monthly revenue trend",
		Prompt:     "Run one SQL query for the monthly revenue trend across 2024, then report it.",
		Tools:      []string{"sql"},
	},
}

// queryByKeyword maps a sub-task keyword to its REAL SQL over the fixture.
// Mirrors analytics.fakes._QUERY_BY_KEYWORD.
var queryByKeyword = map[string]string{
	"product": "SELECT product, ROUND(SUM(revenue)) AS revenue FROM sales " +
		"GROUP BY product ORDER BY revenue DESC LIMIT 3",
	"region": "SELECT region, ROUND(SUM(revenue)) AS revenue FROM sales " +
		"GROUP BY region ORDER BY revenue DESC",
	"monthly": "SELECT substr(order_date,1,7) AS month, ROUND(SUM(revenue)) AS revenue FROM sales " +
		"GROUP BY month ORDER BY month",
}

// queryFor picks a sub-task's SQL by a keyword in its prompt. Mirrors
// analytics.fakes._query_for.
func queryFor(subtask string) string {
	low := strings.ToLower(subtask)
	if strings.Contains(low, "region") {
		return queryByKeyword["region"]
	}
	if strings.Contains(low, "month") || strings.Contains(low, "trend") {
		return queryByKeyword["monthly"]
	}
	return queryByKeyword["product"]
}

// topRow returns the first data row of an SQL tool result (the headline figure).
// Mirrors analytics.fakes._top_row.
func topRow(sqlOut string) string {
	var rows []string
	for _, ln := range strings.Split(sqlOut, "\n") {
		if strings.Contains(ln, "|") {
			rows = append(rows, ln)
		}
	}
	if len(rows) >= 2 {
		return rows[1]
	}
	return "(no rows)"
}

// MakeFakeClient builds the scripted, offline-deterministic model that drives
// the plan / execute / fanin stages. Control flow keys on the stage id + message
// structure, never on sniffing free text. Mirrors analytics.fakes.make_fake_client.
func MakeFakeClient() *clients.FakeClient {
	model := func(stageID, system string, messages []map[string]any, _ []map[string]any) any {
		last := ""
		if len(messages) > 0 {
			last = asString(messages[len(messages)-1]["content"])
		}
		results := sqlResults(messages)

		switch stageID {
		case "plan":
			// Write the 3-step plan, then stop the planning loop.
			if strings.Contains(last, "Todos updated") {
				return "Plan ready — 3 independent analyses."
			}
			return map[string]any{"tools": []map[string]any{
				{"name": "TodoWrite", "input": map[string]any{"todos": todosAsMaps(PlannedTodos)}},
			}}
		case "execute":
			// Each subagent runs its own SQL (isolated), then reports its row.
			if len(results) == 0 {
				return map[string]any{"tools": []map[string]any{
					{"name": "sql", "input": map[string]any{"query": queryFor(last)}},
				}}
			}
			return "Result: " + topRow(results[len(results)-1])
		}
		// fanin — aggregate the subagents' results (rendered into `system`).
		return faninSummary(system)
	}
	return clients.Scripted(model)
}

// faninSummary builds the executive summary from the per-subagent "Result:" rows
// carried in the fan-in system prompt. Mirrors analytics.fakes (the fanin branch).
func faninSummary(system string) string {
	seen := map[string]struct{}{}
	var bullets []string
	for _, ln := range strings.Split(system, "\n") {
		if idx := strings.Index(ln, "Result:"); idx >= 0 {
			val := strings.TrimSpace(ln[idx+len("Result:"):])
			if val == "" {
				continue
			}
			if _, dup := seen[val]; dup {
				continue
			}
			seen[val] = struct{}{}
			bullets = append(bullets, "  • "+val)
		}
	}
	findings := strings.Join(bullets, "\n")
	if findings == "" {
		findings = "  • (results in the per-subagent timelines)"
	}
	return "EXECUTIVE SUMMARY (2024 sales)\n" + findings +
		"\n\nTakeaway: revenue concentrates in the top product and leading region, with a " +
		"clear upward monthly trend."
}

// sqlResults returns every SQL tool-result text already in the transcript (each
// starts with "rows:"). Mirrors analytics.fakes._sql_results.
func sqlResults(messages []map[string]any) []string {
	var out []string
	for _, m := range messages {
		blocks, ok := m["content"].([]any)
		if !ok {
			continue
		}
		for _, b := range blocks {
			bm, ok := b.(map[string]any)
			if !ok {
				continue
			}
			if bm["type"] != "tool_result" {
				continue
			}
			txt := asString(bm["content"])
			if strings.Contains(txt, "rows:") {
				out = append(out, txt)
			}
		}
	}
	return out
}

// todosAsMaps renders the typed todos as the loose maps the TodoWrite tool input
// carries (so the scripted plan matches the Python dict shape).
func todosAsMaps(todos []Todo) []map[string]any {
	out := make([]map[string]any, len(todos))
	for i, t := range todos {
		m := map[string]any{
			"content":    t.Content,
			"status":     t.Status,
			"activeForm": t.ActiveForm,
			"prompt":     t.Prompt,
			"tools":      t.Tools,
		}
		if len(t.Deps) > 0 {
			m["deps"] = t.Deps
		}
		out[i] = m
	}
	return out
}
