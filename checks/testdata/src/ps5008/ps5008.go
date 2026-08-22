package ps5008

import "math"

func rotate(theta float64) (float64, float64) {
	s := math.Sin(theta) // want `math\.Sin and math\.Cos are both computed on theta — each repeats the full argument reduction; fuse to sin, cos := math\.Sincos\(theta\) \(bit-identical\)`
	c := math.Cos(theta)
	return s, c
}

// Cos first: still fusable; the fixed Lhs order follows Sincos's (sin, cos).
func cosFirst(theta float64) (float64, float64) {
	c := math.Cos(theta)
	s := math.Sin(theta) // want `math\.Sin and math\.Cos are both computed on theta — each repeats the full argument reduction; fuse to sin, cos := math\.Sincos\(theta\) \(bit-identical\)`
	return s, c
}

func positional(pos, dim float64) (float64, float64) {
	angle := pos / dim
	return math.Sin(angle), math.Cos(angle) // want `math\.Sin and math\.Cos are both computed on angle — each repeats the full argument reduction; fuse to sin, cos := math\.Sincos\(angle\) \(bit-identical\)`
}

// Separated by another statement: reported, but no auto-fix.
func separated(theta float64) (float64, float64) {
	s := math.Sin(theta) // want `math\.Sin and math\.Cos are both computed on theta — each repeats the full argument reduction; fuse to sin, cos := math\.Sincos\(theta\) \(bit-identical\)`
	scale := theta * 2
	c := math.Cos(theta)
	return s * scale, c
}

// Different arguments: nothing to fuse.
func different(a, b float64) (float64, float64) {
	return math.Sin(a), math.Cos(b)
}

// Already fused: silent.
func fused(theta float64) (float64, float64) {
	return math.Sincos(theta)
}

// Only one of the pair: silent.
func onlySin(theta float64) float64 {
	return math.Sin(theta)
}
