// Package mcp ports agent_sdk/mcp.py — an MCP client + tool runtime: connect,
// check status, discover schema, build provider-compatible tool specs, register.
//
// Per this rung, transport is an in-process JSON-RPC seam (no network): a
// Transport is a func(request) -> (response, error). An MCPToolRuntime resolves
// lazily (connect → discover) and degrades gracefully — a server that never
// connects contributes no tools, with .Error / .Status set for inspection.
package mcp

import (
	"context"
	"fmt"
	"strings"
)

const (
	jsonRPC         = "2.0"
	protocolVersion = "2025-06-18" // MCP protocol revision the client advertises
)

var clientInfo = map[string]any{"name": "agent-sdk", "version": "0.1.0"}

// Transport is the in-process JSON-RPC seam: a request dict → a response dict.
type Transport func(req map[string]any) (map[string]any, error)

// ConnectionStatus is why a server is (not) contributing tools this turn.
//
//	connected    — handshake + schema OK
//	unauthorized — handshake rejected for auth (401/403/auth error)
//	unreachable  — connection refused / DNS / network error
//	timeout      — no response within the deadline
//	bad_response — reachable but the reply wasn't valid JSON-RPC / had no tools
//	unconfigured — the spec has no endpoint to probe
type ConnectionStatus = string

// Error is a JSON-RPC error returned by an MCP server. Mirrors Python MCPError.
type Error struct {
	Data map[string]any
}

func (e *Error) Error() string {
	if e == nil || e.Data == nil {
		return "MCP error"
	}
	if msg, ok := e.Data["message"].(string); ok {
		return msg
	}
	return fmt.Sprintf("%v", e.Data)
}

// ServerSpec is a declarative MCP server definition (pure data). Mirrors
// MCPServerSpec.
type ServerSpec struct {
	Name      string
	Transport string // http | sse | embedded
	Endpoint  string
	AuthType  string // "" | bearer | header
	Auth      string
	Kind      string
	Config    map[string]any
}

// ServerSpecFromObj builds a ServerSpec from a raw map (mirrors from_obj).
func ServerSpecFromObj(obj map[string]any) ServerSpec {
	d := obj
	if d == nil {
		d = map[string]any{}
	}
	getStr := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := d[k].(string); ok && v != "" {
				return v
			}
		}
		return ""
	}
	kind := getStr("kind")
	if kind == "" {
		if caps, ok := d["capabilities"].(map[string]any); ok {
			kind, _ = caps["kind"].(string)
		}
	}
	transport := getStr("transport")
	if transport == "" {
		transport = "http"
	}
	cfg, _ := d["config"].(map[string]any)
	if cfg == nil {
		cfg = map[string]any{}
	}
	return ServerSpec{
		Name:      getStr("name", "mcp_server_ref"),
		Transport: transport,
		Endpoint:  getStr("endpoint", "url"),
		AuthType:  getStr("auth_type"),
		Auth:      getStr("auth", "token"),
		Kind:      kind,
		Config:    cfg,
	}
}

func classifyError(err error) ConnectionStatus {
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "timeout") {
		return "timeout"
	}
	for _, t := range []string{"401", "403", "unauthor", "forbidden", "auth"} {
		if strings.Contains(text, t) {
			return "unauthorized"
		}
	}
	if _, ok := err.(*Error); ok {
		return "bad_response"
	}
	return "unreachable"
}

// MCPToolRuntime is a ToolRuntime backed by an MCP server, resolved lazily.
// Mirrors the Python class.
type MCPToolRuntime struct {
	Spec       ServerSpec
	Connected  bool
	Status     ConnectionStatus
	Error      string
	ServerInfo map[string]any

	transport Transport
	specs     []map[string]any
	resolved  bool
	id        int
}

// NewMCPToolRuntime builds a runtime over the spec (a map or ServerSpec-shaped)
// and an in-process transport (nil ⇒ unconfigured).
func NewMCPToolRuntime(spec map[string]any, transport Transport) *MCPToolRuntime {
	s := ServerSpecFromObj(spec)
	configured := s.Endpoint != "" || transport != nil
	status := ConnectionStatus("unreachable")
	if !configured {
		status = "unconfigured"
	}
	return &MCPToolRuntime{
		Spec:       s,
		Status:     status,
		ServerInfo: map[string]any{},
		transport:  transport,
		specs:      []map[string]any{},
	}
}

// Name returns the server name.
func (r *MCPToolRuntime) Name() string { return r.Spec.Name }

func (r *MCPToolRuntime) nextID() int { r.id++; return r.id }

// rpc sends one JSON-RPC request and returns the result map (or an *Error).
func (r *MCPToolRuntime) rpc(method string, params map[string]any, notify bool) (map[string]any, error) {
	req := map[string]any{"jsonrpc": jsonRPC, "method": method}
	if !notify {
		req["id"] = r.nextID()
	}
	if params != nil {
		req["params"] = params
	}
	if r.transport == nil {
		return nil, &Error{Data: map[string]any{"message": fmt.Sprintf("MCP server %q has no transport", r.Spec.Name)}}
	}
	resp, err := r.transport(req)
	if err != nil {
		return nil, err
	}
	if notify {
		return map[string]any{}, nil
	}
	if resp == nil {
		return map[string]any{}, nil
	}
	if errObj, ok := resp["error"].(map[string]any); ok && errObj != nil {
		return nil, &Error{Data: errObj}
	}
	if result, ok := resp["result"].(map[string]any); ok {
		return result, nil
	}
	return map[string]any{}, nil
}

// Connect performs the initialize handshake and records status. Never returns an
// error to the caller — failure degrades, recorded on Status/Error.
func (r *MCPToolRuntime) Connect(ctx context.Context) bool {
	result, err := r.rpc("initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      clientInfo,
	}, false)
	if err != nil {
		r.Connected = false
		r.Status = classifyError(err)
		r.Error = errString(err)
		return false
	}
	if si, ok := result["serverInfo"].(map[string]any); ok {
		r.ServerInfo = si
	}
	if _, err := r.rpc("notifications/initialized", nil, true); err != nil {
		r.Connected = false
		r.Status = classifyError(err)
		r.Error = errString(err)
		return false
	}
	r.Connected = true
	r.Status = "connected"
	r.Error = ""
	return true
}

// Discover runs tools/list → provider-compatible specs. Requires a prior Connect.
func (r *MCPToolRuntime) Discover(ctx context.Context) []map[string]any {
	if !r.Connected {
		return nil
	}
	result, err := r.rpc("tools/list", nil, false)
	if err != nil {
		r.Status = "bad_response"
		r.Error = errString(err)
		r.specs = []map[string]any{}
		return r.specs
	}
	raw, _ := result["tools"].([]map[string]any)
	if raw == nil {
		if anyList, ok := result["tools"].([]any); ok {
			for _, t := range anyList {
				if tm, ok := t.(map[string]any); ok {
					raw = append(raw, tm)
				}
			}
		}
	}
	specs := []map[string]any{}
	for _, t := range raw {
		if name, _ := t["name"].(string); name != "" {
			specs = append(specs, toSpec(t))
		}
	}
	r.specs = specs
	return r.specs
}

// Resolve is the resolve phase: connect (status), then discover the schema if up.
// Idempotent.
func (r *MCPToolRuntime) Resolve(ctx context.Context) bool {
	if r.resolved {
		return r.Connected
	}
	r.Connect(ctx)
	if r.Connected {
		r.Discover(ctx)
	}
	r.resolved = true
	return r.Connected
}

func toSpec(t map[string]any) map[string]any {
	name, _ := t["name"].(string)
	desc, _ := t["description"].(string)
	var schema any = t["inputSchema"]
	if schema == nil {
		schema = t["input_schema"]
	}
	if schema == nil {
		schema = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return map[string]any{"name": name, "description": desc, "input_schema": schema}
}

// GetToolSpecs returns the discovered specs (empty until Resolve succeeds).
func (r *MCPToolRuntime) GetToolSpecs() []map[string]any {
	return append([]map[string]any(nil), r.specs...)
}

// ExternalNames declares every discovered tool as external (third-party MCP), so
// adaptive selection never scores them out.
func (r *MCPToolRuntime) ExternalNames() map[string]struct{} {
	out := map[string]struct{}{}
	for _, s := range r.specs {
		if name, _ := s["name"].(string); name != "" {
			out[name] = struct{}{}
		}
	}
	return out
}

// CallTool executes one tools/call and renders the result as model-visible text.
func (r *MCPToolRuntime) CallTool(ctx context.Context, name string, inp map[string]any, retrievedChunks []map[string]any, alreadyRead map[string]struct{}) (string, error) {
	if !r.Connected {
		reason := r.Error
		if reason == "" {
			reason = "no status"
		}
		return fmt.Sprintf("Error: MCP server %q is not connected (%s).", r.Spec.Name, reason), nil
	}
	args := inp
	if args == nil {
		args = map[string]any{}
	}
	result, err := r.rpc("tools/call", map[string]any{"name": name, "arguments": args}, false)
	if err != nil {
		return fmt.Sprintf("Error: MCP tool %q failed: %s", name, errString(err)), nil
	}
	return renderContent(result), nil
}

// renderContent maps an MCP tools/call result onto model-visible text.
func renderContent(result map[string]any) string {
	content, _ := result["content"].([]any)
	var parts []string
	for _, b := range content {
		if block, ok := b.(map[string]any); ok {
			if t, _ := block["type"].(string); t == "text" {
				if txt, ok := block["text"].(string); ok {
					parts = append(parts, txt)
				}
			} else if txt, ok := block["text"].(string); ok {
				parts = append(parts, txt)
			}
		}
	}
	text := strings.Join(nonEmpty(parts), "\n")
	if text == "" {
		text = fmt.Sprintf("%v", result)
	}
	if isErr, _ := result["isError"].(bool); isErr {
		return "Error: " + text
	}
	return text
}

func nonEmpty(xs []string) []string {
	out := xs[:0]
	for _, x := range xs {
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}

func errString(err error) string {
	return fmt.Sprintf("%T: %s", err, err.Error())
}
