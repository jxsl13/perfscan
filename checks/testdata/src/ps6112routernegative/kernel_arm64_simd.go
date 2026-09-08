//go:build arm64 && ps6112simd

package ps6112routernegative

const bandRows = 30

func kernelEntry(data []float32, lo, hi int) { kernelRouter(data, lo, hi) }

func kernelRouter(data []float32, lo, hi int) {
	if false {
		tile4(data, lo)
		scalarTail(data, lo, hi)
	}
}

func fakeEntry(data []float32, lo, hi int) { fakeRouter(data, lo, hi) }

func fakeRouter(data []float32, lo, hi int) {
	tile4 := func(_ []float32, _ int) {}
	tile4(data, lo)
	scalarTail(data, lo, hi)
}

func tile4(_ []float32, _ int) {}
