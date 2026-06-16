package clients

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// Request mirrors the LlmRequest contract fields the engine and lobe classes
// pass to a client. System may be a string or []any (cache-split block array);
// Tools are provider-agnostic tool specs (Anthropic-style). CountUsage is
// tri-state: nil means "default to true" (Python's bool=True), true/false
// are honored.
type Request struct {
	Stage       string
	System      any
	Messages    []map[string]any
	MaxTokens   int
	Temperature *float64
	Tools       []map[string]any
	CountUsage  *bool
}

// CountUsageOrDefault returns the effective count_usage flag (defaults to
// true when the caller didn't set it).
func (r Request) CountUsageOrDefault() bool {
	if r.CountUsage == nil {
		return true
	}
	return *r.CountUsage
}

// LlmCall is the narrow, injectable seam a lobe behavior calls. The production
// implementation wraps per-stage model resolution + the provider create call +
// usage roll-up.
type LlmCall interface {
	Call(ctx context.Context, req Request) (any, error)
}

// BaseClient is the common base for provider clients. It holds the model id
// and a per-process usage accumulator; subclasses implement Call.
type BaseClient struct {
	ModelName string
	APIKey    string
	BaseURL   string
	Provider  string

	totalUsage ProviderUsage
}

// Provider returns the provider tag (e.g. "anthropic", "openai", "minimax", "fake", "mixed").
func (b *BaseClient) ProviderName() string {
	if b == nil {
		return "base"
	}
	return b.Provider
}

// Model returns the configured model id.
func (b *BaseClient) Model() string {
	if b == nil {
		return ""
	}
	return b.ModelName
}

// ModelFor returns the model to use for a given stage. MixedClient overrides
// this to route per stage.
func (b *BaseClient) ModelFor(stage string) string {
	return b.ModelName
}

// TotalUsage returns the aggregated usage across all calls made on this client.
func (b *BaseClient) TotalUsage() ProviderUsage {
	if b == nil {
		return ProviderUsage{}
	}
	return b.totalUsage
}

// Record accumulates usage into the running total.
func (b *BaseClient) Record(u ProviderUsage) {
	if b == nil {
		return
	}
	b.totalUsage = b.totalUsage.Add(u)
}

// String returns a "TypeName(model=...)" representation (cosmetic).
func (b *BaseClient) String() string {
	if b == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s(model=%q)", providerTypeName(b), b.ModelName)
}

func providerTypeName(b *BaseClient) string {
	if b == nil {
		return "BaseClient"
	}
	switch b.Provider {
	case "anthropic":
		return "AnthropicClient"
	case "openai":
		return "OpenAIClient"
	case "minimax":
		return "MiniMaxClient"
	case "fake":
		return "FakeClient"
	case "mixed":
		return "MixedClient"
	default:
		return "BaseClient"
	}
}

// makeClient resolves a client from a string shorthand or passes a client
// instance through unchanged. Mirrors agent_sdk/clients/base.py make_client.
func makeClient(spec any) (LlmCall, error) {
	if spec == nil {
		return nil, fmt.Errorf("a client is required")
	}
	if s, ok := spec.(string); ok {
		low := strings.ToLower(s)
		switch {
		case strings.HasPrefix(low, "gpt"),
			strings.HasPrefix(low, "o1"),
			strings.HasPrefix(low, "o3"),
			strings.HasPrefix(low, "o4"):
			return &OpenAIClient{BaseClient: BaseClient{ModelName: s, Provider: "openai"}}, nil
		case strings.HasPrefix(low, "minimax"),
			strings.HasPrefix(low, "abab"):
			return &MiniMaxClient{AnthropicClient: AnthropicClient{BaseClient: BaseClient{ModelName: s, Provider: "minimax"}}}, nil
		default:
			return &AnthropicClient{BaseClient: BaseClient{ModelName: s, Provider: "anthropic"}}, nil
		}
	}
	if c, ok := spec.(LlmCall); ok {
		return c, nil
	}
	// If it's a pointer to one of our concrete client types, wrap it.
	switch v := spec.(type) {
	case *AnthropicClient:
		return v, nil
	case *OpenAIClient:
		return v, nil
	case *MiniMaxClient:
		return v, nil
	case *FakeClient:
		return v, nil
	case *MixedClient:
		return v, nil
	}
	return nil, fmt.Errorf("unsupported client spec: %T", spec)
}

// MakeClient is the public form of make_client.
func MakeClient(spec any) LlmCall {
	c, err := makeClient(spec)
	if err != nil {
		panic(err)
	}
	return c
}

// MakeClientErr is the error-returning public form (callers that want to
// surface the resolution error rather than panic on bad input).
func MakeClientErr(spec any) (LlmCall, error) {
	return makeClient(spec)
}

// envFirst returns the first non-empty environment variable from the given
// names; mirrors the Python `_env` helper.
func envFirst(names ...string) string {
	for _, n := range names {
		if v := os.Getenv(n); v != "" {
			return v
		}
	}
	return ""
}
