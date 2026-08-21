package ps6060

func exactF32(dst, src []float32) {
	for i := range dst { // want `exact float32 ReLU loop src\[i\] > 0 -> dst\[i\] remains scalar; the bit-identical native form is ordered compare against \+0 followed by bit select, not FMAX/math.Max \(NaNs and -0 differ\)`
		if src[i] > 0 {
			dst[i] = src[i]
		}
	}
}

func reversedConditionF64(dst, src []float64) {
	for i := range src { // want `exact float64 ReLU loop src\[i\] > 0 -> dst\[i\] remains scalar`
		if 0 < src[i] {
			dst[i] = src[i]
		}
	}
}

func ifInitTemporary(dst, src []float32) {
	for i := range dst { // want `exact float32 ReLU loop src\[i\] > 0 -> dst\[i\] remains scalar`
		if value := src[i]; value > 0 {
			dst[i] = value
		}
	}
}

// >= retains -0 instead of leaving +0 and is not the exact contract.
func greaterEqual(dst, src []float32) {
	for i := range dst {
		if src[i] >= 0 {
			dst[i] = src[i]
		}
	}
}

func nonzeroThreshold(dst, src []float32) {
	for i := range dst {
		if src[i] > 0.5 {
			dst[i] = src[i]
		}
	}
}

func elseBranch(dst, src []float32) {
	for i := range dst {
		if src[i] > 0 {
			dst[i] = src[i]
		} else {
			dst[i] = 0
		}
	}
}

func transformedStore(dst, src []float32) {
	for i := range dst {
		if src[i] > 0 {
			dst[i] = src[i] * 2
		}
	}
}

func mismatchedIndex(dst, src []float32, j int) {
	for i := range dst {
		if src[j] > 0 {
			dst[i] = src[j]
		}
	}
}

func inPlace(values []float32) {
	for i := range values {
		if values[i] > 0 {
			values[i] = values[i]
		}
	}
}

func integerSlices(dst, src []int) {
	for i := range dst {
		if src[i] > 0 {
			dst[i] = src[i]
		}
	}
}

func extraStatement(dst, src []float32) {
	for i := range dst {
		if src[i] > 0 {
			dst[i] = src[i]
		}
		_ = i
	}
}

//perfscan:measured-fallback PS6060 arm64 tail retained after paired evidence.
func measuredFallback(dst, src []float32) {
	for i := range dst {
		if src[i] > 0 {
			dst[i] = src[i]
		}
	}
}

// An unrelated native kernel makes the package architecture-aware but is not
// a same-dtype ReLU sibling, so exactF32 above must still be reported.
func matmulF32NEON() {}

var _ = []any{exactF32, reversedConditionF64, ifInitTemporary, greaterEqual, nonzeroThreshold, elseBranch, transformedStore, mismatchedIndex, inPlace, integerSlices, extraStatement, measuredFallback, matmulF32NEON}
