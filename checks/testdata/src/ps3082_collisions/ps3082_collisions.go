package ps3082_collisions

import "math"

var runtime = struct{ GOARCH string }{GOARCH: "not-the-runtime-package"}

// psFmax is deliberately not the generated helper despite copying its marker
// phrase (see perfscan PS3082).
func psFmax(a, b float64) float64 { return a - b }

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
