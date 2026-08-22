package ps3082_shadow

import "math"

// Package-level max/min declarations capture the builtin names: an injected
// psFmax/psFmin helper calling max()/min() would dispatch to these functions
// instead of the builtins, so PS3082 must stay advisory (no SuggestedFix).

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

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
