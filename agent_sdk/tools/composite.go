package tools

import "context"

// ToolRuntime is the runtime boundary between the agent harness and executable
// tools. Mirrors contracts.ToolRuntime / the Python ToolRuntime protocol.
type ToolRuntime interface {
	GetToolSpecs() []map[string]any
	CallTool(ctx context.Context, name string, inp map[string]any, retrievedChunks []map[string]any, alreadyRead map[string]struct{}) (string, error)
}

// ExternalNamer is the optional capability a runtime implements to declare which
// of its tools are external (third-party MCP) and thus never scored out by
// adaptive selection. An MCPToolRuntime returns all its names.
type ExternalNamer interface {
	ExternalNames() map[string]struct{}
}

// CompositeToolRuntime fans tool calls across several runtimes — built-in tools
// plus zero or more MCP runtimes. Mirrors contracts/tools.py:CompositeToolRuntime.
type CompositeToolRuntime struct {
	runtimes  []ToolRuntime
	toolOwner map[string]ToolRuntime
}

// NewCompositeToolRuntime builds a composite over the given runtimes.
func NewCompositeToolRuntime(runtimes ...ToolRuntime) *CompositeToolRuntime {
	return &CompositeToolRuntime{runtimes: runtimes, toolOwner: map[string]ToolRuntime{}}
}

// GetToolSpecs returns the merged, de-duplicated specs across runtimes (first
// owner of a name wins), and (re)builds the name→owner map.
func (c *CompositeToolRuntime) GetToolSpecs() []map[string]any {
	specs := []map[string]any{}
	c.toolOwner = map[string]ToolRuntime{}
	for _, rt := range c.runtimes {
		for _, spec := range rt.GetToolSpecs() {
			name, _ := spec["name"].(string)
			if name == "" {
				continue
			}
			if _, dup := c.toolOwner[name]; dup {
				continue
			}
			c.toolOwner[name] = rt
			specs = append(specs, spec)
		}
	}
	return specs
}

// ExternalNames is the set of tool names served by an EXTERNAL MCP installation —
// NOT the engine's well-known surface. Adaptive tool selection never scores these
// out (the engine has no curated relevance for third-party tools). Mirrors
// CompositeToolRuntime.external_names.
func (c *CompositeToolRuntime) ExternalNames() map[string]struct{} {
	if len(c.toolOwner) == 0 {
		c.GetToolSpecs()
	}
	out := map[string]struct{}{}
	for name, rt := range c.toolOwner {
		if en, ok := rt.(ExternalNamer); ok {
			for n := range en.ExternalNames() {
				if n == name {
					out[name] = struct{}{}
				}
			}
		}
	}
	return out
}

// CallTool dispatches a call to the runtime that owns the named tool.
func (c *CompositeToolRuntime) CallTool(ctx context.Context, name string, inp map[string]any, retrievedChunks []map[string]any, alreadyRead map[string]struct{}) (string, error) {
	rt := c.toolOwner[name]
	if rt == nil {
		c.GetToolSpecs()
		rt = c.toolOwner[name]
	}
	if rt == nil {
		return "Error: unknown tool '" + name + "'. Use only the provided tools.", nil
	}
	return rt.CallTool(ctx, name, inp, retrievedChunks, alreadyRead)
}
