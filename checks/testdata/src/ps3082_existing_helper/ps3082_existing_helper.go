package ps3082_existing_helper

import (
	"math"
	rt "runtime"
)

// psFmax preserves math.Max bit for bit while keeping its common path inline
// (see perfscan PS3082).
func psFmax(a, b float64) float64 {
	if rt.GOARCH == "arm64" {
		const positiveInfinityBits = uint64(0x7ff0000000000000)
		positiveInfinity := math.Float64frombits(positiveInfinityBits)
		if a == positiveInfinity || b == positiveInfinity {
			return positiveInfinity
		}
		return max(b, a)
	}
	if r := max(a, b); r == r {
		return r
	}
	return math.Max(a, b)
}

// psFmin is the min builtin with a fallback to math.Min whenever the
// builtin returns NaN — the only inputs on which the two disagree (see
// perfscan PS3082).
func psFmin(a, b float64) float64 {
	if r := min(a, b); r == r {
		return r
	}
	return math.Min(a, b)
}

func maximum(values []float64) float64 {
	result := math.Inf(-1)
	for _, value := range values {
		result = math.Max(result, value) // want `math\.Max in a data-scaled loop can stay an out-of-line architecture call per iteration; use the exact architecture-aware max-builtin helper, validate signed-zero/infinity/NaN raw bits on every target, and retain it only after a complete-operation benchmark`
	}
	return result
}

func minimum(values []float64) float64 {
	result := math.Inf(1)
	for _, value := range values {
		result = math.Min(result, value) // want `math\.Min in a data-scaled loop can stay an out-of-line architecture call per iteration; use the exact architecture-aware min-builtin helper, validate signed-zero/infinity/NaN raw bits on every target, and retain it only after a complete-operation benchmark`
	}
	return result
}
