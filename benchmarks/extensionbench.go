package benchmarks

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/nccasia/agent-sdk-go/agent_sdk/agent"
	"github.com/nccasia/agent-sdk-go/agent_sdk/core/spec"
	"github.com/nccasia/agent-sdk-go/agent_sdk/flows"
	"github.com/nccasia/agent-sdk-go/agent_sdk/lobes"
	"github.com/nccasia/agent-sdk-go/agent_sdk/mcp"
	"github.com/nccasia/agent-sdk-go/agent_sdk/plugins/support_triage"
	"github.com/nccasia/agent-sdk-go/agent_sdk/probe"
)

// extensionbench — the LIVE bench for *plugins as first-class plug-and-play
// components*. Like agentbench, it is a LIVE bench: a real PreactAgent driven
// against a real provider, no stubs/FakeClient. It measures what plugging
// actually changes in the agent's behavior across two scenario kinds:
//
//   - plugin — a full-surface plugin (PluginSupportTriage): an urgent-ticket
//     turn routes to the plugin's flow, lights its lobe, and the model calls
//     its local tool; unplugged it's gone.
//   - mcp — a plugin that OWNS a dedicated MCP server (OrdersPlugin): the agent
//     connects to the MCP (status check), discovers its schema, registers the
//     tool, routes to the plugin's agentic flow, and the live model CALLS the
//     MCP-served tool; unplugged none of it exists. (The MCP server runs
//     in-process over the transport seam — a real MCP server; the agent /
//     provider is live.)
//
// extensionbench is LIVE only: every check needs a real agent + provider, so
// the bench is composed as a SINGLE mode "extension" carrying the 5 plugin.* /
// unplugged.* checks + the 5 mcp.* checks (10 total). Without a provider (the
// deterministic floor) the mode is MISSING → the verdict is UNMEASURED (no
// evidence is never READY) — mirroring run.py's refusal to run without a
// provider token (exit 2). Ported from benchmarks/extensionbench/run.py.

const extensionBenchInstr = "You are a support assistant. When a ticket or order id is mentioned, look it " +
	"up with the available tool before answering, and state the result."

// extensionBenchCheckIDs is the static check-id surface — the 5 plugin/unplugged
// checks + 5 mcp.* checks the live run emits, in run.py order. Asserted for
// cross-language parity independent of the provider.
func extensionBenchCheckIDs() []string {
	return []string{
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
}

// extensionBenchBehavior is one line of dataset/behaviors.jsonl.
type extensionBenchBehavior struct {
	ID         string
	Kind       string
	Query      string
	TriagePath string
	TriageTool string
	TriageLobe string
	MCPServer  string
	MCPTool    string
	MCPPath    string
}

// extensionBenchBehaviors mirrors dataset/behaviors.jsonl (the deterministic
// behavior contract is small and fixed, so it is inlined rather than embedded).
func extensionBenchBehaviors() []extensionBenchBehavior {
	return []extensionBenchBehavior{
		{
			ID:         "triage-urgent",
			Kind:       "plugin",
			Query:      "this incident is urgent — escalate ticket 412, the payments service is down",
			TriagePath: "triage",
			TriageTool: "lookup_ticket",
			TriageLobe: "triage",
		},
		{
			ID:        "mcp-order-status",
			Kind:      "mcp",
			Query:     "what is the current shipping status of order ORD-9? please look it up.",
			MCPServer: "orders",
			MCPTool:   "order_status",
			MCPPath:   "orders",
		},
	}
}

// RunExtensionBench composes the extensionbench verdict. With no model (the
// deterministic floor) the single "extension" mode is missing → UNMEASURED.
// With a model the real PreactAgent is driven (plugged + bare) and each behavior
// scored.
func RunExtensionBench(ctx context.Context, model string) (Verdict, error) {
	var payload *ModePayload
	if model != "" {
		p, err := extensionBenchLive(ctx, model)
		if err != nil {
			return Verdict{}, err
		}
		payload = p
	}
	payloads := map[string]*ModePayload{"extension": payload}
	return ComposeVerdict(payloads, nil), nil
}

// RunExtensionBenchProbes captures inspectable traces for the viewer. With a
// real model it drives the actual agents; offline (model=="") it builds the SAME
// representative agents against a FakeClient and runs one PLUGGED + one BARE turn
// through probe.Probe — the plugged trace shows the plugin's path/flow + lobe +
// tool, the bare trace shows the default path — so the inspection renders a real
// path/flow + the executed stages. Adds traces only — the live verdict (Run)
// stays UNMEASURED without a provider. Mirrors run.py's plugged/unplugged probes
// feeding write_viewer.
func RunExtensionBenchProbes(ctx context.Context, model string) ([]*probe.Record, error) {
	behaviors := extensionBenchBehaviors()
	if len(behaviors) == 0 {
		return nil, nil
	}
	b := behaviors[0] // the full-surface plugin scenario
	plugged, err := probe.Probe(ctx, agent.MustPreactAgent(agent.Config{
		Client:       benchProbeClient(model),
		Instructions: extensionBenchInstr,
		Plugins:      []agent.Plugin{support_triage.NewPluginSupportTriage()},
	}), b.Query, probe.WithLabel("plugged · "+b.ID))
	if err != nil {
		return nil, err
	}
	bare, err := probe.Probe(ctx, agent.MustPreactAgent(agent.Config{
		Client:       benchProbeClient(model),
		Instructions: extensionBenchInstr,
	}), b.Query, probe.WithLabel("unplugged · "+b.ID))
	if err != nil {
		return nil, err
	}
	return []*probe.Record{plugged, bare}, nil
}

func extensionBenchLive(ctx context.Context, model string) (*ModePayload, error) {
	checks := []Check{}
	for _, b := range extensionBenchBehaviors() {
		var (
			cks []Check
			err error
		)
		if b.Kind == "mcp" {
			cks, err = extensionBenchMCPScenario(ctx, model, b)
		} else {
			cks, err = extensionBenchPluginScenario(ctx, model, b)
		}
		if err != nil {
			return nil, err
		}
		checks = append(checks, cks...)
	}
	return NewPayload(checks, nil), nil
}

// ── plugin scenario (full-surface PluginSupportTriage) ───────────────────────

func extensionBenchPluginScenario(ctx context.Context, model string, b extensionBenchBehavior) ([]Check, error) {
	plugged, err := probe.Probe(ctx, agent.MustPreactAgent(agent.Config{
		Client:       model,
		Instructions: extensionBenchInstr,
		Plugins:      []agent.Plugin{support_triage.NewPluginSupportTriage()},
	}), b.Query, probe.WithLabel("plugged · "+b.ID))
	if err != nil {
		return nil, err
	}
	bare, err := probe.Probe(ctx, agent.MustPreactAgent(agent.Config{
		Client:       model,
		Instructions: extensionBenchInstr,
	}), b.Query, probe.WithLabel("unplugged · "+b.ID))
	if err != nil {
		return nil, err
	}

	pluggedTools := extensionBenchToolNames(plugged)
	bareTools := extensionBenchToolNames(bare)
	return []Check{
		ck("plugin.path_active", plugged.Flow == b.TriagePath, fmt.Sprintf("flow=%q", plugged.Flow)),
		ck("plugin.lobe_active", extensionBenchContains(plugged.ActivatedLobes(), b.TriageLobe),
			fmt.Sprintf("lobes=%v", plugged.ActivatedLobes())),
		ck("plugin.tool_active", extensionBenchSetHas(pluggedTools, b.TriageTool),
			fmt.Sprintf("tools=%v", extensionBenchSorted(pluggedTools))),
		ck("unplugged.tool_gone", !extensionBenchSetHas(bareTools, b.TriageTool),
			fmt.Sprintf("tools=%v", extensionBenchSorted(bareTools))),
		ck("unplugged.path_gone", bare.Flow != b.TriagePath, fmt.Sprintf("flow=%q", bare.Flow)),
	}, nil
}

// ── mcp scenario (OrdersPlugin owns a dedicated MCP server) ──────────────────

func extensionBenchMCPScenario(ctx context.Context, model string, b extensionBenchBehavior) ([]Check, error) {
	pluggedAgent := agent.MustPreactAgent(agent.Config{
		Client:       model,
		Instructions: extensionBenchInstr,
		Plugins:      []agent.Plugin{newExtensionBenchOrdersPlugin()},
	})
	status := pluggedAgent.Connect(ctx) // connect + discover the owned MCP server
	discovered := extensionBenchDiscoveredTools(pluggedAgent)
	plugged, err := probe.Probe(ctx, pluggedAgent, b.Query, probe.WithLabel("plugged(mcp) · "+b.ID))
	if err != nil {
		return nil, err
	}
	bare, err := probe.Probe(ctx, agent.MustPreactAgent(agent.Config{
		Client:       model,
		Instructions: extensionBenchInstr,
	}), b.Query, probe.WithLabel("unplugged · "+b.ID))
	if err != nil {
		return nil, err
	}

	pluggedTools := extensionBenchToolNames(plugged)
	bareTools := extensionBenchToolNames(bare)
	return []Check{
		ck("mcp.connected", status[b.MCPServer], fmt.Sprintf("status=%v", status)),
		ck("mcp.tool_discovered", extensionBenchSetHas(discovered, b.MCPTool),
			fmt.Sprintf("discovered=%v", extensionBenchSorted(discovered))),
		ck("mcp.path_active", plugged.Flow == b.MCPPath, fmt.Sprintf("flow=%q", plugged.Flow)),
		ck("mcp.tool_called", extensionBenchSetHas(pluggedTools, b.MCPTool),
			fmt.Sprintf("tools=%v", extensionBenchSorted(pluggedTools))),
		ck("mcp.unplugged_gone", !extensionBenchSetHas(bareTools, b.MCPTool),
			fmt.Sprintf("tools=%v", extensionBenchSorted(bareTools))),
	}, nil
}

// ── OrdersPlugin: a plugin that OWNS a dedicated MCP server + an agentic flow ──

// extensionBenchOrdersPlugin is the canonical "plugin with its own MCP" shape:
// it owns the `orders` MCP server (a real protocol over the in-process
// transport seam) plus a lobe/stage/flow that route to and use the discovered
// order_status tool. Mirrors run.py's OrdersPlugin.
type extensionBenchOrdersPlugin struct{}

func newExtensionBenchOrdersPlugin() *extensionBenchOrdersPlugin {
	return &extensionBenchOrdersPlugin{}
}

func (p *extensionBenchOrdersPlugin) Name() string { return "orders_mcp" }

func (p *extensionBenchOrdersPlugin) Install(setup *agent.AgentSetup) {
	setup.AddLobe(extensionBenchOrderLobe().Spec())
	setup.AddStage(flows.NewFlowStep(flows.FlowStep{
		Name:        "order_lookup",
		Description: "Look up the order via order_status and report it.",
		Loop:        "agentic",
		Tools:       []string{"order_status"},
		Lobes:       []string{"order_lookup"},
	}))
	setup.AddFlow(flows.NewFlow("orders",
		flows.FlowUseWhen("an order shipping-status question"),
		flows.FlowStages("order_lookup"),
		flows.FlowThreshold(0.5),
		flows.FlowSignalFn(extensionBenchOrderSignal),
	))
	setup.AddMCPServer(mcp.NewMCPToolRuntime(map[string]any{"name": "orders"}, extensionBenchOrdersTransport()))
}

func extensionBenchOrderLobe() lobes.Lobe {
	return lobes.Lobe{
		ID:           "order_lookup",
		Name:         "Order lookup",
		Description:  "Frame the turn as an order-status lookup.",
		UseWhen:      "a question about an order's shipping status",
		Layer:        spec.LayerCognition,
		Behavior:     "select",
		SystemPrompt: "Look up the order with the order_status tool, then report its status concisely.",
		BuildContext: true,
		Activation:   extensionBenchOrderActivation,
	}
}

func extensionBenchOrderActivation(ctx map[string]any) float64 {
	q := strings.ToLower(fmt.Sprint(ctx["query"]))
	if strings.Contains(q, "order") || strings.Contains(q, "ord-") {
		return 1.0
	}
	return 0
}

func extensionBenchOrderSignal(ctx map[string]any) float64 {
	q := strings.ToLower(fmt.Sprint(ctx["query"]))
	for _, cue := range []string{"order", "ord-", "shipment", "delivery"} {
		if strings.Contains(q, cue) {
			return 1.0
		}
	}
	return 0
}

// extensionBenchOrdersTransport is the in-process MCP server the OrdersPlugin
// owns (the real JSON-RPC protocol over the transport seam). Mirrors run.py's
// _orders_mcp_transport.
func extensionBenchOrdersTransport() mcp.Transport {
	tool := map[string]any{
		"name":        "order_status",
		"description": "Look up the current shipping status of an order by its id.",
		"inputSchema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"order_id": map[string]any{"type": "string"}},
			"required":   []string{"order_id"},
		},
	}
	return func(req map[string]any) (map[string]any, error) {
		method, _ := req["method"].(string)
		rid := req["id"]
		switch method {
		case "initialize":
			return map[string]any{"jsonrpc": "2.0", "id": rid,
				"result": map[string]any{"protocolVersion": "2025-06-18",
					"serverInfo": map[string]any{"name": "orders"}}}, nil
		case "notifications/initialized":
			return map[string]any{}, nil
		case "tools/list":
			return map[string]any{"jsonrpc": "2.0", "id": rid,
				"result": map[string]any{"tools": []any{tool}}}, nil
		case "tools/call":
			oid := "?"
			if params, ok := req["params"].(map[string]any); ok {
				if args, ok := params["arguments"].(map[string]any); ok {
					if v, ok := args["order_id"].(string); ok {
						oid = v
					}
				}
			}
			return map[string]any{"jsonrpc": "2.0", "id": rid,
				"result": map[string]any{"content": []any{map[string]any{"type": "text",
					"text": fmt.Sprintf("Order %s: shipped, arriving Friday.", oid)}}}}, nil
		}
		return map[string]any{"jsonrpc": "2.0", "id": rid,
			"error": map[string]any{"code": -32601, "message": "no method"}}, nil
	}
}

// ── helpers (namespaced by the extensionBench prefix) ────────────────────────

func extensionBenchToolNames(rec *probe.Record) map[string]struct{} {
	out := map[string]struct{}{}
	for _, c := range rec.ToolCalls {
		if n, _ := c["name"].(string); n != "" {
			out[n] = struct{}{}
		}
	}
	return out
}

func extensionBenchDiscoveredTools(a *agent.PreactAgent) map[string]struct{} {
	out := map[string]struct{}{}
	eng := a.Engine()
	if eng == nil {
		return out
	}
	rt, ok := eng.Tools.(interface{ GetToolSpecs() []map[string]any })
	if !ok {
		return out
	}
	for _, s := range rt.GetToolSpecs() {
		if n, _ := s["name"].(string); n != "" {
			out[n] = struct{}{}
		}
	}
	return out
}

func extensionBenchContains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func extensionBenchSetHas(set map[string]struct{}, x string) bool {
	_, ok := set[x]
	return ok
}

func extensionBenchSorted(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
