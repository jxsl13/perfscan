package benchmarks

import (
	"regexp"
	"testing"
)

// PS2127 — a regexp.MustCompile of a constant pattern used inline in a function
// body recompiles the matcher on EVERY call (parse + program build, several
// allocations). Hoisting it to a package-level var compiles it once at init and
// reuses the shared matcher. Before pays the compile per call; After does not.

const ps2127Pattern = `^word-[0-9]+$`

var ps2127Re = regexp.MustCompile(ps2127Pattern)

func BenchmarkPS2127_Before(b *testing.B) {
	b.ReportAllocs()
	hits := 0
	for range b.N {
		if regexp.MustCompile(ps2127Pattern).MatchString("word-1234") {
			hits++
		}
	}
	sinkI = hits
}

func BenchmarkPS2127_After(b *testing.B) {
	b.ReportAllocs()
	hits := 0
	for range b.N {
		if ps2127Re.MatchString("word-1234") {
			hits++
		}
	}
	sinkI = hits
}
