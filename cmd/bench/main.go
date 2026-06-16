// Command bench runs the registered benchmarks' deterministic floors and prints
// each normalized verdict, then exits non-zero unless the free-gate passes — i.e.
// unless every bench reproduces its Python source-of-truth verdict. Mirrors
// benchmarks/ci-free-gates.sh (the no-provider gate).
//
//	go run ./cmd/bench
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/mezon/agent-sdk-go/benchmarks"
)

func main() {
	htmlDir := flag.String("html", "benchmarks/results", "directory to write the inspectable viewer reports into")
	noHTML := flag.Bool("no-html", false, "skip writing the viewer reports")
	flag.Parse()

	ctx := context.Background()
	r := benchmarks.DefaultRegistry()
	rows, ok, err := r.FreeGate(ctx)
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

	// By default, also write an inspectable viewer HTML per bench (+ index) into
	// benchmarks/results (gitignored). The deterministic floor (model="") drives
	// both the verdicts and any captured probe traces.
	if !*noHTML {
		paths, err := r.WriteReports(ctx, "", *htmlDir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bench: report:", err)
			os.Exit(2)
		}
		if n := len(paths); n > 0 {
			fmt.Printf("bench: wrote %d reports · index %s\n", n-1, paths[n-1])
		}
	}
}
