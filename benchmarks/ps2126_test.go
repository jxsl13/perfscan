package benchmarks

import (
	"errors"
	"fmt"
	"testing"
)

// PS2126 — fmt.Errorf("constant") vs errors.New("constant"). With no verbs
// fmt.Errorf returns errors.New(format) verbatim, so the two build the
// identical *errorString — but Errorf first spins up a printer, walks the
// format through doPrintf and assembles the message in a pooled pp buffer.
// For a constant message that whole formatting path is pure overhead.

var sinkErr error

func BenchmarkPS2126_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkErr = fmt.Errorf("database connection failed")
	}
}

func BenchmarkPS2126_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkErr = errors.New("database connection failed")
	}
}
