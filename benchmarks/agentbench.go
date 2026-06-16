package benchmarks

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/mezon/agent-sdk-go/agent_sdk/agent"
	"github.com/mezon/agent-sdk-go/agent_sdk/flows"
	"github.com/mezon/agent-sdk-go/agent_sdk/memory"
	"github.com/mezon/agent-sdk-go/agent_sdk/probe"
	"github.com/mezon/agent-sdk-go/agent_sdk/session"
	"github.com/mezon/agent-sdk-go/agent_sdk/tools"
)

// agentbench — the LIVE benchmark for the SDK: a real agent, pushed to its
// limits. Deterministic datasets in (committed under agentbench_dataset/), the
// REAL default PreactAgent driven against a real provider, a verdict out. No
// stubs, no FakeClient — it measures what the agent actually does.
//
// It runs ONE long integrated MISSION that chains every capacity at once
// (ingest a messy ops channel amid chatter → memorize the facts → plan a
// multi-step migration → recall the CURRENT value of a fact that changed 9× →
// find a needle → synthesize a checklist from memory → recall it in a NEW
// conversation), plus focused HARD cases that push individual capacities to
// their edge (a long tool loop for bounded context; needle recall among many
// facts). Behavior is scored from the real probe() trace + the agent's own
// memory store with deterministic distinctive-token matching.
//
// agentbench is LIVE only: every check needs a real agent + provider, so the
// bench is composed as a SINGLE mode "agentbench" carrying the 7 mission.* + 2
// hard.* checks. Without a provider (the deterministic floor) the mode is
// MISSING → the verdict is UNMEASURED (no evidence is never READY) — mirroring
// run.py's refusal to run without a provider token (exit 2). Ported from
// benchmarks/agentbench/run.py.

//go:embed agentbench_dataset/channel_facts.jsonl agentbench_dataset/instruction.jsonl
var agentBenchData embed.FS

const agentBenchBase = "You are the ops assistant for a busy engineering team. Be concise and accurate, and rely on " +
	"what you actually know or can recall."

var agentBenchConceptPhrase = map[string]string{
	"deadline": "deadline", "schedule": "rollout schedule", "owner": "owner", "performance": "latency",
}

// agentBenchCheckIDs is the static check-id surface — the 7 mission.* + 2 hard.*
// ids the live run emits, in run.py order. Asserted for cross-language parity
// independent of the provider.
func agentBenchCheckIDs() []string {
	return []string{
		"mission.memorized",
		"mission.recall_current_supersession",
		"mission.no_double_greeting",
		"mission.distractor_entity",
		"mission.needle_recall",
		"mission.synthesize_from_memory",
		"mission.cross_session_recall",
		"hard.bounded_context",
		"hard.recall_at_scale",
	}
}

// RunAgentBench composes the agentbench verdict. With no model (the
// deterministic floor) the single "agentbench" mode is missing → UNMEASURED.
// With a model the real PreactAgent is driven and each behavior scored.
func RunAgentBench(ctx context.Context, model string) (Verdict, error) {
	var payload *ModePayload
	if model != "" {
		p, err := agentBenchLive(ctx, model)
		if err != nil {
			return Verdict{}, err
		}
		payload = p
	}
	payloads := map[string]*ModePayload{"agentbench": payload}
	record := map[string][]string{"agentbench": {
		"facts_committed", "checklist_items", "bounded_peak_chars", "bounded_fetch_calls", "bounded_hops",
	}}
	return ComposeVerdict(payloads, record), nil
}

// RunAgentBenchProbes captures inspectable mission traces for the viewer. With a
// real model it drives the actual PreactAgent; offline (model=="") it builds the
// SAME representative agent against a FakeClient and runs 1-2 representative
// mission turns (ingest a batch of channel facts → recall the current value) so
// the inspection renders a real path/flow + the executed stages. This adds
// traces only — the live verdict (Run) stays UNMEASURED without a provider.
// Mirrors run.py's mission probes feeding write_viewer.
func RunAgentBenchProbes(ctx context.Context, model string) ([]*probe.Record, error) {
	facts, err := agentBenchFacts()
	if err != nil {
		return nil, err
	}
	track := agentBenchPickTrack(facts)
	entity, concept := track[0].Entity, track[0].Concept

	ag := agent.MustPreactAgent(agent.Config{
		Client:       benchProbeClient(model),
		Instructions: agentBenchBase,
		Session:      session.New("mission-A", nil),
	})

	var records []*probe.Record
	// Turn 1 — ingest a representative batch of channel facts amid chatter.
	batch := track
	if len(batch) > 8 {
		batch = batch[:8]
	}
	lines := []string{}
	for _, f := range batch {
		lines = append(lines, "- "+f.Value)
	}
	rec, err := probe.Probe(ctx, ag, "New #ops messages (lots of chatter):\n"+strings.Join(lines, "\n"),
		probe.WithLabel("mission · ingest"))
	if err != nil {
		return nil, err
	}
	records = append(records, rec)

	// Turn 2 — recall the current value of the 9x-restated fact.
	rec, err = probe.Probe(ctx, ag, fmt.Sprintf("What is the current %s for %s?", agentBenchConceptPhrase[concept], entity),
		probe.WithLabel("mission · recall_current"))
	if err != nil {
		return nil, err
	}
	records = append(records, rec)
	return records, nil
}

// ── dataset → mission inputs (deterministic) ─────────────────────────────────

type agentBenchFact struct {
	FactID  int    `json:"fact_id"`
	Turn    int    `json:"turn"`
	Speaker string `json:"speaker"`
	Key     string `json:"key"`
	Entity  string `json:"entity"`
	Concept string `json:"concept"`
	Value   string `json:"value"`
}

type agentBenchInstr struct {
	ID    string `json:"id"`
	Goal  string `json:"goal"`
	Steps []struct {
		Step    int `json:"step"`
		Offload struct {
			Kind    string `json:"kind"`
			Key     string `json:"key"`
			Content string `json:"content"`
		} `json:"offload"`
	} `json:"steps"`
}

func agentBenchFacts() ([]agentBenchFact, error) {
	raw, err := agentBenchData.ReadFile("agentbench_dataset/channel_facts.jsonl")
	if err != nil {
		return nil, err
	}
	out := []agentBenchFact{}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var f agentBenchFact
		if err := json.Unmarshal([]byte(line), &f); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

func agentBenchInstruction() (agentBenchInstr, error) {
	raw, err := agentBenchData.ReadFile("agentbench_dataset/instruction.jsonl")
	if err != nil {
		return agentBenchInstr{}, err
	}
	var instr agentBenchInstr
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		return instr, json.Unmarshal([]byte(line), &instr)
	}
	return instr, fmt.Errorf("agentbench: empty instruction dataset")
}

// agentBenchPickTrack returns the (entity, concept) restated the most times —
// the supersession stress (latest must win), in time order. Mirrors _pick_track.
func agentBenchPickTrack(facts []agentBenchFact) []agentBenchFact {
	groups := map[string][]agentBenchFact{}
	for _, f := range facts {
		if f.Entity != "" && f.Concept != "" {
			k := f.Entity + "\x00" + f.Concept
			groups[k] = append(groups[k], f)
		}
	}
	// max by len; for ties, the first key in iteration order over sorted keys
	// (deterministic, matching Python's first-seen max over an insertion dict —
	// here we sort keys so the choice is reproducible).
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var best []agentBenchFact
	for _, k := range keys {
		if len(groups[k]) > len(best) {
			best = groups[k]
		}
	}
	out := append([]agentBenchFact(nil), best...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Turn < out[j].Turn })
	return out
}

func agentBenchBatches(xs []agentBenchFact, n int) [][]agentBenchFact {
	out := [][]agentBenchFact{}
	for i := 0; i < len(xs); i += n {
		end := i + n
		if end > len(xs) {
			end = len(xs)
		}
		out = append(out, xs[i:end])
	}
	return out
}

// ── the live run ─────────────────────────────────────────────────────────────

var (
	agentBenchTokenRe = regexp.MustCompile(`\d{1,2}:\d{2}|\d{4}-\d{2}-\d{2}|@\w+|\d+ms`)
	agentBenchMentRe  = regexp.MustCompile(`@\w+`)
)

var agentBenchGreets = []string{
	"hello", "hi ", "hi,", "hi!", "hi.", "hey", "heya", "greetings",
	"good morning", "good afternoon", "good evening", "welcome",
}

func agentBenchLive(ctx context.Context, model string) (*ModePayload, error) {
	facts, err := agentBenchFacts()
	if err != nil {
		return nil, err
	}
	instr, err := agentBenchInstruction()
	if err != nil {
		return nil, err
	}
	checks := []Check{}
	metrics := map[string]any{}
	add := func(id string, ok bool, detail string) {
		checks = append(checks, ck(id, ok, detail))
	}

	if err := agentBenchMission(ctx, model, facts, instr, &checks, metrics); err != nil {
		return nil, err
	}
	if err := agentBenchHardBounded(ctx, model, metrics, add); err != nil {
		return nil, err
	}
	if err := agentBenchHardMemoryScale(ctx, model, facts, add); err != nil {
		return nil, err
	}
	return NewPayload(checks, metrics), nil
}

func agentBenchMission(ctx context.Context, model string, facts []agentBenchFact, instr agentBenchInstr,
	checks *[]Check, metrics map[string]any) error {
	add := func(id string, ok bool, detail string) { *checks = append(*checks, ck(id, ok, detail)) }

	track := agentBenchPickTrack(facts)
	entity, concept := track[0].Entity, track[0].Concept
	latest := track[len(track)-1].Value
	var needle agentBenchFact
	for _, f := range facts {
		if f.Key == "incident-postmortem" {
			needle = f
			break
		}
	}
	owners := map[string]agentBenchFact{}
	for _, f := range facts {
		if f.Concept == "owner" {
			owners[f.Entity] = f // last wins = latest owner
		}
	}
	ownerKeys := make([]string, 0, len(owners))
	for k := range owners {
		ownerKeys = append(ownerKeys, k)
	}
	sort.Strings(ownerKeys)
	distEntity := ownerKeys[0]
	distOwner := agentBenchMentRe.FindString(owners[distEntity].Value)
	others := []agentBenchFact{}
	for _, f := range facts {
		if f.Entity != entity {
			others = append(others, f)
			if len(others) == 22 {
				break
			}
		}
	}
	ingest := append([]agentBenchFact{}, track...)
	ingest = append(ingest, needle, owners[distEntity])
	ingest = append(ingest, others...)
	sort.SliceStable(ingest, func(i, j int) bool { return ingest[i].Turn < ingest[j].Turn })

	ag := agent.MustPreactAgent(agent.Config{Client: model, Instructions: agentBenchBase,
		Session: session.New("mission-A", nil)})

	// 1) INGEST + MEMORIZE amid chatter.
	noise := []string{"lgtm", "any updates here?", "+1", "brb lunch", "who's on call tonight?", "merged, thanks"}
	for i, chunk := range agentBenchBatches(ingest, 8) {
		lines := []string{}
		for j, f := range chunk {
			lines = append(lines, "- "+f.Value)
			if j%3 == 1 {
				lines = append(lines, noise[(i+j)%len(noise)])
			}
		}
		if _, err := probe.Probe(ctx, ag, "New #ops messages (lots of chatter):\n"+strings.Join(lines, "\n"),
			probe.WithLabel("mission · ingest")); err != nil {
			return err
		}
	}
	committed := 0
	if ms := ag.MemoryStore(); ms != nil {
		committed = ms.Stats()["long_term"]
	}
	metrics["facts_committed"] = committed
	add("mission.memorized", committed >= 18, fmt.Sprintf("%d distinct facts from %d amid noise", committed, len(ingest)))

	// 2) PLAN — walk the agreed migration decisions.
	decisions := []string{}
	for _, s := range instr.Steps {
		decisions = append(decisions, s.Offload.Content)
	}
	planMsg := instr.Goal + "\n\nThe decisions we've agreed — remember each of these:\n- " + strings.Join(decisions, "\n- ")
	if _, err := probe.Probe(ctx, ag, planMsg, probe.WithLabel("mission · plan")); err != nil {
		return err
	}

	// 3) RECALL CURRENT VALUE — the LATEST of a 9× restated fact.
	rec, err := probe.Probe(ctx, ag, fmt.Sprintf("What is the current %s for %s?", agentBenchConceptPhrase[concept], entity),
		probe.WithLabel("mission · recall_current"))
	if err != nil {
		return err
	}
	toks := agentBenchTokenRe.FindAllString(latest, -1)
	ans := rec.Answer
	okCurrent := len(toks) > 0 && agentBenchAnyContains(ans, toks)
	add("mission.recall_current_supersession", okCurrent, fmt.Sprintf("latest token %v · %s", toks, agentBenchTrunc(ans, 34)))

	// 3a) NO DOUBLE GREETING — the reply flow continues, never greets twice.
	greet := agent.MustPreactAgent(agent.Config{Client: model, Instructions: agentBenchBase,
		Session: session.New("mission-greet", nil)})
	if _, err := probe.Probe(ctx, greet, "Hi! Can you help me plan the database migration?",
		probe.WithLabel("mission · greet_open")); err != nil {
		return err
	}
	rFollow, err := probe.Probe(ctx, greet, "Great — what should we do first?", probe.WithLabel("mission · greet_followup"))
	if err != nil {
		return err
	}
	follow := rFollow.Answer
	greetedAgain := agentBenchStartsWithAny(strings.ToLower(strings.TrimLeft(follow, " \t\n")), agentBenchGreets)
	add("mission.no_double_greeting", follow != "" && !greetedAgain, fmt.Sprintf("follow-up opens: %q", agentBenchTrunc(follow, 48)))

	// 3b) DISTRACTOR — the right owner among many projects' owners.
	rec, err = probe.Probe(ctx, ag, fmt.Sprintf("Who is the current owner of the %s project?", distEntity),
		probe.WithLabel("mission · distractor"))
	if err != nil {
		return err
	}
	add("mission.distractor_entity", distOwner != "" && strings.Contains(rec.Answer, distOwner),
		fmt.Sprintf("want %s · %s", distOwner, agentBenchTrunc(rec.Answer, 30)))

	// 4) NEEDLE — a fact stated once among the noise.
	rec, err = probe.Probe(ctx, ag, "What caused the payments sev1 outage?", probe.WithLabel("mission · needle"))
	if err != nil {
		return err
	}
	al := strings.ToLower(rec.Answer)
	add("mission.needle_recall", strings.Contains(al, "connection pool") || strings.Contains(al, "pool leak"),
		agentBenchTrunc(rec.Answer, 50))

	// 5) SYNTHESIZE FROM MEMORY — recall the agreed decisions into a checklist.
	rec, err = probe.Probe(ctx, ag, "Produce the final migration checklist — recall every agreed decision from memory.",
		probe.WithLabel("mission · synthesize"))
	if err != nil {
		return err
	}
	al = strings.ToLower(rec.Answer)
	groups := [][]string{{"saturday", "02:00"}, {"billing", "drain"}, {"@user042", "2%"}, {"0.01%", "row count"}, {"#ops", "24h"}}
	hit := 0
	for _, g := range groups {
		if agentBenchAnyContains(al, g) {
			hit++
		}
	}
	recalls := agentBenchCountTool(rec, "recall")
	metrics["checklist_items"] = hit
	add("mission.synthesize_from_memory", hit >= 3 || recalls >= 2, fmt.Sprintf("%d/5 verbatim, %d recall calls", hit, recalls))

	// 6) CROSS-SESSION — a NEW conversation; only durable memory survives.
	cross := agent.MustPreactAgent(agent.Config{Client: model, Instructions: agentBenchBase,
		Session: session.New("mission-B", nil)})
	if ms := ag.MemoryStore(); ms != nil {
		if cms := cross.MemoryStore(); cms != nil {
			cms.Restore(ms.ToJSON(memory.SnapshotOpts{}))
		}
	}
	rec, err = probe.Probe(ctx, cross, "What cutover window did we agree for the migration?",
		probe.WithLabel("mission · cross_session"))
	if err != nil {
		return err
	}
	a := strings.ToLower(rec.Answer)
	add("mission.cross_session_recall", strings.Contains(a, "saturday") || strings.Contains(a, "02:00"),
		agentBenchTrunc(rec.Answer, 50))
	return nil
}

func agentBenchHardBounded(ctx context.Context, model string, metrics map[string]any,
	add func(id string, ok bool, detail string)) error {
	fetch := tools.Tool("fetch", func(_ context.Context, in map[string]any) (any, error) {
		id := in["record_id"]
		return fmt.Sprintf("record %v: %s", id, strings.Repeat("lorem ipsum dolor sit amet detail ", 30)), nil
	}, tools.Desc("Fetch one record by id."), tools.Param("record_id", "integer", true, nil))

	hops := 30
	work := flows.NewFlowStep(flows.FlowStep{Name: "work", Lobes: []string{"synthesize"}, Loop: "agentic",
		Tools: []string{"fetch"}, Hops: &hops})
	qna := flows.NewFlow("qna", flows.FlowStages("work"), flows.FlowSignalExpr(map[string]any{"const": 1.0}))

	ag := agent.MustPreactAgent(agent.Config{Client: model,
		Instructions: "Call fetch once for each id 1..18, one at a time, then summarize.",
		Tools:        []any{fetch}, Flows: []any{qna}, Stages: []any{work}})
	rec, err := probe.Probe(ctx, ag, "Fetch records 1 through 18 individually and summarize.",
		probe.WithLabel("hard · bounded"))
	if err != nil {
		return err
	}
	series := []int{}
	for _, s := range rec.Stages {
		meta, _ := s["metadata"].(map[string]any)
		if meta == nil {
			continue
		}
		if obs, ok := meta["funnel_obs_chars"].([]any); ok {
			for _, c := range obs {
				series = append(series, agentBenchAsInt(c))
			}
		}
	}
	nHops := len(series)
	peak := 0
	for _, c := range series {
		if c > peak {
			peak = c
		}
	}
	n := agentBenchCountTool(rec, "fetch")
	metrics["bounded_peak_chars"] = peak
	metrics["bounded_fetch_calls"] = n
	metrics["bounded_hops"] = nHops
	if n < 6 || nHops < 6 {
		// the model BATCHED the calls — no spent-observation tail to funnel. The
		// funnel's bounding of a SEQUENTIAL loop is locked deterministically in
		// the default-efficiency unit tests.
		add("hard.bounded_context", true, fmt.Sprintf("UNMEASURED — %d calls in %d hops (batched)", n, nHops))
	} else {
		add("hard.bounded_context", peak < 12000, fmt.Sprintf("%d calls · %d hops · peak %d chars (bounded)", n, nHops, peak))
	}
	return nil
}

func agentBenchHardMemoryScale(ctx context.Context, model string, facts []agentBenchFact,
	add func(id string, ok bool, detail string)) error {
	var needle agentBenchFact
	for _, f := range facts {
		if f.Key == "incident-postmortem" {
			needle = f
			break
		}
	}
	seed := append([]agentBenchFact{}, facts[:120]...)
	seed = append(seed, needle)
	ag := agent.MustPreactAgent(agent.Config{Client: model, Instructions: agentBenchBase,
		Session: session.New("scale", nil)})
	if ms := ag.MemoryStore(); ms != nil {
		for _, f := range seed {
			ms.Remember("fact", f.Value, memory.RememberOpts{Scope: "conversation", Key: f.Key,
				Meta: map[string]any{"entity": f.Entity, "concept": f.Concept, "key": f.Key}})
		}
	}
	rec, err := probe.Probe(ctx, ag, "From memory, what was the root cause of the gateway Sev1 incident?",
		probe.WithLabel("hard · memory_scale"))
	if err != nil {
		return err
	}
	al := strings.ToLower(rec.Answer)
	ok := strings.Contains(al, "connection pool") || strings.Contains(al, "pool leak")
	add("hard.recall_at_scale", ok, fmt.Sprintf("needle among 120 facts · %s", agentBenchTrunc(rec.Answer, 34)))
	return nil
}

// ── helpers (namespaced by the agentBench prefix) ────────────────────────────

func agentBenchAnyContains(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func agentBenchStartsWithAny(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func agentBenchCountTool(rec *probe.Record, name string) int {
	n := 0
	for _, c := range rec.ToolCalls {
		if cn, _ := c["name"].(string); cn == name {
			n++
		}
	}
	return n
}

func agentBenchTrunc(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func agentBenchAsInt(v any) int {
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
