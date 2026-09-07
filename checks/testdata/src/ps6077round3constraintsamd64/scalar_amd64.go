package ps6077round3constraintsamd64

import "math"

type constraintScalar[T ~float64] struct{}
type incompatibleConstraint[T ~float32] struct{}
type compatibleConstraint[T ~float64] struct{}

func (constraintScalar[T]) ConstraintSigmoidF64(values []T) T { // want `constraintScalar.ConstraintSigmoidF64 has an architecture-specific scalar transcendental implementation \(math.Exp\).*same-partition discovery evidence: shape-compatible sigmoid-family vector leaf compatibleConstraint.ConstraintSiLUAVX .*Treat related leaves and consumers as discovery evidence only`
	return T(1 / (1 + math.Exp(-float64(values[0]))))
}

func (incompatibleConstraint[U]) ConstraintSiLUNEON(values []U) U {
	var sum U
	for index := 0; index+1 < len(values); index += 2 {
		sum += values[index] + values[index+1]
	}
	return sum
}

func (compatibleConstraint[U]) ConstraintSiLUAVX(values []U) U {
	var sum U
	for index := 0; index+1 < len(values); index += 2 {
		sum += values[index] + values[index+1]
	}
	return sum
}
