// Ported from agent_sdk/skills/definition.py — the authoring façade Skill.
//
// A Skill carries the uniform Activable surface: When is its use_when and its
// Signal is what the skill-select step uses to decide whether to surface it this
// turn. It compiles to the ported SkillPack the runtime consumes.
package skills

import (
	"fmt"

	"github.com/mezon/agent-sdk-go/agent_sdk/core/signal"
)

// Skill is the authoring façade — procedural knowledge, progressively disclosed.
type Skill struct {
	ID           string
	UseWhen      string
	Instructions string
	Tools        []string
	// Disclosure — "eager" (inline) | "on_demand" (model calls skill.read).
	Disclosure  string
	Files       map[string]string
	Name        string
	Description string
	Stages      []string
	Checklist   []map[string]any
	ContextVars []map[string]any
	SourceDir   string

	signalFn signal.Signal
}

// SkillOption configures a Skill in the functional-options builder.
type SkillOption func(*Skill)

// When sets the skill's use_when (the surface signal description).
func When(s string) SkillOption { return func(sk *Skill) { sk.UseWhen = s } }

// Instructions sets the skill body.
func Instructions(s string) SkillOption { return func(sk *Skill) { sk.Instructions = s } }

// WithTools sets the skill's required tools.
func WithTools(tools ...string) SkillOption { return func(sk *Skill) { sk.Tools = tools } }

// Disclosure sets "eager" | "on_demand".
func Disclosure(d string) SkillOption { return func(sk *Skill) { sk.Disclosure = d } }

// WithFiles sets the reference files.
func WithFiles(files map[string]string) SkillOption {
	return func(sk *Skill) { sk.Files = files }
}

// WithName overrides the display name (defaults to id).
func WithName(n string) SkillOption { return func(sk *Skill) { sk.Name = n } }

// WithDescription overrides the description (defaults to when).
func WithDescription(d string) SkillOption { return func(sk *Skill) { sk.Description = d } }

// WithStages sets the stages this skill targets.
func WithStages(stages ...string) SkillOption { return func(sk *Skill) { sk.Stages = stages } }

// WithChecklist sets declarative wizard steps.
func WithChecklist(c []map[string]any) SkillOption {
	return func(sk *Skill) { sk.Checklist = c }
}

// WithContextVars sets skill-scoped workspace state.
func WithContextVars(v []map[string]any) SkillOption {
	return func(sk *Skill) { sk.ContextVars = v }
}

// WithSourceDir records the folder the skill was loaded from.
func WithSourceDir(dir string) SkillOption { return func(sk *Skill) { sk.SourceDir = dir } }

// WithSignalFn sets a compiled signal function directly.
func WithSignalFn(fn signal.Signal) SkillOption {
	return func(sk *Skill) { sk.signalFn = fn }
}

// WithSignal compiles a declarative signal expression for the skill. A malformed
// expression panics (authoring-time error, like the Python compile_signal that
// raises in the constructor).
func WithSignal(expr any) SkillOption {
	return func(sk *Skill) {
		fn, err := signal.Compile(expr)
		if err != nil {
			panic(fmt.Sprintf("invalid skill signal: %v", err))
		}
		sk.signalFn = fn
	}
}

// NewSkill builds a Skill. disclosure defaults to "on_demand"; an invalid
// disclosure panics (mirrors the Python ValueError at construction).
func NewSkill(id string, opts ...SkillOption) *Skill {
	sk := &Skill{ID: id, Disclosure: "on_demand"}
	for _, o := range opts {
		o(sk)
	}
	if sk.Disclosure != "eager" && sk.Disclosure != "on_demand" {
		panic("disclosure must be 'eager' or 'on_demand'")
	}
	if sk.Name == "" {
		sk.Name = id
	}
	if sk.Description == "" {
		sk.Description = sk.UseWhen
	}
	return sk
}

// Signal evaluates the skill's surface signal against a context map.
func (s *Skill) Signal(ctx map[string]any) float64 {
	if s.signalFn != nil {
		return s.signalFn(ctx)
	}
	return 0.0
}

// ToPack compiles the authoring Skill into the runtime SkillPack.
func (s *Skill) ToPack() *SkillPack {
	files := map[string]string{}
	for k, v := range s.Files {
		files[k] = v
	}
	return &SkillPack{
		ID:            s.ID,
		Name:          s.Name,
		Description:   s.Description,
		Stages:        append([]string(nil), s.Stages...),
		Instructions:  s.Instructions,
		RequiredTools: append([]string(nil), s.Tools...),
		Injection:     s.Disclosure,
		Files:         files,
		Checklist:     copyDicts(s.Checklist),
		ContextVars:   copyDicts(s.ContextVars),
		SourceDir:     s.SourceDir,
	}
}

func copyDicts(in []map[string]any) []map[string]any {
	if in == nil {
		return nil
	}
	out := make([]map[string]any, len(in))
	for i, d := range in {
		cp := make(map[string]any, len(d))
		for k, v := range d {
			cp[k] = v
		}
		out[i] = cp
	}
	return out
}
