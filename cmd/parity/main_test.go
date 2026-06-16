package main

import (
	"os"
	"strings"
	"testing"
)

// TestParityLedgerComplete is the gateway gate: the real PARITY.md must be a
// fully-checked 116-export __all__ ledger, with the trailing bench-progress
// section excluded from the tally (it holds no __all__ entries).
func TestParityLedgerComplete(t *testing.T) {
	f, err := os.Open("../../PARITY.md")
	if err != nil {
		t.Fatalf("open PARITY.md: %v", err)
	}
	defer f.Close()

	done, total, pending := count(f)
	if total != 116 {
		t.Fatalf("export-ledger total = %d, want 116 (bench section must be excluded)", total)
	}
	if done != total {
		t.Fatalf("parity: %d/%d, pending: %v", done, total, pending)
	}
	if len(pending) != 0 {
		t.Fatalf("unexpected pending exports: %v", pending)
	}
}

// TestBenchSectionExcluded proves the parser stops at the bench heading: an
// unchecked box living below that heading must not count toward total.
func TestBenchSectionExcluded(t *testing.T) {
	const md = "## Façade\n" +
		"- [x] A → a.A\n" +
		"- [ ] B → b.B\n" +
		"## Benchmarks / verdict / ratchet (rung 14)\n" +
		"- [ ] flowbench → benchmarks\n" +
		"- [x] something → benchmarks\n"

	done, total, pending := count(strings.NewReader(md))
	if total != 2 {
		t.Fatalf("total = %d, want 2 (bench rows excluded)", total)
	}
	if done != 1 {
		t.Fatalf("done = %d, want 1", done)
	}
	if len(pending) != 1 || pending[0] != "B → b.B" {
		t.Fatalf("pending = %v, want [B → b.B]", pending)
	}
}
