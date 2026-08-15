package benchmarks

import (
	"errors"
	"fmt"
	"testing"
)

// PS2023 — fmt.Errorf("%s", msg) vs errors.New(msg) for msg a plain
// runtime string. With no %w verb fmt.Errorf returns errors.New of the
// formatted string, and "%s" of a string operand formats to the operand
// verbatim — so the Before computes exactly the After, after first
// boxing msg into an interface (one extra allocation), parsing the
// format, walking doPrintf and copying the pooled printer's buffer into
// a fresh string.

var ps2023Sink error

// ps2023Msg is a package-level var so the compiler cannot constant-fold
// the message; 46 bytes, a typical error message length.
var ps2023Msg = "connection refused by upstream after 3 retries"

func BenchmarkPS2023_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2023Sink = fmt.Errorf("%s", ps2023Msg)
	}
}

func BenchmarkPS2023_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps2023Sink = errors.New(ps2023Msg)
	}
}
