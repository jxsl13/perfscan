package benchmarks

import (
	"bytes"
	"encoding/json"
	"net/http"
	"slices"
	"testing"
)

var (
	ps5081Left  = bytes.Repeat([]byte("clone-observer-payload-"), 2849)
	ps5081Right = bytes.Clone(ps5081Left)
	ps5081Sink  bool
	ps5081Type  string
	ps5081JSON  = append(append([]byte(`{"payload":"`), bytes.Repeat([]byte("x"), 65_500)...), []byte(`"}`)...)
)

func BenchmarkPS5081_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5081Sink = bytes.Equal(
			bytes.Clone(slices.Clone(bytes.Clone(ps5081Left))),
			slices.Clone(ps5081Right),
		)
	}
}

func BenchmarkPS5081_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5081Sink = bytes.Equal(ps5081Left, ps5081Right)
	}
}

func BenchmarkPS5081ContentType_Before(b *testing.B) {
	for b.Loop() {
		ps5081Type = http.DetectContentType(bytes.Clone(ps5081Left))
	}
}

func BenchmarkPS5081ContentType_After(b *testing.B) {
	for b.Loop() {
		ps5081Type = http.DetectContentType(ps5081Left)
	}
}

func BenchmarkPS5081JSONValid_Before(b *testing.B) {
	for b.Loop() {
		ps5081Sink = json.Valid(bytes.Clone(ps5081JSON))
	}
}

func BenchmarkPS5081JSONValid_After(b *testing.B) {
	for b.Loop() {
		ps5081Sink = json.Valid(ps5081JSON)
	}
}
