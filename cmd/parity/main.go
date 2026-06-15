// Command parity is the 100%-API-parity gateway gate. It parses PARITY.md (the
// agent_sdk.__all__ contract ledger) and reports how many of the 82 public
// exports are checked off. Exits non-zero while any box is unchecked, so the
// porting ladder's gateway rung only passes once every export exists in Go.
//
//	go run ./cmd/parity            # print N/82 + the unchecked exports, exit 0 iff complete
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

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

	var done, total int
	var pending []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "- [x] "), strings.HasPrefix(line, "- [X] "):
			done++
			total++
		case strings.HasPrefix(line, "- [ ] "):
			total++
			pending = append(pending, strings.TrimPrefix(line, "- [ ] "))
		}
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "parity:", err)
		os.Exit(2)
	}

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
