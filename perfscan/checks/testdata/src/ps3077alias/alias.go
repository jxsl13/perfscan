package ps3077alias

// The math package is imported under an ALIAS. astutil.PkgFuncCall matches
// aliased stdlib imports, so PS3077's DIAGNOSTIC fires on m.Min(m.Max(…)); this
// pins that the FIX now applies too. The file-level would-orphan guard counts
// the file's math refs via ps3077MathRefs, which now resolves the math package
// by import PATH (not the literal `math` qualifier), so the surviving m.Abs ref
// below keeps the import alive under the count (2 clamp refs removed, 1 abs ref
// left) and the clamp is rewritten to the psClamp helper — reusing NO hardcoded
// `math.` qualifier, so the alias needs no rewrite.

import m "math"

func clampAliased(xs []float64) {
	for i, v := range xs {
		xs[i] = m.Min(m.Max(v, 0), 1) // want `a clamp written as math\.Min\(math\.Max\(…\)\) pays two calls with the full NaN/signed-zero contract per iteration; a comparison chain is far cheaper but must be gated on -0/NaN/Inf edge cases`
	}
}

// A surviving aliased math ref: keeps the import alive after the fix, so the
// would-orphan guard does not withhold.
func absAliased(x float64) float64 {
	return m.Abs(x)
}
