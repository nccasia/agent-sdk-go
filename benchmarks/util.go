package benchmarks

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nccasia/agent-sdk-go/agent_sdk/clients"
)

// benchProbeClient picks the client a live bench's Probe closure drives: the
// real model when one is configured, else an offline FakeClient so the report
// stays inspectable with no provider. The FakeClient yields a short scripted
// reply per turn (enough to flow a real path + >=1 stage through probe.Probe);
// the live verdict still stays UNMEASURED without a model (Run never sees the
// FakeClient — only the inspection Probe does). Mirrors the free benches' probe
// loops feeding write_viewer.
func benchProbeClient(model string) any {
	if model != "" {
		return model
	}
	return clients.NewFakeClient([]any{"ok", "ok", "ok", "ok", "ok", "ok", "ok", "ok"}, nil)
}

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
