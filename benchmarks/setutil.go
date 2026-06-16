package benchmarks

import (
	"math"
	"sort"
)

func subset(want []string, have map[string]struct{}) bool {
	for _, w := range want {
		if _, ok := have[w]; !ok {
			return false
		}
	}
	return true
}

func disjoint(xs []string, have map[string]struct{}) bool {
	for _, x := range xs {
		if _, ok := have[x]; ok {
			return false
		}
	}
	return true
}

func setEqual(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

func sortedSet(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func round3(f float64) float64 { return math.Round(f*1000) / 1000 }
