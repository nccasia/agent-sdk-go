// Package bench — a thin benchmark harness over scenarios + trace/route
// assertions.
//
// Wraps the no-LLM Inspect snapshot (routing) and optional full Query runs into
// one public surface — the attentionbench/flowbench/skillbench trace-reading
// patterns as a small library:
//
//	report, _ := bench.NewHarness(agent).Run(ctx, []bench.Scenario{
//		{Input: "compare A and B", ExpectPath: "research"},
//		{Input: "hello", ExpectPath: "qna"},
//	})
//	report.Summary() // PathAccuracy, LobeRecall, ...
//
// Ported from agent_sdk/bench.py.
package bench

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/nccasia/agent-sdk-go/agent_sdk/agent"
	"github.com/nccasia/agent-sdk-go/agent_sdk/result"
)

// Scenario is one routing/behavior assertion. Mirrors agent_sdk/bench.py:Scenario.
type Scenario struct {
	Input        string
	ExpectPath   string   // empty ⇒ unasserted
	ExpectLobes  []string // lobes that must be activated
	ExpectFlow   []string // resolved stage ids (nil ⇒ unasserted)
	RunLLM       bool     // also run a full Query() (needs a real/fake client)
	ExpectStatus string   // empty ⇒ unasserted
}

// ScenarioResult is the outcome of running one Scenario. Mirrors
// agent_sdk/bench.py:ScenarioResult.
type ScenarioResult struct {
	Scenario       Scenario
	Path           result.PathScore
	ActivatedLobes []string
	Flow           []string
	Passed         bool
	Failures       []string
	LatencyMs      float64
	Status         string
}

// Report is a list of ScenarioResults + a summary roll-up. Mirrors
// agent_sdk/bench.py:Report.
type Report struct {
	Results []ScenarioResult
}

// Summary is the aggregate scorecard for a Report.
type Summary struct {
	Scenarios    int      `json:"scenarios"`
	Passed       int      `json:"passed"`
	PassRate     float64  `json:"pass_rate"`
	PathAccuracy float64  `json:"path_accuracy"`
	LobeRecall   *float64 `json:"lobe_recall"`
	P95LatencyMs float64  `json:"p95_latency_ms"`
}

// Summary computes the aggregate scorecard. Mirrors Report.summary().
func (r *Report) Summary() Summary {
	n := len(r.Results)
	denom := n
	if denom == 0 {
		denom = 1
	}
	pathHits := 0
	for _, res := range r.Results {
		if res.Scenario.ExpectPath == "" || res.Path.Name == res.Scenario.ExpectPath {
			pathHits++
		}
	}
	// lobe recall: fraction of expected lobes that activated, averaged over
	// scenarios that assert lobes.
	var recalls []float64
	for _, res := range r.Results {
		if len(res.Scenario.ExpectLobes) == 0 {
			continue
		}
		want := toSet(res.Scenario.ExpectLobes)
		got := toSet(res.ActivatedLobes)
		hit := 0
		for k := range want {
			if got[k] {
				hit++
			}
		}
		recalls = append(recalls, float64(hit)/float64(len(want)))
	}
	passed := 0
	for _, res := range r.Results {
		if res.Passed {
			passed++
		}
	}
	lat := make([]float64, len(r.Results))
	for i, res := range r.Results {
		lat[i] = res.LatencyMs
	}
	sort.Float64s(lat)
	p95 := 0.0
	if len(lat) > 0 {
		idx := int(0.95 * float64(len(lat)-1))
		p95 = lat[idx]
	}
	var recall *float64
	if len(recalls) > 0 {
		sum := 0.0
		for _, x := range recalls {
			sum += x
		}
		v := round4(sum / float64(len(recalls)))
		recall = &v
	}
	return Summary{
		Scenarios:    n,
		Passed:       passed,
		PassRate:     round4(float64(passed) / float64(denom)),
		PathAccuracy: round4(float64(pathHits) / float64(denom)),
		LobeRecall:   recall,
		P95LatencyMs: round2(p95),
	}
}

// Harness drives a sequence of Scenarios through an agent. Mirrors
// agent_sdk/bench.py:Harness.
type Harness struct {
	agent *agent.PreactAgent
}

// NewHarness builds a Harness over the given agent.
func NewHarness(a *agent.PreactAgent) *Harness { return &Harness{agent: a} }

// Run executes every scenario and returns the collected Report. Mirrors
// Harness.run (async in Python; here it returns an error only when a RunLLM
// query fails).
func (h *Harness) Run(ctx context.Context, scenarios []Scenario) (*Report, error) {
	results := make([]ScenarioResult, 0, len(scenarios))
	for _, sc := range scenarios {
		t0 := time.Now()
		snap := h.agent.Inspect(sc.Input)
		activated := activatedLobes(snap.Lobes)
		var failures []string
		status := ""
		if sc.ExpectPath != "" && snap.Path.Name != sc.ExpectPath {
			failures = append(failures, fmt.Sprintf("path %q != expected %q", snap.Path.Name, sc.ExpectPath))
		}
		if len(sc.ExpectLobes) > 0 {
			got := toSet(activated)
			var missing []string
			for _, lb := range sc.ExpectLobes {
				if !got[lb] {
					missing = append(missing, lb)
				}
			}
			if len(missing) > 0 {
				sort.Strings(missing)
				failures = append(failures, fmt.Sprintf("lobes not activated: %v", missing))
			}
		}
		if sc.ExpectFlow != nil && !equalStrs(snap.Flow, sc.ExpectFlow) {
			failures = append(failures, fmt.Sprintf("flow %v != expected %v", snap.Flow, sc.ExpectFlow))
		}
		if sc.RunLLM {
			res, err := h.agent.Query(ctx, sc.Input)
			if err != nil {
				return nil, err
			}
			status = res.Status
			if sc.ExpectStatus != "" && status != sc.ExpectStatus {
				failures = append(failures, fmt.Sprintf("status %q != expected %q", status, sc.ExpectStatus))
			}
		}
		results = append(results, ScenarioResult{
			Scenario:       sc,
			Path:           snap.Path,
			ActivatedLobes: activated,
			Flow:           append([]string(nil), snap.Flow...),
			Passed:         len(failures) == 0,
			Failures:       failures,
			LatencyMs:      float64(time.Since(t0).Microseconds()) / 1000.0,
			Status:         status,
		})
	}
	return &Report{Results: results}, nil
}

func activatedLobes(lobes []map[string]any) []string {
	out := []string{}
	for _, lb := range lobes {
		if on, _ := lb["activated"].(bool); on {
			if id, _ := lb["id"].(string); id != "" {
				out = append(out, id)
			}
		}
	}
	return out
}

func toSet(xs []string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
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

func round4(x float64) float64 { return math.Round(x*10000) / 10000 }
func round2(x float64) float64 { return math.Round(x*100) / 100 }
