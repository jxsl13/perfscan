package benchmarks

import (
	"fmt"
	"strings"
	"testing"
)

// PS2034 — fmt.Sprintf("host=%s;port=%s", host, port) vs
// "host=" + host + ";port=" + port. For a format of literal text spliced
// with bare %s verbs over plain strings the two are byte-identical, but
// Sprintf parses the format, boxes each argument into an interface and
// walks fmt's formatter state machine through a pp buffer, while the +
// chain folds the literal runs into compile-time constants and lowers to
// one runtime concatenation. The operands are a realistic host/port pair
// (24/5 bytes).

var (
	ps2034Host = strings.Repeat("h", 15) + ".internal"
	ps2034Port = "58080"
)

func BenchmarkPS2034_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = fmt.Sprintf("host=%s;port=%s", ps2034Host, ps2034Port)
	}
}

func BenchmarkPS2034_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = "host=" + ps2034Host + ";port=" + ps2034Port
	}
}
