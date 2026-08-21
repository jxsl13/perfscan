package benchmarks

import (
	"bytes"
	"testing"
)

var (
	ps5080Input = bytes.Repeat([]byte("alpha-beta-gamma-delta-"), 2731)
	ps5080Sink  []byte
)

func BenchmarkPS5080_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5080Sink = bytes.ReplaceAll(
			bytes.Replace(
				bytes.ReplaceAll(
					bytes.Replace(
						bytes.ReplaceAll(ps5080Input, []byte("a"), []byte("a")),
						[]byte("b"), []byte("b"), -1,
					),
					[]byte("m"), []byte("m"),
				),
				[]byte("d"), []byte("d"), -1,
			),
			[]byte("-"), []byte("-"),
		)
	}
}

func BenchmarkPS5080_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5080Sink = bytes.ReplaceAll(ps5080Input, []byte("-"), []byte("-"))
	}
}
