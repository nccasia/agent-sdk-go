package memory

import (
	"context"
	"strconv"
	"testing"
)

func BenchmarkMemorySearch(b *testing.B) {
	ctx := context.Background()
	st := NewMemoryStoreInMemory()
	for i := 0; i < 200; i++ {
		_ = st.Write(ctx, "user", "key_"+strconv.Itoa(i), "deploy window value "+strconv.Itoa(i))
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = st.Search(ctx, "user", "deploy window", 5)
	}
}
