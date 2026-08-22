package benchmarks

import (
	"bytes"
	"os"
	"testing"
)

var (
	ps5089N   int
	ps5089Err error
)

func BenchmarkPS5089_Before(b *testing.B) {
	file, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = file.Close() })
	for b.Loop() {
		ps5089N, ps5089Err = file.Write(bytes.Clone(ps5086Input))
	}
}

func BenchmarkPS5089_After(b *testing.B) {
	file, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = file.Close() })
	for b.Loop() {
		ps5089N, ps5089Err = file.Write(ps5086Input)
	}
}
