package benchmarks

import (
	"encoding/base64"
	"strings"
	"testing"
)

var (
	ps5087Input = base64.StdEncoding.EncodeToString(ps5086Input)
	ps5087Bytes []byte
	ps5087Err   error
)

func BenchmarkPS5087_Before(b *testing.B) {
	for b.Loop() {
		ps5087Bytes, ps5087Err = base64.StdEncoding.DecodeString(strings.Clone(ps5087Input))
	}
}

func BenchmarkPS5087_After(b *testing.B) {
	for b.Loop() {
		ps5087Bytes, ps5087Err = base64.StdEncoding.DecodeString(ps5087Input)
	}
}
