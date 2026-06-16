package codingagent

import "sort"

// closeMatches is a faithful subset of Python's difflib.get_close_matches:
// returns up to n possibilities from candidates whose SequenceMatcher ratio
// against word is >= cutoff, ordered by descending ratio.
func closeMatches(word string, candidates []string, n int, cutoff float64) []string {
	type scored struct {
		s     string
		ratio float64
	}
	var hits []scored
	for _, c := range candidates {
		r := seqRatio(word, c)
		if r >= cutoff {
			hits = append(hits, scored{c, r})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].ratio > hits[j].ratio })
	if len(hits) > n {
		hits = hits[:n]
	}
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.s)
	}
	return out
}

// seqRatio computes Python difflib.SequenceMatcher.ratio() = 2*M/T, where M is
// the total number of matched characters (via the longest-matching-block
// recursion) and T is the sum of both lengths.
func seqRatio(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	total := len(ra) + len(rb)
	if total == 0 {
		return 1.0
	}
	matches := matchCount(ra, rb)
	return 2.0 * float64(matches) / float64(total)
}

// matchCount sums the matched characters using the recursive
// find-longest-match decomposition (matches difflib's get_matching_blocks).
func matchCount(a, b []rune) int {
	alo, ahi, blo, bhi := 0, len(a), 0, len(b)
	i, j, k := longestMatch(a, b, alo, ahi, blo, bhi)
	if k == 0 {
		return 0
	}
	total := k
	if alo < i && blo < j {
		total += matchCount(a[:i], b[:j])
	}
	if i+k < ahi && j+k < bhi {
		total += matchCount(a[i+k:ahi], b[j+k:bhi])
	}
	return total
}

// longestMatch finds the longest matching block in a[alo:ahi] / b[blo:bhi].
func longestMatch(a, b []rune, alo, ahi, blo, bhi int) (int, int, int) {
	b2j := map[rune][]int{}
	for j := blo; j < bhi; j++ {
		b2j[b[j]] = append(b2j[b[j]], j)
	}
	besti, bestj, bestsize := alo, blo, 0
	j2len := map[int]int{}
	for i := alo; i < ahi; i++ {
		newj2len := map[int]int{}
		for _, j := range b2j[a[i]] {
			if j < blo {
				continue
			}
			if j >= bhi {
				break
			}
			k := j2len[j-1] + 1
			newj2len[j] = k
			if k > bestsize {
				besti, bestj, bestsize = i-k+1, j-k+1, k
			}
		}
		j2len = newj2len
	}
	return besti, bestj, bestsize
}
