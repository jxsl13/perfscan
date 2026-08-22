package benchmarks

import (
	"bufio"
	"io"
	"strings"
	"testing"
)

var (
	ps5097Buffered           = bufio.NewReader(strings.NewReader("already buffered"))
	ps5097Input    io.Reader = ps5097Buffered
	ps5097Sink     *bufio.Reader
)

func BenchmarkPS5097_Before(b *testing.B) {
	for b.Loop() {
		ps5097Sink = bufio.NewReader(bufio.NewReader(bufio.NewReader(ps5097Input)))
	}
}

func BenchmarkPS5097_After(b *testing.B) {
	for b.Loop() {
		ps5097Sink = bufio.NewReader(ps5097Input)
	}
}
