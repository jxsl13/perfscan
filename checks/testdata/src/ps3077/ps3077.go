package ps3077

import "math"

func clampAll(xs []float64, lo, hi float64) {
	for i, v := range xs {
		xs[i] = math.Min(math.Max(v, lo), hi) // want `a clamp written as math\.Min\(math\.Max\(…\)\) pays two calls with the full NaN/signed-zero contract per iteration; a comparison chain is far cheaper but must be gated on -0/NaN/Inf edge cases`
	}
}

func clampReverse(xs []float64, lo, hi float64) {
	for i, v := range xs {
		xs[i] = math.Max(math.Min(v, hi), lo) // want `a clamp written as math\.Max\(math\.Min\(…\)\) pays two calls with the full NaN/signed-zero contract per iteration; a comparison chain is far cheaper but must be gated on -0/NaN/Inf edge cases`
	}
}

func clampOnce(v, lo, hi float64) float64 {
	return math.Min(math.Max(v, lo), hi)
}

func noClamp(xs []float64) float64 {
	hi := math.Inf(-1)
	for _, v := range xs {
		hi = math.Max(hi, v)
	}
	return hi
}
