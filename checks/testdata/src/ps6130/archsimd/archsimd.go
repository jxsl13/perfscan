// Package archsimd is a synthetic typed fixture, never compiler evidence.
package archsimd

type Float64x2 struct{}

func BroadcastFloat64x2(float64) Float64x2              { return Float64x2{} }
func (Float64x2) MulAdd(Float64x2, Float64x2) Float64x2 { return Float64x2{} }
