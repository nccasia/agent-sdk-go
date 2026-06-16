package benchmarks

import "testing"

// TestLoadProviderNoCreds mirrors provider.load_provider: with no ANTHROPIC creds
// in the environment, returns "" (a bench prints a clean no-cred message + exits).
func TestLoadProviderNoCreds(t *testing.T) {
	env := map[string]string{}
	if got := LoadProviderFrom(env); got != "" {
		t.Fatalf("model = %q, want empty (no creds)", got)
	}
}

// TestLoadProviderDefaultsClaude mirrors: ANTHROPIC creds present, no model pinned,
// no MiniMax ⇒ the Anthropic default model.
func TestLoadProviderDefaultsClaude(t *testing.T) {
	env := map[string]string{"ANTHROPIC_AUTH_TOKEN": "tok"}
	if got := LoadProviderFrom(env); got != "claude-opus-4-6" {
		t.Fatalf("model = %q, want claude-opus-4-6", got)
	}
}

// TestLoadProviderBridgesMiniMax mirrors _bridge_minimax: MINIMAX_API_KEY maps onto
// ANTHROPIC_AUTH_TOKEN and the MiniMax default model is selected.
func TestLoadProviderBridgesMiniMax(t *testing.T) {
	env := map[string]string{"MINIMAX_API_KEY": "mm", "MINIMAX_BASE_URL": "https://x.example"}
	if got := LoadProviderFrom(env); got != "MiniMax-M2.7" {
		t.Fatalf("model = %q, want MiniMax-M2.7", got)
	}
	if env["ANTHROPIC_AUTH_TOKEN"] != "mm" {
		t.Fatalf("ANTHROPIC_AUTH_TOKEN = %q, want bridged mm", env["ANTHROPIC_AUTH_TOKEN"])
	}
	if env["ANTHROPIC_BASE_URL"] != "https://x.example/anthropic" {
		t.Fatalf("ANTHROPIC_BASE_URL = %q, want …/anthropic", env["ANTHROPIC_BASE_URL"])
	}
}

// TestLoadProviderHonoursPinnedModel mirrors: an explicit ANTHROPIC_MODEL wins.
func TestLoadProviderHonoursPinnedModel(t *testing.T) {
	env := map[string]string{"ANTHROPIC_AUTH_TOKEN": "tok", "ANTHROPIC_MODEL": "pinned-x"}
	if got := LoadProviderFrom(env); got != "pinned-x" {
		t.Fatalf("model = %q, want pinned-x", got)
	}
}
