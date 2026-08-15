package benchmarks

import (
	"errors"
	"fmt"
	"testing"
)

// PS2031 — fmt.Errorf("%s", s) vs errors.New(s) on a plain string.
// With no %w verb fmt.Errorf returns errors.New(formatted), and a bare
// %s/%v copies a string operand verbatim, so the two build the
// identical *errorString — but the Errorf spelling first gets a pooled
// printer, scans the format, boxes s into an any (one allocation) and
// materializes a throwaway copy of s via string(p.buf) (a second
// allocation) before paying the errorString allocation both sides
// share. The message is a package-level var so the boxing is not
// constant-folded away.

var ps2031Msg = "database connection failed"

var sinkErrPS2031 error

func BenchmarkPS2031_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkErrPS2031 = fmt.Errorf("%s", ps2031Msg)
	}
}

func BenchmarkPS2031_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkErrPS2031 = errors.New(ps2031Msg)
	}
}
