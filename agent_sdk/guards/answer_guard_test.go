package guards

import "testing"

func TestSecretShapedStringBlocked(t *testing.T) {
	if got := SecretViolation("here it is: sk-ABCDEF0123456789ghij"); got != "secret_shaped_string" {
		t.Fatalf("SecretViolation = %q", got)
	}
	if got := AnswerLeakViolation("token = aB3xZ9kLmN0pQ7rS2tU5", nil, 0); got != "secret_shaped_string" {
		t.Fatalf("AnswerLeakViolation = %q", got)
	}
}

func TestCleanProseIsAllowed(t *testing.T) {
	if got := AnswerLeakViolation("The capital of France is Paris.", nil, 0); got != "" {
		t.Fatalf("AnswerLeakViolation = %q", got)
	}
	if got := SecretViolation("a perfectly normal sentence"); got != "" {
		t.Fatalf("SecretViolation = %q", got)
	}
}

func TestBulkPIIEmailsBlocked(t *testing.T) {
	dump := "a@x.com b@x.com c@x.com"
	if got := AnswerLeakViolation(dump, nil, 0); got != "bulk_pii_emails" {
		t.Fatalf("AnswerLeakViolation = %q", got)
	}
	if got := AnswerLeakViolation("reach me at a@x.com", nil, 0); got != "" {
		t.Fatalf("under-threshold should be allowed, got %q", got)
	}
}

func TestForbiddenSubstringBlocked(t *testing.T) {
	if got := AnswerLeakViolation("this is internal-only data", []string{"internal-only"}, 0); got != "forbidden:internal-only" {
		t.Fatalf("AnswerLeakViolation = %q", got)
	}
}

func TestCommitmentToImpossibleAction(t *testing.T) {
	if CommitmentViolation("Sure, I will delete the account now", []string{"delete the account"}, nil, nil) == "" {
		t.Fatal("committed action should be a violation")
	}
	if CommitmentViolation("I cannot delete the account", []string{"delete the account"}, nil, nil) != "" {
		t.Fatal("negated mention should be safe")
	}
	if CommitmentViolation("I will delete the account", nil, nil, nil) != "" {
		t.Fatal("no declared actions should be a no-op")
	}
}

func TestInjectableCuesOtherLanguage(t *testing.T) {
	text := "rồi, mình sẽ xoá tài khoản"
	actions := []string{"xoá tài khoản"}
	if CommitmentViolation(text, actions, nil, nil) != "" {
		t.Fatal("English cues should miss a non-English commitment")
	}
	if CommitmentViolation(text, actions, []string{"mình sẽ"}, nil) == "" {
		t.Fatal("injected cues should catch it")
	}
}

func TestHasRefusalMarker(t *testing.T) {
	if !HasRefusalMarker("Sorry, I can't help with that", nil) {
		t.Fatal("expected refusal marker")
	}
	if HasRefusalMarker("Sure, here you go", nil) {
		t.Fatal("did not expect refusal marker")
	}
	if !HasRefusalMarker("mình không thể", []string{"không thể"}) {
		t.Fatal("expected injected refusal marker")
	}
}

func TestPostCheckFactoryRaisesOnLeak(t *testing.T) {
	check := MakeAnswerLeakCheck(AnswerLeakConfig{Forbidden: []string{"internal-only"}})
	if err := check(leakResult{text: "all good"}); err != nil {
		t.Fatalf("clean text should not raise: %v", err)
	}
	if err := check(leakResult{text: "this is internal-only"}); err == nil {
		t.Fatal("expected GuardrailError on leak")
	} else if _, ok := err.(*GuardrailError); !ok {
		t.Fatalf("expected *GuardrailError, got %T", err)
	}
}

func TestPostCheckFactoryBlocksImpossibleCommitment(t *testing.T) {
	check := MakeAnswerLeakCheck(AnswerLeakConfig{ImpossibleActions: []string{"change the grade"}})
	if err := check(leakResult{text: "Okay, I will change the grade for you"}); err == nil {
		t.Fatal("expected GuardrailError on impossible commitment")
	}
	if err := check(leakResult{text: "I cannot change the grade"}); err != nil {
		t.Fatalf("negated commitment should be allowed: %v", err)
	}
}

// leakResult is a minimal result carrying answer text (mirrors the test's _Result).
type leakResult struct{ text string }

func (r leakResult) Text() string { return r.text }
