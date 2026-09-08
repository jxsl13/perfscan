//go:build arm64 && ps6112simd

package ps6112aligned

const bandRows = 32

func kernelEntry(data []float32, lo, hi int) {
	kernelRouter(data, lo, hi)
}

func kernelRouter(data []float32, lo, hi int) {
	i := lo
	for ; i+3 < hi; i += 4 {
		tile4(data, i)
	}
	if i < hi {
		scalarTail(data, i, hi)
	}
}

func tile4(_ []float32, _ int) {}
