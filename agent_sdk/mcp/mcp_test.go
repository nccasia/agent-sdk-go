package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"
)

var weatherTool = map[string]any{
	"name":        "weather",
	"description": "Current weather for a city.",
	"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}}},
}

// fakeMCP returns a JSON-RPC handler emulating an MCP server over the transport
// seam. If record is non-nil, each method is appended to it.
func fakeMCP(tools []map[string]any, record *[]string) Transport {
	return func(req map[string]any) (map[string]any, error) {
		method, _ := req["method"].(string)
		if record != nil {
			*record = append(*record, method)
		}
		rid := req["id"]
		switch method {
		case "initialize":
			return map[string]any{"jsonrpc": "2.0", "id": rid, "result": map[string]any{
				"protocolVersion": "2025-06-18", "serverInfo": map[string]any{"name": "fake"},
			}}, nil
		case "notifications/initialized":
			return map[string]any{}, nil
		case "tools/list":
			return map[string]any{"jsonrpc": "2.0", "id": rid, "result": map[string]any{"tools": tools}}, nil
		case "tools/call":
			p, _ := req["params"].(map[string]any)
			text := stringifyAny(p["name"]) + "(" + stringifyAny(p["arguments"]) + ")"
			return map[string]any{"jsonrpc": "2.0", "id": rid, "result": map[string]any{
				"content": []any{map[string]any{"type": "text", "text": text}},
			}}, nil
		}
		return map[string]any{"jsonrpc": "2.0", "id": rid, "error": map[string]any{"code": -32601, "message": "no method"}}, nil
	}
}

func stringifyAny(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case map[string]any:
		parts := []string{}
		for k, val := range x {
			parts = append(parts, k+"="+stringifyAny(val))
		}
		return "{" + strings.Join(parts, ",") + "}"
	default:
		return ""
	}
}

// TestConnectThenDiscoverBuildsSpecs mirrors test_connect_then_discover_builds_specs.
func TestConnectThenDiscoverBuildsSpecs(t *testing.T) {
	rt := NewMCPToolRuntime(map[string]any{"name": "wx"}, fakeMCP([]map[string]any{weatherTool}, nil))
	if ok := rt.Connect(context.Background()); !ok {
		t.Fatal("connect failed")
	}
	if !rt.Connected || rt.Error != "" {
		t.Fatalf("connected=%v error=%q", rt.Connected, rt.Error)
	}
	specs := rt.Discover(context.Background())
	if len(specs) != 1 || specs[0]["name"] != "weather" {
		t.Fatalf("specs = %v", specs)
	}
	props := specs[0]["input_schema"].(map[string]any)["properties"].(map[string]any)
	if props["city"].(map[string]any)["type"] != "string" {
		t.Fatalf("city schema = %v", props["city"])
	}
}

// TestResolveIsIdempotent mirrors test_resolve_is_idempotent.
func TestResolveIsIdempotent(t *testing.T) {
	var calls []string
	rt := NewMCPToolRuntime(map[string]any{"name": "wx"}, fakeMCP([]map[string]any{weatherTool}, &calls))
	rt.Resolve(context.Background())
	rt.Resolve(context.Background())
	if c := count(calls, "initialize"); c != 1 {
		t.Fatalf("initialize count = %d", c)
	}
	if c := count(calls, "tools/list"); c != 1 {
		t.Fatalf("tools/list count = %d", c)
	}
}

// TestCallToolRoundtrips mirrors test_call_tool_roundtrips.
func TestCallToolRoundtrips(t *testing.T) {
	rt := NewMCPToolRuntime(map[string]any{"name": "wx"}, fakeMCP([]map[string]any{weatherTool}, nil))
	rt.Resolve(context.Background())
	out, _ := rt.CallTool(context.Background(), "weather", map[string]any{"city": "hanoi"}, nil, nil)
	if !strings.Contains(out, "weather") || !strings.Contains(out, "hanoi") {
		t.Fatalf("out = %q", out)
	}
}

// TestFailedConnectDegradesGracefully mirrors test_failed_connect_degrades_gracefully.
func TestFailedConnectDegradesGracefully(t *testing.T) {
	boom := func(req map[string]any) (map[string]any, error) { return nil, errors.New("server down") }
	rt := NewMCPToolRuntime(map[string]any{"name": "down"}, boom)
	if rt.Resolve(context.Background()) {
		t.Fatal("expected resolve false")
	}
	if rt.Connected || !strings.Contains(rt.Error, "server down") {
		t.Fatalf("connected=%v error=%q", rt.Connected, rt.Error)
	}
	if len(rt.GetToolSpecs()) != 0 {
		t.Fatalf("expected no specs from a dead server")
	}
	out, _ := rt.CallTool(context.Background(), "weather", map[string]any{"city": "x"}, nil, nil)
	if !strings.Contains(out, "not connected") {
		t.Fatalf("out = %q", out)
	}
}

// TestSpecsEmptyBeforeResolve mirrors test_specs_empty_before_resolve.
func TestSpecsEmptyBeforeResolve(t *testing.T) {
	rt := NewMCPToolRuntime(map[string]any{"name": "wx"}, fakeMCP([]map[string]any{weatherTool}, nil))
	if len(rt.GetToolSpecs()) != 0 {
		t.Fatalf("expected empty specs before resolve")
	}
}

// TestStatusConnectedAfterResolve mirrors test_status_connected_after_resolve.
func TestStatusConnectedAfterResolve(t *testing.T) {
	rt := NewMCPToolRuntime(map[string]any{"name": "wx"}, fakeMCP([]map[string]any{weatherTool}, nil))
	if rt.Status != "unreachable" {
		t.Fatalf("status = %q", rt.Status)
	}
	rt.Resolve(context.Background())
	if rt.Status != "connected" {
		t.Fatalf("status = %q", rt.Status)
	}
}

// TestStatusUnconfiguredWithoutEndpointOrTransport mirrors
// test_status_unconfigured_without_endpoint_or_transport.
func TestStatusUnconfiguredWithoutEndpointOrTransport(t *testing.T) {
	rt := NewMCPToolRuntime(map[string]any{"name": "wx"}, nil)
	if rt.Status != "unconfigured" {
		t.Fatalf("status = %q", rt.Status)
	}
}

// TestStatusTimeoutClassified mirrors test_status_timeout_classified.
func TestStatusTimeoutClassified(t *testing.T) {
	slow := func(req map[string]any) (map[string]any, error) { return nil, errors.New("Timeout: no response") }
	rt := NewMCPToolRuntime(map[string]any{"name": "down"}, slow)
	rt.Resolve(context.Background())
	if rt.Status != "timeout" || rt.Connected {
		t.Fatalf("status=%q connected=%v", rt.Status, rt.Connected)
	}
}

// TestStatusUnauthorizedClassified mirrors test_status_unauthorized_classified.
func TestStatusUnauthorizedClassified(t *testing.T) {
	deny := func(req map[string]any) (map[string]any, error) { return nil, errors.New("HTTP 401 unauthorized") }
	rt := NewMCPToolRuntime(map[string]any{"name": "wx"}, deny)
	rt.Resolve(context.Background())
	if rt.Status != "unauthorized" {
		t.Fatalf("status = %q", rt.Status)
	}
}

// TestStatusUnreachableClassified mirrors test_status_unreachable_classified.
func TestStatusUnreachableClassified(t *testing.T) {
	boom := func(req map[string]any) (map[string]any, error) { return nil, errors.New("connection refused") }
	rt := NewMCPToolRuntime(map[string]any{"name": "wx"}, boom)
	rt.Resolve(context.Background())
	if rt.Status != "unreachable" {
		t.Fatalf("status = %q", rt.Status)
	}
}

// TestStatusBadResponseWhenToolsListErrors mirrors
// test_status_bad_response_when_tools_list_errors.
func TestStatusBadResponseWhenToolsListErrors(t *testing.T) {
	halfUp := func(req map[string]any) (map[string]any, error) {
		m, _ := req["method"].(string)
		if m == "initialize" || m == "notifications/initialized" {
			return map[string]any{"jsonrpc": "2.0", "id": req["id"], "result": map[string]any{
				"protocolVersion": "2025-06-18", "serverInfo": map[string]any{},
			}}, nil
		}
		return map[string]any{"jsonrpc": "2.0", "id": req["id"], "error": map[string]any{"code": -32000, "message": "boom"}}, nil
	}
	rt := NewMCPToolRuntime(map[string]any{"name": "wx"}, halfUp)
	rt.Resolve(context.Background())
	if !rt.Connected || rt.Status != "bad_response" || len(rt.GetToolSpecs()) != 0 {
		t.Fatalf("connected=%v status=%q specs=%v", rt.Connected, rt.Status, rt.GetToolSpecs())
	}
}

func count(xs []string, target string) int {
	n := 0
	for _, x := range xs {
		if x == target {
			n++
		}
	}
	return n
}
