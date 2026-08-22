package benchmarks

import (
	"fmt"
	"strings"
	"testing"
)

// PS5029 — fmt.Sprintln(host, port) vs host + " " + port + "\n". When
// every operand is a plain string, Sprintln's unconditional rule (one
// space between adjacent operands, one trailing newline, no format
// interpretation) makes the two byte-identical, but Sprintln boxes each
// operand into an interface and walks fmt's default formatter through a
// pooled pp buffer while + is one direct compiler-emitted concatenation.
// The operands are a realistic host + port log join (24/5 bytes).

var (
	ps5029Host = strings.Repeat("h", 15) + ".internal"
	ps5029Port = "58080"
)

func BenchmarkPS5029_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = fmt.Sprintln(ps5029Host, ps5029Port)
	}
}

func BenchmarkPS5029_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = ps5029Host + " " + ps5029Port + "\n"
	}
}
