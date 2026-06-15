package clients

import (
	"context"
	"fmt"
)

// AnthropicClientOption is a functional option for AnthropicClient construction.
type AnthropicClientOption func(*AnthropicClient)

// WithTimeout overrides the per-request timeout.
func WithTimeout(t float64) AnthropicClientOption {
	return func(c *AnthropicClient) { c.timeout = t }
}

// WithAPIKey sets the API key.
func WithAPIKey(k string) AnthropicClientOption {
	return func(c *AnthropicClient) { c.BaseClient.APIKey = k }
}

// WithBaseURL sets the Anthropic-compatible base URL.
func WithBaseURL(u string) AnthropicClientOption {
	return func(c *AnthropicClient) { c.BaseClient.BaseURL = u }
}

// WithMaxRetries sets the SDK max_retries value.
func WithMaxRetries(n int) AnthropicClientOption {
	return func(c *AnthropicClient) { c.maxRetries = n }
}

// AnthropicClient is the Anthropic Messages API adapter. The raw response
// already has the .content / .stop_reason / .usage shape the engine consumes,
// so the client returns it as-is and a subclass (MiniMaxClient) overrides
// Postprocess to repair provider-specific quirks.
type AnthropicClient struct {
	BaseClient
	timeout    float64
	maxRetries int
	client     any // the real anthropic.AsyncAnthropic instance, lazily initialized
}

// NewAnthropicClient builds a client with the given model + options.
func NewAnthropicClient(model string, opts ...AnthropicClientOption) *AnthropicClient {
	if model == "" {
		model = "claude-opus-4-6"
	}
	c := &AnthropicClient{
		BaseClient: BaseClient{ModelName: model, Provider: "anthropic"},
		timeout:    300.0,
		maxRetries: 2,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// AnthropicClientFromMap is a helper for callers that build a client from a
// config map (used by future tasks that wire the LLM from YAML).
func AnthropicClientFromMap(model string, cfg map[string]any) *AnthropicClient {
	opts := []AnthropicClientOption{}
	if v, ok := cfg["timeout"].(float64); ok {
		opts = append(opts, WithTimeout(v))
	}
	if v, ok := cfg["api_key"].(string); ok {
		opts = append(opts, WithAPIKey(v))
	}
	if v, ok := cfg["base_url"].(string); ok {
		opts = append(opts, WithBaseURL(v))
	}
	if v, ok := cfg["max_retries"].(int); ok {
		opts = append(opts, WithMaxRetries(v))
	}
	return NewAnthropicClient(model, opts...)
}

// Timeout returns the per-request timeout in seconds.
func (c *AnthropicClient) Timeout() float64 { return c.timeout }

// ensure lazily builds the real anthropic SDK client. We do not import the
// anthropic SDK at compile time (the Go port keeps network code behind this
// seam) — callers that want to wire a real client can swap c.client at runtime
// (see `SetClient`).
func (c *AnthropicClient) ensure() any {
	if c.client != nil {
		return c.client
	}
	return nil
}

// SetClient lets a host application inject a real *anthropic.AsyncAnthropic
// (or any compatible client implementing the messages.create surface). The
// FakeClient + tests never call this; production wires it at startup.
func (c *AnthropicClient) SetClient(cl any) { c.client = cl }

// Call performs one Anthropic Messages call. When no real client has been
// injected, the call returns an explicit error — the contract is "no network
// in the SDK" so tests must use FakeClient.
func (c *AnthropicClient) Call(ctx context.Context, req Request) (any, error) {
	cl := c.ensure()
	if cl == nil {
		return nil, fmt.Errorf("anthropic client: no provider wired; inject via SetClient or use FakeClient for tests")
	}
	// The real client exposes a `messages.create` method matching the
	// anthropic.AsyncAnthropic surface. We use a small reflection adapter so
	// we can stay decoupled from the SDK package at compile time.
	return callAnthropic(ctx, cl, c.ModelFor(req.Stage), req.System, req.Messages, req.MaxTokens, temperatureOrZero(req.Temperature), req.Tools)
}

// Postprocess is the provider-specific response hook. Base Anthropic is a
// faithful passthrough — real Anthropic never emits MiniMax-style markup.
func (c *AnthropicClient) postprocess(resp any) any {
	return resp
}

// Postprocess is the public Postprocess form (callable from outside the
// package — used by tests that drive the recovery logic directly).
func (c *AnthropicClient) Postprocess(resp any) any { return c.postprocess(resp) }

// usageFromAnthropic extracts a ProviderUsage from a response with the
// anthropic shape (.usage.input_tokens / .output_tokens / etc.).
func usageFromAnthropic(resp any) ProviderUsage {
	u, ok := getField(resp, "Usage")
	if !ok || u == nil {
		return ProviderUsage{}
	}
	in, _ := getInt(u, "InputTokens")
	out, _ := getInt(u, "OutputTokens")
	cr, _ := getInt(u, "CacheReadInputTokens")
	cw, _ := getInt(u, "CacheCreationInputTokens")
	return ProviderUsage{
		InputTokens:      in,
		OutputTokens:     out,
		CacheReadTokens:  cr,
		CacheWriteTokens: cw,
	}
}

func temperatureOrZero(t *float64) float64 {
	if t == nil {
		return 0.0
	}
	return *t
}
