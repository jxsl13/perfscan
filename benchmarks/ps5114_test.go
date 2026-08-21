package benchmarks

import (
	"strings"
	"testing"
)

// The filepath producer result is precomputed so the benchmark isolates the
// work PS5114 deletes: Go 1.26's exact filepathlite FromSlash replacement
// algorithm scanning an already-native Windows path and finding no '/'.
var (
	ps5114NativeProducerResult = `C:\\` + strings.Repeat(`root\\folder\\mixed\\leaf\\`, 4096) + `leaf.ext`
	ps5114Sink                 string
)

func BenchmarkPS5114_Before(b *testing.B) {
	for b.Loop() {
		ps5114Sink = ps5113WindowsFromSlash(ps5114NativeProducerResult)
	}
}

func BenchmarkPS5114_After(b *testing.B) {
	for b.Loop() {
		ps5114Sink = ps5114NativeProducerResult
	}
}
