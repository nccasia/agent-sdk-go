package benchmarks

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/nccasia/agent-sdk-go/agent_sdk/agent"
	"github.com/nccasia/agent-sdk-go/agent_sdk/plugins/tasks"
	"github.com/nccasia/agent-sdk-go/agent_sdk/probe"
	"github.com/nccasia/agent-sdk-go/agent_sdk/tools"
)

// taskbench — can the SDK drive a LIVE model to PLAN and SOLVE realistic
// multi-step tasks? No mocks, no scripted model: the agent works over a seeded
// SQLite DB with REAL tools — db.schema / db.query (a wrong query really errors)
// — plus the opt-in TaskPlugin's checklist (todos) + deliver. The flow/stages
// are the DRIVER we grade; the final answer is graded against ground truth
// computed from each task's reference SQL (taskBenchGroundTruth), so nothing is
// hand-entered.
//
// taskbench is LIVE only. Each capability becomes a mode cap{N}_{name} carrying
// per-task gating checks {task_id}.{answered|answer_correct|bounded}, plus the
// cross-cutting cap2_drive_to_completion. cap10_error_recovery is UNMEASURED
// when no db.query errored this run. Without a provider (the deterministic
// floor) the capability matrix is empty — every mode is missing — so the verdict
// is UNMEASURED (no evidence is never READY), mirroring run.py's refusal to run
// without a provider token (exit 2). Ported from benchmarks/taskbench/run.py.

// taskBenchHopCeiling bounds the per-task agentic loop. Mirrors HOP_CEILING.
const taskBenchHopCeiling = 70

// taskBenchCapNames maps a capability id to its matrix label suffix. Mirrors
// run.py CAP_NAMES.
var taskBenchCapNames = map[int]string{
	1: "decompose", 2: "drive_to_completion", 3: "state_carry", 4: "tool_orchestration",
	5: "predefined_fastpath", 6: "dependency_order", 8: "branching", 10: "error_recovery",
	11: "long_horizon",
}

// taskBenchTask is one dataset row (dataset/tasks.jsonl).
type taskBenchTask struct {
	ID         string
	Capability int
	Question   string
	AnswerSQL  string
}

// taskBenchTasks returns the committed task dataset (mirrors tasks.jsonl).
func taskBenchTasks() []taskBenchTask {
	return []taskBenchTask{
		{ID: "top3-products", Capability: 1,
			Question:  "What are the names of the top 3 products by total revenue, where revenue = quantity * unit_price summed across all order_items?",
			AnswerSQL: "SELECT p.name FROM order_items oi JOIN products p ON p.id=oi.product_id GROUP BY p.id ORDER BY SUM(oi.quantity*oi.unit_price) DESC LIMIT 3"},
		{ID: "top-country", Capability: 4,
			Question:  "Which country has the most customers, and how many customers does it have?",
			AnswerSQL: "SELECT country, COUNT(*) AS n FROM customers GROUP BY country ORDER BY n DESC LIMIT 1"},
		{ID: "top-customer-spend", Capability: 3,
			Question:  "Identify the single customer with the highest total spend (sum of quantity*unit_price across all their orders). Report the customer's name and their total spend.",
			AnswerSQL: "SELECT cu.name, ROUND(SUM(oi.quantity*oi.unit_price),2) AS spend FROM order_items oi JOIN orders o ON o.id=oi.order_id JOIN customers cu ON cu.id=o.customer_id GROUP BY cu.id ORDER BY spend DESC LIMIT 1"},
		{ID: "completed-revenue", Capability: 2,
			Question:  "What is the total revenue (sum of quantity*unit_price) from orders whose status is 'completed'?",
			AnswerSQL: "SELECT ROUND(SUM(oi.quantity*oi.unit_price),2) FROM order_items oi JOIN orders o ON o.id=oi.order_id WHERE o.status='completed'"},
		{ID: "top-customer-distinct-products", Capability: 6,
			Question:  "First find the top-spending customer (by total quantity*unit_price). Then report how many DISTINCT products that customer has purchased.",
			AnswerSQL: "WITH top AS (SELECT o.customer_id cid FROM order_items oi JOIN orders o ON o.id=oi.order_id GROUP BY o.customer_id ORDER BY SUM(oi.quantity*oi.unit_price) DESC LIMIT 1) SELECT COUNT(DISTINCT oi.product_id) FROM order_items oi JOIN orders o ON o.id=oi.order_id WHERE o.customer_id IN (SELECT cid FROM top)"},
		{ID: "conditional-category", Capability: 8,
			Question:  "If total revenue across all order_items exceeds 200000, report the product category with the HIGHEST total revenue; otherwise report the category with the LOWEST total revenue. Name the category.",
			AnswerSQL: "SELECT CASE WHEN (SELECT SUM(quantity*unit_price) FROM order_items) > 200000 THEN (SELECT p.category FROM order_items oi JOIN products p ON p.id=oi.product_id GROUP BY p.category ORDER BY SUM(oi.quantity*oi.unit_price) DESC LIMIT 1) ELSE (SELECT p.category FROM order_items oi JOIN products p ON p.id=oi.product_id GROUP BY p.category ORDER BY SUM(oi.quantity*oi.unit_price) ASC LIMIT 1) END"},
		{ID: "h2-noncancelled-revenue", Capability: 11,
			Question:  "What is the total revenue (quantity*unit_price) from orders placed in the second half of 2023 (July through December) that were NOT cancelled?",
			AnswerSQL: "SELECT ROUND(SUM(oi.quantity*oi.unit_price),2) FROM order_items oi JOIN orders o ON o.id=oi.order_id WHERE o.order_date >= '2023-07-01' AND o.status <> 'cancelled'"},
		{ID: "standard-sales-summary", Capability: 5,
			Question:  "Run the standard sales summary report: (1) total revenue across ALL order items (do not filter by order status), (2) the number of orders with status 'completed', (3) the single top product by revenue. Report all three values.",
			AnswerSQL: "SELECT (SELECT ROUND(SUM(quantity*unit_price),2) FROM order_items) AS total_rev, (SELECT COUNT(*) FROM orders WHERE status='completed') AS completed_orders, (SELECT p.name FROM order_items oi JOIN products p ON p.id=oi.product_id GROUP BY p.id ORDER BY SUM(oi.quantity*oi.unit_price) DESC LIMIT 1) AS top_product"},
		{ID: "standard-top-customer-report", Capability: 5,
			Question:  "Run the standard top-customer report: (1) find the customer with the highest total spend, then (2) report that customer's name and how many orders they placed.",
			AnswerSQL: "WITH top AS (SELECT o.customer_id cid, cu.name nm FROM order_items oi JOIN orders o ON o.id=oi.order_id JOIN customers cu ON cu.id=o.customer_id GROUP BY o.customer_id ORDER BY SUM(oi.quantity*oi.unit_price) DESC LIMIT 1) SELECT (SELECT nm FROM top) AS name, (SELECT COUNT(*) FROM orders WHERE customer_id=(SELECT cid FROM top)) AS norders"},
	}
}

// taskBenchGatingCheckIDs is the static gating check-id surface — the three
// gating ids run.py:score() emits per task (the diag ids routed_task / planned
// are shown, not gating). Asserted for cross-language parity independent of the
// provider.
func taskBenchGatingCheckIDs() []string {
	return []string{"answered", "answer_correct", "bounded"}
}

// taskBenchModeNames is the capability matrix mode labels build_matrix emits
// when every capability is measured: cap{N}_{name} per dataset capability plus
// the cross-cutting cap2_drive_to_completion. cap10_error_recovery is left out
// (UNMEASURED unless a db.query errored). Asserted for parity.
func taskBenchModeNames() []string {
	seen := map[string]struct{}{}
	for _, t := range taskBenchTasks() {
		seen["cap"+strconv.Itoa(t.Capability)+"_"+taskBenchCapNames[t.Capability]] = struct{}{}
	}
	seen["cap2_drive_to_completion"] = struct{}{}
	out := sortedSet(seen)
	return out
}

// RunTaskBench composes the taskbench verdict. With no model (the deterministic
// floor) the capability matrix is empty → every mode missing → UNMEASURED. With
// a model the real task agent is driven over each task and the final answer
// graded vs reference SQL.
func RunTaskBench(ctx context.Context, model string) (Verdict, error) {
	payloads := map[string]*ModePayload{}
	for _, m := range taskBenchModeNames() {
		payloads[m] = nil // missing without a provider
	}
	record := map[string][]string{}
	if model != "" {
		measured, err := taskBenchLive(ctx, model)
		if err != nil {
			return Verdict{}, err
		}
		for m, p := range measured {
			payloads[m] = p
		}
	}
	return ComposeVerdict(payloads, record), nil
}

// RunTaskBenchProbes captures inspectable traces for the viewer. With a real
// model it drives the actual task agent; offline (model=="") it builds the SAME
// representative agent (TaskPlugin + the real db.schema/db.query tools over a
// seeded SQLite DB) against a FakeClient and runs ONE representative task through
// probe.Probe, so the inspection renders a real path/flow + the executed stages.
// Adds traces only — the live verdict (Run) stays UNMEASURED without a provider.
// Mirrors run.py's live probe feeding write_viewer.
func RunTaskBenchProbes(ctx context.Context, model string) ([]*probe.Record, error) {
	taskList := taskBenchTasks()
	if len(taskList) == 0 {
		return nil, nil
	}
	db, err := taskBenchBuildDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	store := &taskBenchStore{db: db}
	ag := agent.MustPreactAgent(agent.Config{
		Client:        benchProbeClient(model),
		Instructions:  taskBenchInstructions(),
		Plugins:       []any{tasks.NewTaskPlugin(), store.plugin()},
		Funnel:        true,
		ToolsInPrompt: true,
		Budgets:       map[string]any{"stall_patience": 4},
	})
	rec, err := probe.Probe(ctx, ag, taskList[0].Question, probe.WithLabel("live · "+taskList[0].ID))
	if err != nil {
		return nil, err
	}
	return []*probe.Record{rec}, nil
}

// ── live run ─────────────────────────────────────────────────────────────────

// taskBenchResult holds one task's measured outcome.
type taskBenchResult struct {
	task        taskBenchTask
	gating      []Check
	answered    bool
	correct     bool
	queryErrors int
}

// taskBenchLive drives the real task agent over every task and folds the results
// into the capability matrix (mirrors run.py:build_matrix). Each capability's
// mode carries that capability's per-task gating checks; cap2_drive_to_completion
// aggregates answered across all tasks; cap10_error_recovery only gates when a
// db.query errored this run.
func taskBenchLive(ctx context.Context, model string) (map[string]*ModePayload, error) {
	results := []taskBenchResult{}
	for _, task := range taskBenchTasks() {
		r, err := taskBenchRunTask(ctx, model, task)
		if err != nil {
			return nil, err
		}
		results = append(results, r)
	}

	byCap := map[int][]taskBenchResult{}
	for _, r := range results {
		byCap[r.task.Capability] = append(byCap[r.task.Capability], r)
	}
	out := map[string]*ModePayload{}
	caps := make([]int, 0, len(byCap))
	for c := range byCap {
		caps = append(caps, c)
	}
	sort.Ints(caps)
	for _, cap := range caps {
		label := "cap" + strconv.Itoa(cap) + "_" + taskBenchCapNames[cap]
		checks := []Check{}
		for _, r := range byCap[cap] {
			for _, c := range r.gating {
				checks = append(checks, ck(r.task.ID+"."+c.ID, c.OK, c.Detail))
			}
		}
		out[label] = NewPayload(checks, nil)
	}

	// cross-cutting: cap2_drive_to_completion = every task answered.
	drive := []Check{}
	for _, r := range results {
		drive = append(drive, ck(r.task.ID+".answered", r.answered, ""))
	}
	out["cap2_drive_to_completion"] = NewPayload(drive, nil)

	// cap10_error_recovery only gates if a db.query errored this run.
	errRows := []taskBenchResult{}
	for _, r := range results {
		if r.queryErrors > 0 {
			errRows = append(errRows, r)
		}
	}
	if len(errRows) > 0 {
		rec := []Check{}
		for _, r := range errRows {
			rec = append(rec, ck(r.task.ID+".recovered", r.correct, ""))
		}
		out["cap10_error_recovery"] = NewPayload(rec, nil)
	}
	return out, nil
}

// taskBenchRunTask drives the real task agent over one task and scores it.
func taskBenchRunTask(ctx context.Context, model string, task taskBenchTask) (taskBenchResult, error) {
	db, err := taskBenchBuildDB()
	if err != nil {
		return taskBenchResult{}, err
	}
	defer db.Close()
	store := &taskBenchStore{db: db}

	ag := agent.MustPreactAgent(agent.Config{
		Client:        model,
		Instructions:  taskBenchInstructions(),
		Plugins:       []any{tasks.NewTaskPlugin(), store.plugin()},
		Funnel:        true,
		ToolsInPrompt: true,
		Budgets:       map[string]any{"stall_patience": 4},
	})
	rec, err := probe.Probe(ctx, ag, task.Question, probe.WithLabel("live · "+task.ID))
	if err != nil {
		return taskBenchResult{}, err
	}

	facts, err := taskBenchGroundTruth(db, task.AnswerSQL)
	if err != nil {
		return taskBenchResult{}, err
	}
	answer := strings.TrimSpace(rec.Answer)
	correct, factDetail := taskBenchGradeAnswer(rec.Answer, facts)
	hops := len(rec.LlmCalls)
	trunc := 0
	for _, m := range rec.MetaActions {
		if a, _ := m["action"].(string); a == "truncated_final" {
			trunc++
		}
	}
	nErr := 0
	for _, q := range store.queries {
		if _, ok := q["error"]; ok {
			nErr++
		}
	}

	missing := []string{}
	for _, d := range factDetail {
		if !d.OK {
			missing = append(missing, fmt.Sprint(d.Fact))
		}
	}
	gating := []Check{
		ck("answered", answer != "", fmt.Sprintf("answer_len=%d", len(answer))),
		ck("answer_correct", correct, taskBenchCorrectDetail(correct, missing)),
		ck("bounded", hops <= taskBenchHopCeiling && trunc == 0,
			fmt.Sprintf("hops=%d≤%d truncated=%d queries=%d", hops, taskBenchHopCeiling, trunc, len(store.queries))),
	}
	return taskBenchResult{task: task, gating: gating, answered: answer != "", correct: correct, queryErrors: nErr}, nil
}

func taskBenchCorrectDetail(correct bool, missing []string) string {
	if correct {
		return "all facts present"
	}
	return "missing=[" + strings.Join(missing, " ") + "]"
}

func taskBenchInstructions() string {
	return "You answer questions over a real SQLite database by planning a checklist of steps and " +
		"executing each with SQL. Compute every fact with db.query (read-only SELECT) — never guess " +
		"values or column names. When all steps are done, state the final answer with the concrete " +
		"values/names asked for.\n\n" + taskBenchSchemaText()
}

// ── domain plugin: real SQL over the seeded DB ───────────────────────────────

// taskBenchStore is the connection + query log the bench reads to score (errors,
// count). Mirrors sqlite_plugin.SqliteStore.
type taskBenchStore struct {
	db      *sql.DB
	queries []map[string]any
}

const taskBenchMaxRows = 50

// query runs a read-only SQL SELECT and returns the rows as text, logging each
// query (with an error key on failure). Mirrors the db.query tool body.
func (s *taskBenchStore) query(ctx context.Context, sqlText string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(sqlText), ";")
	low := strings.ToLower(trimmed)
	if !strings.HasPrefix(low, "select") && !strings.HasPrefix(low, "with") {
		return "Error: only read-only SELECT/WITH queries are allowed.", nil
	}
	rows, err := s.db.QueryContext(ctx, trimmed)
	if err != nil {
		s.queries = append(s.queries, map[string]any{"sql": trimmed, "error": err.Error()})
		return fmt.Sprintf("Error: %s. Check db.schema for exact table/column names.", err.Error()), nil
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		s.queries = append(s.queries, map[string]any{"sql": trimmed, "error": err.Error()})
		return fmt.Sprintf("Error: %s. Check db.schema for exact table/column names.", err.Error()), nil
	}
	out := []string{strings.Join(cols, " | ")}
	n := 0
	more := false
	for rows.Next() {
		if n >= taskBenchMaxRows {
			more = true
			break
		}
		cells := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range cells {
			ptrs[i] = &cells[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			s.queries = append(s.queries, map[string]any{"sql": trimmed, "error": err.Error()})
			return fmt.Sprintf("Error: %s. Check db.schema for exact table/column names.", err.Error()), nil
		}
		parts := make([]string, len(cells))
		for i, c := range cells {
			parts[i] = taskBenchCellString(c)
		}
		out = append(out, strings.Join(parts, " | "))
		n++
	}
	s.queries = append(s.queries, map[string]any{"sql": trimmed, "n": n})
	body := strings.Join(out, "\n")
	if more {
		body += fmt.Sprintf("\n… (>%d rows; add LIMIT or aggregate)", taskBenchMaxRows)
	}
	return body, nil
}

// plugin builds the SqlitePlugin (db.schema + db.query) bound to this store.
func (s *taskBenchStore) plugin() *taskBenchPlugin { return &taskBenchPlugin{store: s} }

// taskBenchPlugin installs db.schema / db.query (the bench's domain capability).
// Mirrors sqlite_plugin.SqlitePlugin.
type taskBenchPlugin struct{ store *taskBenchStore }

func (p *taskBenchPlugin) Name() string  { return "sqlite" }
func (p *taskBenchPlugin) Enabled() bool { return true }

func (p *taskBenchPlugin) Install(setup *agent.AgentSetup) {
	store := p.store
	schema := tools.Tool("db.schema", func(_ context.Context, _ map[string]any) (any, error) {
		return taskBenchSchemaText(), nil
	}, tools.Desc("Return the database schema (tables, columns, how to join + compute revenue)."))
	query := tools.Tool("db.query", func(ctx context.Context, in map[string]any) (any, error) {
		sqlText, _ := in["sql"].(string)
		return store.query(ctx, sqlText)
	}, tools.Desc("Run a read-only SQL SELECT and return the rows. Compute every fact with this."),
		tools.Param("sql", "string", true, nil))
	setup.AddTool(schema)
	setup.AddTool(query)
}

// ── grader: facts vs reference SQL ───────────────────────────────────────────

// taskBenchFactDetail is one ground-truth fact's match outcome.
type taskBenchFactDetail struct {
	Fact any
	OK   bool
}

var taskBenchNumRe = regexp.MustCompile(`-?\d[\d,]*\.?\d*`)

// taskBenchGradeAnswer returns (all_facts_present, per-fact detail): strings by
// case-insensitive containment, numbers by 3%/±1 tolerance, bools skipped.
// Mirrors grade.grade_answer.
func taskBenchGradeAnswer(submitted string, facts []any) (bool, []taskBenchFactDetail) {
	low := strings.ToLower(submitted)
	nums := taskBenchNumbers(submitted)
	detail := []taskBenchFactDetail{}
	for _, f := range facts {
		switch v := f.(type) {
		case bool:
			continue
		case int:
			detail = append(detail, taskBenchFactDetail{Fact: f, OK: taskBenchNumMatch(float64(v), nums)})
		case int64:
			detail = append(detail, taskBenchFactDetail{Fact: f, OK: taskBenchNumMatch(float64(v), nums)})
		case float64:
			detail = append(detail, taskBenchFactDetail{Fact: f, OK: taskBenchNumMatch(v, nums)})
		default:
			s := strings.TrimSpace(fmt.Sprint(f))
			detail = append(detail, taskBenchFactDetail{Fact: f, OK: s != "" && strings.Contains(low, strings.ToLower(s))})
		}
	}
	if len(detail) == 0 {
		return false, detail
	}
	for _, d := range detail {
		if !d.OK {
			return false, detail
		}
	}
	return true, detail
}

func taskBenchNumbers(text string) []float64 {
	out := []float64{}
	for _, m := range taskBenchNumRe.FindAllString(text, -1) {
		if v, err := strconv.ParseFloat(strings.ReplaceAll(m, ",", ""), 64); err == nil {
			out = append(out, v)
		}
	}
	return out
}

func taskBenchNumMatch(target float64, nums []float64) bool {
	tol := math.Max(math.Abs(target)*0.03, 1.0)
	for _, n := range nums {
		if math.Abs(n-target) <= tol {
			return true
		}
	}
	return false
}

// taskBenchGroundTruth flattens the reference query's result cells into facts.
// Mirrors grade.ground_truth.
func taskBenchGroundTruth(db *sql.DB, answerSQL string) ([]any, error) {
	rows, err := db.Query(answerSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []any{}
	for rows.Next() {
		cells := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range cells {
			ptrs[i] = &cells[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		for _, c := range cells {
			out = append(out, taskBenchNormCell(c))
		}
	}
	return out, rows.Err()
}

// taskBenchNormCell maps a scanned SQLite cell to the Go fact type the grader
// expects (int64/float64 numbers; string text; nil for NULL).
func taskBenchNormCell(c any) any {
	switch v := c.(type) {
	case []byte:
		return string(v)
	default:
		return v
	}
}

func taskBenchCellString(c any) string {
	if c == nil {
		return ""
	}
	switch v := c.(type) {
	case []byte:
		return string(v)
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		return fmt.Sprint(v)
	}
}

// ── seed: deterministic e-commerce SQLite DB ─────────────────────────────────

const taskBenchSchema = `
CREATE TABLE customers (
  id INTEGER PRIMARY KEY, name TEXT, email TEXT, country TEXT, signup_date TEXT);
CREATE TABLE products (
  id INTEGER PRIMARY KEY, name TEXT, category TEXT, price REAL);
CREATE TABLE orders (
  id INTEGER PRIMARY KEY, customer_id INTEGER, order_date TEXT, status TEXT);
CREATE TABLE order_items (
  order_id INTEGER, product_id INTEGER, quantity INTEGER, unit_price REAL);
`

var taskBenchFirst = []string{
	"Anna", "Ben", "Carla", "David", "Elena", "Frank", "Grace", "Hugo", "Ivy",
	"Jack", "Kira", "Leo", "Mia", "Noah", "Olive", "Paul", "Quinn", "Rosa",
	"Sam", "Tara", "Uma", "Victor", "Wendy", "Xander", "Yara", "Zane",
}

var taskBenchLast = []string{
	"Adler", "Brooks", "Cruz", "Diaz", "Evans", "Ford", "Gupta", "Hale", "Ito",
	"Jones", "Khan", "Lopez", "Meyer", "Novak", "Owens", "Park", "Reyes", "Singh",
}

var taskBenchCountries = []string{"US", "US", "US", "DE", "DE", "VN", "VN", "FR", "GB", "JP"}

type taskBenchProduct struct {
	name     string
	category string
	price    float64
}

var taskBenchProducts = []taskBenchProduct{
	{"Wireless Mouse", "Accessories", 24.99}, {"Mechanical Keyboard", "Accessories", 89.00},
	{"USB-C Cable 2m", "Accessories", 12.50}, {"Laptop Stand", "Accessories", 39.95},
	{"Webcam 1080p", "Accessories", 54.00}, {"Noise-Cancel Headphones", "Audio", 199.00},
	{"Bluetooth Speaker", "Audio", 79.00}, {"Wired Earbuds", "Audio", 19.99},
	{"27in 4K Monitor", "Displays", 329.00}, {"24in FHD Monitor", "Displays", 159.00},
	{"Ultrawide Monitor", "Displays", 499.00}, {"Laptop Pro 14", "Computers", 1799.00},
	{"Laptop Air 13", "Computers", 999.00}, {"Mini Desktop", "Computers", 649.00},
	{"Tablet 10", "Computers", 449.00}, {"Office Chair", "Furniture", 219.00},
	{"Standing Desk", "Furniture", 379.00}, {"Desk Lamp LED", "Furniture", 34.50},
	{"Power Bank 20k", "Mobile", 45.00}, {"Phone Case", "Mobile", 14.99},
}

var taskBenchStatuses = []string{"completed", "completed", "completed", "completed", "shipped", "cancelled"}

// taskBenchBuildDB builds an in-memory SQLite DB seeded deterministically (same
// data every run), reproducing seed.build_db's Python random.Random(2024) draw
// sequence so every task's ground truth matches. Mirrors seed.build_db.
func taskBenchBuildDB() (*sql.DB, error) {
	rng := newTaskBenchRand(2024)
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // one shared in-memory connection
	if _, err := db.Exec(taskBenchSchema); err != nil {
		db.Close()
		return nil, err
	}

	tx, err := db.Begin()
	if err != nil {
		db.Close()
		return nil, err
	}
	for i := 1; i <= 60; i++ {
		name := taskBenchFirst[rng.intn(len(taskBenchFirst))] + " " + taskBenchLast[rng.intn(len(taskBenchLast))]
		month := rng.randint(1, 12)
		day := rng.randint(1, 28)
		country := taskBenchCountries[rng.intn(len(taskBenchCountries))]
		if _, err := tx.Exec("INSERT INTO customers VALUES (?,?,?,?,?)",
			i, name, fmt.Sprintf("user%d@example.com", i), country, fmt.Sprintf("2023-%02d-%02d", month, day)); err != nil {
			tx.Rollback()
			db.Close()
			return nil, err
		}
	}
	for i, p := range taskBenchProducts {
		if _, err := tx.Exec("INSERT INTO products VALUES (?,?,?,?)", i+1, p.name, p.category, p.price); err != nil {
			tx.Rollback()
			db.Close()
			return nil, err
		}
	}
	for oid := 1; oid <= 400; oid++ {
		cust := rng.randint(1, 60)
		month := rng.randint(1, 12)
		day := rng.randint(1, 28)
		status := taskBenchStatuses[rng.intn(len(taskBenchStatuses))]
		if _, err := tx.Exec("INSERT INTO orders VALUES (?,?,?,?)",
			oid, cust, fmt.Sprintf("2023-%02d-%02d", month, day), status); err != nil {
			tx.Rollback()
			db.Close()
			return nil, err
		}
		nItems := rng.randint(1, 4)
		for k := 0; k < nItems; k++ {
			pid := rng.randint(1, len(taskBenchProducts))
			qty := rng.randint(1, 3)
			unit := taskBenchProducts[pid-1].price
			if _, err := tx.Exec("INSERT INTO order_items VALUES (?,?,?,?)", oid, pid, qty, unit); err != nil {
				tx.Rollback()
				db.Close()
				return nil, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func taskBenchSchemaText() string {
	return "Tables (SQLite):\n" +
		"- customers(id, name, email, country, signup_date)  -- country e.g. US/DE/VN/FR/GB/JP\n" +
		"- products(id, name, category, price)               -- category e.g. Accessories/Audio/Displays/Computers/Furniture/Mobile\n" +
		"- orders(id, customer_id, order_date, status)       -- status e.g. completed/shipped/cancelled; order_date 'YYYY-MM-DD'\n" +
		"- order_items(order_id, product_id, quantity, unit_price)\n" +
		"Revenue of a line item = quantity * unit_price. Join order_items→orders for dates/status, " +
		"order_items→products for names/categories, orders→customers for customer info."
}

// taskBenchRand reproduces CPython's random.Random draw sequence for the methods
// the seed uses (choice via _randbelow, randint). It wraps math/rand with a
// Mersenne-Twister-compatible source so the seeded DB matches Python's exactly.
type taskBenchRand struct{ mt *taskBenchMT }

func newTaskBenchRand(seed uint32) *taskBenchRand { return &taskBenchRand{mt: newTaskBenchMT(seed)} }

// intn mirrors random._randbelow(n) (rejection sampling over getrandbits(k)),
// used by random.choice(seq) as _randbelow(len(seq)).
func (r *taskBenchRand) intn(n int) int { return r.mt.randbelow(n) }

// randint mirrors random.randint(a, b) == a + _randbelow(b-a+1).
func (r *taskBenchRand) randint(a, b int) int { return a + r.mt.randbelow(b-a+1) }

// taskBenchMT is a CPython-compatible Mersenne Twister + getrandbits/_randbelow,
// enough to reproduce random.Random(seed)'s choice/randint draws.
type taskBenchMT struct {
	mt  [624]uint32
	idx int
}

func newTaskBenchMT(seed uint32) *taskBenchMT {
	m := &taskBenchMT{}
	m.initByArray([]uint32{seed})
	return m
}

func (m *taskBenchMT) initGenrand(s uint32) {
	m.mt[0] = s
	for i := uint32(1); i < 624; i++ {
		m.mt[i] = 1812433253*(m.mt[i-1]^(m.mt[i-1]>>30)) + i
	}
	m.idx = 624
}

func (m *taskBenchMT) initByArray(key []uint32) {
	m.initGenrand(19650218)
	i, j := uint32(1), uint32(0)
	k := uint32(624)
	if uint32(len(key)) > k {
		k = uint32(len(key))
	}
	for ; k > 0; k-- {
		m.mt[i] = (m.mt[i] ^ ((m.mt[i-1] ^ (m.mt[i-1] >> 30)) * 1664525)) + key[j] + j
		i++
		j++
		if i >= 624 {
			m.mt[0] = m.mt[623]
			i = 1
		}
		if j >= uint32(len(key)) {
			j = 0
		}
	}
	for k = 623; k > 0; k-- {
		m.mt[i] = (m.mt[i] ^ ((m.mt[i-1] ^ (m.mt[i-1] >> 30)) * 1566083941)) - i
		i++
		if i >= 624 {
			m.mt[0] = m.mt[623]
			i = 1
		}
	}
	m.mt[0] = 0x80000000
}

func (m *taskBenchMT) genrandUint32() uint32 {
	if m.idx >= 624 {
		for i := 0; i < 624; i++ {
			y := (m.mt[i] & 0x80000000) | (m.mt[(i+1)%624] & 0x7fffffff)
			next := m.mt[(i+397)%624] ^ (y >> 1)
			if y&1 != 0 {
				next ^= 2567483615
			}
			m.mt[i] = next
		}
		m.idx = 0
	}
	y := m.mt[m.idx]
	m.idx++
	y ^= y >> 11
	y ^= (y << 7) & 2636928640
	y ^= (y << 15) & 4022730752
	y ^= y >> 18
	return y
}

// getrandbits mirrors CPython's random.getrandbits(k) for k ≤ 32.
func (m *taskBenchMT) getrandbits(k uint) uint32 {
	if k == 0 {
		return 0
	}
	return m.genrandUint32() >> (32 - k)
}

// randbelow mirrors random._randbelow_with_getrandbits(n): k = n.bit_length(),
// reject draws ≥ n. Returns 0 for n ≤ 0.
func (m *taskBenchMT) randbelow(n int) int {
	if n <= 0 {
		return 0
	}
	k := uint(taskBenchBitLen(uint32(n)))
	for {
		r := m.getrandbits(k)
		if int(r) < n {
			return int(r)
		}
	}
}

func taskBenchBitLen(n uint32) int {
	bits := 0
	for n > 0 {
		bits++
		n >>= 1
	}
	return bits
}
