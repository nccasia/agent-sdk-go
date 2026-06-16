// Package codingagent is a Claude-Code-grade coding agent built on agent_sdk —
// it operates on a real filesystem workspace. The whole capability is packaged
// as a first-class CodingPlugin; BuildCodingAgent mounts it on a bare base
// network. Ported from examples/coding-agent/.
package codingagent

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// skipDirs are directories the tools never walk or list (VCS, caches, build
// output). Mirrors examples/coding-agent/coding_agent/tools/workspace.py:SKIP_DIRS.
var skipDirs = map[string]struct{}{
	".git": {}, "__pycache__": {}, "node_modules": {}, ".venv": {}, "venv": {},
	".mypy_cache": {}, ".ruff_cache": {}, ".pytest_cache": {}, "dist": {},
	"build": {}, ".next": {}, "target": {},
}

func isSkipDir(name string) bool {
	_, ok := skipDirs[name]
	return ok
}

// Workspace is a real directory the tools are sandboxed to (all paths resolve
// under it). Mirrors tools/workspace.py:Workspace.
type Workspace struct {
	Root string
}

// NewWorkspace builds a Workspace rooted at the absolute form of root.
func NewWorkspace(root string) *Workspace {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	return &Workspace{Root: abs}
}

// safe resolves path under the root, rejecting any escape (.. / absolute).
func (w *Workspace) safe(path string) (string, error) {
	full := filepath.Clean(filepath.Join(w.Root, strings.TrimLeft(path, "/")))
	if full != w.Root && !strings.HasPrefix(full, w.Root+string(os.PathSeparator)) {
		return "", &pathEscapeError{path: path}
	}
	return full, nil
}

type pathEscapeError struct{ path string }

func (e *pathEscapeError) Error() string { return "path escapes workspace: " + e.path }

// rel returns full relative to the root, using forward slashes.
func (w *Workspace) rel(full string) string {
	r, err := filepath.Rel(w.Root, full)
	if err != nil {
		return full
	}
	return filepath.ToSlash(r)
}

// nearby is a self-correction hint for a path that doesn't exist: name the
// closest real siblings in its deepest existing ancestor dir. Mirrors
// Workspace.nearby.
func (w *Workspace) nearby(path string) string {
	parts := []string{}
	for _, p := range strings.Split(strings.Trim(path, "/"), "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	cur, missing := w.Root, path
	for _, part := range parts {
		nxt := filepath.Join(cur, part)
		if fi, err := os.Stat(nxt); err == nil && fi.IsDir() {
			cur = nxt
			continue
		}
		missing = part
		break
	}
	ents, err := os.ReadDir(cur)
	if err != nil {
		return ""
	}
	names := []string{}
	for _, e := range ents {
		if !isSkipDir(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	where := "."
	if cur != w.Root {
		where = w.rel(cur)
	}
	close := closeMatches(missing, names, 3, 0.4)
	if len(close) > 0 {
		return " — in '" + where + "', did you mean: " + strings.Join(close, ", ") + "?"
	}
	shown := names
	suffix := ""
	if len(shown) > 8 {
		shown = shown[:8]
		suffix = " …"
	}
	joined := strings.Join(shown, ", ") + suffix
	if joined != "" && joined != " …" {
		return " — '" + where + "' contains: " + joined
	}
	return ""
}
