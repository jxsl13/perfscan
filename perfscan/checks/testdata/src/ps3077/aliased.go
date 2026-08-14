package ps3077

// The math package is imported under an ALIAS: PkgFuncCall matches aliased
// stdlib imports, so the DIAGNOSTIC fires on m.Min(m.Max(…)). The FIX is
// withheld, though: the file-level import-orphan guard counts the file's
// math refs via ps3077MathRefs, which only recognizes the literal `math`
// qualifier — alias-qualified refs count zero, so every aliased file trips
// the would-orphan clause and stays advisory. The extra m.Abs ref below
// would keep the import alive under an alias-aware count (3 refs - 2 per
// fix = 1 > 0), pinning that the withhold is purely the alias-unaware
// counter, not genuine orphaning. The golden is therefore identical to
// this file.

import m "math"

// CONSTANT literal bounds through the alias: reported, but not rewritten.
func clampAliased(xs []float64) {
	for i, v := range xs {
		xs[i] = m.Min(m.Max(v, 0), 1) // want `a clamp written as math\.Min\(math\.Max\(…\)\) pays two calls with the full NaN/signed-zero contract per iteration; a comparison chain is far cheaper but must be gated on -0/NaN/Inf edge cases`
	}
}

// A surviving aliased math ref: under an alias-aware ref count the fix
// above would not orphan the import.
func absAliased(x float64) float64 {
	return m.Abs(x)
}
