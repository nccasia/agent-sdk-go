package guards

import "regexp"

// Secret/PII redaction patterns, ported from agent_sdk/guards/safety.py. The
// pydantic ChatTurnPayload model is host-wire validation and is intentionally
// not ported — the leaf-level value is RedactText / SanitizeForEvent.
var (
	secretPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(authorization|bearer|api[_-]?key|token|password|secret)\b\s*[:=]\s*[^\s,;]+`),
		regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/=-]{12,}`),
		regexp.MustCompile(`\b[A-Za-z0-9_-]{24,}\.[A-Za-z0-9_-]{12,}\.[A-Za-z0-9_-]{12,}\b`),
		regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}\b`),
	}
	emailRedactRe = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	phoneRedactRe = regexp.MustCompile(`\b(?:\+?\d[\d .-]{7,}\d)\b`)
	secretKeyRe   = regexp.MustCompile(`(?i)(authorization|api[_-]?key|token|password|secret)`)
)

// RedactText scrubs secret-shaped strings, emails, and phones from a value and
// truncates to maxChars (pass 0 for the 500-char default).
func RedactText(value string, maxChars int) string {
	if maxChars <= 0 {
		maxChars = 500
	}
	text := value
	for _, pattern := range secretPatterns {
		text = pattern.ReplaceAllStringFunc(text, func(m string) string {
			sub := pattern.FindStringSubmatch(m)
			label := "secret"
			if len(sub) > 1 && sub[1] != "" {
				label = sub[1]
			}
			return label + "=[REDACTED]"
		})
	}
	text = emailRedactRe.ReplaceAllString(text, "[REDACTED_EMAIL]")
	text = phoneRedactRe.ReplaceAllString(text, "[REDACTED_PHONE]")
	if len(text) > maxChars {
		return text[:maxChars-1] + "..."
	}
	return text
}

// SanitizeForEvent recursively redacts strings, bytes, maps, and slices, keying
// off secret-looking dict keys. depth caps recursion at 4 (pass 0 to start).
func SanitizeForEvent(value any, maxChars, depth int) any {
	if maxChars <= 0 {
		maxChars = 500
	}
	if depth > 4 {
		return "[TRUNCATED]"
	}
	switch v := value.(type) {
	case string:
		return RedactText(v, maxChars)
	case []byte:
		return RedactText(string(v), maxChars)
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			if secretKeyRe.MatchString(key) {
				out[key] = "[REDACTED]"
			} else {
				out[key] = SanitizeForEvent(item, maxChars, depth+1)
			}
		}
		return out
	case []any:
		n := len(v)
		if n > 20 {
			n = 20
		}
		out := make([]any, 0, n)
		for _, item := range v[:n] {
			out = append(out, SanitizeForEvent(item, maxChars, depth+1))
		}
		return out
	default:
		return value
	}
}
