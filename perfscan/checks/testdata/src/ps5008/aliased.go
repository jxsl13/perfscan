package ps5008

import m "math"

// Aliased math import: the detector matches m.Sin/m.Cos and the fused call
// must reuse the alias (m.Sincos) to compile.
func aliased(x float64) (float64, float64) {
	s := m.Sin(x) // want `math\.Sin and math\.Cos are both computed on x — each repeats the full argument reduction; fuse to sin, cos := math\.Sincos\(x\) \(bit-identical\)`
	c := m.Cos(x)
	return s, c
}
