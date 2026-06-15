// Command bench runs the free/deterministic benchmarks and prints normalized
// verdicts as JSON, exiting non-zero if any bench is not READY. Benches are
// registered here as the porting ladder reaches rung 14; until then this is a
// no-op gate that succeeds (nothing to measure yet).
//
//	go run ./cmd/bench
package main

import "fmt"

func main() {
	// Rung 14 registers the ported benches (attention/flow/corgiction/stateless/
	// prompt/tool + live) and wires the verdict/ratchet here.
	fmt.Println("bench: no free benches registered yet (porting ladder pre-rung-14)")
}
