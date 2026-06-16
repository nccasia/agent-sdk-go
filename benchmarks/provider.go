package benchmarks

import (
	"os"
	"strings"
)

// Provider env for the LIVE benches — ported from benchmarks/_shared/provider.py.
// Bridges MiniMax-native names onto the Anthropic-compatible env the clients read,
// and returns the model name to drive the live tiers (or "" when no credentials).

// LoadProvider reads the process environment (after bridging MiniMax names) and
// returns the model to drive make_client, or "" when no credentials are set.
// Mirrors provider.load_provider (env-file loading is the caller's concern; the
// process env is the source of truth in the Go port).
func LoadProvider() string {
	env := map[string]string{}
	for _, k := range []string{
		"MINIMAX_API_KEY", "MINIMAX_BASE_URL", "ANTHROPIC_AUTH_TOKEN",
		"ANTHROPIC_API_KEY", "ANTHROPIC_BASE_URL", "ANTHROPIC_MODEL",
	} {
		if v, ok := os.LookupEnv(k); ok {
			env[k] = v
		}
	}
	model := LoadProviderFrom(env)
	// Reflect any bridged values back into the process env so make_client reads them.
	for _, k := range []string{"ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL"} {
		if v, ok := env[k]; ok {
			_ = os.Setenv(k, v)
		}
	}
	return model
}

// LoadProviderFrom resolves the model from an explicit env map, mutating it to
// bridge MINIMAX_* onto ANTHROPIC_*. Returns "" when no credentials are present.
// Pure + testable; mirrors _bridge_minimax + load_provider's resolution.
func LoadProviderFrom(env map[string]string) string {
	bridgeMiniMax(env)
	if env["ANTHROPIC_AUTH_TOKEN"] == "" && env["ANTHROPIC_API_KEY"] == "" {
		return ""
	}
	if m := env["ANTHROPIC_MODEL"]; m != "" {
		return m
	}
	if env["MINIMAX_API_KEY"] != "" {
		return "MiniMax-M2.7"
	}
	return "claude-opus-4-6"
}

func bridgeMiniMax(env map[string]string) {
	if key := env["MINIMAX_API_KEY"]; key != "" &&
		env["ANTHROPIC_AUTH_TOKEN"] == "" && env["ANTHROPIC_API_KEY"] == "" {
		env["ANTHROPIC_AUTH_TOKEN"] = key
	}
	if base := env["MINIMAX_BASE_URL"]; base != "" && env["ANTHROPIC_BASE_URL"] == "" {
		base = strings.TrimRight(base, "/")
		if !strings.HasSuffix(base, "/anthropic") {
			base += "/anthropic"
		}
		env["ANTHROPIC_BASE_URL"] = base
	}
}
