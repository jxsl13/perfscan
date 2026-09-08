//go:build !arm64 || !ps6112simd

package ps6112

const bandRows = 30
const typedBandRows int = 30
const narrowBandRows uint8 = 30

func kernelEntry(data []float32, lo, hi int) {
	i := lo
	for ; i+5 < hi; i += 6 {
		tile6(data, i)
	}
	if i < hi {
		scalarTail(data, i, hi)
	}
}

func tile6(_ []float32, _ int) {}
