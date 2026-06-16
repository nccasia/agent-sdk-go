package tasks

import (
	"regexp"
	"strings"
)

// Recognize is the task-path recognizer (matches Python
// agent_sdk.plugins.tasks.path.recognize). Pure, deterministic.
//
//	0.0  — chitchat / plain questions
//	0.9  — analytical / multi-step cues
//	1.0  — explicit fired_prompt flag
func Recognize(ctx map[string]any) float64 {
	if fired, ok := ctx["fired_prompt"].(bool); ok && fired {
		return 1.0
	}
	q, _ := ctx["query"].(string)
	if q == "" {
		return 0.0
	}
	q = strings.ToLower(strings.TrimSpace(q))
	if analyticalCue.MatchString(q) {
		return 0.9
	}
	return 0.0
}

// analyticalCue matches analytical / multi-step queries that
// suggest a task should be broken down.
var analyticalCue = regexp.MustCompile(`(?i)\b(` +
	// "compute / calculate the …"
	`compute|calculate|calculation|total|sum|count|aggregate|breakdown|analy[sz]e|analysis|` +
	// "what are the top N"
	`top\s+\d+|top\s+three|top\s+five|top\s+ten|` +
	// "how many …"
	`how\s+many\s+\w+|how\s+much\s+total|` +
	// "list the …"
	`list\s+(the\s+)?\w+|list\s+customers|list\s+orders|list\s+products|` +
	// "average …"
	`average\s+\w+|per\s+(region|customer|product|user|day|week|month|quarter)|` +
	// "revenue / profit"
	`revenue|profit|sales|` +
	// generic task-y cues
	`step[- ]by[- ]step|breakdown|decompose|task\s+list|checklist|action\s+items|todo\s+list|to-do\s+list` +
	`)\b`)
