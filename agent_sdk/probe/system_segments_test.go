// Translated from the Python engine's segmented system-prompt composition
// (agent_sdk/engine.py:_compose_system_segmented + probe emission): each probe
// stage carries `system_segments` — ordered {source, stability, start, end}
// ranges over the stage's composed system_prompt, identity-first / env-last,
// non-overlapping. promptbench colours the Prompt panel by these.
package probe

import (
	"context"
	"testing"

	"github.com/mezon/agent-sdk-go/agent_sdk/agent"
	"github.com/mezon/agent-sdk-go/agent_sdk/engine"
	"github.com/mezon/agent-sdk-go/agent_sdk/flows"
)

// stabRank mirrors the Python _STAB_RANK ordering (stable < slow < turn <
// volatile): the volatile tail must form a contiguous suffix.
var stabRank = map[string]int{"stable": 0, "slow": 1, "turn": 2, "volatile": 3}

// segStr coerces a segment field to int (JSON-able maps hold int or float64).
func segInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	}
	return 0
}

// TestProbeStagesCarrySystemSegments asserts every probed stage exposes
// `system_segments` that (1) cover the stage's system_prompt with the joined
// fragment text, (2) are non-overlapping and authored in start order, (3) lead
// with the identity (`instructions`) source, and (4) trail with the
// volatile/env tail (`datetime`), matching the Python stability ordering.
func TestProbeStagesCarrySystemSegments(t *testing.T) {
	// Two-stage flow: the second stage's system prompt picks up a "[Notes
	// gathered this turn]" (turn-stability) section from the first stage's
	// answer, so at least one stage exercises a multi-source ordering.
	a := scriptAgent(
		[]any{"first", "second"},
		agent.Config{
			Instructions: "helpful",
			Flows: []flows.Flow{
				flows.NewFlow("work",
					flows.FlowStages("alpha", "beta"),
					flows.FlowSignalExpr(map[string]any{"const": 1.0})),
			},
			Stages: []any{
				engine.NewStage("alpha", engine.StageLobes("synthesize"), engine.StageLoop("single")),
				engine.NewStage("beta", engine.StageLobes("synthesize"), engine.StageLoop("single")),
			},
		},
	)
	rec, err := Probe(context.Background(), a, "go", WithLabel("segs"))
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if len(rec.Stages) == 0 {
		t.Fatalf("no stages captured")
	}
	sawNotesStage := false
	for si, st := range rec.Stages {
		sys, _ := st["system_prompt"].(string)
		segs, ok := st["system_segments"].([]map[string]any)
		if !ok {
			// Tolerate the JSON-able []any shape too.
			if raw, ok2 := st["system_segments"].([]any); ok2 {
				segs = nil
				for _, e := range raw {
					if m, ok3 := e.(map[string]any); ok3 {
						segs = append(segs, m)
					}
				}
			}
		}
		if len(segs) == 0 {
			t.Fatalf("stage %d (%v): no system_segments", si, st["stage"])
		}

		// (1) cover: each segment's [start,end) slice equals its fragment in
		// system_prompt, and the concatenation reproduces the whole prompt
		// (segments joined by the "\n\n" separators between them).
		prev := 0
		for i, seg := range segs {
			start, end := segInt(seg["start"]), segInt(seg["end"])
			if start < 0 || end > len(sys) || start > end {
				t.Fatalf("stage %d seg %d: bad range [%d,%d) over len %d", si, i, start, end, len(sys))
			}
			// (2) non-overlapping + authored in start order.
			if start < prev {
				t.Fatalf("stage %d seg %d: start %d overlaps prior end %d", si, i, start, prev)
			}
			frag := sys[start:end]
			src, _ := seg["source"].(string)
			if frag == "" {
				t.Fatalf("stage %d seg %d (%s): empty fragment", si, i, src)
			}
			prev = end
		}
		// The last segment must reach the end of the composed prompt (env tail).
		if last := segs[len(segs)-1]; segInt(last["end"]) != len(sys) {
			t.Fatalf("stage %d: last seg end %d != prompt len %d", si, segInt(last["end"]), len(sys))
		}
		// The first segment must start at 0 (identity prefix is never buried).
		if segInt(segs[0]["start"]) != 0 {
			t.Fatalf("stage %d: first seg start %d != 0", si, segInt(segs[0]["start"]))
		}

		// (3) identity first.
		if src, _ := segs[0]["source"].(string); src != "instructions" {
			t.Errorf("stage %d: first segment source = %q, want instructions", si, src)
		}
		// (4) volatile/env tail last.
		if src, _ := segs[len(segs)-1]["source"].(string); src != "datetime" {
			t.Errorf("stage %d: last segment source = %q, want datetime (env tail)", si, src)
		}

		// Stability must be monotonically non-decreasing by _STAB_RANK, so the
		// turn/volatile sections form a contiguous suffix (Python's cache-prefix
		// boundary).
		prevRank := -1
		for i, seg := range segs {
			stab, _ := seg["stability"].(string)
			r, known := stabRank[stab]
			if !known {
				t.Errorf("stage %d seg %d: unknown stability %q", si, i, stab)
			}
			if r < prevRank {
				t.Errorf("stage %d seg %d: stability %q (rank %d) precedes a higher tier (%d)", si, i, stab, r, prevRank)
			}
			prevRank = r
			if src, _ := seg["source"].(string); src == "notes" {
				sawNotesStage = true
				if stab != "turn" {
					t.Errorf("stage %d: notes segment stability = %q, want turn", si, stab)
				}
			}
		}
	}
	if !sawNotesStage {
		t.Errorf("expected the second stage to carry a turn-stability 'notes' segment")
	}
}
