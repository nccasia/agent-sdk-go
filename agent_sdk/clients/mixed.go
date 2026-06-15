package clients

import (
	"context"
	"fmt"
)

// MixedClient dispatches each call to a per-stage (per-provider) client.
// Unmapped stages fall back to Default. Usage is aggregated across all
// sub-clients (each sub-client owns its own accounting; MixedClient computes
// the aggregate on read).
type MixedClient struct {
	Provider string
	Default  LlmCall
	ByStage  map[string]LlmCall
	Model_   string
}

// NewMixedClient builds a stage-dispatching composite from a map of clients.
// Keys include "default" + any number of stage names. String shorthand is
// resolved through MakeClient.
func NewMixedClient(spec map[string]any) *MixedClient {
	defSpec, ok := spec["default"]
	if !ok {
		panic("MixedClient requires a 'default' client")
	}
	def, err := makeClient(defSpec)
	if err != nil {
		panic(err)
	}
	by := map[string]LlmCall{}
	for k, v := range spec {
		if k == "default" {
			continue
		}
		c, err := makeClient(v)
		if err != nil {
			panic(err)
		}
		by[k] = c
	}
	model := "mixed"
	if mr, ok := def.(interface{ Model() string }); ok {
		if m2 := mr.Model(); m2 != "" {
			model = m2
		}
	}
	return &MixedClient{Provider: "mixed", Default: def, ByStage: by, Model_: model}
}

// ClientFor returns the client that handles the given stage.
func (m *MixedClient) ClientFor(stage string) LlmCall {
	if c, ok := m.ByStage[stage]; ok {
		return c
	}
	return m.Default
}

// Model returns the default client's model id (for the BaseClient contract).
func (m *MixedClient) Model() string { return m.Model_ }

// ProviderName returns "mixed".
func (m *MixedClient) ProviderName() string { return "mixed" }

// TotalUsage aggregates usage across the default + every per-stage client.
func (m *MixedClient) TotalUsage() ProviderUsage {
	total := ProviderUsage{}
	clients := []LlmCall{m.Default}
	for _, c := range m.ByStage {
		clients = append(clients, c)
	}
	for _, c := range clients {
		if c == nil {
			continue
		}
		// Anything implementing a TotalUsage() ProviderUsage method is summed.
		if ur, ok := c.(interface{ TotalUsage() ProviderUsage }); ok {
			total = total.Add(ur.TotalUsage())
		}
	}
	return total
}

// Call dispatches the call to the per-stage client (or default).
func (m *MixedClient) Call(ctx context.Context, req Request) (any, error) {
	client := m.ClientFor(req.Stage)
	if client == nil {
		return nil, fmt.Errorf("mixed client: no default client wired")
	}
	return client.Call(ctx, req)
}
