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

type acc struct{ hi float64 }

func fieldMax(a *acc, xs []float64) {
	for i := range xs {
		a.hi = math.Max(a.hi, xs[i]) // want `math\.Max in a loop pays a function call per iteration; the max builtin is one instruction but differs on NaN-vs-Inf pairs — use a NaN-correct wrapper and gate with planted edge values`
	}
}

// A call operand is not side-effect-free: reported, but never rewritten.
func callOperand(xs []float64, next func() float64) float64 {
	hi := math.Inf(-1)
	for range xs {
		hi = math.Max(hi, next()) // want `math\.Max in a loop pays a function call per iteration; the max builtin is one instruction but differs on NaN-vs-Inf pairs — use a NaN-correct wrapper and gate with planted edge values`
	}
	return hi
}

// A compound operand is not in the proven shape: reported, but never
// rewritten.
func scaledOperand(xs []float64) float64 {
	hi := math.Inf(-1)
	for _, v := range xs {
		hi = math.Max(hi, 2*v) // want `math\.Max in a loop pays a function call per iteration; the max builtin is one instruction but differs on NaN-vs-Inf pairs — use a NaN-correct wrapper and gate with planted edge values`
	}
	return hi
}

// A local name capturing the wrapper at the call site: reported, but never
// rewritten.
func shadowedWrapper(xs []float64) float64 {
	psFmax := math.Inf(-1)
	for _, v := range xs {
		psFmax = math.Max(psFmax, v) // want `math\.Max in a loop pays a function call per iteration; the max builtin is one instruction but differs on NaN-vs-Inf pairs — use a NaN-correct wrapper and gate with planted edge values`
	}
	return psFmax
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
