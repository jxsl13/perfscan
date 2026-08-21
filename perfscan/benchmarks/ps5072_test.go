package benchmarks

import (
	"encoding/binary"
	"hash/maphash"
	"testing"
)

// PS5072 — decode Hash.Sum(nil)'s temporary little-endian bytes versus using
// Hash.Sum64 directly. The same seeded hash is read repeatedly; neither form
// mutates it.
var (
	ps5072Hash maphash.Hash
	ps5072Sink uint64
)

func init() {
	ps5072Hash.SetSeed(maphash.MakeSeed())
	_, _ = ps5072Hash.WriteString("perfscan/multi-call-autofix")
}

func BenchmarkPS5072_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5072Sink = binary.LittleEndian.Uint64(ps5072Hash.Sum(nil))
	}
}

func BenchmarkPS5072_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		ps5072Sink = ps5072Hash.Sum64()
	}
}
