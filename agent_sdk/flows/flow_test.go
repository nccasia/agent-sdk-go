package flows

import (
	"reflect"
	"testing"
)

func TestFlowStepDefaults(t *testing.T) {
	fs := NewFlowStep(FlowStep{Name: "synthesize"})
	if fs.Loop != "single" {
		t.Fatalf("loop = %q, want single", fs.Loop)
	}
	if fs.FanoutMax != 40 {
		t.Fatalf("fanout_max = %d, want 40", fs.FanoutMax)
	}
	if fs.Type() != "simple" {
		t.Fatalf("type = %q, want simple", fs.Type())
	}
	if got := fs.Signals(map[string]any{}); len(got) != 0 {
		t.Fatalf("default signals = %v, want empty", got)
	}
}

func TestFlowStepTypeMapping(t *testing.T) {
	cases := map[string]string{"agentic": "react", "single": "simple", "map": "map", "none": "none"}
	for loop, want := range cases {
		step := FlowStep{Name: "s", Loop: loop}
		if loop == "map" {
			step.FanoutKey = "k"
		}
		fs := NewFlowStep(step)
		if fs.Type() != want {
			t.Fatalf("loop %q type = %q, want %q", loop, fs.Type(), want)
		}
	}
}

func TestFlowStepInvalidLoopPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on invalid loop")
		}
	}()
	NewFlowStep(FlowStep{Name: "s", Loop: "bogus"})
}

func TestFlowStepMapRequiresFanoutKey(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic when loop=map without fanout_key")
		}
	}()
	NewFlowStep(FlowStep{Name: "s", Loop: "map"})
}

func TestNewFlowDefaults(t *testing.T) {
	f := NewFlow("qna", FlowStages("plan", "synthesize"))
	if f.ID() != "qna" {
		t.Fatalf("id = %q, want qna", f.ID())
	}
	if f.Name() != "qna" {
		t.Fatalf("name = %q, want qna (defaults to id)", f.Name())
	}
	if f.Threshold != 0.5 {
		t.Fatalf("threshold = %v, want 0.5", f.Threshold)
	}
	if !f.Grounds {
		t.Fatal("grounds should default to true")
	}
	if !reflect.DeepEqual(f.Stages, []string{"plan", "synthesize"}) {
		t.Fatalf("stages = %v", f.Stages)
	}
	if got := f.Signal(map[string]any{}); got != 0.0 {
		t.Fatalf("default signal = %v, want 0.0", got)
	}
}

func TestNewFlowSignalFn(t *testing.T) {
	f := NewFlow("qna", FlowSignalFn(func(map[string]any) float64 { return 1.0 }))
	if got := f.Signal(map[string]any{}); got != 1.0 {
		t.Fatalf("signal = %v, want 1.0", got)
	}
}

func TestNewFlowSignalExprConst(t *testing.T) {
	f := NewFlow("qna", FlowSignalExpr(map[string]any{"const": 1.0}))
	if got := f.Signal(map[string]any{}); got != 1.0 {
		t.Fatalf("const signal = %v, want 1.0", got)
	}
	if f.SignalExpr == nil {
		t.Fatal("SignalExpr should be retained for round-tripping")
	}
}

func TestStepFlowID(t *testing.T) {
	sf := StepFlow{Name: "research", Steps: []FlowStep{NewFlowStep(FlowStep{Name: "plan"})}}
	if sf.ID() != "research" {
		t.Fatalf("id = %q, want research", sf.ID())
	}
}

func TestFlowStepResultAliases(t *testing.T) {
	r := FlowStepResult{Flow: "qna", Step: "synthesize"}
	if r.StageName() != "synthesize" {
		t.Fatalf("StageName = %q", r.StageName())
	}
	if r.Path() != "qna" {
		t.Fatalf("Path = %q", r.Path())
	}
}
