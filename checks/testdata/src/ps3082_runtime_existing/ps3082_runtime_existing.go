package ps3082_runtime_existing

import (
	"math"
	"runtime"
)

var _ = runtime.GOARCH

func minimum(values []float64) float64 {
	result := math.Inf(1)
	for _, value := range values {
		result = math.Min(result, value) // want `math\.Min in a data-scaled loop can stay an out-of-line architecture call per iteration; use the exact architecture-aware min-builtin helper, validate signed-zero/infinity/NaN raw bits on every target, and retain it only after a complete-operation benchmark`
	}
	return result
}
