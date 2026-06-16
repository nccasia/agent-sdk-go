package codingagent

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const maxSymbolsPerFile = 8

// topLevelDefRE matches a top-level (column 0) class/def/async def declaration
// in a Python file — the Go stand-in for the reference's ast walk.
var topLevelDefRE = regexp.MustCompile(`(?m)^(?:async\s+def|def|class)\s+([A-Za-z_][A-Za-z0-9_]*)`)

// topLevelSymbols returns the top-level class/def names in a Python file
// (best-effort, parse-tolerant). Mirrors repomap._top_level_symbols.
func topLevelSymbols(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var names []string
	for _, m := range topLevelDefRE.FindAllStringSubmatch(string(data), -1) {
		names = append(names, m[1])
		if len(names) >= maxSymbolsPerFile {
			break
		}
	}
	return names
}

// BuildRepoMap renders a compact, deterministic structural map of root (the
// real file tree + top-level symbols). Mirrors repomap.build_repo_map.
func BuildRepoMap(root string) string {
	return buildRepoMap(root, 600, 6000)
}

func buildRepoMap(root string, maxFiles, maxChars int) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	type row struct{ rel, syms string }
	var rows []row
	nfiles := 0
	truncated := false

	// Walk like os.walk: per-directory, sorted dirs + sorted files.
	var walk func(dir string) bool // returns false to stop entirely
	walk = func(dir string) bool {
		ents, rerr := os.ReadDir(dir)
		if rerr != nil {
			return true
		}
		var dirs, files []string
		for _, e := range ents {
			if e.IsDir() {
				if !isSkipDir(e.Name()) {
					dirs = append(dirs, e.Name())
				}
			} else {
				files = append(files, e.Name())
			}
		}
		sort.Strings(dirs)
		sort.Strings(files)
		for _, fname := range files {
			if nfiles >= maxFiles {
				truncated = true
				return false
			}
			full := filepath.Join(dir, fname)
			rel, _ := filepath.Rel(abs, full)
			rel = filepath.ToSlash(rel)
			if strings.HasSuffix(fname, ".py") {
				rows = append(rows, row{rel, strings.Join(topLevelSymbols(full), ", ")})
			} else {
				rows = append(rows, row{rel, ""})
			}
			nfiles++
		}
		if nfiles >= maxFiles {
			truncated = true
			return false
		}
		for _, d := range dirs {
			if !walk(filepath.Join(dir, d)) {
				return false
			}
		}
		return true
	}
	walk(abs)

	lines := []string{
		"Repository map (deterministic — the REAL file tree + top-level symbols). " +
			"Use these exact paths; do not guess at conventional layouts that may not exist.",
		"",
	}
	lastDir := "\x00"
	for _, r := range rows {
		d := filepath.ToSlash(filepath.Dir(r.rel))
		if d == "." {
			d = ""
		}
		if d != lastDir {
			if d == "" {
				lines = append(lines, ".")
			} else {
				lines = append(lines, d+"/")
			}
			lastDir = d
		}
		base := filepath.Base(r.rel)
		if r.syms != "" {
			lines = append(lines, "  "+base+" — "+r.syms)
		} else {
			lines = append(lines, "  "+base)
		}
	}
	if truncated {
		lines = append(lines, "… (map truncated at 600 files)")
	}
	out := strings.Join(lines, "\n")
	if len(out) > maxChars {
		cut := out[:maxChars]
		if idx := strings.LastIndex(cut, "\n"); idx >= 0 {
			cut = cut[:idx]
		}
		out = cut + "\n… (map truncated to fit budget)"
	}
	return out
}
