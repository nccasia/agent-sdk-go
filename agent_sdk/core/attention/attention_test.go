// Ported from agent-sdk/tests/test_blocks_smoke.py (attention + blackboard
// cases) plus the ScoreText numeric-parity fixture (text-only: rel=1.0,
// disjoint=0.0).
package attention

import (
	"math"
	"testing"
)

func TestBlackboardRejectsRawChunks(t *testing.T) {
	board := NewBlackboard(nil, nil)
	raw := NewContextNode("c1", "kb_chunk", "secret", "")
	defer func() {
		if r := recover(); r == nil {
			t.Error("blackboard must reject raw chunk kinds")
		}
	}()
	board.WriteBack("research", []*ContextNode{raw}, DefaultWriteBack())
}

func TestBuildAttentionLexicalSelection(t *testing.T) {
	nodes := []*ContextNode{
		NewContextNode("n1", "fact", "alpha beta gamma", ""),
		NewContextNode("n2", "fact", "zeta eta theta", ""),
	}
	selected, _ := Build(nodes, "alpha beta", nil, MergeWeights(nil), 1600, 0.0, nil)
	found := false
	for _, n := range selected {
		if n.ID == "n1" {
			found = true
		}
	}
	if !found {
		t.Error("n1 (lexical match) must be selected")
	}
}

func TestScoreTextRelevantAndDisjoint(t *testing.T) {
	// Text-only parity: full overlap -> l1=1.0; disjoint -> l1=0.0.
	rel := ScoreText("alpha beta", nil, "alpha beta gamma", nil, nil, 0.0)
	if math.Abs(rel.L1-1.0) > 1e-6 {
		t.Errorf("relevant L1 = %v, want 1.0", rel.L1)
	}
	dis := ScoreText("alpha beta", nil, "zeta eta theta", nil, nil, 0.0)
	if math.Abs(dis.L1-0.0) > 1e-6 {
		t.Errorf("disjoint L1 = %v, want 0.0", dis.L1)
	}
	// Activation = w_l1*l1 (text-only) with the default weights.
	if math.Abs(rel.Activation-DefaultNodeWeights["w_l1"]) > 1e-6 {
		t.Errorf("relevant activation = %v", rel.Activation)
	}
}

func TestScoreTextPartialOverlap(t *testing.T) {
	// query has 2 tokens, 1 overlaps -> l1 = 0.5.
	r := ScoreText("alpha zulu", nil, "alpha beta gamma", nil, nil, 0.0)
	if math.Abs(r.L1-0.5) > 1e-6 {
		t.Errorf("partial L1 = %v, want 0.5", r.L1)
	}
}

func TestBlackboardWriteBackProvenance(t *testing.T) {
	board := NewBlackboard([]*ContextNode{NewContextNode("seed", "fact", "x", "")}, nil)
	out := NewContextNode("o1", "fact", "alpha", "")
	written := board.WriteBack("lobeA", []*ContextNode{out}, DefaultWriteBack())
	if len(written) != 1 || written[0] != "o1" {
		t.Errorf("written = %v", written)
	}
	if board.Provenance("o1") != "lobeA" {
		t.Errorf("provenance = %q", board.Provenance("o1"))
	}
	if board.Provenance("seed") != "turn" {
		t.Errorf("seed provenance = %q", board.Provenance("seed"))
	}
}

func TestBlackboardWriteBackIncompleteDropsNodes(t *testing.T) {
	board := NewBlackboard(nil, nil)
	out := NewContextNode("o1", "fact", "x", "")
	written := board.WriteBack("lobeA", []*ContextNode{out}, WriteBackOptions{Completed: false})
	if len(written) != 0 {
		t.Errorf("incomplete write must drop nodes, got %v", written)
	}
	if len(board.Nodes()) != 0 {
		t.Error("pool must stay empty after incomplete write")
	}
}

func TestRound4Bankers(t *testing.T) {
	// Build emits trace floats via Round4 (banker's rounding). Spot-check the
	// half-to-even behavior round(0.12345,4)=0.1234, round(0.12355,4)=0.1236.
	cases := []struct{ in, want float64 }{
		{0.12345, 0.1234},
		{0.12355, 0.1236},
		{0.5, 0.5},
	}
	for _, c := range cases {
		if got := round4(c.in); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("round4(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func BenchmarkBuild(b *testing.B) {
	nodes := []*ContextNode{
		NewContextNode("n1", "fact", "alpha beta gamma delta", ""),
		NewContextNode("n2", "memory", "zeta eta theta iota", ""),
		NewContextNode("n3", "fact", "alpha omega", ""),
	}
	w := MergeWeights(nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// reset selection state
		for _, n := range nodes {
			n.Selected = false
		}
		_, _ = Build(nodes, "alpha beta", nil, w, 1600, 0.0, nil)
	}
}
