// Package subagentsanalytics is the Go port of the Python
// examples/subagents-analytics example: a data-analyst agent that plans a
// multi-part analytics question with TodoWrite, fans out one isolated subagent
// per independent todo (each running its own REAL SQL against an in-memory
// SQLite fixture), then aggregates the results into an executive summary.
//
// It runs offline-deterministic via a scripted FakeClient — the reasoning is
// scripted, but every SQL query is real (modernc.org/sqlite, pure-Go).
package subagentsanalytics

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// regions, products (product, category, unit_price), and months mirror the
// Python fixture's deterministic data grid (4 × 4 × 6 = 96 rows).
var regions = []string{"North", "South", "East", "West"}

type product struct {
	name     string
	category string
	price    int
}

var products = []product{
	{"Widget", "Hardware", 20},
	{"Gadget", "Hardware", 35},
	{"Gizmo", "Accessory", 50},
	{"Doohickey", "Accessory", 15},
}

var months = []string{"2024-01", "2024-02", "2024-03", "2024-04", "2024-05", "2024-06"}

// BuildDB creates an in-memory sales DB with deterministic, trend-bearing data,
// registering cleanup with the test. Mirrors analytics.fixture.build_db.
func BuildDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := openDB()
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// openDB is the cleanup-free constructor (used by demo / non-test callers).
func openDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	// An in-memory database lives only as long as its single connection.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(
		"CREATE TABLE sales (id INTEGER PRIMARY KEY, region TEXT, product TEXT, " +
			"category TEXT, units INTEGER, revenue REAL, order_date TEXT)"); err != nil {
		db.Close()
		return nil, err
	}
	rid := 0
	for ri, region := range regions {
		for pi, p := range products {
			for mi, month := range months {
				rid++
				units := 10 + (ri*7+pi*5+mi*3)%40
				// ~6% monthly growth, rounded to 2 dp (round half away from zero).
				revenue := round2(float64(units) * float64(p.price) * (1 + float64(mi)*0.06))
				if _, err := db.Exec(
					"INSERT INTO sales VALUES (?,?,?,?,?,?,?)",
					rid, region, p.name, p.category, units, revenue, fmt.Sprintf("%s-15", month)); err != nil {
					db.Close()
					return nil, err
				}
			}
		}
	}
	return db, nil
}

// round2 rounds to 2 decimal places (matching Python's round(x, 2) for the
// magnitudes used here).
func round2(x float64) float64 { return math.Round(x*100) / 100 }

// SqlToolRuntime exposes one read-only `sql` tool over the analytics DB.
// Mirrors analytics.fixture.SqlToolRuntime.
type SqlToolRuntime struct {
	db *sql.DB
}

// NewSqlToolRuntime wraps a DB as the `sql` tool runtime.
func NewSqlToolRuntime(db *sql.DB) *SqlToolRuntime { return &SqlToolRuntime{db: db} }

// Name identifies the runtime.
func (r *SqlToolRuntime) Name() string { return "sql" }

// GetToolSpecs returns the single `sql` tool spec.
func (r *SqlToolRuntime) GetToolSpecs() []map[string]any {
	return []map[string]any{
		{
			"name": "sql",
			"description": "Run a read-only SQL SELECT against the analytics database and get the rows " +
				"back as a table. Schema: sales(region, product, category, units, revenue, " +
				"order_date). One query per call.",
			"input_schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "a SELECT query"},
				},
				"required": []string{"query"},
			},
		},
	}
}

// CallTool runs a read-only SELECT and renders up to 20 rows as a table.
// Mirrors analytics.fixture.SqlToolRuntime.call_tool.
func (r *SqlToolRuntime) CallTool(
	ctx context.Context, name string, inp map[string]any,
	_ []map[string]any, _ map[string]struct{},
) (string, error) {
	q := strings.TrimSpace(asString(inp["query"]))
	if !strings.HasPrefix(strings.ToLower(q), "select") {
		return "Error: only read-only SELECT queries are allowed.", nil
	}
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}
	lines := []string{strings.Join(cols, " | ")}
	count := 0
	for rows.Next() && count < 20 {
		cells := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range cells {
			ptrs[i] = &cells[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return fmt.Sprintf("Error: %v", err), nil
		}
		parts := make([]string, len(cols))
		for i, c := range cells {
			parts[i] = renderCell(c)
		}
		lines = append(lines, strings.Join(parts, " | "))
		count++
	}
	return "rows:\n" + strings.Join(lines, "\n"), nil
}

// renderCell formats a scanned SQL value the way Python's str() would for the
// fixture's column types (ints without a trailing .0, floats compactly).
func renderCell(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case int64:
		return fmt.Sprintf("%d", x)
	case float64:
		if x == math.Trunc(x) {
			return fmt.Sprintf("%.1f", x)
		}
		return fmt.Sprintf("%g", x)
	case []byte:
		return string(x)
	case string:
		return x
	default:
		return fmt.Sprintf("%v", x)
	}
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
