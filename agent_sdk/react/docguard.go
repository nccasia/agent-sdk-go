package react

import (
	"fmt"
	"regexp"
)

// DocWriteGuard is a tool-call filter that bounds redundant heavy writes +
// read-only writes — heavy-output write discipline. Ported from
// agent_sdk/react/docguard.py.
//
// It is a (stage_id, name, input) -> string filter for the engine's tool-filter
// seam (an empty return ⇒ allow). RecordOnly keeps it measure-only (logs Events
// without intercepting). Events is always populated for telemetry.
type DocWriteGuard struct {
	WriteTools     map[string]struct{}
	PathKeys       []string
	BashTool       string
	ReadonlyStages map[string]struct{}
	RecordOnly     bool

	Events []DocGuardEvent

	writes     map[[2]string]int // (stage, path) -> count
	pathWrites map[string]int    // path -> count across all stages
}

// DocGuardEvent is one telemetry record (stage, path, action).
type DocGuardEvent struct {
	Stage  string `json:"stage"`
	Path   string `json:"path,omitempty"`
	Action string `json:"action"`
}

// DocGuardOpts configures a DocWriteGuard (zero value uses Python defaults).
type DocGuardOpts struct {
	WriteTools     []string
	PathKeys       []string
	BashTool       string
	ReadonlyStages []string
	RecordOnly     bool
}

// `cat > f`, `cat >> f`, `tee f`, `> f` redirections in a shell command.
var bashWriteRE = regexp.MustCompile(`(^|[\s|&;])(cat\s+>>?|tee(\s+-a)?\s|>>?)\s*([^\s|&;<>]+)`)

// NewDocWriteGuard builds a guard. Defaults: write tool "write_file", path keys
// {path,file,filename,file_path}, bash tool "bash".
func NewDocWriteGuard(opts DocGuardOpts) *DocWriteGuard {
	writeTools := opts.WriteTools
	if writeTools == nil {
		writeTools = []string{"write_file"}
	}
	pathKeys := opts.PathKeys
	if pathKeys == nil {
		pathKeys = []string{"path", "file", "filename", "file_path"}
	}
	bashTool := opts.BashTool
	if bashTool == "" {
		bashTool = "bash"
	}
	g := &DocWriteGuard{
		WriteTools:     toSet(writeTools),
		PathKeys:       pathKeys,
		BashTool:       bashTool,
		ReadonlyStages: toSet(opts.ReadonlyStages),
		RecordOnly:     opts.RecordOnly,
		writes:         map[[2]string]int{},
		pathWrites:     map[string]int{},
	}
	return g
}

func toSet(xs []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, x := range xs {
		out[x] = struct{}{}
	}
	return out
}

func (g *DocWriteGuard) targetPath(name string, inp map[string]any) string {
	if _, ok := g.WriteTools[name]; ok {
		for _, k := range g.PathKeys {
			if v, ok := inp[k].(string); ok && v != "" {
				return v
			}
		}
	}
	return ""
}

// Check is the filter entry point: returns a steering/refusal message to surface
// to the model, or "" to allow the call. Mirrors DocWriteGuard.__call__.
func (g *DocWriteGuard) Check(stageID, name string, inp map[string]any) string {
	// 1) A bash write inside a read-only stage — refuse (heredoc bypass guard).
	if name == g.BashTool {
		if _, ro := g.ReadonlyStages[stageID]; ro {
			cmd, _ := inp["command"].(string)
			if m := bashWriteRE.FindStringSubmatch(cmd); m != nil {
				target := m[4]
				g.Events = append(g.Events, DocGuardEvent{Stage: stageID, Path: target, Action: "blocked_readonly_write"})
				if !g.RecordOnly {
					return readonlyMsg(stageID, target)
				}
			}
		}
		return ""
	}

	path := g.targetPath(name, inp)
	if path == "" {
		return ""
	}

	// 2) A write tool inside a read-only stage — refuse outright.
	if _, ro := g.ReadonlyStages[stageID]; ro {
		g.Events = append(g.Events, DocGuardEvent{Stage: stageID, Path: path, Action: "blocked_readonly_write"})
		if !g.RecordOnly {
			return readonlyMsg(stageID, path)
		}
		return ""
	}

	// 3) A repeated full write of the same target within a stage — steer to edit.
	key := [2]string{stageID, path}
	g.writes[key]++
	priorTotal := g.pathWrites[path]
	g.pathWrites[path] = priorTotal + 1
	if g.writes[key] > 1 {
		g.Events = append(g.Events, DocGuardEvent{Stage: stageID, Path: path, Action: "redundant_rewrite"})
		if !g.RecordOnly {
			return fmt.Sprintf("Note: '%s' was already written in this step. Don't rewrite the whole "+
				"file — use an edit/append tool to change only what needs to change, or "+
				"write the complete document once.", path)
		}
	} else if priorTotal > 0 {
		g.Events = append(g.Events, DocGuardEvent{Stage: stageID, Path: path, Action: "redundant_rewrite_cross_stage"})
		if !g.RecordOnly {
			return fmt.Sprintf("Note: '%s' was already written in an earlier step. Read it and make a "+
				"targeted edit instead of rewriting the whole file from scratch.", path)
		}
	}
	return ""
}

func readonlyMsg(stageID, path string) string {
	return fmt.Sprintf("Refused: '%s' is a read-only step — do not write files here "+
		"(attempted to write '%s'). Explore/read only; defer writing to a "+
		"later step that has a write tool.", stageID, path)
}
