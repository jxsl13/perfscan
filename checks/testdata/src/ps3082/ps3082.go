package ps3082

import "math"

func runningMax(xs []float64) float64 {
	hi := math.Inf(-1)
	for _, v := range xs {
		hi = math.Max(hi, v) // want `math\.Max in a loop pays a function call per iteration; the max builtin is one instruction but differs on NaN-vs-Inf pairs — use a NaN-correct wrapper and gate with planted edge values`
	}
	return hi
}

func runningMin(xs []float64) float64 {
	lo := math.Inf(1)
	for _, v := range xs {
		lo = math.Min(lo, v) // want `math\.Min in a loop pays a function call per iteration; the min builtin is one instruction but differs on NaN-vs-Inf pairs — use a NaN-correct wrapper and gate with planted edge values`
	}
	return lo
}

// A clamp is PS3077's finding, not this check's: neither the outer nor the
// inner call is reported here.
func clamp(xs []float64, lo, hi float64) {
	for i, v := range xs {
		xs[i] = math.Min(math.Max(v, lo), hi)
	}
}

func outsideLoop(a, b float64) float64 {
	return math.Max(a, b)
}
