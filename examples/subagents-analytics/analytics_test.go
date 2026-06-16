package subagentsanalytics

import (
	"context"
	"strings"
	"testing"
)

const question = "Give me a 2024 sales review in three parts — (1) the top products by revenue, (2) total " +
	"revenue by region, and (3) the monthly revenue trend — then combine them into an executive " +
	"summary."

// TestFixtureIsDeterministicAndTrendBearing verifies the SQLite fixture holds
// 4 regions × 4 products × 6 months = 96 rows, with a built-in ~6% monthly
// growth trend (revenue for a fixed region/product rises month over month).
func TestFixtureIsDeterministicAndTrendBearing(t *testing.T) {
	db := BuildDB(t)
	defer db.Close()

	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM sales").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 96 {
		t.Fatalf("row count = %d, want 96", n)
	}

	// The monthly total revenue should be strictly increasing (the growth trend).
	rows, err := db.Query(
		"SELECT substr(order_date,1,7) AS month, SUM(revenue) FROM sales GROUP BY month ORDER BY month")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var prev float64 = -1
	var months int
	for rows.Next() {
		var m string
		var rev float64
		if err := rows.Scan(&m, &rev); err != nil {
			t.Fatal(err)
		}
		if rev <= prev {
			t.Fatalf("month %s revenue %.2f not greater than prev %.2f (no trend)", m, rev, prev)
		}
		prev = rev
		months++
	}
	if months != 6 {
		t.Fatalf("month count = %d, want 6", months)
	}
}

// TestSqlToolRunsRealSelect verifies the sql tool runs a real SELECT against the
// fixture and refuses non-SELECT (read-only) queries.
func TestSqlToolRunsRealSelect(t *testing.T) {
	db := BuildDB(t)
	defer db.Close()
	rt := NewSqlToolRuntime(db)

	specs := rt.GetToolSpecs()
	if len(specs) != 1 || specs[0]["name"] != "sql" {
		t.Fatalf("expected one sql tool, got %v", specs)
	}

	out, err := rt.CallTool(context.Background(), "sql",
		map[string]any{"query": "SELECT product, ROUND(SUM(revenue)) AS revenue FROM sales " +
			"GROUP BY product ORDER BY revenue DESC LIMIT 3"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "rows:") {
		t.Fatalf("sql output should start with rows:, got %q", out)
	}
	if !strings.Contains(out, "product | revenue") {
		t.Fatalf("sql output missing header: %q", out)
	}

	bad, _ := rt.CallTool(context.Background(), "sql",
		map[string]any{"query": "DELETE FROM sales"}, nil, nil)
	if !strings.Contains(bad, "only read-only SELECT") {
		t.Fatalf("non-SELECT should be refused, got %q", bad)
	}
}

// TestPlanFansOutThreeSubagents verifies the scripted model plans 3 independent
// analyses (TodoWrite), the supervisor picks fanout (no deps), and one subagent
// runs per todo — each running its own REAL SQL.
func TestPlanFansOutThreeSubagents(t *testing.T) {
	db := BuildDB(t)
	defer db.Close()
	agent := BuildAnalyticsAgent(db, MakeFakeClient())

	res, err := agent.Query(context.Background(), question)
	if err != nil {
		t.Fatal(err)
	}

	if got := len(res.Todos); got != 3 {
		t.Fatalf("planned %d todos, want 3", got)
	}
	if res.PlanStructure != "fanout" {
		t.Fatalf("plan structure = %q, want fanout (no deps)", res.PlanStructure)
	}
	if got := len(res.SubagentResults); got != 3 {
		t.Fatalf("ran %d subagents, want 3", got)
	}
	// each subagent ran its own real SQL (a top row from the fixture)
	for i, sr := range res.SubagentResults {
		if !strings.Contains(sr, "Result:") {
			t.Fatalf("subagent %d result missing Result: %q", i, sr)
		}
	}
}

// TestFaninProducesExecutiveSummary verifies the fan-in step combines the three
// subagent results into one executive summary that names the takeaway.
func TestFaninProducesExecutiveSummary(t *testing.T) {
	db := BuildDB(t)
	defer db.Close()
	agent := BuildAnalyticsAgent(db, MakeFakeClient())

	res, err := agent.Query(context.Background(), question)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "EXECUTIVE SUMMARY") {
		t.Fatalf("summary missing header: %q", res.Text)
	}
	if !strings.Contains(strings.ToLower(res.Text), "upward monthly trend") {
		t.Fatalf("summary missing the trend takeaway: %q", res.Text)
	}
	if res.Status != "answered" {
		t.Fatalf("status = %q, want answered", res.Status)
	}
	// the summary bullets carry the three concrete subagent findings
	bullets := strings.Count(res.Text, "•")
	if bullets != 3 {
		t.Fatalf("expected 3 finding bullets, got %d in:\n%s", bullets, res.Text)
	}
}

// TestSubagentQueriesAreRoutedByKeyword verifies each todo's SQL is chosen by a
// keyword in its sub-task (product / region / month), so the three subagents run
// three distinct real analyses.
func TestSubagentQueriesAreRoutedByKeyword(t *testing.T) {
	if got := queryFor("total revenue by region, then report it"); !strings.Contains(got, "GROUP BY region") {
		t.Fatalf("region sub-task routed to %q", got)
	}
	if got := queryFor("the monthly revenue trend across 2024"); !strings.Contains(got, "substr(order_date") {
		t.Fatalf("monthly sub-task routed to %q", got)
	}
	if got := queryFor("the top 3 products by total revenue"); !strings.Contains(got, "GROUP BY product") {
		t.Fatalf("product sub-task routed to %q", got)
	}
}
