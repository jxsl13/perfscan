package ps3082_runtime_dot

import (
	"math"
	. "runtime"
)

var _ = GOARCH

func maximum(values []float64) float64 {
	result := math.Inf(-1)
	for _, value := range values {
		result = math.Max(result, value) // want `math\.Max in a data-scaled loop can stay an out-of-line architecture call per iteration; use the exact architecture-aware max-builtin helper, validate signed-zero/infinity/NaN raw bits on every target, and retain it only after a complete-operation benchmark`
	}
	return result
}
