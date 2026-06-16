// Benchmarks — micro-benchmarks for the citation extraction /
// backfill / finalize hot path. A agent turn runs the finalize
// hook once per turn; the per-call cost should be well under 1ms
// for the typical (≤10 chunk) corpus.
package rag

import (
	"testing"
)

// benchChunks is a representative grounded corpus.
var benchChunks = []map[string]any{
	{"chunk_id": "c1", "source_ref": "doc#1", "score": 0.9, "text": "the deploy day is friday in the release window"},
	{"chunk_id": "c2", "source_ref": "doc#2", "score": 0.8, "text": "the deploy window is between 9am and noon pacific"},
	{"chunk_id": "c3", "source_ref": "doc#3", "score": 0.7, "text": "rollback procedure requires signoff from two on-call engineers"},
	{"chunk_id": "c4", "source_ref": "doc#4", "score": 0.6, "text": "the change-management policy lists every step"},
	{"chunk_id": "c5", "source_ref": "doc#5", "score": 0.5, "text": "the on-call rotation is published weekly"},
	{"chunk_id": "c6", "source_ref": "doc#6", "score": 0.4, "text": "deploys are blocked during the freeze window"},
}

// BenchmarkFinalizeGrounding exercises the full finalize hook
// (extract + backfill + strip + ground-or-refuse).
func BenchmarkFinalizeGrounding(b *testing.B) {
	answer := "Deploy is on Friday [a1b2c3d4] in the release window, and the rollback procedure [e5f6a7b8] requires signoff."
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, _ = FinalizeGrounding(answer, nil, benchChunks, true, true)
	}
}

// BenchmarkBackfillCitations exercises the backfill ranker.
func BenchmarkBackfillCitations(b *testing.B) {
	answer := "Deploy is on Friday in the release window; rollback requires signoff from two on-call engineers; the on-call rotation is published weekly."
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = BackfillCitations(answer, benchChunks, nil)
	}
}

// BenchmarkCitationsFromText exercises the marker extraction.
func BenchmarkCitationsFromText(b *testing.B) {
	answer := "Deploy is on Friday [a1b2c3d4] in the release window [e5f6a7b8]."
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = CitationsFromText(answer, benchChunks)
	}
}

// BenchmarkStripCitationMarkers exercises the conservative strip
// regex (the user-facing text rewrite).
func BenchmarkStripCitationMarkers(b *testing.B) {
	answer := "Deploy is on Friday [a1b2c3d4] in the release window [e5f6a7b8]."
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = StripCitationMarkers(answer)
	}
}
