package ps6130

import "ps6130/archsimd"

var c0 = archsimd.BroadcastFloat64x2(0)
var c1 = archsimd.BroadcastFloat64x2(1)
var c2 = archsimd.BroadcastFloat64x2(2)
var c3 = archsimd.BroadcastFloat64x2(3)
var c4 = archsimd.BroadcastFloat64x2(4)
var c5 = archsimd.BroadcastFloat64x2(5)
var c6 = archsimd.BroadcastFloat64x2(6)
var c7 = archsimd.BroadcastFloat64x2(7)

func leaf(x archsimd.Float64x2) archsimd.Float64x2 { // want `repeats fresh ADRP/ADD/LDR-Q setup for 8 distinct SIMD polynomial coefficients`
	p := c0.MulAdd(x, c1)
	p = p.MulAdd(x, c2)
	p = p.MulAdd(x, c3)
	p = p.MulAdd(x, c4)
	p = p.MulAdd(x, c5)
	p = p.MulAdd(x, c6)
	p = p.MulAdd(x, c7)
	return p
}

func hot(values []archsimd.Float64x2) {
	for i := range values {
		_ = leaf(values[i])
	}
}

func cold(x archsimd.Float64x2) { _ = leaf(x) }
