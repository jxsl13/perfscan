package benchmarks

import "testing"

// PS2117 — string([]byte) bound to a variable used only as map keys vs the
// inline m[string(b)] index the compiler elides the allocation for. The
// variable keys TWO lookups: with a single adjacent use current compilers
// (observed on go1.26) elide the variable form too, but with more than one
// use — or any statement between declaration and use — only the inline
// spelling stays allocation-free. Keys are longer than 32 bytes: below that
// a non-escaping conversion lands in a stack buffer either way.
var (
	ps2117Words = func() []string {
		out := make([]string, n)
		for i, w := range words {
			out[i] = w + "-0123456789abcdef0123456789abcdef0123456789abcdef"
		}
		return out
	}()
	ps2117Map = func() map[string]int {
		m := make(map[string]int, n)
		for i, w := range ps2117Words {
			m[w] = i
		}
		return m
	}()
	ps2117Keys = func() [][]byte {
		out := make([][]byte, n)
		for i, w := range ps2117Words {
			out[i] = []byte(w)
		}
		return out
	}()
)

func BenchmarkPS2117_Before(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		hits := 0
		for _, bk := range ps2117Keys {
			//lint:ignore SA6001 this is the measured Before shape of PS2117
			k := string(bk)
			if _, ok := ps2117Map[k]; ok {
				hits++
			}
			hits += ps2117Map[k]
		}
		sinkI = hits
	}
}

func BenchmarkPS2117_After(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		hits := 0
		for _, bk := range ps2117Keys {
			if _, ok := ps2117Map[string(bk)]; ok {
				hits++
			}
			hits += ps2117Map[string(bk)]
		}
		sinkI = hits
	}
}
