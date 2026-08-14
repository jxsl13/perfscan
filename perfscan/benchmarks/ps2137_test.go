package benchmarks

import (
	"fmt"
	"strconv"
	"testing"
)

// PS2137 — fmt.Sprint(n) vs strconv.Itoa(n) for a plain int. The Sprint
// side boxes n into an interface (one heap allocation) and dispatches
// through fmt's reflection-driven printer before allocating the result
// string; strconv.Itoa emits the decimal digits directly with just the
// result-string allocation. Output is bit-identical (%v/Sprint of an
// unnamed integer prints exactly the base-10 digits strconv produces —
// an unnamed type cannot be a Stringer, so no method dispatch differs).
var ps2137N = 123456

func BenchmarkPS2137_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = fmt.Sprint(ps2137N)
	}
}

func BenchmarkPS2137_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkS = strconv.Itoa(ps2137N)
	}
}
