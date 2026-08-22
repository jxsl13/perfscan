package benchmarks

import (
	"strings"
	"testing"
)

var (
	ps5119Prefix = strings.Repeat("0123456789abcdef", 6*1024) // exactly 96 KiB
	ps5119Value  = ps5119Prefix + "/payload"
	ps5119Sink   string
)

func BenchmarkPS5119Before(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		value := ps5119Value
		//lint:ignore S1017 This benchmark intentionally preserves PS5119's before shape.
		if strings.HasPrefix(value, ps5119Prefix) {
			value = strings.TrimPrefix(value, ps5119Prefix)
		}
		ps5119Sink = value
	}
}

func BenchmarkPS5119After(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		value := ps5119Value
		if after, found := strings.CutPrefix(value, ps5119Prefix); found {
			value = after
		}
		ps5119Sink = value
	}
}
