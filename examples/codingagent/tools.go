package codingagent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mezon/agent-sdk-go/agent_sdk/tools"
)

const readMaxLines = 2000
const grepMaxHits = 200
const globMax = 300
const bashMaxOutput = 30000

// PytestCmd runs the workspace's tests with a pure-stdlib python3 runner (no
// pytest dependency): it imports every test_*.py module under the cwd and runs
// each top-level test_* function, exiting non-zero on the first failure. The
// Python reference used `python -m pytest -q`; the Go port keeps the "agent
// runs the REAL suite on disk and it passes" property without a pytest install.
const PytestCmd = `python3 -c '
import glob, importlib.util, os, sys, traceback
sys.path.insert(0, os.getcwd())
failed = passed = 0
for path in sorted(glob.glob("**/test_*.py", recursive=True)):
    name = os.path.splitext(os.path.basename(path))[0]
    spec = importlib.util.spec_from_file_location(name, path)
    mod = importlib.util.module_from_spec(spec)
    try:
        spec.loader.exec_module(mod)
    except Exception:
        traceback.print_exc(); failed += 1; continue
    for fn in sorted(n for n in dir(mod) if n.startswith("test_")):
        obj = getattr(mod, fn)
        if callable(obj):
            try:
                obj(); passed += 1
            except Exception:
                traceback.print_exc(); failed += 1
print(f"{passed} passed, {failed} failed")
sys.exit(1 if failed else 0)
'`

// CodingTools builds the workspace-bound Claude-Code-grade tools rooted at root.
// Mirrors coding_agent.tools.coding_tools.
func CodingTools(root string) []*tools.ToolDef {
	ws := NewWorkspace(root)
	out := []*tools.ToolDef{}
	out = append(out, fsTools(ws)...)
	out = append(out, searchTools(ws)...)
	out = append(out, shellTools(ws)...)
	return out
}

// CodingToolsAny is CodingTools as []any (for setup.AddTool / cfg.Tools).
func CodingToolsAny(root string) []any {
	defs := CodingTools(root)
	out := make([]any, len(defs))
	for i, d := range defs {
		out[i] = d
	}
	return out
}

func strArg(in map[string]any, key, def string) string {
	if v, ok := in[key].(string); ok {
		return v
	}
	return def
}

func intArg(in map[string]any, key string, def int) int {
	switch v := in[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return def
}

func boolArg(in map[string]any, key string, def bool) bool {
	if v, ok := in[key].(bool); ok {
		return v
	}
	return def
}

func fsTools(ws *Workspace) []*tools.ToolDef {
	read := tools.Tool("Read", func(_ context.Context, in map[string]any) (any, error) {
		filePath := strArg(in, "file_path", "")
		offset := intArg(in, "offset", 1)
		limit := intArg(in, "limit", readMaxLines)
		full, err := ws.safe(filePath)
		if err != nil {
			return err.Error(), nil
		}
		fi, statErr := os.Stat(full)
		if statErr != nil || fi.IsDir() {
			return "Error: not a file: " + filePath + ws.nearby(filePath), nil
		}
		data, rerr := os.ReadFile(full)
		if rerr != nil {
			return fmt.Sprintf("Error reading %s: %v", filePath, rerr), nil
		}
		lines := splitKeepCount(string(data))
		start := offset
		if start < 1 {
			start = 1
		}
		l := limit
		if l < 1 {
			l = 1
		}
		end := start - 1 + l
		if end > len(lines) {
			end = len(lines)
		}
		if start > len(lines) {
			return fmt.Sprintf("(file has %d lines; offset %d is past the end)", len(lines), start), nil
		}
		var b strings.Builder
		for i := start; i <= end; i++ {
			fmt.Fprintf(&b, "%6d\t%s\n", i, strings.TrimRight(lines[i-1], "\n"))
		}
		more := ""
		if end < len(lines) {
			more = fmt.Sprintf("\n… (%d more lines; read with offset=%d)", len(lines)-end, end+1)
		}
		return b.String() + more, nil
	},
		tools.Desc("Reads a file from the local filesystem. Returns cat -n style line numbers. For large files pass offset (1-based start line) and limit (max lines, default 2000) to page through it."),
		tools.Param("file_path", "string", true, nil),
		tools.Param("offset", "integer", false, 1),
		tools.Param("limit", "integer", false, readMaxLines),
	)

	write := tools.Tool("Write", func(_ context.Context, in map[string]any) (any, error) {
		filePath := strArg(in, "file_path", "")
		content := strArg(in, "content", "")
		full, err := ws.safe(filePath)
		if err != nil {
			return err.Error(), nil
		}
		dir := filepath.Dir(full)
		if dir == "" {
			dir = ws.Root
		}
		if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
			return fmt.Sprintf("Error writing %s: %v", filePath, mkErr), nil
		}
		if werr := os.WriteFile(full, []byte(content), 0o644); werr != nil {
			return fmt.Sprintf("Error writing %s: %v", filePath, werr), nil
		}
		return fmt.Sprintf("Wrote %s (%d lines).", filePath, strings.Count(content, "\n")+1), nil
	},
		tools.Desc("Writes a file, overwriting if it exists. Prefer Edit for changes to an existing file."),
		tools.Param("file_path", "string", true, nil),
		tools.Param("content", "string", true, nil),
	)

	edit := tools.Tool("Edit", func(_ context.Context, in map[string]any) (any, error) {
		filePath := strArg(in, "file_path", "")
		oldStr := strArg(in, "old_string", "")
		newStr := strArg(in, "new_string", "")
		replaceAll := boolArg(in, "replace_all", false)
		full, err := ws.safe(filePath)
		if err != nil {
			return err.Error(), nil
		}
		fi, statErr := os.Stat(full)
		if statErr != nil || fi.IsDir() {
			return "Error: not a file: " + filePath + ws.nearby(filePath), nil
		}
		data, rerr := os.ReadFile(full)
		if rerr != nil {
			return fmt.Sprintf("Error reading %s: %v", filePath, rerr), nil
		}
		text := string(data)
		count := strings.Count(text, oldStr)
		if count == 0 {
			return fmt.Sprintf("Error: old_string not found in %s.", filePath), nil
		}
		if count > 1 && !replaceAll {
			return fmt.Sprintf("Error: old_string is not unique in %s (%d matches) — add context or set replace_all.", filePath, count), nil
		}
		newText := strings.ReplaceAll(text, oldStr, newStr)
		if werr := os.WriteFile(full, []byte(newText), 0o644); werr != nil {
			return fmt.Sprintf("Error writing %s: %v", filePath, werr), nil
		}
		plural := "s"
		if count == 1 {
			plural = ""
		}
		return fmt.Sprintf("Edited %s (%d replacement%s).", filePath, count, plural), nil
	},
		tools.Desc("Performs an exact string replacement in a file. old_string must be unique unless replace_all is set — Read the file first so the match is exact (including indentation)."),
		tools.Param("file_path", "string", true, nil),
		tools.Param("old_string", "string", true, nil),
		tools.Param("new_string", "string", true, nil),
		tools.Param("replace_all", "boolean", false, false),
	)

	ls := tools.Tool("LS", func(_ context.Context, in map[string]any) (any, error) {
		path := strArg(in, "path", ".")
		target, err := ws.safe(path)
		if err != nil {
			return err.Error(), nil
		}
		fi, statErr := os.Stat(target)
		if statErr != nil || !fi.IsDir() {
			return "Error: not a directory: " + path + ws.nearby(path), nil
		}
		ents, rerr := os.ReadDir(target)
		if rerr != nil {
			return "Error: not a directory: " + path + ws.nearby(path), nil
		}
		names := []string{}
		for _, e := range ents {
			names = append(names, e.Name())
		}
		sort.Strings(names)
		out := []string{}
		for _, name := range names {
			if isSkipDir(name) {
				continue
			}
			if fi, _ := os.Stat(filepath.Join(target, name)); fi != nil && fi.IsDir() {
				out = append(out, name+"/")
			} else {
				out = append(out, name)
			}
		}
		if len(out) == 0 {
			return "(empty)", nil
		}
		return strings.Join(out, "\n"), nil
	},
		tools.Desc("Lists files and directories (dirs marked with /) under a path."),
		tools.Param("path", "string", false, "."),
	)

	return []*tools.ToolDef{read, write, edit, ls}
}

// splitKeepCount splits text into lines, each retaining its trailing newline,
// matching Python's readlines() line count semantics.
func splitKeepCount(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i+1])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func searchTools(ws *Workspace) []*tools.ToolDef {
	glob := tools.Tool("Glob", func(_ context.Context, in map[string]any) (any, error) {
		pattern := strArg(in, "pattern", "")
		path := strArg(in, "path", ".")
		base, err := ws.safe(path)
		if err != nil {
			return err.Error(), nil
		}
		recursive := strings.Contains(pattern, "**")
		var rootPattern string
		hasRoot := strings.HasPrefix(pattern, "**/")
		if hasRoot {
			rootPattern = pattern[3:]
		}
		type hit struct {
			mtime int64
			rel   string
		}
		var hits []hit
		walkDir(base, ws, !recursive, func(full string) {
			rel := ws.rel(full)
			matchee := filepath.Base(full)
			if recursive {
				matchee = rel
			}
			if fnmatch(matchee, pattern) || fnmatch(rel, pattern) ||
				(hasRoot && fnmatch(rel, rootPattern)) {
				if fi, e := os.Stat(full); e == nil {
					hits = append(hits, hit{fi.ModTime().UnixNano(), rel})
				}
			}
		})
		sort.SliceStable(hits, func(i, j int) bool {
			if hits[i].mtime != hits[j].mtime {
				return hits[i].mtime > hits[j].mtime
			}
			return hits[i].rel > hits[j].rel
		})
		out := []string{}
		for i, h := range hits {
			if i >= globMax {
				break
			}
			out = append(out, h.rel)
		}
		tail := ""
		if len(hits) > globMax {
			tail = fmt.Sprintf("\n… (%d more)", len(hits)-globMax)
		}
		if len(out) == 0 {
			return "(no files match)", nil
		}
		return strings.Join(out, "\n") + tail, nil
	},
		tools.Desc("Fast file matching by glob pattern (e.g. **/*.py, apps/**/test_*.py). Returns matching paths sorted by modification time (newest first)."),
		tools.Param("pattern", "string", true, nil),
		tools.Param("path", "string", false, "."),
	)

	grep := tools.Tool("Grep", func(_ context.Context, in map[string]any) (any, error) {
		pattern := strArg(in, "pattern", "")
		path := strArg(in, "path", ".")
		globFilter := strArg(in, "glob", "")
		rx, cerr := regexp.Compile(pattern)
		if cerr != nil {
			return "Bad regex: " + cerr.Error(), nil
		}
		base, err := ws.safe(path)
		if err != nil {
			return err.Error(), nil
		}
		var hits []string
		capped := false
		walkDir(base, ws, false, func(full string) {
			if capped {
				return
			}
			fname := filepath.Base(full)
			if globFilter != "" && !fnmatch(fname, globFilter) {
				return
			}
			data, e := os.ReadFile(full)
			if e != nil {
				return
			}
			for i, line := range strings.Split(string(data), "\n") {
				if rx.MatchString(line) {
					hits = append(hits, fmt.Sprintf("%s:%d: %s", ws.rel(full), i+1, strings.TrimRight(line, "\r")))
					if len(hits) >= grepMaxHits {
						capped = true
						return
					}
				}
			}
		})
		if capped {
			return strings.Join(hits, "\n") + fmt.Sprintf("\n… (≥%d matches; narrow the pattern/glob)", grepMaxHits), nil
		}
		if len(hits) == 0 {
			return "(no matches)", nil
		}
		return strings.Join(hits, "\n"), nil
	},
		tools.Desc("Searches file contents with a regex. Optional glob filters files (e.g. *.py). Returns file:line: match."),
		tools.Param("pattern", "string", true, nil),
		tools.Param("path", "string", false, "."),
		tools.Param("glob", "string", false, ""),
	)

	return []*tools.ToolDef{glob, grep}
}

// walkDir walks base, skipping skipDirs. If topOnly, only the immediate
// directory's files are visited (no recursion). fn is called per file with its
// full path. Files in each directory are visited in directory-listing order.
func walkDir(base string, ws *Workspace, topOnly bool, fn func(full string)) {
	var rec func(dir string)
	rec = func(dir string) {
		ents, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		var subdirs []string
		for _, e := range ents {
			full := filepath.Join(dir, e.Name())
			if e.IsDir() {
				if !isSkipDir(e.Name()) {
					subdirs = append(subdirs, full)
				}
				continue
			}
			fn(full)
		}
		if topOnly {
			return
		}
		for _, sd := range subdirs {
			rec(sd)
		}
	}
	rec(base)
}

func shellTools(ws *Workspace) []*tools.ToolDef {
	bash := tools.Tool("Bash", func(ctx context.Context, in map[string]any) (any, error) {
		command := strArg(in, "command", "")
		cmd := exec.CommandContext(ctx, "bash", "-c", command)
		cmd.Dir = ws.Root
		cmd.Env = append(os.Environ(), "PYTHONPATH="+ws.Root)
		out, _ := cmd.CombinedOutput()
		exit := 0
		if cmd.ProcessState != nil {
			exit = cmd.ProcessState.ExitCode()
		}
		text := string(out)
		if len(text) > bashMaxOutput {
			head := text[:bashMaxOutput/2]
			tail := text[len(text)-bashMaxOutput/2:]
			text = head + fmt.Sprintf("\n… (%d chars elided) …\n", len(text)-bashMaxOutput) + tail
		}
		return fmt.Sprintf("$ %s\n(exit %d)\n%s", command, exit, text), nil
	},
		tools.Desc("Executes a shell command in the workspace (build, run tests, git, …). Use Read/Glob/Grep — not cat/find/grep — to inspect files."),
		tools.Param("command", "string", true, nil),
	)
	return []*tools.ToolDef{bash}
}
