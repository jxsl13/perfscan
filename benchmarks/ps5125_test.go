package benchmarks

import (
	"strings"
	"testing"
)

var (
	ps5125Value = strings.Repeat("field", 4096) + ":" + strings.Repeat("tail", 32)
	ps5125Sink  string
)

func BenchmarkPS5125Before(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		value := ps5125Value
		//lint:ignore S1017 benchmark control intentionally retains the redundant guard
		if strings.Contains(value, ":") {
			value = strings.Replace(value, ":", "-", 1)
		}
		ps5125Sink = value
	}
}

func BenchmarkPS5125After(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		ps5125Sink = strings.Replace(ps5125Value, ":", "-", 1)
	}
}
