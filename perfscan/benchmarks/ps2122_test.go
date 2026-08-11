package benchmarks

import (
	"fmt"
	"strings"
	"testing"
)

// PS2122 — fmt.Sprintf("%s%s%s", a, b, c) vs a + b + c. For a format of
// only %s verbs over plain strings the two are byte-identical, but
// Sprintf boxes each argument into an interface and walks fmt's
// formatter state machine through a pp buffer while + is one direct
// compiler-emitted concatenation. The operands are a realistic
// host + ":" + port join (24/1/5 bytes).

var (
	ps2122Host = strings.Repeat("h", 15) + ".internal"
	ps2122Sep  = ":"
	ps2122Port = "58080"
)

func BenchmarkPS2122_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = fmt.Sprintf("%s%s%s", ps2122Host, ps2122Sep, ps2122Port)
	}
}

func BenchmarkPS2122_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = ps2122Host + ps2122Sep + ps2122Port
	}
}
