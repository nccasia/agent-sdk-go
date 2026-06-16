package benchmarks

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/nccasia/agent-sdk-go/agent_sdk/agent"
	"github.com/nccasia/agent-sdk-go/agent_sdk/probe"
	"github.com/nccasia/agent-sdk-go/agent_sdk/session"
	"github.com/nccasia/agent-sdk-go/agent_sdk/skills"
)

// skillbench — the LIVE bench for the SDK's skill system. It drives the REAL
// PreactAgent over a corpus of SKILL.md folders + scenarios, against a real
// provider, and scores — deterministically, from the probe trace — how it
// PARSES a skill, MAPS it onto the engine (stages/tools), ACTIVATES the right
// one, FOLLOWS its instructions, and lets content FUNNEL (navigate, not dump).
// No LLM judge. Six groups, each a Mode:
//
//   - lint    — adversarial fixtures (slug "_…") are rejected by SOME validator.
//   - parse   — the SOP folder parses into a navigable structure (sections, ToC, search).
//   - mapping — the skill maps onto the engine: present only in declared stages,
//     on_demand→index+ActivateSkill / eager→inline, the activation tool exposed.
//   - activation — the model loads the right skill, not the distractors (precision/recall ≥0.8).
//   - follow  — the answer obeys the loaded skill's mandated behavior.
//   - funnel  — skill content navigates (search/section reads), it does not dump.
//
// lint/parse/mapping are pure (functions of the fixtures + rendered prompt);
// activation/follow/funnel need the model. Like Python's run.py, the whole bench
// is LIVE: run.py refuses to compose a verdict without a provider token (exit
// 2). The Go bench reproduces that — with no model every group mode is missing,
// so the verdict is UNMEASURED (no evidence is never READY). Ported from
// benchmarks/skillbench/{run,scoring,loader}.py.

// all: keeps the adversarial fixtures whose folder name starts with "_" — the
// default //go:embed pattern skips names beginning with "_" or ".".
//
//go:embed all:skillbench_dataset
var skillBenchData embed.FS

const skillBenchInstr = "You are a capable assistant. When a skill matches the user's request, activate it " +
	"and follow its procedure. Answer in the user's language."

// ── thresholds (the readiness bar) — mirror scoring.py ───────────────────────
const (
	skillBenchActivationRecallMin    = 0.8
	skillBenchActivationPrecisionMin = 0.8
	skillBenchDisclosureRatioMax     = 0.35
	skillBenchFunnelPeakCharsMax     = 12000
)

var skillBenchVague = []string{
	"help you with stuff", "do the thing", "i can help", "various things", "anything",
}

// skillBenchModeNames is the group surface (the six Modes). Asserted for parity
// and used to pre-populate the missing (no-provider) floor.
func skillBenchModeNames() []string {
	return []string{"lint", "parse", "mapping", "activation", "follow", "funnel"}
}

// RunSkillBench composes the skillbench verdict. With no model (the deterministic
// floor) every group mode is missing → UNMEASURED. With a model the real
// PreactAgent is driven over the scenarios and each group scored.
func RunSkillBench(ctx context.Context, model string) (Verdict, error) {
	payloads := map[string]*ModePayload{}
	for _, m := range skillBenchModeNames() {
		payloads[m] = nil // missing without a provider
	}
	if model != "" {
		measured, err := skillBenchLive(ctx, model)
		if err != nil {
			return Verdict{}, err
		}
		for m, p := range measured {
			payloads[m] = p
		}
	}
	return ComposeVerdict(payloads, nil), nil
}

// RunSkillBenchProbes captures inspectable traces for the viewer. With a real
// model it drives the actual agent; offline (model=="") it builds the SAME
// representative agent (the production skill corpus mounted, Funnel +
// ToolsInPrompt) against a FakeClient and runs ONE representative scenario
// through probe.Probe, so the inspection renders a real path/flow + the executed
// stages (and any activated skill). Adds traces only — the live verdict (Run)
// stays UNMEASURED without a provider. Mirrors run.py's scenario probe feeding
// write_viewer.
func RunSkillBenchProbes(ctx context.Context, model string) ([]*probe.Record, error) {
	production, _, err := skillBenchLoad()
	if err != nil {
		return nil, err
	}
	bySlug := map[string]*skills.SkillPack{}
	for _, p := range production {
		bySlug[p.ID] = p
	}
	scenarios, err := skillBenchScenarios()
	if err != nil {
		return nil, err
	}
	if len(scenarios) == 0 {
		return nil, nil
	}
	sc := scenarios[0]
	underTest := []any{}
	for _, s := range sc.SkillsUnderTest {
		if p := bySlug[s]; p != nil {
			underTest = append(underTest, p)
		}
	}
	ag := agent.MustPreactAgent(agent.Config{
		Client:          benchProbeClient(model),
		Instructions:    skillBenchInstr,
		UniversalMemory: false,
		Skills:          underTest,
		Funnel:          true,
		ToolsInPrompt:   true,
		Session:         session.New("sb-"+sc.ID, nil),
	})
	rec, err := probe.Probe(ctx, ag, sc.Query, probe.WithLabel(sc.Category+" · "+sc.ID))
	if err != nil {
		return nil, err
	}
	return []*probe.Record{rec}, nil
}

// ── corpus loading (the bench's own loader, mirrors loader.py) ───────────────

// skillBenchLoad loads the embedded SKILL.md corpus and splits it into the
// production skills and the adversarial negatives (slug starts with "_").
func skillBenchLoad() (production, negatives []*skills.SkillPack, err error) {
	root := "skillbench_dataset/skills"
	entries, derr := skillBenchData.ReadDir(root)
	if derr != nil {
		return nil, nil, derr
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		pack, lerr := skillBenchLoadPack(root + "/" + n)
		if lerr != nil {
			return nil, nil, lerr
		}
		if pack == nil {
			continue
		}
		if strings.HasPrefix(pack.ID, "_") {
			negatives = append(negatives, pack)
		} else {
			production = append(production, pack)
		}
	}
	return production, negatives, nil
}

// skillBenchLoadPack parses one <dir>/SKILL.md (+ sibling text files) from the
// embedded FS into a SkillPack — mirrors loader.py:load_skill (default stages
// "synthesize", disclosure from `injection`).
func skillBenchLoadPack(dir string) (*skills.SkillPack, error) {
	raw, err := skillBenchData.ReadFile(dir + "/SKILL.md")
	if err != nil {
		return nil, nil // no SKILL.md ⇒ skip
	}
	front, body, err := skills.ParseSkillMD(string(raw))
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(skillBenchFrontStr(front, "name"))
	description := strings.TrimSpace(skillBenchFrontStr(front, "description"))
	if name == "" || description == "" {
		return nil, fmt.Errorf("SKILL.md in %s must declare name and description", dir)
	}

	files := map[string]string{}
	if err := skillBenchWalkFiles(dir, files); err != nil {
		return nil, err
	}

	stages := skillBenchFrontStrSlice(front, "stages")
	if len(stages) == 0 {
		stages = []string{"synthesize"}
	}
	injection := strings.TrimSpace(skillBenchFrontStr(front, "injection"))
	if injection == "" {
		injection = "on_demand"
	}
	slug := strings.TrimSpace(skillBenchFrontStr(front, "slug"))
	if slug == "" {
		slug = dir[strings.LastIndex(dir, "/")+1:]
	}
	instructions := strings.TrimSpace(body)
	if instructions == "" {
		instructions = "SKILL: " + name
	}
	return &skills.SkillPack{
		ID:            slug,
		Name:          name,
		Description:   description,
		Stages:        stages,
		Instructions:  instructions,
		RequiredTools: skillBenchFrontStrSlice(front, "required_tools"),
		Injection:     injection,
		Files:         files,
		Checklist:     skillBenchFrontDictSlice(front, "checklist"),
		ContextVars:   skillBenchFrontDictSlice(front, "context_vars"),
		SourceDir:     dir,
	}, nil
}

var skillBenchTextSuffix = map[string]struct{}{".md": {}, ".markdown": {}, ".txt": {}}

// skillBenchWalkFiles loads every sibling *.md/*.txt under dir (recursively,
// excluding SKILL.md) keyed by its slash path relative to dir.
func skillBenchWalkFiles(dir string, out map[string]string) error {
	entries, err := skillBenchData.ReadDir(dir)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		path := dir + "/" + e.Name()
		if e.IsDir() {
			sub := map[string]string{}
			if err := skillBenchWalkFiles(path, sub); err != nil {
				return err
			}
			rel := e.Name() + "/"
			for k, v := range sub {
				out[rel+k] = v
			}
			continue
		}
		if e.Name() == "SKILL.md" {
			continue
		}
		dot := strings.LastIndex(e.Name(), ".")
		if dot < 0 {
			continue
		}
		if _, ok := skillBenchTextSuffix[strings.ToLower(e.Name()[dot:])]; !ok {
			continue
		}
		data, rerr := skillBenchData.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		out[e.Name()] = string(data)
	}
	return nil
}

func skillBenchFrontStr(front map[string]any, key string) string {
	if s, ok := front[key].(string); ok {
		return s
	}
	return ""
}

func skillBenchFrontStrSlice(front map[string]any, key string) []string {
	switch v := front[key].(type) {
	case []string:
		return append([]string(nil), v...)
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	}
	return nil
}

func skillBenchFrontDictSlice(front map[string]any, key string) []map[string]any {
	if v, ok := front[key].([]map[string]any); ok {
		return v
	}
	return nil
}

// ── lint (adversarial fixtures rejected) ─────────────────────────────────────

// skillBenchLintChecks: a deliberately-bad skill must be flagged by SOME
// validator. Mirrors scoring.py:lint_checks.
func skillBenchLintChecks(negatives []*skills.SkillPack) []Check {
	out := []Check{}
	for _, sk := range negatives {
		issues := skillBenchNegativeDefects(sk)
		detail := "NOT flagged (bad!)"
		if len(issues) > 0 {
			detail = "flagged: " + strings.Join(issues, "; ")
		}
		out = append(out, ck("lint.rejects["+sk.ID+"]", len(issues) > 0, detail))
	}
	return out
}

// skillBenchNegativeDefects is the union of every deterministic defect an
// adversarial fixture trips — the description lint plus the structural
// validators (ToC navigability, checklist). Mirrors scoring.py:negative_defects.
func skillBenchNegativeDefects(sk *skills.SkillPack) []string {
	defects := append([]string(nil), skillBenchDescriptionIssues(sk.Description)...)
	for _, fname := range skillBenchLargeFileNames(sk) {
		if !skillBenchTocNavigable(sk.Files[fname]) {
			defects = append(defects, fname+": large file not navigable (no ToC)")
		}
	}
	if len(sk.Checklist) > 0 && !skillBenchChecklistValid(sk) {
		defects = append(defects, "degenerate checklist (step missing title/ask)")
	}
	return defects
}

// ── parse (SOP folder → structure) ───────────────────────────────────────────

// skillBenchDescriptionIssues lints a skill description. Mirrors
// scoring.py:description_issues.
func skillBenchDescriptionIssues(description string) []string {
	d := strings.TrimSpace(description)
	issues := []string{}
	if len(d) < 30 {
		issues = append(issues, "description too short (needs WHAT + WHEN)")
	}
	if len(d) > 1024 {
		issues = append(issues, "description > 1024 chars")
	}
	low := strings.ToLower(d)
	for _, v := range skillBenchVague {
		if strings.Contains(low, v) {
			issues = append(issues, "vague description (no concrete trigger)")
			break
		}
	}
	hasWhen := strings.Contains(low, "use when") || strings.Contains(low, "use this") ||
		strings.Contains(low, "когда")
	if !hasWhen {
		for _, w := range []string{"when ", "khi ", "if the user", "for "} {
			if strings.Contains(low, w) {
				hasWhen = true
				break
			}
		}
	}
	if !hasWhen {
		issues = append(issues, "no WHEN signal (say when to use it)")
	}
	return issues
}

// skillBenchLargeFileNames returns the reference files large enough to require
// layered reading (sorted). Mirrors scoring.py:large_files (iterated by name).
func skillBenchLargeFileNames(sk *skills.SkillPack) []string {
	out := []string{}
	for f, c := range sk.Files {
		if skills.EstTokens(c) > skills.FullFileTokens {
			out = append(out, f)
		}
	}
	sort.Strings(out)
	return out
}

// skillBenchTocNavigable: a large file is navigable when it splits into ≥2 real
// sections. Mirrors scoring.py:toc_navigable.
func skillBenchTocNavigable(content string) bool {
	return len(skills.SplitSections(content)) >= 2
}

// skillBenchChecklistValid: each step materializes something to present (title
// or ask). Empty checklist is vacuously valid. Mirrors scoring.py:checklist_valid.
func skillBenchChecklistValid(sk *skills.SkillPack) bool {
	if len(sk.Checklist) == 0 {
		return true
	}
	for _, s := range sk.Checklist {
		title, _ := s["title"].(string)
		ask, _ := s["ask"].(string)
		if title == "" && ask == "" {
			return false
		}
	}
	return true
}

// skillBenchParseChecks: deterministic per-skill structure checks. Mirrors
// scoring.py:parse_checks.
func skillBenchParseChecks(sk *skills.SkillPack) []Check {
	sid := sk.ID
	out := []Check{}
	issues := skillBenchDescriptionIssues(sk.Description)
	detail := "ok"
	if len(issues) > 0 {
		detail = strings.Join(issues, "; ")
	}
	out = append(out, ck("parse."+sid+".description", len(issues) == 0, detail))

	body := sk.Instructions
	secs := skills.SplitSections(body)
	out = append(out, ck("parse."+sid+".body", strings.TrimSpace(body) != "" && len(secs) > 0,
		fmt.Sprintf("%d section(s)", len(secs))))

	for _, fname := range skillBenchLargeFileNames(sk) {
		n := len(skills.SplitSections(sk.Files[fname]))
		out = append(out, ck("parse."+sid+".toc["+fname+"]", skillBenchTocNavigable(sk.Files[fname]),
			fmt.Sprintf("large file → ToC of %d section(s)", n)))
	}

	if len(sk.Checklist) > 0 {
		out = append(out, ck("parse."+sid+".checklist", skillBenchChecklistValid(sk),
			fmt.Sprintf("%d step(s)", len(sk.Checklist))))
	}
	return out
}

// skillBenchSearchSelfLocates: search_bundle finds term in the expected
// skill/file. Mirrors scoring.py:search_self_locates.
func skillBenchSearchSelfLocates(packs []*skills.SkillPack, term, wantSkill, wantFile string) Check {
	hits := skills.SearchBundle(packs, term, 3)
	ok := false
	parts := []string{}
	for _, h := range hits {
		if h.File == wantFile {
			ok = true
		}
		parts = append(parts, h.File+"#"+h.Section)
	}
	detail := "(no hits)"
	if len(parts) > 0 {
		detail = strings.Join(parts, ", ")
	}
	return ck("parse."+wantSkill+".search", ok, fmt.Sprintf("%q → %s", term, detail))
}

// ── mapping (skill → stages / tools) ─────────────────────────────────────────

// skillBenchMappingChecks: deterministic stage/tool mapping over the rendered
// skill prompt block. Mirrors scoring.py:mapping_checks.
func skillBenchMappingChecks(packs []*skills.SkillPack, exposedToolNames map[string]struct{}) []Check {
	registry := skills.NewSkillRegistry(packs)
	ids := make([]any, len(packs))
	for i, p := range packs {
		ids[i] = p.ID
	}
	policy := map[string]any{
		"capabilities":   map[string]any{"skills": ids},
		"skill_strategy": "static",
	}
	out := []Check{}
	hasOnDemand := false
	for _, p := range packs {
		if p.Injection == "on_demand" {
			hasOnDemand = true
			break
		}
	}

	for _, sk := range packs {
		sid := sk.ID
		declared := "synthesize"
		if len(sk.Stages) > 0 {
			declared = sk.Stages[0]
		}
		blockIn := skills.BuildPromptBlock(registry, policy, declared, skills.PromptOptions{})
		off := "cite"
		if skillBenchContains(sk.Stages, "cite") {
			off = "filter"
		}
		blockOff := skills.BuildPromptBlock(registry, policy, off, skills.PromptOptions{})

		inDeclared := strings.Contains(blockIn, sk.Name) ||
			strings.Contains(blockIn, skillBenchHead(sk.Description, 40)) ||
			strings.Contains(blockIn, skillBenchHead(sk.Instructions, 40))
		out = append(out, ck("mapping."+sid+".in_declared_stage", inDeclared,
			fmt.Sprintf("present in %q", declared)))

		leaked := strings.Contains(blockOff, sk.Name) ||
			strings.Contains(blockOff, skillBenchHead(sk.Instructions, 40))
		out = append(out, ck("mapping."+sid+".absent_off_stage", !leaked,
			fmt.Sprintf("absent from %q", off)))

		if sk.Injection == "on_demand" {
			ok := strings.Contains(blockIn, sk.Name) && strings.Contains(blockIn, skills.ACTIVATE)
			out = append(out, ck("mapping."+sid+".index_and_directive", ok,
				"one-line index + ActivateSkill directive"))
		} else {
			ok := strings.Contains(blockIn, skillBenchHead(sk.Instructions, 40))
			out = append(out, ck("mapping."+sid+".inlined", ok,
				"body inlined (no activation needed)"))
		}
	}

	if hasOnDemand {
		_, exposed := exposedToolNames[skills.ACTIVATE]
		out = append(out, ck("mapping.activation_tool_exposed", exposed,
			fmt.Sprintf("ActivateSkill in exposed tools: %v", exposed)))
	}
	return out
}

// ── live run (activation / follow / funnel) ──────────────────────────────────

// skillBenchScenario is one line of dataset/scenarios.jsonl.
type skillBenchScenario struct {
	ID                    string            `json:"id"`
	Category              string            `json:"category"`
	Query                 string            `json:"query"`
	SkillsUnderTest       []string          `json:"skills_under_test"`
	Turns                 []string          `json:"turns"`
	ExpectActivation      map[string]bool   `json:"expect_activation"`
	ExpectActivationTurns []map[string]bool `json:"expect_activation_turns"`
	Uplift                map[string]any    `json:"uplift"`
}

// skillBenchScenarios reads the embedded scenarios.jsonl.
func skillBenchScenarios() ([]skillBenchScenario, error) {
	raw, err := skillBenchData.ReadFile("skillbench_dataset/scenarios.jsonl")
	if err != nil {
		return nil, err
	}
	var out []skillBenchScenario
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var s skillBenchScenario
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// skillBenchTally holds the per-skill activation confusion counts.
type skillBenchTally struct{ tp, fp, fn, tn int }

// skillBenchLive drives the real PreactAgent over each scenario and scores the
// activation / follow / funnel groups. Mirrors run.py's live section.
func skillBenchLive(ctx context.Context, model string) (map[string]*ModePayload, error) {
	production, _, err := skillBenchLoad()
	if err != nil {
		return nil, err
	}
	bySlug := map[string]*skills.SkillPack{}
	for _, p := range production {
		bySlug[p.ID] = p
	}
	scenarios, err := skillBenchScenarios()
	if err != nil {
		return nil, err
	}

	counts := map[string]*skillBenchTally{}
	follow := []Check{}
	funnel := []Check{}
	for _, sc := range scenarios {
		f1, f2, terr := skillBenchRunScenario(ctx, model, sc, bySlug, counts)
		if terr != nil {
			return nil, terr
		}
		follow = append(follow, f1...)
		funnel = append(funnel, f2...)
	}

	out := map[string]*ModePayload{
		"activation": NewPayload(skillBenchActivationChecks(counts), nil),
	}
	if len(follow) > 0 {
		out["follow"] = NewPayload(follow, nil)
	}
	if len(funnel) > 0 {
		out["funnel"] = NewPayload(funnel, nil)
	}
	return out, nil
}

// skillBenchRunScenario runs one scenario (with its follow-up turns), tallies
// activation, and returns its follow + funnel checks. Mirrors run.py:_run_scenario.
func skillBenchRunScenario(ctx context.Context, model string, sc skillBenchScenario,
	bySlug map[string]*skills.SkillPack, counts map[string]*skillBenchTally) (follow, funnel []Check, err error) {

	underTest := []any{}
	var primary *skills.SkillPack
	for _, s := range sc.SkillsUnderTest {
		if p := bySlug[s]; p != nil {
			underTest = append(underTest, p)
			if primary == nil {
				primary = p
			}
		}
	}
	ag := agent.MustPreactAgent(agent.Config{
		Client:          model,
		Instructions:    skillBenchInstr,
		UniversalMemory: false,
		Skills:          underTest,
		Funnel:          true,
		ToolsInPrompt:   true,
		Session:         session.New("sb-"+sc.ID, nil),
	})

	rec, err := probe.Probe(ctx, ag, sc.Query, probe.WithLabel(sc.Category+" · "+sc.ID))
	if err != nil {
		return nil, nil, err
	}
	perTurn := []map[string]struct{}{skillBenchActivatedSlugs(rec)}
	for i, followUp := range sc.Turns {
		r, perr := probe.Probe(ctx, ag, followUp, probe.WithLabel(fmt.Sprintf("%s · turn%d", sc.ID, i+1)))
		if perr != nil {
			return nil, nil, perr
		}
		perTurn = append(perTurn, skillBenchActivatedSlugs(r))
		rec = r
	}
	activated := skillBenchUnion(perTurn)

	ondemand := func(slug string) bool {
		sk := bySlug[slug]
		return sk != nil && sk.Injection == "on_demand"
	}

	if len(sc.ExpectActivationTurns) > 0 {
		for ti, exp := range sc.ExpectActivationTurns {
			if ti >= len(perTurn) {
				break
			}
			for slug, expected := range exp {
				if ondemand(slug) {
					skillBenchTallyOne(counts, slug, expected, skillBenchHas(perTurn[ti], slug))
				}
			}
		}
	} else {
		for slug, expected := range sc.ExpectActivation {
			if ondemand(slug) {
				skillBenchTallyOne(counts, slug, expected, skillBenchHas(activated, slug))
			}
		}
	}

	if len(sc.Uplift) > 0 {
		follow = skillBenchFollowChecks(sc, rec)
	}
	if primary != nil && skillBenchBundleTokens(primary) > 0 {
		funnel = skillBenchFunnelChecks(sc, rec, skillBenchBundleTokens(primary))
	}
	return follow, funnel, nil
}

func skillBenchTallyOne(counts map[string]*skillBenchTally, slug string, expected, activated bool) {
	c := counts[slug]
	if c == nil {
		c = &skillBenchTally{}
		counts[slug] = c
	}
	switch {
	case expected && activated:
		c.tp++
	case expected && !activated:
		c.fn++
	case !expected && activated:
		c.fp++
	default:
		c.tn++
	}
}

// skillBenchActivatedSlugs: the skills the model ACTIVATED this turn (structural:
// ActivateSkill calls). Mirrors scoring.py:activated_slugs.
func skillBenchActivatedSlugs(rec *probe.Record) map[string]struct{} {
	out := map[string]struct{}{}
	for _, c := range rec.ToolCalls {
		if name, _ := c["name"].(string); name != skills.ACTIVATE {
			continue
		}
		if inp, ok := c["input"].(map[string]any); ok {
			if slug, _ := inp["slug"].(string); slug != "" {
				out[slug] = struct{}{}
			}
		}
	}
	return out
}

// skillBenchActivationChecks rolls TP/FP/FN per skill into precision/recall
// checks. Mirrors scoring.py:activation_checks.
func skillBenchActivationChecks(counts map[string]*skillBenchTally) []Check {
	out := []Check{}
	for _, sid := range sortedKeys(counts) {
		c := counts[sid]
		precision, recall := skillBenchPRF(c.tp, c.fp, c.fn)
		ok := recall >= skillBenchActivationRecallMin && precision >= skillBenchActivationPrecisionMin
		out = append(out, ck("activation."+sid, ok,
			fmt.Sprintf("P=%.3g R=%.3g (tp%d fp%d fn%d)", precision, recall, c.tp, c.fp, c.fn)))
	}
	return out
}

func skillBenchPRF(tp, fp, fn int) (precision, recall float64) {
	precision = 1.0
	if tp+fp > 0 {
		precision = float64(tp) / float64(tp+fp)
	}
	recall = 1.0
	if tp+fn > 0 {
		recall = float64(tp) / float64(tp+fn)
	}
	return round3f(precision), round3f(recall)
}

// ── follow (did the answer obey the loaded skill?) ───────────────────────────

var skillBenchBlockRepr = regexp.MustCompile(`(?s)\b\w+Block\(.*?type='[a-z_]+'\)`)

// skillBenchCleanAnswer strips block-object reprs from a captured answer.
// Mirrors scoring.py:clean_answer.
func skillBenchCleanAnswer(text string) string {
	return strings.TrimSpace(skillBenchBlockRepr.ReplaceAllString(text, ""))
}

// skillBenchFollowChecks scores the answer against the loaded skill's mandated
// behavior. Mirrors scoring.py:follow_checks.
func skillBenchFollowChecks(sc skillBenchScenario, rec *probe.Record) []Check {
	// Python carries the uplift skill id on each row for the per-skill rollup;
	// the Go Check has no skill field, so it is not threaded here.
	up := sc.Uplift
	ans := skillBenchCleanAnswer(rec.Answer)
	out := []Check{}
	for _, must := range skillBenchStrList(up["must_include"]) {
		out = append(out, ck(fmt.Sprintf("follow.%s.has[%s]", sc.ID, skillBenchHead(must, 24)),
			strings.Contains(ans, must), fmt.Sprintf("includes %q", must)))
	}
	for _, nope := range skillBenchStrList(up["must_not_include"]) {
		out = append(out, ck(fmt.Sprintf("follow.%s.not[%s]", sc.ID, skillBenchHead(nope, 24)),
			!strings.Contains(ans, nope), fmt.Sprintf("excludes %q", nope)))
	}
	anyset := skillBenchStrList(up["must_include_any"])
	if len(anyset) > 0 {
		low := strings.ToLower(ans)
		hit := ""
		for _, t := range anyset {
			if strings.Contains(low, strings.ToLower(t)) {
				hit = t
				break
			}
		}
		out = append(out, ck(fmt.Sprintf("follow.%s.any", sc.ID), hit != "",
			fmt.Sprintf("any of %v → %q", anyset, hit)))
	}
	return out
}

// ── funnel (did skill content navigate, not dump?) ───────────────────────────

// skillBenchFunnelPeakChars is the peak funnel observation tail size across
// stages. Mirrors scoring.py:funnel_peak_chars.
func skillBenchFunnelPeakChars(rec *probe.Record) int {
	peak := 0
	for _, s := range rec.Stages {
		md, _ := s["metadata"].(map[string]any)
		series, _ := md["funnel_obs_chars"].([]any)
		for _, v := range series {
			if n := skillBenchToInt(v); n > peak {
				peak = n
			}
		}
	}
	return peak
}

// skillBenchNavigated: did the model use skill.read / skill.search rather than
// dump via one ActivateSkill? Mirrors scoring.py:navigated.
func skillBenchNavigated(rec *probe.Record) bool {
	for _, c := range rec.ToolCalls {
		if n, _ := c["name"].(string); n == "skill.read" || n == "skill.search" {
			return true
		}
	}
	return false
}

// skillBenchDisclosureRatio: skill-tool output tokens ÷ bundle tokens. Mirrors
// scoring.py:disclosure_ratio.
func skillBenchDisclosureRatio(rec *probe.Record, bundleTokens int) float64 {
	if bundleTokens == 0 {
		return 0
	}
	pulled := 0
	for _, c := range rec.ToolCalls {
		name, _ := c["name"].(string)
		if strings.HasPrefix(name, "skill") || name == skills.ACTIVATE {
			out, _ := c["output"].(string)
			pulled += skills.EstTokens(out)
		}
	}
	return round3f(float64(pulled) / float64(bundleTokens))
}

// skillBenchFunnelChecks scores whether skill content navigated, not dumped.
// Mirrors scoring.py:funnel_checks (the disclosure ratio is diagnostic — the
// deterministic Go verdict contract has no diag flag, so only the gating rows
// — bounded + navigated — are emitted).
func skillBenchFunnelChecks(sc skillBenchScenario, rec *probe.Record, bundleTokens int) []Check {
	out := []Check{}
	peak := skillBenchFunnelPeakChars(rec)
	if peak > 0 {
		out = append(out, ck(fmt.Sprintf("funnel.%s.bounded", sc.ID), peak < skillBenchFunnelPeakCharsMax,
			fmt.Sprintf("obs tail peak %d chars", peak)))
	}
	if bundleTokens > skills.FullFileTokens {
		dr := skillBenchDisclosureRatio(rec, bundleTokens)
		out = append(out, ck(fmt.Sprintf("funnel.%s.navigated", sc.ID), skillBenchNavigated(rec),
			fmt.Sprintf("used skill.read/skill.search (layered, not a dump); disclosure ratio %.3g (≤%.2g; diagnostic)",
				dr, skillBenchDisclosureRatioMax)))
	}
	return out
}

// skillBenchBundleTokens estimates the skill's bundle token size (instructions +
// reference files). Mirrors run.py:_bundle_tokens.
func skillBenchBundleTokens(sk *skills.SkillPack) int {
	n := skills.EstTokens(sk.Instructions)
	for _, c := range sk.Files {
		n += skills.EstTokens(c)
	}
	return n
}

// ── helpers (namespaced by the skillBench prefix) ────────────────────────────

func skillBenchContains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func skillBenchHas(set map[string]struct{}, x string) bool {
	_, ok := set[x]
	return ok
}

func skillBenchUnion(sets []map[string]struct{}) map[string]struct{} {
	out := map[string]struct{}{}
	for _, s := range sets {
		for k := range s {
			out[k] = struct{}{}
		}
	}
	return out
}

func skillBenchHead(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func skillBenchStrList(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func skillBenchToInt(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	}
	return 0
}

func round3f(x float64) float64 { return math.Round(x*1000) / 1000 }
