package ps6061native

import "math"

func absScalar(dst, src []float32) {
	for i := range dst {
		dst[i] = float32(math.Abs(float64(src[i])))
	}
}

func absF32NEON(dst, src []float32)

var _ = absScalar
