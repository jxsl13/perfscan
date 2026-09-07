package ps6077round3constraintsamd64

type constraintScalar[T ~float64] struct{}

func (constraintScalar[T]) ConstraintSigmoidF64(values []T) T {
	var sum T
	for index := 0; index+1 < len(values); index += 2 {
		sum += values[index] + values[index+1]
	}
	return sum
}
