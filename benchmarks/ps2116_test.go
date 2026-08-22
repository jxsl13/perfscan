package benchmarks

import "testing"

// PS2116 — a slice zeroed element-by-element vs clear(s).
//
// PARITY is the expected result: gc recognizes the exact canonical loop
// and lowers it to runtime memclr, which is what clear compiles to as
// well. The check's value is directness and robustness — any later edit
// to the loop body silently drops the memclr lowering, clear cannot.
var ps2116Buf = make([]byte, 4096)

func BenchmarkPS2116_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		for i := range ps2116Buf {
			ps2116Buf[i] = 0
		}
	}
	sinkI = int(ps2116Buf[0])
}

func BenchmarkPS2116_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		clear(ps2116Buf)
	}
	sinkI = int(ps2116Buf[0])
}
