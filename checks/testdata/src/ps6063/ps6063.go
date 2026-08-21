package ps6063

type F32 = float32
type F64 float64
type F32Slice []F32

func exactF32(dst, src []float32) {
	for i := range dst { // want `exact float32 negation loop src\[i\] -> -src\[i\] -> dst\[i\] remains scalar; native SIMD can XOR raw lanes with 0x80000000`
		dst[i] = -src[i]
	}
}

func exactF64(dst, src []float64) {
	for i := range src { // want `exact float64 negation loop src\[i\] -> -src\[i\] -> dst\[i\] remains scalar; native SIMD can XOR raw lanes with 0x8000000000000000`
		dst[i] = -src[i]
	}
}

func exactAliases(dst, src F32Slice) {
	for i := range src { // want `exact float32 negation loop src\[i\] -> -src\[i\] -> dst\[i\] remains scalar`
		dst[i] = -src[i]
	}
}

func exactNamedFloat(dst, src []F64) {
	for i := range dst { // want `exact float64 negation loop src\[i\] -> -src\[i\] -> dst\[i\] remains scalar`
		dst[i] = -src[i]
	}
}

func exactInPlace(values []float32) {
	for i := range values { // want `exact float32 negation loop values\[i\] -> -values\[i\] -> values\[i\] remains scalar`
		values[i] = -values[i]
	}
}

func integerNegation(dst, src []int32) {
	for i := range dst {
		dst[i] = -src[i]
	}
}

func convertedInput(dst []float32, src []float64) {
	for i := range dst {
		dst[i] = -float32(src[i])
	}
}

func doubleNegation(dst, src []float32) {
	for i := range dst {
		dst[i] = -(-src[i])
	}
}

func mismatchedIndex(dst, src []float32, j int) {
	for i := range dst {
		dst[i] = -src[j]
	}
}

func compoundAssignment(dst, src []float32) {
	for i := range dst {
		dst[i] += -src[i]
	}
}

func extraStatement(dst, src []float32) {
	for i := range dst {
		dst[i] = -src[i]
		_ = i
	}
}

func unrelatedRange(dst, src, other []float32) {
	for i := range other {
		dst[i] = -src[i]
	}
}

//perfscan:measured-neg-fallback scalar tail retained after paired evidence.
func measuredFallback(dst, src []float32) {
	for i := range dst {
		dst[i] = -src[i]
	}
}

var _ = []any{exactF32, exactF64, exactAliases, exactNamedFloat, exactInPlace, integerNegation, convertedInput, doubleNegation, mismatchedIndex, compoundAssignment, extraStatement, unrelatedRange, measuredFallback}
