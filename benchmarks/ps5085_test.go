package benchmarks

import (
	"bytes"
	"strings"
	"testing"
)

var (
	ps5085Bytes  = bytes.Repeat([]byte("perfscan-clone-write-"), 3277)
	ps5085String = string(ps5085Bytes)
	ps5085N      int
)

func BenchmarkPS5085Bytes_Before(b *testing.B) {
	var buffer bytes.Buffer
	buffer.Grow(len(ps5085Bytes))
	for b.Loop() {
		buffer.Reset()
		ps5085N, _ = buffer.Write(bytes.Clone(ps5085Bytes))
	}
}

func BenchmarkPS5085Bytes_After(b *testing.B) {
	var buffer bytes.Buffer
	buffer.Grow(len(ps5085Bytes))
	for b.Loop() {
		buffer.Reset()
		ps5085N, _ = buffer.Write(ps5085Bytes)
	}
}

func BenchmarkPS5085String_Before(b *testing.B) {
	var buffer bytes.Buffer
	buffer.Grow(len(ps5085String))
	for b.Loop() {
		buffer.Reset()
		ps5085N, _ = buffer.WriteString(strings.Clone(ps5085String))
	}
}

func BenchmarkPS5085String_After(b *testing.B) {
	var buffer bytes.Buffer
	buffer.Grow(len(ps5085String))
	for b.Loop() {
		buffer.Reset()
		ps5085N, _ = buffer.WriteString(ps5085String)
	}
}
