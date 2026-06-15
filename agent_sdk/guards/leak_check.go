package guards

// GuardrailError is raised by a guardrail post-check to block a reply.
// Mirrors agent_sdk/plugins/guardrails/errors.py:GuardrailError. It is co-located
// in the guards package in Go (the factory it backs is the answer-leak post-check).
type GuardrailError struct{ Message string }

func (e *GuardrailError) Error() string { return e.Message }

// TextCarrier is anything with answer text — the turn's AgentResult-shaped value.
type TextCarrier interface{ Text() string }

const defaultLeakMessage = "This reply was blocked by the answer-leak guard."

// AnswerLeakConfig configures MakeAnswerLeakCheck. Zero values are inert.
type AnswerLeakConfig struct {
	Forbidden         []string
	BulkPIIThreshold  int // 0 → BulkPIIThreshold (3)
	ImpossibleActions []string
	CommitmentCues    []string // nil → DefaultCommitmentCues
	NegationCues      []string // nil → DefaultNegationCues
	Message           string   // "" → defaultLeakMessage
}

// MakeAnswerLeakCheck builds a post-check that returns a *GuardrailError when an
// answer must not ship (secret-shaped string, bulk PII, forbidden substring, or
// a commitment to a declared-impossible action), else nil. Ported from
// agent_sdk/plugins/guardrails/leak_guard.py:make_answer_leak_check.
func MakeAnswerLeakCheck(cfg AnswerLeakConfig) func(TextCarrier) error {
	message := cfg.Message
	if message == "" {
		message = defaultLeakMessage
	}
	return func(res TextCarrier) error {
		text := ""
		if res != nil {
			text = res.Text()
		}
		tag := AnswerLeakViolation(text, cfg.Forbidden, cfg.BulkPIIThreshold)
		if tag == "" && len(cfg.ImpossibleActions) > 0 {
			tag = CommitmentViolation(text, cfg.ImpossibleActions, cfg.CommitmentCues, cfg.NegationCues)
		}
		if tag != "" {
			return &GuardrailError{Message: message + " [" + tag + "]"}
		}
		return nil
	}
}
