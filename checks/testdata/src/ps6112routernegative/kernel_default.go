//go:build !arm64 || !ps6112simd

package ps6112routernegative

const bandRows = 30

func kernelEntry(data []float32, lo, hi int) {
	i := lo
	for ; i+5 < hi; i += 6 {
		tile6(data, i)
	}
	if i < hi {
		scalarTail(data, i, hi)
	}
}

func fakeEntry(data []float32, lo, hi int) { fakeRouter(data, lo, hi) }

func fakeRouter(data []float32, lo, hi int) {
	tile6(data, lo)
	scalarTail(data, lo, hi)
}

func tile6(_ []float32, _ int) {}
