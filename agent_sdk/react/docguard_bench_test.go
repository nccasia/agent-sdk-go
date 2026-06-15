package react

import "testing"

func BenchmarkDocGuardCheck(b *testing.B) {
	g := NewDocWriteGuard(DocGuardOpts{WriteTools: []string{"write_file"}, ReadonlyStages: []string{"survey"}})
	inp := map[string]any{"path": "ARCHITECTURE.md", "content": "body"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = g.Check("document", "write_file", inp)
	}
}

func BenchmarkDocGuardBashScan(b *testing.B) {
	g := NewDocWriteGuard(DocGuardOpts{BashTool: "bash", ReadonlyStages: []string{"survey"}})
	inp := map[string]any{"command": "cat > ARCHITECTURE.md << 'EOF'\n# Doc\nEOF"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = g.Check("survey", "bash", inp)
	}
}
