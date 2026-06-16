// Command bench runs the registered benchmarks' deterministic floors and prints
// each normalized verdict, then exits non-zero unless the free-gate passes — i.e.
// unless every bench reproduces its Python source-of-truth verdict. Mirrors
// benchmarks/ci-free-gates.sh (the no-provider gate).
//
//	go run ./cmd/bench
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/mezon/agent-sdk-go/benchmarks"
)

func main() {
	r := benchmarks.DefaultRegistry()
	rows, ok, err := r.FreeGate(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, "bench: error:", err)
		os.Exit(2)
	}
	fmt.Println("── free-gate (deterministic floor) ────────────────────────────")
	for _, row := range rows {
		mark := "PASS"
		if !row.OK {
			mark = "FAIL"
		}
		fmt.Printf("  [%s] %-16s verdict %-10s (parity %s)\n", mark, row.Name, row.Status, row.Expect)
	}
	if !ok {
		fmt.Fprintln(os.Stderr, "\nbench: free-gate FAILED (a bench diverged from its parity verdict)")
		os.Exit(1)
	}
	fmt.Println("\nbench: free-gate green")
}
