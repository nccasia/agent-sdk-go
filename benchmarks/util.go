package benchmarks

import (
	"fmt"
	"sort"
	"strings"
)

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func joinComma(xs []string) string { return strings.Join(xs, ", ") }

func modeFailReason(mode string, n int, failed []string) string {
	return fmt.Sprintf("%s: %d failing — [%s]", mode, n, strings.Join(quoteAll(failed), " "))
}

func quoteAll(xs []string) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = "'" + x + "'"
	}
	return out
}
