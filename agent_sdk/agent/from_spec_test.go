package agent

import (
	"context"
	"reflect"
	"testing"

	"github.com/mezon/agent-sdk-go/agent_sdk/clients"
	"github.com/mezon/agent-sdk-go/agent_sdk/core/spec"
	"github.com/mezon/agent-sdk-go/agent_sdk/preact"
)

// TestFromSpecRoundTripsDefault is the core contract: build agent A, capture
// its spec, rebuild via FromSpec, and re-render — the JSON must deep-equal.
// Mirrors test_spec.py::test_spec_json_round_trips + the agent_from_spec rebuild.
func TestFromSpecRoundTripsDefault(t *testing.T) {
	a := MustPreactAgent(Config{
		Client:       "echo",
		Instructions: "You are helpful.",
	})
	want := a.Spec().ToJSON()

	rebuilt, err := FromSpec(a.Spec(), "echo")
	if err != nil {
		t.Fatalf("FromSpec: %v", err)
	}
	got := rebuilt.Spec().ToJSON()
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("round-trip mismatch:\n want=%#v\n got =%#v", want, got)
	}
}

// TestFromSpecRoundTripsCustomNetwork exercises a custom lobes/stages/flows
// network (so stages/flows rows are non-empty) and asserts an exact re-render.
func TestFromSpecRoundTripsCustomNetwork(t *testing.T) {
	a := MustPreactAgent(Config{
		Client:           "echo",
		Instructions:     "Be precise.",
		Lobes:            preact.Lobes{}.Minimal(),
		Stages:           preact.Stages{}.Minimal(),
		Flows:            preact.Flows{}.Minimal(),
		RequireCitations: true,
		TZ:               "Asia/Bangkok",
		Lang:             "vi",
		Weights:          map[string]any{"flow_qna__lobe_synthesize": 2.0},
		Budgets:          map[string]any{"layer_4": 3},
	})
	want := a.Spec().ToJSON()

	rebuilt, err := FromSpec(a.Spec(), "echo")
	if err != nil {
		t.Fatalf("FromSpec: %v", err)
	}
	got := rebuilt.Spec().ToJSON()
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("round-trip mismatch:\n want=%#v\n got =%#v", want, got)
	}
}

// TestFromSpecFoldsNamedAliases mirrors agent_from_spec's field-folding: the
// named authoring aliases (flow_lobe_weights / flow_layer_budgets) fold into
// weights/budgets, with the named field winning a key collision.
func TestFromSpecFoldsNamedAliases(t *testing.T) {
	s := spec.NewSpec()
	s.Weights = map[string]any{"a": 1.0, "shared": 1.0}
	s.Budgets = map[string]any{"b": 2.0, "sharedb": 2.0}
	s.FlowLobeWeights = map[string]any{"c": 3.0, "shared": 9.0}
	s.FlowLayerBudgets = map[string]any{"d": 4.0, "sharedb": 8.0}

	rebuilt, err := FromSpec(s, "echo")
	if err != nil {
		t.Fatalf("FromSpec: %v", err)
	}
	w := rebuilt.Engine().Weights
	if w["a"] != 1.0 || w["c"] != 3.0 || w["shared"] != 9.0 {
		t.Fatalf("weights fold wrong: %#v", w)
	}
	bud := rebuilt.Engine().Budgets
	if bud["b"] != 2.0 || bud["d"] != 4.0 || bud["sharedb"] != 8.0 {
		t.Fatalf("budgets fold wrong: %#v", bud)
	}
}

// TestFromSpecAgentRuns confirms the rebuilt agent is wired and runnable.
func TestFromSpecAgentRuns(t *testing.T) {
	a := MustPreactAgent(Config{
		Client:       clients.NewFakeClient([]any{"rebuilt answer"}, nil),
		Instructions: "hello",
		Lobes:        preact.Lobes{}.Minimal(),
		Stages:       preact.Stages{}.Minimal(),
		Flows:        preact.Flows{}.Minimal(),
	})
	rebuilt, err := FromSpec(a.Spec(), clients.NewFakeClient([]any{"rebuilt answer"}, nil))
	if err != nil {
		t.Fatalf("FromSpec: %v", err)
	}
	res, err := rebuilt.Query(context.Background(), "a question?")
	if err != nil {
		t.Fatalf("rebuilt query: %v", err)
	}
	if res.Text != "rebuilt answer" {
		t.Fatalf("rebuilt answer = %q; want %q", res.Text, "rebuilt answer")
	}
}
