package benchmarks

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mezon/agent-sdk-go/agent_sdk/agent"
	"github.com/mezon/agent-sdk-go/agent_sdk/clients"
	"github.com/mezon/agent-sdk-go/agent_sdk/core/attention"
	"github.com/mezon/agent-sdk-go/agent_sdk/mcp"
	"github.com/mezon/agent-sdk-go/agent_sdk/probe"
	"github.com/mezon/agent-sdk-go/agent_sdk/tools"
)

// toolbench — the bench for the SDK's TOOL-USE concepts. Certifies @tool spec
// generation, the FunctionToolRuntime/CompositeToolRuntime boundary, embedded
// MCPToolRuntime discovery, and the ToolSelectLobe adaptive-exposure algorithm.
// The free deterministic modes (spec/select/composite) run with no provider; the
// live agentic-loop mode registers only with a provider. Ported from
// benchmarks/toolbench/run.py (free tier).

func tbWeather() *tools.ToolDef {
	return tools.Tool("get_weather", func(_ context.Context, inp map[string]any) (any, error) {
		city, _ := inp["city"].(string)
		units, _ := inp["units"].(string)
		if units == "" {
			units = "celsius"
		}
		return fmt.Sprintf("%s: 21 %s", city, units), nil
	},
		tools.Desc("Report the current weather for a city."),
		tools.Param("city", "string", true, nil),
		tools.Param("units", "string", false, "celsius"),
	)
}

func tbTicket() *tools.ToolDef {
	return tools.Tool("tickets.create", func(_ context.Context, inp map[string]any) (any, error) {
		title, _ := inp["title"].(string)
		pri := 3
		if p, ok := inp["priority"].(int); ok {
			pri = p
		}
		return fmt.Sprintf("created %q p%d", title, pri), nil
	},
		tools.Desc("Open a support ticket."),
		tools.Requires("acl"),
		tools.Param("title", "string", true, nil),
		tools.Param("priority", "integer", false, 3),
	)
}

func tbOrderLocal() *tools.ToolDef {
	return tools.Tool("order_status_local", func(_ context.Context, inp map[string]any) (any, error) {
		oid, _ := inp["order_id"].(string)
		return fmt.Sprintf("Order %s: shipped, arriving Friday.", oid), nil
	},
		tools.Desc("Look up the shipping status of an order by id."),
		tools.Param("order_id", "string", true, nil),
	)
}

func tbOrdersTransport() mcp.Transport {
	spec := map[string]any{"name": "order_status", "description": "Shipping status of an order by id.",
		"inputSchema": map[string]any{"type": "object",
			"properties": map[string]any{"order_id": map[string]any{"type": "string"}},
			"required":   []any{"order_id"}}}
	return func(req map[string]any) (map[string]any, error) {
		method, _ := req["method"].(string)
		rid := req["id"]
		switch method {
		case "initialize":
			return map[string]any{"jsonrpc": "2.0", "id": rid,
				"result": map[string]any{"protocolVersion": "2025-06-18", "serverInfo": map[string]any{"name": "orders"}}}, nil
		case "notifications/initialized":
			return map[string]any{}, nil
		case "tools/list":
			return map[string]any{"jsonrpc": "2.0", "id": rid, "result": map[string]any{"tools": []any{spec}}}, nil
		case "tools/call":
			oid := "?"
			if p, ok := req["params"].(map[string]any); ok {
				if a, ok := p["arguments"].(map[string]any); ok {
					if v, ok := a["order_id"].(string); ok {
						oid = v
					}
				}
			}
			return map[string]any{"jsonrpc": "2.0", "id": rid,
				"result": map[string]any{"content": []any{map[string]any{"type": "text", "text": fmt.Sprintf("Order %s: in transit.", oid)}}}}, nil
		}
		return map[string]any{"jsonrpc": "2.0", "id": rid, "error": map[string]any{"code": -32601, "message": "no method"}}, nil
	}
}

// run_spec: @tool spec generation + validation + invocation.
func tbRunSpec(ctx context.Context) *ModePayload {
	gw := tbWeather()
	tk := tbTicket()
	s := gw.Spec()
	props, _ := (s["input_schema"].(map[string]any))["properties"].(map[string]any)
	rt := tools.NewFunctionToolRuntime(gw)
	invoked, _ := rt.CallTool(ctx, "get_weather", map[string]any{"city": "Hanoi"}, nil, nil)
	missing, _ := rt.CallTool(ctx, "get_weather", map[string]any{}, nil, nil)
	unknown, _ := rt.CallTool(ctx, "nope", map[string]any{}, nil, nil)
	name, _ := s["name"].(string)
	desc, _ := s["description"].(string)
	_, hasCity := props["city"]
	_, hasUnits := props["units"]
	tkSchema := tk.InputSchema
	tkProps, _ := tkSchema["properties"].(map[string]any)
	_, hasTitle := tkProps["title"]
	checks := []Check{
		ck("spec.wellformed", name == "get_weather" && strings.HasPrefix(desc, "Report") && hasCity && hasUnits,
			fmt.Sprintf("props=%v", sortedKeys(props))),
		ck("spec.required_inference", equal(gw.MissingRequired(map[string]any{}), []string{"city"}) &&
			len(gw.MissingRequired(map[string]any{"city": "x"})) == 0, "city required, units optional"),
		ck("spec.pydantic_schema", tkSchema["type"] == "object" && hasTitle && len(tk.MissingRequired(map[string]any{"title": "x"})) == 0,
			"structured arg → model schema"),
		ck("spec.requires_captured", equal(tk.Requires, []string{"acl"}), fmt.Sprintf("%v", tk.Requires)),
		ck("spec.invoke_stringifies", invoked == "Hanoi: 21 celsius" &&
			strings.HasPrefix(missing, "Error") && strings.HasPrefix(unknown, "Error"),
			fmt.Sprintf("invoke=%q; missing-arg & unknown-tool → clean errors", invoked)),
	}
	return NewPayload(checks, nil)
}

// run_select: ToolSelectLobe adaptive exposure (essentials firewall + relevance trim).
func tbCatalog() []map[string]any {
	rows := [][2]string{
		{"kb.search", "Search the knowledge base for documents."},
		{"memory", "Save or recall durable memory entries."},
		{"search_docs", "Search internal documentation and knowledge base articles about refunds and policies."},
		{"delete_account", "Permanently delete a user account."},
		{"send_email", "Send an email to a recipient."},
		{"convert_currency", "Convert an amount between two currencies."},
		{"schedule_meeting", "Schedule a calendar meeting with attendees."},
		{"resize_image", "Resize an image file to given dimensions."},
		{"translate_text", "Translate text from one language to another."},
		{"roll_dice", "Roll an n-sided die and return the result."},
	}
	out := make([]map[string]any, len(rows))
	for i, r := range rows {
		out[i] = map[string]any{"name": r[0], "description": r[1]}
	}
	return out
}

func tbRunSelect() *ModePayload {
	lobe := tools.ToolSelectLobe{}
	essential := func(n string) bool { return strings.HasPrefix(n, "kb.") || n == "memory" }
	kept, rec := lobe.Select(tbCatalog(), "find knowledge base articles about refunds", nil, nil,
		essential, attention.DefaultNodeWeights, 0.05, 4)
	keptNames := map[string]struct{}{}
	nonEssKept := 0
	for _, k := range rec.Kept {
		keptNames[k.Name] = struct{}{}
		if !k.Essential {
			nonEssKept++
		}
	}
	dropped := map[string]string{}
	for _, d := range rec.Dropped {
		dropped[d.Name] = d.Reason
	}
	parityDark := lobe.Lobe().Activation(map[string]any{}) == 0.0 &&
		lobe.Lobe().Activation(map[string]any{"tool_strategy": "adaptive"}) == 1.0
	checks := []Check{
		ck("select.essentials_kept", subset([]string{"kb.search", "memory"}, keptNames),
			fmt.Sprintf("kept=%v", sortedSet(keptNames))),
		ck("select.relevant_kept", hasKey(keptNames, "search_docs"), "the on-topic tool survives the trim"),
		ck("select.irrelevant_dropped", dropped["delete_account"] == "below_floor", fmt.Sprintf("dropped=%v", dropped)),
		ck("select.budget_respected", nonEssKept <= 4-2, fmt.Sprintf("non-essential kept=%d ≤ 2", nonEssKept)),
		ck("select.parity_dark", parityDark, "inert unless tool_strategy=adaptive"),
	}
	return NewPayload(checks, map[string]any{"tools_dropped": len(dropped), "kept": len(kept)})
}

// run_composite: CompositeToolRuntime dispatch + embedded MCP discovery.
func tbRunComposite(ctx context.Context) *ModePayload {
	funcRT := tools.NewFunctionToolRuntime(tbWeather(), tbOrderLocal())
	mrt := mcp.NewMCPToolRuntime(map[string]any{"name": "orders"}, tbOrdersTransport())
	mrt.Resolve(ctx)
	mcpNames := specNames(mrt.GetToolSpecs())
	comp := tools.NewCompositeToolRuntime(funcRT, mrt)
	allNames := specNames(comp.GetToolSpecs())
	routedFn, _ := comp.CallTool(ctx, "get_weather", map[string]any{"city": "Hanoi"}, nil, nil)
	routedMCP, _ := comp.CallTool(ctx, "order_status", map[string]any{"order_id": "A1"}, nil, nil)
	_, mcpHasOrder := comp.ExternalNames()["order_status"]
	checks := []Check{
		ck("composite.mcp_discovered", hasKey(mcpNames, "order_status"), fmt.Sprintf("mcp tools=%v", sortedSet(mcpNames))),
		ck("composite.mcp_called", strings.Contains(routedMCP, "A1"), fmt.Sprintf("call→%q", routedMCP)),
		ck("composite.routing", routedFn == "Hanoi: 21 celsius" &&
			subset([]string{"get_weather", "order_status"}, allNames), fmt.Sprintf("specs=%v", sortedSet(allNames))),
		ck("composite.external_marked", mcpHasOrder, "external MCP tools are never scored out by adaptive selection"),
	}
	return NewPayload(checks, nil)
}

// tbLoopClient scripts a deterministic tool-using turn for the probe: the first
// stage offered the get_weather tool emits a tool_use call, every later turn (and
// any stage without the tool) replies with a final text answer. This drives a
// REAL agentic tool loop through the engine with no provider, so the captured
// trace carries the tool call + the bounded answer.
func tbLoopClient() *clients.FakeClient {
	called := false
	return clients.Scripted(func(_, _ string, _ []map[string]any, toolSpecs []map[string]any) any {
		offered := false
		for _, t := range toolSpecs {
			if n, _ := t["name"].(string); n == "get_weather" {
				offered = true
			}
		}
		if offered && !called {
			called = true
			return map[string]any{
				"text":  "Let me check the weather.",
				"tools": []map[string]any{{"name": "get_weather", "input": map[string]any{"city": "Hanoi"}}},
			}
		}
		return "It is 21 celsius in Hanoi right now."
	})
}

// RunToolBenchProbes captures a representative probe trace of the bench's tool-use
// agent on a tool-call query. toolbench is a plumbing bench (it certifies @tool
// spec generation, the runtime boundary, MCP discovery, and adaptive selection),
// but this probe drives the REAL PreactAgent through one agentic tool loop with
// the get_weather + order_status_local tools registered — so the viewer inspection
// renders a populated trace (path/flow, the executed stages, the tool call, and
// the tool_selection record), not an empty one. Mirrors run.py:run_loop's single
// probe(agent, "...weather...") record. Offline-deterministic via FakeClient (the
// free gate runs with no provider), so model is ignored.
func RunToolBenchProbes(ctx context.Context, _ string) ([]*probe.Record, error) {
	a := agent.MustPreactAgent(agent.Config{
		Client:       tbLoopClient(),
		Tools:        []any{tbWeather(), tbOrderLocal()},
		Instructions: "You answer by calling tools, then reporting the result.",
	})
	rec, err := probe.Probe(ctx, a, "What is the weather in Hanoi right now?", probe.WithLabel("loop · weather"))
	if err != nil {
		return nil, err
	}
	return []*probe.Record{rec}, nil
}

// RunToolBench composes the toolbench verdict. The live loop mode registers only
// with a provider; here only the free modes gate.
func RunToolBench(ctx context.Context, model string) (Verdict, error) {
	payloads := map[string]*ModePayload{
		"spec":      tbRunSpec(ctx),
		"select":    tbRunSelect(),
		"composite": tbRunComposite(ctx),
	}
	return ComposeVerdict(payloads, map[string][]string{"select": {"tools_dropped"}}), nil
}

func specNames(specs []map[string]any) map[string]struct{} {
	out := map[string]struct{}{}
	for _, s := range specs {
		if n, ok := s["name"].(string); ok {
			out[n] = struct{}{}
		}
	}
	return out
}

func hasKey(m map[string]struct{}, k string) bool { _, ok := m[k]; return ok }

var _ = sort.Strings
