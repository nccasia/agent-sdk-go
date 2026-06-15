// Ported from the Skill authoring tests in test_extra_coverage.py and the
// Signal wiring scattered across the suite.
package skills

import (
	"reflect"
	"strings"
	"testing"
)

func TestNewSkillDefaultsOnDemand(t *testing.T) {
	sk := NewSkill("x", When("w"), Instructions("i"))
	if sk.ID != "x" {
		t.Errorf("id: got %q", sk.ID)
	}
	if sk.Disclosure != "on_demand" {
		t.Errorf("disclosure: got %q, want on_demand", sk.Disclosure)
	}
	if sk.Name != "x" {
		t.Errorf("name default: got %q, want x", sk.Name)
	}
	if sk.Description != "w" {
		t.Errorf("description default: got %q, want w", sk.Description)
	}
}

func TestNewSkillInvalidDisclosurePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic for invalid disclosure")
		}
	}()
	NewSkill("x", When("w"), Instructions("i"), Disclosure("bogus"))
}

func TestNewSkillEagerAllowed(t *testing.T) {
	sk := NewSkill("x", When("w"), Instructions("i"), Disclosure("eager"))
	if sk.Disclosure != "eager" {
		t.Errorf("eager: got %q", sk.Disclosure)
	}
}

func TestNewSkillOptionsOverrideDefaults(t *testing.T) {
	sk := NewSkill("x",
		When("w"),
		Instructions("i"),
		WithName("My Name"),
		WithDescription("My desc"),
		WithTools("a", "b"),
		WithStages("s1", "s2"),
		WithChecklist([]map[string]any{{"key": "k"}}),
		WithContextVars([]map[string]any{{"key": "v"}}),
		WithSourceDir("/tmp"),
	)
	if sk.Name != "My Name" {
		t.Errorf("name override: %q", sk.Name)
	}
	if sk.Description != "My desc" {
		t.Errorf("description override: %q", sk.Description)
	}
	if !reflect.DeepEqual(sk.Tools, []string{"a", "b"}) {
		t.Errorf("tools: %v", sk.Tools)
	}
	if !reflect.DeepEqual(sk.Stages, []string{"s1", "s2"}) {
		t.Errorf("stages: %v", sk.Stages)
	}
	if len(sk.Checklist) != 1 {
		t.Errorf("checklist: %v", sk.Checklist)
	}
	if sk.SourceDir != "/tmp" {
		t.Errorf("source_dir: %q", sk.SourceDir)
	}
}

func TestToPackCopiesAndShapesFields(t *testing.T) {
	sk := NewSkill("x",
		When("w"),
		Instructions("body"),
		WithTools("a", "b"),
		WithStages("s"),
		WithFiles(map[string]string{"f": "content"}),
		WithChecklist([]map[string]any{{"key": "c"}}),
		WithContextVars([]map[string]any{{"key": "v"}}),
		WithSourceDir("/src"),
	)
	p := sk.ToPack()
	if p.ID != "x" || p.Injection != "on_demand" {
		t.Errorf("pack shape wrong: %+v", p)
	}
	if p.Instructions != "body" {
		t.Errorf("instructions: %q", p.Instructions)
	}
	if !reflect.DeepEqual(p.RequiredTools, []string{"a", "b"}) {
		t.Errorf("required_tools: %v", p.RequiredTools)
	}
	// Files is a copy — mutating the original should not change the pack.
	sk.Files["new"] = "x"
	if _, ok := p.Files["new"]; ok {
		t.Errorf("ToPack did not copy files map")
	}
	// Stages is a copy too.
	sk.Stages = append(sk.Stages, "extra")
	if len(p.Stages) != 1 {
		t.Errorf("ToPack did not copy stages slice")
	}
}

func TestSignalDefaultZero(t *testing.T) {
	sk := NewSkill("x", When("w"), Instructions("i"))
	if got := sk.Signal(map[string]any{"any": 1}); got != 0.0 {
		t.Errorf("default signal: got %v, want 0", got)
	}
}

func TestSignalConstantWiring(t *testing.T) {
	sk := NewSkill("x", When("w"), Instructions("i"), WithSignal(map[string]any{"const": 0.7}))
	if got := sk.Signal(map[string]any{}); got != 0.7 {
		t.Errorf("const signal: got %v, want 0.7", got)
	}
}

func TestSignalPanicsOnInvalidExpression(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic for invalid signal expr")
		}
	}()
	// signal.Compile raises on an unknown op.
	NewSkill("x", When("w"), Instructions("i"),
		WithSignal(map[string]any{"op": "no_such_op", "value": 1.0}))
	// Defensive: panic message should mention "signal".
	defer func() {
		if r := recover(); r != nil {
			msg, _ := r.(string)
			if !strings.Contains(msg, "signal") {
				t.Errorf("panic message should mention signal, got: %v", r)
			}
		}
	}()
}
