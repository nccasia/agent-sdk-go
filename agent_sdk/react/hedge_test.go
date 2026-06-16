// Translated from tests/test_hedge_retry.py — the anti-hedge answer-retry
// builder (react/hedge.py). The hedge builder returns a forced-answer
// directive when the assistant's reply opens with a hedge marker (the first
// 160 chars are checked, NFC + lower). A direct answer / empty answer
// returns nil (no retry). Custom markers and directive pass through.
package react

import "testing"

// TestHedgeOpeningTriggersDirective mirrors test_hedge_opening_triggers_directive.
func TestHedgeOpeningTriggersDirective(t *testing.T) {
	retry := MakeHedgeRetry()
	if got := retry("Sorry, I couldn't find specifics on that."); got == nil || *got != DefaultHedgeDirective {
		t.Fatalf("got %v, want %q", got, DefaultHedgeDirective)
	}
	if got := retry("Unfortunately I don't have that information."); got == nil || *got != DefaultHedgeDirective {
		t.Fatalf("got %v, want %q", got, DefaultHedgeDirective)
	}
}

// TestDirectAnswerDoesNotRetry mirrors test_direct_answer_does_not_retry.
func TestDirectAnswerDoesNotRetry(t *testing.T) {
	retry := MakeHedgeRetry()
	if got := retry("The deadline is March 1, per [c12]."); got != nil {
		t.Fatalf("direct answer must not retry, got %q", *got)
	}
	if got := retry(""); got != nil {
		t.Fatalf("empty answer must not retry, got %q", *got)
	}
}

// TestOnlyChecksTheOpening mirrors test_only_checks_the_opening. A hedge
// phrase past 160 chars does not trigger the directive.
func TestOnlyChecksTheOpening(t *testing.T) {
	retry := MakeHedgeRetry()
	body := ""
	for i := 0; i < 20; i++ {
		body += "The policy states X. "
	}
	body += "sorry"
	if got := retry(body); got != nil {
		t.Fatalf("body-only hedge must not retry, got %q", *got)
	}
}

// TestCustomMarkersAndDirective mirrors test_custom_markers_and_directive.
func TestCustomMarkersAndDirective(t *testing.T) {
	retry := MakeHedgeRetry(
		WithHedgeMarkers("rất tiếc", "chưa tìm thấy"),
		WithHedgeDirective("TRA_LOI_TRUC_TIEP"),
	)
	if got := retry("Rất tiếc, mình chưa tìm thấy thông tin."); got == nil || *got != "TRA_LOI_TRUC_TIEP" {
		t.Fatalf("custom marker must trigger, got %v", got)
	}
	if got := retry("Sorry, no info"); got != nil {
		t.Fatalf("English default marker is excluded from custom set, got %q", *got)
	}
}
