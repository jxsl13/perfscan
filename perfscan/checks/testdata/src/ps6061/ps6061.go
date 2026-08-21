package ps6061

import m "math"

type F32 = float32
type F64 = float64

func exact(dst, src []float32) {
	for i := range dst { // want `exact float32 Abs loop src\[i\] -> float64 -> math.Abs -> float32 -> dst\[i\] remains scalar; preserve the observable widening contract in native SIMD`
		dst[i] = float32(m.Abs(float64(src[i])))
	}
}

func exactAliases(dst, src []F32) {
	for i := range src { // want `exact float32 Abs loop src\[i\] -> float64 -> math.Abs -> float32 -> dst\[i\] remains scalar`
		dst[i] = F32(m.Abs(F64(src[i])))
	}
}

func exactInPlace(values []float32) {
	for i := range values { // want `exact float32 Abs loop values\[i\] -> float64 -> math.Abs -> float32 -> values\[i\] remains scalar`
		values[i] = float32(m.Abs(float64(values[i])))
	}
}

func noWiden(dst, src []float32) {
	for i := range dst {
		dst[i] = float32(m.Abs(float64(src[i]))) + 0
	}
}

func float64Input(dst []float32, src []float64) {
	for i := range dst {
		dst[i] = float32(m.Abs(src[i]))
	}
}

func differentMath(dst, src []float32) {
	for i := range dst {
		dst[i] = float32(m.Sqrt(float64(src[i])))
	}
}

func mismatchedIndex(dst, src []float32, j int) {
	for i := range dst {
		dst[i] = float32(m.Abs(float64(src[j])))
	}
}

func extraStatement(dst, src []float32) {
	for i := range dst {
		dst[i] = float32(m.Abs(float64(src[i])))
		_ = i
	}
}

//perfscan:measured-abs-fallback scalar tail retained after paired evidence.
func measuredFallback(dst, src []float32) {
	for i := range dst {
		dst[i] = float32(m.Abs(float64(src[i])))
	}
}

var _ = []any{exact, exactAliases, exactInPlace, noWiden, float64Input, differentMath, mismatchedIndex, extraStatement, measuredFallback}
