package search

import (
	"fmt"
	"testing"
)

func BenchmarkSearchEngine_Index(b *testing.B) {
	engine := NewEngine()
	doc := Document{
		ID:    "bench-1",
		Type:  "articles",
		Title: "Enterprise Go Microservices and Concurrent Caching Systems",
		Body:  "Go delivers native lightweight goroutines, channels, and low-latency garbage collection for high-throughput distributed systems.",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doc.ID = fmt.Sprintf("bench-%d", i)
		engine.Index(doc)
	}
}

func BenchmarkSearchEngine_Search(b *testing.B) {
	engine := NewEngine()
	// Pre-populate with 100 documents
	for i := 0; i < 100; i++ {
		engine.Index(Document{
			ID:    fmt.Sprintf("doc-%d", i),
			Type:  "articles",
			Title: fmt.Sprintf("High Performance Computing Guide Volume %d", i),
			Body:  "Comprehensive overview of memory architectures, distributed systems, cache coherence, and parallel algorithm optimization.",
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = engine.Search("distributed memory cache", "articles", 10)
	}
}
