package ps5009

import m "math"

// The stdlib math imported under an alias still resolves to the same
// package: the check must fire through the alias exactly as it does
// through the canonical qualifier.
func aliasedPositives(x float64) float64 {
	a := m.Pow(x, 2)   // want `math\.Pow with constant exponent 2 pays a general libm call ~50-100x slower than a multiply chain \(x\*x, x\*x\*x\); not auto-fixed: x\*x evaluates the base twice, and a NaN base yields a different NaN payload than math\.Pow`
	b := m.Pow(x, 0.5) // want `math\.Pow\(x, 0\.5\) pays a general libm call where math\.Sqrt\(x\) is far cheaper; not auto-fixed: math\.Sqrt differs from math\.Pow\(x, 0\.5\) for -Inf and -0 bases`
	return a + b
}

// NEGATIVE: a variable exponent through the alias is what math.Pow is for: silent.
func aliasedVariableExponent(x, y float64) float64 {
	return m.Pow(x, y)
}
