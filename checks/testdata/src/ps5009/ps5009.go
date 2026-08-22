package ps5009

import "math"

const cubeExp = 3

func positives(x float64) float64 {
	a := math.Pow(x, 2)       // want `math\.Pow with constant exponent 2 pays a general libm call ~50-100x slower than a multiply chain \(x\*x, x\*x\*x\); not auto-fixed: x\*x evaluates the base twice, and a NaN base yields a different NaN payload than math\.Pow`
	b := math.Pow(x, 3)       // want `math\.Pow with constant exponent 3 pays a general libm call ~50-100x slower than a multiply chain \(x\*x, x\*x\*x\); not auto-fixed: x\*x evaluates the base twice, and a NaN base yields a different NaN payload than math\.Pow`
	c := math.Pow(x, 0.5)     // want `math\.Pow\(x, 0\.5\) pays a general libm call where math\.Sqrt\(x\) is far cheaper; not auto-fixed: math\.Sqrt differs from math\.Pow\(x, 0\.5\) for -Inf and -0 bases`
	d := math.Pow(x, 8)       // want `math\.Pow with constant exponent 8 pays a general libm call ~50-100x slower than a multiply chain \(x\*x, x\*x\*x\); not auto-fixed: x\*x evaluates the base twice, and a NaN base yields a different NaN payload than math\.Pow`
	e := math.Pow(x, 2.0)     // want `math\.Pow with constant exponent 2 pays a general libm call ~50-100x slower than a multiply chain \(x\*x, x\*x\*x\); not auto-fixed: x\*x evaluates the base twice, and a NaN base yields a different NaN payload than math\.Pow`
	f := math.Pow(x, cubeExp) // want `math\.Pow with constant exponent 3 pays a general libm call ~50-100x slower than a multiply chain \(x\*x, x\*x\*x\); not auto-fixed: x\*x evaluates the base twice, and a NaN base yields a different NaN payload than math\.Pow`
	return a + b + c + d + e + f
}

// compoundBase mirrors the shape found during corpus validation on gonum
// (stat/distuv/logistic.go: math.Pow(1+E, 2); optimize/functions: math.Pow(x0-1, 4)):
// the base is an arbitrary sub-expression, not a bare identifier. The check keys
// only off the constant exponent, so it fires here too — and the double-
// evaluation caveat is especially pointed (a hand-rewrite must hoist the
// sub-expression into a local first).
func compoundBase(e, x0 float64) float64 {
	a := math.Pow(1+e, 2)  // want `math\.Pow with constant exponent 2 pays a general libm call ~50-100x slower than a multiply chain \(x\*x, x\*x\*x\); not auto-fixed: x\*x evaluates the base twice, and a NaN base yields a different NaN payload than math\.Pow`
	b := math.Pow(x0-1, 4) // want `math\.Pow with constant exponent 4 pays a general libm call ~50-100x slower than a multiply chain \(x\*x, x\*x\*x\); not auto-fixed: x\*x evaluates the base twice, and a NaN base yields a different NaN payload than math\.Pow`
	return a + b
}

// A variable exponent is exactly what math.Pow is for: silent.
func variableExponent(x, y float64) float64 {
	return math.Pow(x, y)
}

// Constants outside {0.5, 2..8} are free already or have no obviously
// better spelling: silent.
func outOfRangeConstants(x float64) float64 {
	a := math.Pow(x, 0)
	b := math.Pow(x, 1)
	c := math.Pow(x, 2.5)
	d := math.Pow(x, 9)
	e := math.Pow(x, 100)
	f := math.Pow(x, -2)
	return a + b + c + d + e + f
}

type fakeMath struct{}

func (fakeMath) Pow(a, b float64) float64 { return a + b }

// A shadowed math qualifier must not fire: the resolution is type-aware,
// not syntactic.
func shadowed(x float64) float64 {
	math := fakeMath{}
	return math.Pow(x, 2)
}
