package benchmarks

import (
	"fmt"
	"io"
	"testing"
)

// PS2049 — fmt.Fprintln(w, host, port) over plain-string operands vs
// io.WriteString(w, host+" "+port+"\n"). The Fprintln side takes a pp
// printer from fmt's sync.Pool, boxes EVERY operand into an interface
// (one heap allocation per string header), walks doPrintln's
// per-operand default formatter through the pooled buffer and performs
// one w.Write; the rewrite forms the joined string in a single
// runtime.concatstrings allocation (the constant " " and "\n" fold into
// adjacent literals at compile time) and hands it straight to the
// writer. Output and (n, err) are identical — doPrintln writes exactly
// one space between adjacent operands and one trailing newline, and %v
// of a plain string is the verbatim bytes.
var (
	ps2049Host = "control-plane.internal" // 22 bytes
	ps2049Port = "8443"                   // 4 bytes
)

func BenchmarkPS2049_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		n, _ := fmt.Fprintln(io.Discard, ps2049Host, ps2049Port)
		sinkI = n
	}
}

func BenchmarkPS2049_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		n, _ := io.WriteString(io.Discard, ps2049Host+" "+ps2049Port+"\n")
		sinkI = n
	}
}
