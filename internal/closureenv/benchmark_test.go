package closureenv

import "testing"

var benchmarkClosure func()

//go:noinline
func beforeClosure(a0, a1, a2, a3, a4, a5, a6, a7, a8 []byte) func() {
	return func() { _, _, _, _, _, _, _, _, _ = a0, a1, a2, a3, a4, a5, a6, a7, a8 }
}

//go:noinline
func afterClosure(a0, a1, a2, a3, a4, a5, a6, a7, a8 []byte, width byte) func() {
	return func() { _, _, _, _, _, _, _, _, _, _ = a0, a1, a2, a3, a4, a5, a6, a7, a8, width }
}

func BenchmarkClosureEnvironmentClass(b *testing.B) {
	var values [9][]byte
	b.Run("before", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			benchmarkClosure = beforeClosure(values[0], values[1], values[2], values[3], values[4], values[5], values[6], values[7], values[8])
		}
	})
	b.Run("after", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			benchmarkClosure = afterClosure(values[0], values[1], values[2], values[3], values[4], values[5], values[6], values[7], values[8], 1)
		}
	})
}
