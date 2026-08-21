package ps6024

import "testing"

func coefficient(src []byte, block int) float32 { return float32(src[block]) }

func decodeWithOutputTailScratch(src []byte, blocks, blockSize int) []float32 {
	out := make([]float32, blocks*blockSize)
	for block := 0; block < blocks; block++ {
		out[len(out)-blocks+block] = coefficient(src, block) // want "freshly allocated destination out is partially/strided pre-touched as scratch in one pass and then fully overwritten in a later pass"
	}
	for i := range out {
		out[i] = float32(src[i%len(src)]) + out[len(out)-blocks+i/blockSize]
	}
	return out
}

func decodeWithStridedScratch(src []byte, blocks, blockSize int) []float32 {
	out := make([]float32, blocks*blockSize)
	for block := 0; block < blocks; block++ {
		out[block*blockSize] = coefficient(src, block) // want "freshly allocated destination out is partially/strided pre-touched as scratch in one pass and then fully overwritten in a later pass"
	}
	for i := 0; i < len(out); i++ {
		out[i] = float32(src[i%len(src)])
	}
	return out
}

// A separate scratch buffer does not pre-touch the output.
func decodeSeparateScratch(src []byte, blocks, blockSize int) []float32 {
	out := make([]float32, blocks*blockSize)
	scratch := make([]float32, blocks)
	for block := range blocks {
		scratch[block] = coefficient(src, block)
	}
	for i := range out {
		out[i] = float32(src[i%len(src)]) + scratch[i/blockSize]
	}
	return out
}

// The later pass is conditional, so a full overwrite is not proven.
func decodeConditional(src []byte, blocks, blockSize int) []float32 {
	out := make([]float32, blocks*blockSize)
	for block := range blocks {
		out[block*blockSize] = coefficient(src, block)
	}
	for i := range out {
		if src[i%len(src)] != 0 {
			out[i] = float32(src[i%len(src)])
		}
	}
	return out
}

// Full initialization happens first, so there is no scratch prepass.
func decodeReverseOrder(src []byte, blocks, blockSize int) []float32 {
	out := make([]float32, blocks*blockSize)
	for i := range out {
		out[i] = float32(src[i%len(src)])
	}
	for block := range blocks {
		out[block*blockSize] = coefficient(src, block)
	}
	return out
}

func decodeInto(dst []float32, src []byte) {
	for i := range dst {
		dst[i] = float32(src[i%len(src)])
	}
}

func BenchmarkDecodeScratchWarm(b *testing.B) {
	size := 1 << 20
	dst := make([]float32, size) // want "benchmark reuses runtime-sized destination dst across b.N for a bulk decode/transform path but has no fresh-allocation/demand-zero comparison cell"
	src := make([]byte, size)
	b.ResetTimer()
	for range b.N {
		decodeInto(dst, src)
	}
}

func BenchmarkDecodeScratchFreshAllocation(b *testing.B) {
	// Fresh allocation arm; the destination is allocated per iteration.
	size := 1 << 20
	src := make([]byte, size)
	b.ResetTimer()
	for range b.N {
		dst := make([]float32, size)
		decodeInto(dst, src)
	}
}
