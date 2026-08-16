package benchmarks

import (
	"strconv"
	"testing"
)

// PS3033 — the guarded map delete. `if _, ok := m[k]; ok { delete(m, k) }`
// hashes k twice on the present-key path: the comma-ok guard probes the map
// just to decide whether to call delete, then delete probes it again. The
// builtin delete of an absent key is a no-op, so the unconditional
// `delete(m, k)` is bit-identical and hashes once. The map is rebuilt each
// iteration (both sides identically) so every delete hits a present key —
// the path where the guard's second probe costs.

var ps3033Keys = func() []string {
	k := make([]string, 1024)
	for i := range k {
		k[i] = strconv.Itoa(i)
	}
	return k
}()

var sinkPS3033 int

func ps3033Fill(m map[string]int) {
	for i, k := range ps3033Keys {
		m[k] = i
	}
}

func BenchmarkPS3033_Before(b *testing.B) {
	b.ReportAllocs()
	m := make(map[string]int, len(ps3033Keys))
	for range b.N {
		ps3033Fill(m)
		for _, k := range ps3033Keys {
			//lint:ignore S1033 deliberately the guarded "before" form under measurement
			if _, ok := m[k]; ok {
				delete(m, k)
			}
		}
		sinkPS3033 = len(m)
	}
}

func BenchmarkPS3033_After(b *testing.B) {
	b.ReportAllocs()
	m := make(map[string]int, len(ps3033Keys))
	for range b.N {
		ps3033Fill(m)
		for _, k := range ps3033Keys {
			delete(m, k)
		}
		sinkPS3033 = len(m)
	}
}
