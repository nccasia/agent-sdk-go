// Package lobes is the OY (context) axis — the lobe authoring API, the per-turn
// lobe registry (the G6 extend seam), the shared lexical-cue patterns, and the
// declarative free signal/recognizer extractors. Ported from
// agent_sdk/lobes/{patterns,rows,registry,runtime}.py.
package lobes

import (
	"regexp"
	"strings"
)

// Shared lexical cue patterns (vi + en) for lobe signals and path recognition.
// Each is the case-insensitive (RE2) port of the corresponding pattern in
// agent_sdk/lobes/patterns.py. These are the B1 substrate's free, deterministic
// feature detectors.
var (
	anaphoraRE = regexp.MustCompile(`(?i)(\bnày\b|\bđó\b|\bđấy\b|\bkia\b|\bnó\b|\bvậy\b|\bấy\b|\bở trên\b` +
		`|\bthis\b|\bthat\b|\bit\b|\bthese\b|\bthose\b)`)

	reminderRE = regexp.MustCompile(`(?i)(nhắc (mình|tôi|tớ|em|anh|chị|nhở)?|đặt lịch|lên lịch|hẹn giờ|tạo (task|nhắc|lịch)` +
		`|\bremind\b|\bschedule\b|set (a |an )?(reminder|alarm|task))`)

	mutationRE = regexp.MustCompile(`(?i)(đổi (sang|lại|giờ|lịch)|đổi [^,.;!?]{0,24}sang|sửa (lại|giờ|lịch)` +
		`|hủy|huỷ|xóa|xoá|tắt (nhắc|lịch|task)` +
		`|dừng (nhắc|lịch|task)|tạm dừng|bật lại|chạy lại` +
		`|\bcancel\b|\breschedule\b|change (it|the (time|schedule|task))` +
		`|\bdelete\b|\bpause\b|\bresume\b|turn off)`)

	greetingRE = regexp.MustCompile(`(?i)^(xin )?(chào|chao|hello|hi|hey|yo|alo)\b|` +
		`(cảm ơn|cám ơn|thank(s| you)?|tạm biệt|good (morning|night)|bye)\b`)

	selfRefRE = regexp.MustCompile(`(?i)\bbạn (là (ai|gì|người (gì|nào))|tên (là )?gì|giúp (được )?(gì|được gì)` +
		`|làm (được )?gì|hỗ trợ (được )?gì|có thể (làm|giúp))` +
		`|giới thiệu (về )?(bạn|bản thân)` +
		`|who are you|what can you (do|help)|introduce yourself`)

	softCancelRE = regexp.MustCompile(`(?i)(\bthôi\b.{0,16}(bỏ|khỏi|dừng|đừng|hủy|huỷ)|khỏi cần|không cần (nữa|đâu)` +
		`|bỏ (cái )?đó đi|dẹp (cái )?đó|never ?mind|forget (it|that)|drop (it|that))`)

	interrogativeRE = regexp.MustCompile(`(?i)(là gì|là ai|khi nào|lúc nào|ở đâu|bao nhiêu|bao lâu|mấy giờ|thế nào|làm sao` +
		`|vì sao|tại sao|có (được|phải|đúng)? ?không` +
		`|\bwhat\b|\bwhen\b|\bwhere\b|\bwho\b|\bwhy\b|\bhow\b|\?)`)

	infoRequestRE = regexp.MustCompile(`(?i)(cần (biết|hiểu|rõ|hướng dẫn|thông tin|tư vấn|hỗ trợ|tìm hiểu|giải thích)` +
		`|hướng dẫn (tôi|mình|em|giúp|cách|về|sử dụng|dùng)` +
		`|chỉ (tôi|mình|em|giúp) (cách|cách dùng|cách sử dụng)` +
		`|cho (tôi|mình|em) (biết|hỏi|xin) (về|thông tin|cách|chi tiết)?` +
		`|giải thích (giúp|cho|về)?|giúp (tôi|mình|em) (hiểu|tìm hiểu|nắm|với)` +
		`|muốn (biết|hiểu|tìm hiểu|hỏi|nắm)` +
		`|tìm hiểu về|thông tin về` +
		`|\bi need (info|information|help|a guide|guidance|to understand)\b` +
		`|\bhelp me (understand|with)\b|\bguide me\b|\bexplain\b|\btell me about\b` +
		`|\bhow (do|can) i (use|do)\b)`)

	comparativeRE = regexp.MustCompile(`(?i)(so sánh|đối chiếu|khác (nhau|gì)|giống (nhau|gì)|ưu (và )?nhược|hơn hay` +
		`|\bcompare\b|\bversus\b|\bvs\.?\b|pros and cons|trade-?offs?` +
		`|phân tích|tổng hợp|liệt kê (tất cả|toàn bộ)|\banalyze\b|\bsummarize all\b)`)

	firedPromptRE = regexp.MustCompile(`(?i)\[Scheduled task execution\]`)

	cadenceRE = regexp.MustCompile(`(?i)(hằng|hàng|mỗi (sáng|trưa|chiều|tối|ngày|tuần|tháng)` +
		`|thứ [2-7]\b|thứ (hai|ba|tư|năm|sáu|bảy)|chủ nhật` +
		`|every (day|morning|evening|week|month|monday|tuesday|wednesday|thursday|friday` +
		`|saturday|sunday)|daily|weekly|monthly)`)

	clockRE = regexp.MustCompile(`(?i)(lúc \d{1,2}\s?(h|g|giờ|:)|\d{1,2}\s?(giờ|h)(\d{2})?\b|\d{1,2}:\d{2}` +
		`|at \d{1,2}(:\d{2})?\s?(am|pm)?\b)`)
)

// wordCount counts whitespace-separated tokens in the query (Python str.split()).
func wordCount(query string) int {
	return len(strings.Fields(query))
}

// isRecurringSchedule is true when the query pairs a recurring cadence with an
// explicit clock time — an unambiguous scheduling intent.
func isRecurringSchedule(query string) bool {
	return cadenceRE.MatchString(query) && clockRE.MatchString(query)
}
