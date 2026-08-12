package benchmarks

import (
	"errors"
	"fmt"
	"testing"
)

// PS2126 — fmt.Errorf on a constant verb-free message vs errors.New.
// fmt.Errorf without %w is errors.New(fmt.Sprintf(format)); for a
// constant string the Sprintf pass is a byte-for-byte copy through the
// borrowed printer state plus a rune-by-rune verb scan that finds
// nothing. errors.New skips all of it and both spellings return the
// identical *errors.errorString. The Before arm pays the printer's
// intermediate string allocation on top of the errorString itself.

var sinkErr error

func BenchmarkPS2126_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkErr = fmt.Errorf("some fixed message")
	}
}

func BenchmarkPS2126_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		sinkErr = errors.New("some fixed message")
	}
}
