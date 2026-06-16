package benchmarks

import (
	"context"
	"strings"
	"testing"
)

// TestExtensionBench_Ready is the cross-language parity gate for
// extensionbench's deterministic floor. extensionbench is a LIVE bench: every
// behavior drives the REAL PreactAgent (plugged + bare) against a real provider
// — there is NO deterministic floor. Python's run.py refuses to run without a
// provider token ("extensionbench is a LIVE bench — set a provider token …",
// exit 2). The Go bench reproduces that: with no model the single "extension"
// mode is missing evidence, so the verdict is UNMEASURED (no evidence is never
// READY).
func TestExtensionBench_Ready(t *testing.T) {
	v, err := RunExtensionBench(context.Background(), "")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if v.Status != "UNMEASURED" {
		t.Fatalf("status = %q, want UNMEASURED (Python parity — live bench, no provider) — reasons: %v",
			v.Status, v.Reasons)
	}
	// The missing mode gates as nil (it did not run), never as a pass — so it
	// can never trip the free-gate.
	if g, ok := v.Gates["extension_all_pass"]; ok && g != nil {
		t.Fatalf("extension_all_pass gate = %v, want absent/nil without a provider", *g)
	}
}

// TestExtensionBench_CheckIDParity asserts the Go bench emits the SAME mode +
// check-id surface as the Python run.py: ONE mode "extension" carrying the 5
// plugin.* / unplugged.* checks (the full-surface PluginSupportTriage scenario)
// and the 5 mcp.* checks (the OrdersPlugin owns-an-MCP scenario) — 10 total.
// Mirrors the check ids built in benchmarks/extensionbench/run.py. The ids are a
// static, provider-independent property of the bench, so they are asserted on
// the deterministic-floor surface.
func TestExtensionBench_CheckIDParity(t *testing.T) {
	ids := extensionBenchCheckIDs()

	want := []string{
		"plugin.path_active",
		"plugin.lobe_active",
		"plugin.tool_active",
		"unplugged.tool_gone",
		"unplugged.path_gone",
		"mcp.connected",
		"mcp.tool_discovered",
		"mcp.path_active",
		"mcp.tool_called",
		"mcp.unplugged_gone",
	}
	if len(ids) != len(want) {
		t.Fatalf("check ids = %d, want %d (%v)", len(ids), len(want), ids)
	}
	have := map[string]bool{}
	for _, id := range ids {
		have[id] = true
	}
	for _, id := range want {
		if !have[id] {
			t.Errorf("missing check id %q", id)
		}
	}
	plugin, mcp := 0, 0
	for _, id := range ids {
		switch {
		case strings.HasPrefix(id, "plugin."), strings.HasPrefix(id, "unplugged."):
			plugin++
		case strings.HasPrefix(id, "mcp."):
			mcp++
		}
	}
	if plugin != 5 {
		t.Errorf("plugin/unplugged checks = %d, want 5", plugin)
	}
	if mcp != 5 {
		t.Errorf("mcp.* checks = %d, want 5", mcp)
	}
}
