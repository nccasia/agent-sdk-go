// RAG plugin — citation backfill / extraction / finalize tests.
// Mirrors agent_sdk/plugins/rag/tests/*.
package rag

import (
	"strings"
	"testing"

	"github.com/nccasia/agent-sdk-go/agent_sdk/contracts"
)

var testChunks = []map[string]any{
	{
		"chunk_id":   "c1",
		"source_ref": "Quy chế Đào tạo cho SE.pdf",
		"score":      0.9,
		"text":       "Điều kiện tốt nghiệp chương trình SE2019 yêu cầu hoàn thành đầy đủ các môn học bắt buộc và tích lũy đủ tín chỉ theo quy định đào tạo.",
	},
	{
		"chunk_id":   "c2",
		"source_ref": "Quy định chấm bài thi.docx",
		"score":      0.6,
		"text":       "Điểm cuối cùng của môn là điểm của đợt thi cuối cùng học viên tham gia.",
	},
}

// TestBackfillCitesParaphrasedAnswer mirrors test_backfill_cites_paraphrased_answer.
func TestBackfillCitesParaphrasedAnswer(t *testing.T) {
	answer := "Để tốt nghiệp chương trình SE2019 bạn cần hoàn thành đầy đủ các môn học bắt buộc và tích lũy đủ tín chỉ theo quy định đào tạo của FUNiX."
	out := BackfillCitations(answer, testChunks, nil)
	if len(out) == 0 {
		t.Fatalf("expected at least one citation, got 0")
	}
	if out[0].ChunkID != "c1" {
		t.Fatalf("expected first citation to be c1, got %q", out[0].ChunkID)
	}
	if out[0].SourceRef != "Quy chế Đào tạo cho SE.pdf" {
		t.Fatalf("expected source_ref=Quy chế..., got %q", out[0].SourceRef)
	}
}

// TestNoBackfillOnRefusal mirrors test_no_backfill_on_refusal.
func TestNoBackfillOnRefusal(t *testing.T) {
	refusal := "Rất tiếc, mình chưa tìm thấy thông tin này trong tài liệu FUNiX."
	if got := BackfillCitations(refusal, testChunks, nil); len(got) != 0 {
		t.Fatalf("expected no backfill on refusal, got %v", got)
	}
}

// TestNoBackfillOnChitchatNoOverlap mirrors test_no_backfill_on_chitchat_no_overlap.
func TestNoBackfillOnChitchatNoOverlap(t *testing.T) {
	chitchat := "Chào bạn! Mình là trợ lý học tập của FUNiX, rất vui được hỗ trợ bạn hôm nay. Bạn cứ thoải mái đặt câu hỏi nhé."
	if got := BackfillCitations(chitchat, testChunks, nil); len(got) != 0 {
		t.Fatalf("expected no backfill on chitchat, got %v", got)
	}
}

// TestBackfillSkipsAlreadyCited mirrors test_backfill_skips_already_cited.
func TestBackfillSkipsAlreadyCited(t *testing.T) {
	answer := "Điều kiện tốt nghiệp SE2019: hoàn thành các môn học bắt buộc và tích lũy đủ tín chỉ theo quy định đào tạo. [c1]"
	existing := CitationsFromText(answer, testChunks)
	out := BackfillCitations(answer, testChunks, existing)
	for _, c := range out {
		if c.ChunkID == "c1" {
			t.Fatalf("c1 should not be duplicated; got %v", out)
		}
	}
}

// TestBackfillCapped mirrors test_backfill_capped.
func TestBackfillCapped(t *testing.T) {
	many := []map[string]any{}
	for i := 0; i < 10; i++ {
		many = append(many, map[string]any{
			"chunk_id":   chunkID("c", i),
			"source_ref": "d",
			"score":      1.0 - float64(i)*0.01,
			"text":       "hoàn thành các môn học bắt buộc tích lũy tín chỉ quy định đào tạo",
		})
	}
	answer := "Cần hoàn thành các môn học bắt buộc và tích lũy tín chỉ theo quy định đào tạo."
	out := BackfillCitations(answer, many, nil)
	if len(out) > 6 {
		t.Fatalf("expected cap of 6, got %d", len(out))
	}
}

// TestStripLeavesOrdinaryBrackets mirrors test_strip_leaves_ordinary_brackets.
func TestStripLeavesOrdinaryBrackets(t *testing.T) {
	in := "see [1] and the year [2025]"
	if got := StripCitationMarkers(in); got != in {
		t.Fatalf("expected unchanged %q, got %q", in, got)
	}
}

// TestCitationsFromTextExtractsMarker mirrors the marker extraction
// half of the finalize hook.
func TestCitationsFromTextExtractsMarker(t *testing.T) {
	cid := "a1b2c3d4"
	answer := "Deploy is on Friday [" + cid + "]."
	cits := CitationsFromText(answer, []map[string]any{
		{"chunk_id": cid, "source_ref": "doc#1", "score": 0.9, "text": "the deploy day is friday in the release window"},
	})
	if len(cits) != 1 || cits[0].ChunkID != cid {
		t.Fatalf("expected one citation c1b2..., got %v", cits)
	}
}

// TestFinalizeExtractsMarkerAndStrips mirrors test_finalize_extracts_marker_and_strips.
func TestFinalizeExtractsMarkerAndStrips(t *testing.T) {
	cid := "a1b2c3d4"
	chunks := []map[string]any{
		{"chunk_id": cid, "source_ref": "doc#1", "score": 0.9, "text": "the deploy day is friday in the release window"},
	}
	answer, cites, refusal := FinalizeGrounding("Deploy is on Friday ["+cid+"].", nil, chunks, true, true)
	if refusal != "" {
		t.Fatalf("expected no refusal, got %q", refusal)
	}
	if len(cites) != 1 || cites[0].ChunkID != cid {
		t.Fatalf("expected one citation c1b2..., got %v", cites)
	}
	if strings.Contains(answer, "["+cid+"]") {
		t.Fatalf("expected marker stripped from answer, got %q", answer)
	}
}

// TestFinalizeGroundOrRefuseWhenNoCitations mirrors test_finalize_ground_or_refuse_when_no_citations.
func TestFinalizeGroundOrRefuseWhenNoCitations(t *testing.T) {
	_, _, refusal := FinalizeGrounding("Some ungrounded claim.", nil, nil, true, true)
	if refusal != "no_citations" {
		t.Fatalf("expected refusal=no_citations, got %q", refusal)
	}
}

// TestFinalizeNoRefusalWhenNotGrounding mirrors test_finalize_no_refusal_when_not_grounding.
func TestFinalizeNoRefusalWhenNotGrounding(t *testing.T) {
	_, _, refusal := FinalizeGrounding("hi there", nil, nil, false, true)
	if refusal != "" {
		t.Fatalf("expected no refusal, got %q", refusal)
	}
}

// TestCitationsAreContractCitations asserts the backfill returns the
// typed Citation (not a loose map) so consumers can typecheck.
func TestCitationsAreContractCitations(t *testing.T) {
	out := BackfillCitations("Cần hoàn thành các môn học bắt buộc và tích lũy tín chỉ theo quy định đào tạo.", testChunks, nil)
	for _, c := range out {
		var _ contracts.Citation = c
	}
}

func chunkID(prefix string, i int) string {
	return prefix + itoa(i)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	out := ""
	for i > 0 {
		out = string(rune('0'+i%10)) + out
		i /= 10
	}
	return out
}
