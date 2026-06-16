// Command parity is the 100%-API-parity gateway gate. It parses PARITY.md (the
// agent_sdk.__all__ contract ledger) and reports how many of the 116 public
// exports are checked off. Exits non-zero while any box is unchecked, so the
// porting ladder's gateway rung only passes once every export exists in Go.
//
// Only the `agent_sdk.__all__` export ledger counts toward the gate. PARITY.md
// also carries a trailing "Benchmarks / verdict / ratchet" section that tracks
// bench-port progress; those rows are explicitly NOT __all__ entries (they have
// no Python public-export counterpart), so the parser stops at that heading and
// excludes them from the N/total tally.
//
//	go run ./cmd/parity            # print N/116 + the unchecked exports, exit 0 iff complete
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// count tallies the agent_sdk.__all__ export ledger from a PARITY.md stream.
// It stops at the trailing "Benchmarks / verdict / ratchet" heading: those rows
// track bench-port progress and are not __all__ entries, so they are excluded
// from the gate's N/total.
func count(r io.Reader) (done, total int, pending []string) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "## ") && strings.Contains(line, "Benchmarks / verdict / ratchet") {
			break
		}
		switch {
		case strings.HasPrefix(line, "- [x] "), strings.HasPrefix(line, "- [X] "):
			done++
			total++
		case strings.HasPrefix(line, "- [ ] "):
			total++
			pending = append(pending, strings.TrimPrefix(line, "- [ ] "))
		}
	}
	return done, total, pending
}

func main() {
	path := "PARITY.md"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "parity:", err)
		os.Exit(2)
	}
	defer f.Close()

	done, total, pending := count(f)

	fmt.Printf("parity: %d/%d exports implemented\n", done, total)
	if len(pending) > 0 {
		fmt.Printf("pending (%d):\n", len(pending))
		for _, p := range pending {
			fmt.Println("  -", p)
		}
		os.Exit(1)
	}
	fmt.Println("PARITY: 100% — all public exports present")
}
