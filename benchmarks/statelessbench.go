package benchmarks

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/mezon/agent-sdk-go/agent_sdk/agent"
	"github.com/mezon/agent-sdk-go/agent_sdk/clients"
	"github.com/mezon/agent-sdk-go/agent_sdk/serve"
	"github.com/mezon/agent-sdk-go/agent_sdk/session"
	"github.com/mezon/agent-sdk-go/agent_sdk/stores/sqlite"
)

// statelessbench — the deterministic gate that proves the SDK runs STATELESS:
// a process holds only the immutable agent config; ALL per-session state
// (conversation AND universal working memory) rides in one plain-JSON snapshot.
// So any worker/replica serves any session, a restart loses nothing, and a
// pooled AgentWorker serves N concurrent sessions without cross-contamination.
//
// FREE / deterministic (FakeClient + auto-establish): this measures the
// snapshot/restore/serve plumbing, not LLM quality. Ported from
// benchmarks/statelessbench/run.py. Five modes (snapshot/store/isolation/
// spec/schema), each a ComposeVerdict payload:
//
//	snapshot   — RunSnapshot: a fact stored on turn 1 survives a hop to a FRESH
//	             agent on turn 2 (memory + history carried across the hop).
//	store      — any SessionStore (here SQLite via modernc) round-trips the WHOLE
//	             state; a new agent + new Session on the same id resumes it.
//	isolation  — an AgentWorker(agent_factory=…) pool (pool < N) runs N sessions;
//	             each session's memory holds ONLY its own facts (no bleed).
//	spec       — agent.Spec().ToJSON() → FromSpec rebuilds a byte-identical config.
//	schema     — the snapshot is versioned + tolerant (unknown/missing keys load).
//
// Deviation: the Python run.py also ships a `worker` mode (a store-bound,
// resume-by-id worker). The task's mode set is snapshot/store/isolation/spec/
// schema, so the Go bench ports those five — the same READY deterministic floor.

// statelessAgent builds the universal-memory agent under test. universal_memory
// is on by default (the working-memory store the modes exercise); the FakeClient
// scripts a fixed "Noted." for any hop. Mirrors run.py:_agent.
func statelessAgent() *agent.PreactAgent {
	script := make([]any, 8)
	for i := range script {
		script[i] = statelessAnswer
	}
	return agent.MustPreactAgent(agent.Config{
		Client:       clients.NewFakeClient(script, nil),
		Instructions: "You are helpful.",
	})
}

// statelessAnswer is the fixed FakeClient reply (auto-establish stores facts
// deterministically, so memory content is a pure function of the input).
const statelessAnswer = "Noted."

// statelessFactsIn returns all long-term memory bodies in a snapshot, lowered —
// for substring assertions. Mirrors run.py:_facts_in.
func statelessFactsIn(snapshot map[string]any) string {
	mem, _ := snapshot["memory"].(map[string]any)
	parts := []string{}
	for _, e := range statelessLong(mem) {
		parts = append(parts, fmt.Sprint(e["body"]))
	}
	return strings.ToLower(strings.Join(parts, " "))
}

// statelessLong coerces a snapshot's memory.long into rows (tolerating both the
// native []map[string]any and a JSON-decoded []any).
func statelessLong(mem map[string]any) []map[string]any {
	if mem == nil {
		return nil
	}
	switch v := mem["long"].(type) {
	case []map[string]any:
		return v
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, e := range v {
			if m, ok := e.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

// statelessHistoryLen returns the snapshot's history length (tolerating native
// + JSON-decoded shapes).
func statelessHistoryLen(snapshot map[string]any) int {
	switch v := snapshot["history"].(type) {
	case []map[string]any:
		return len(v)
	case []any:
		return len(v)
	}
	return 0
}

// statelessRunSnapshot: the easy stateless API — a fact stored on turn 1 survives
// a hop to a FRESH agent on turn 2. Mirrors run.py:run_snapshot.
func statelessRunSnapshot(ctx context.Context) *ModePayload {
	checks := []Check{}
	a1 := statelessAgent()
	_, snap1, err := a1.RunSnapshot(ctx, "Remember:\n- My name is Alice\n- I work at Acme Corp", nil)
	if err != nil {
		return NewPayload([]Check{ck("snapshot.fact_stored", false, "run error: "+err.Error())}, nil)
	}
	checks = append(checks,
		ck("snapshot.fact_stored", strings.Contains(statelessFactsIn(snap1), "alice"),
			"turn-1 fact captured into the snapshot's memory"),
		ck("snapshot.history_grew", statelessHistoryLen(snap1) == 2,
			"user+assistant turn recorded"),
		ck("snapshot.versioned", asInt(snap1["v"]) == session.SnapshotVersion,
			fmt.Sprintf("snapshot stamps v=%d", session.SnapshotVersion)),
	)

	// A DIFFERENT agent instance (a new process / replica) continues from snap1.
	a2 := statelessAgent()
	_, snap2, err := a2.RunSnapshot(ctx, "What is my name?", snap1)
	if err != nil {
		return NewPayload(append(checks, ck("snapshot.memory_survived_hop", false, "hop error: "+err.Error())), nil)
	}
	checks = append(checks,
		ck("snapshot.memory_survived_hop", strings.Contains(statelessFactsIn(snap2), "alice"),
			"the fact restored into a fresh agent and persisted forward"),
		ck("snapshot.history_carried", statelessHistoryLen(snap2) == 4,
			"prior turns carried across the stateless hop"),
	)
	return NewPayload(checks, map[string]any{"modes": 1})
}

// statelessRunStore: any SessionStore round-trips the WHOLE state (history +
// universal memory); a new agent resumes on the same id. Mirrors run.py:run_store.
func statelessRunStore(ctx context.Context) *ModePayload {
	store, err := sqlite.NewStore(":memory:")
	if err != nil {
		return NewPayload([]Check{ck("store.memory_persisted", false, "store open: "+err.Error())}, nil)
	}
	defer store.Close()
	const sid = "conv-1"

	a1 := statelessAgent()
	if _, err := a1.QueryWithSession(ctx, "Remember:\n- The deploy window is Thursday 14:00 UTC", session.New(sid, store)); err != nil {
		return NewPayload([]Check{ck("store.memory_persisted", false, "query: "+err.Error())}, nil)
	}
	persisted, _ := store.Load(ctx, sid)
	checks := []Check{
		ck("store.memory_persisted", strings.Contains(statelessFactsIn(persisted.ToJSON()), "deploy window"),
			"universal memory saved into the store (not just the transcript)"),
		ck("store.history_persisted", len(persisted.History) == 2, "turns saved"),
	}

	// A fresh agent + fresh Session on the SAME store/id resumes with full state.
	a2 := statelessAgent()
	if _, err := a2.QueryWithSession(ctx, "Anything else?", session.New(sid, store)); err != nil {
		return NewPayload(append(checks, ck("store.resumes_across_agents", false, "resume: "+err.Error())), nil)
	}
	resumed, _ := store.Load(ctx, sid)
	checks = append(checks,
		ck("store.resumes_across_agents", strings.Contains(statelessFactsIn(resumed.ToJSON()), "deploy window"),
			"memory still present after a different agent continued the session"),
		ck("store.history_continued", len(resumed.History) == 4, "history continued"),
	)
	return NewPayload(checks, map[string]any{"modes": 1})
}

// statelessRunIsolation: the concurrency proof — a pooled AgentWorker runs N
// distinct sessions; no memory bleeds across them. The pool (4) is smaller than
// the session count (6), so agents are REUSED across sessions (the bleed risk).
// Mirrors run.py:run_isolation.
func statelessRunIsolation(ctx context.Context) *ModePayload {
	store, err := sqlite.NewStore(":memory:")
	if err != nil {
		return NewPayload([]Check{ck("isolation.open", false, "store open: "+err.Error())}, nil)
	}
	defer store.Close()
	people := [][2]string{
		{"alice", "acme"}, {"bob", "bytecorp"}, {"carol", "cogni"},
		{"dave", "dynamo"}, {"erin", "edgeware"}, {"frank", "fathom"},
	}
	worker := serve.NewAgentWorker(serve.WorkerConfig{
		AgentFactory: statelessAgent, // a POOL of agents — each turn checks one out exclusively
		Queue:        serve.NewInProcessQueue(),
		Sink:         serve.NewInProcessEventSink(),
		Concurrency:  4, // < number of sessions, so agents are REUSED (the bleed risk)
	})
	for _, p := range people {
		name, org := p[0], p[1]
		if _, err := worker.Queue().Enqueue(serve.Job{
			Input:   fmt.Sprintf("Remember:\n- My name is %s\n- I work at %s", name, org),
			Session: session.New("sess-"+name, store),
			TraceID: "t-" + name,
		}); err != nil {
			return NewPayload([]Check{ck("isolation.enqueue", false, "enqueue: "+err.Error())}, nil)
		}
	}
	if err := worker.Serve(ctx, len(people)); err != nil {
		return NewPayload([]Check{ck("isolation.serve", false, "serve: "+err.Error())}, nil)
	}

	checks := []Check{}
	clean := 0
	for _, p := range people {
		name, org := p[0], p[1]
		st, _ := store.Load(ctx, "sess-"+name)
		facts := statelessFactsIn(st.ToJSON())
		own := strings.Contains(facts, name) && strings.Contains(facts, org)
		others := []string{}
		for _, q := range people {
			if q[0] != name && strings.Contains(facts, q[0]) {
				others = append(others, q[0])
			}
		}
		ok := own && len(others) == 0
		if ok {
			clean++
		}
		detail := "isolated"
		if !ok {
			detail = fmt.Sprintf("own facts present, no bleed (stray: %v)", others)
		}
		checks = append(checks, ck("isolation."+name+"_only_own", ok, detail))
	}
	return NewPayload(checks, map[string]any{"sessions": len(people), "pool": 4, "clean": clean})
}

// statelessRunSpec: config (the immutable half) is JSON-portable —
// Spec().ToJSON() → FromSpec → identical agent. Mirrors run.py:run_spec.
func statelessRunSpec() *ModePayload {
	a := agent.MustPreactAgent(agent.Config{
		Client:           clients.NewFakeClient([]any{statelessAnswer}, nil),
		Instructions:     "You are a concise assistant.",
		RequireCitations: false,
	})
	j := a.Spec().ToJSON()
	rebuilt, err := agent.FromSpec(a.Spec(), clients.NewFakeClient([]any{statelessAnswer}, nil))
	if err != nil {
		return NewPayload([]Check{ck("spec.roundtrips", false, "from_spec: "+err.Error())}, nil)
	}
	hasLobes := len(statelessRows(j["lobes"])) > 0
	hasFlows := len(statelessRows(j["flows"])) > 0
	checks := []Check{
		ck("spec.roundtrips", reflect.DeepEqual(rebuilt.Spec().ToJSON(), j),
			"agent rebuilt from JSON spec is byte-identical"),
		ck("spec.has_network", hasLobes && hasFlows, "spec captures the network (lobes/flows)"),
	}
	return NewPayload(checks, map[string]any{"modes": 1})
}

// statelessRows coerces a spec field into a row count (native or JSON-decoded).
func statelessRows(v any) []any {
	switch x := v.(type) {
	case []any:
		return x
	case []map[string]any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = e
		}
		return out
	}
	return nil
}

// statelessRunSchema: the snapshot is versioned + tolerant (unknown/missing keys
// load) → extensible later with no migration. Mirrors run.py:run_schema.
func statelessRunSchema() *ModePayload {
	full := session.SessionState{
		Summary:      "hi",
		SkillsInUse:  []string{"x"},
		MetaFlowBias: "research",
		Memory:       map[string]any{"seq": 1, "long": []any{}, "docs": map[string]any{}},
	}.ToJSON()
	_, hasMem := full["memory"]
	checks := []Check{
		ck("schema.versioned", asInt(full["v"]) == session.SnapshotVersion, "carries a version stamp"),
		ck("schema.carries_memory", hasMem, "memory rides the snapshot"),
	}

	// forward-compat: an UNKNOWN future key loads without error and is ignored.
	fwd := map[string]any{}
	for k, v := range full {
		fwd[k] = v
	}
	fwd["_future_field"] = map[string]any{"anything": true}
	st := session.SessionStateFromJSON(fwd)
	checks = append(checks, ck("schema.tolerates_unknown", reflect.DeepEqual(st.SkillsInUse, []string{"x"}),
		"unknown keys ignored, known state intact"))

	// backward-compat: an OLD snapshot (no memory / no v) loads with safe defaults.
	old := map[string]any{"history": []any{}, "summary": "legacy"}
	st2 := session.SessionStateFromJSON(old)
	checks = append(checks, ck("schema.tolerates_missing",
		st2.Summary == "legacy" && len(st2.Memory) == 0,
		"missing keys default; no crash"))
	return NewPayload(checks, map[string]any{"version": session.SnapshotVersion})
}

// RunStatelessBench composes the statelessbench verdict (deterministic floor) —
// READY iff every mode's checks pass. Mirrors run.py:main's compose_verdict.
func RunStatelessBench(ctx context.Context, _ string) (Verdict, error) {
	payloads := map[string]*ModePayload{
		"snapshot":  statelessRunSnapshot(ctx),
		"store":     statelessRunStore(ctx),
		"isolation": statelessRunIsolation(ctx),
		"spec":      statelessRunSpec(),
		"schema":    statelessRunSchema(),
	}
	return ComposeVerdict(payloads, map[string][]string{"isolation": {"clean", "sessions"}}), nil
}

// asInt coerces a snapshot's numeric field (native int or JSON float64).
func asInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}
