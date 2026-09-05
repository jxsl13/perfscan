package ps4008

type round13A [1][2]float64
type round13B [2][2]float64
type round13C [2][2]float64

func round13Identity(f func()) func() { return f }

func round13Miss(p *int) bool {
	*p = 0
	return false
}

func round13SetAndMiss(p *int, value int) bool {
	*p = value
	return false
}

func round13ResetFloat(p *float64) { *p = 0 }

// The fixed-array edit is unsafe when the outer index is not bounded by the
// exact A value: the original panics before storing, while the edit zeroes C.
func outerIndexMustStayAdvisoryRound13(index int, a round13A, b round13B) (c round13C) {
	for outer := 0; outer < 1; outer++ {
		for column := range b[0] {
			sum := 0.0
			for inner := range b { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				sum += a[index][inner] * b[inner][column]
			}
			c[index][column] = sum
		}
	}
	return c
}

func coupledAccumulatorRound13(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column+3 < columns; column += 4 {
			var sum0, sum1, sum2, sum3 float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				sum0 += a[row][inner] * b[inner][column]
				sum1 = sum0
				sum1 += a[row][inner] * b[inner][column+1]
				sum2 = sum1
				sum2 += a[row][inner] * b[inner][column+2]
				sum3 = sum2
				sum3 += a[row][inner] * b[inner][column+3]
			}
			output[column] = sum0 + sum1 + sum2 + sum3
		}
	}
}

func factoryCallableRound13(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column+3 < columns; column += 4 {
			var sum0, sum1, sum2, sum3 float64
			base := 0
			pointer := &base
			mutate := round13Identity(func() { *pointer = 0 })
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base = inner
				sum0 += a[row][base] * b[base][column]
				mutate()
				sum1 += a[row][base] * b[base][column+1]
				sum2 += a[row][base] * b[base][column+2]
				sum3 += a[row][base] * b[base][column+3]
			}
			output[column] = sum0 + sum1 + sum2 + sum3
		}
	}
}

func assertedCallableRound13(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column+3 < columns; column += 4 {
			var sum0, sum1, sum2, sum3 float64
			base := 0
			pointer := &base
			mutate := any(func() { *pointer = 0 }).(func())
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base = inner
				sum0 += a[row][base] * b[base][column]
				mutate()
				sum1 += a[row][base] * b[base][column+1]
				sum2 += a[row][base] * b[base][column+2]
				sum3 += a[row][base] * b[base][column+3]
			}
			output[column] = sum0 + sum1 + sum2 + sum3
		}
	}
}

func priorCaseExpressionCreatesDependencyRound13(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := 0
				switch {
				case round13SetAndMiss(&base, inner):
				case true:
					sum += a[row][base] * b[base][column]
				}
			}
			output[column] = sum
		}
	}
}

// The prior expression definitely resets base and has no serial input. The
// selected case is value-only and must remain a negative control.
func priorCaseExpressionResetRound13(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base := inner
				switch {
				case round13Miss(&base):
				case true:
					sum += a[row][base] * b[base][column]
				}
			}
			output[column] = sum
		}
	}
}

func unreachableLaterCaseExpressionRound13(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := inner
				switch {
				case true, round13Miss(&base):
					sum += a[row][base] * b[base][column]
				}
			}
			output[column] = sum
		}
	}
}

// Adjacent control: genuinely independent lanes still suppress the warning.
func independentAccumulatorControlRound13(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column+3 < columns; column += 4 {
			var sum0, sum1, sum2, sum3 float64
			for inner := 0; inner < rows; inner++ {
				sum0 += a[row][inner] * b[inner][column]
				sum1 += a[row][inner] * b[inner][column+1]
				sum2 += a[row][inner] * b[inner][column+2]
				sum3 += a[row][inner] * b[inner][column+3]
			}
			output[column] = sum0 + sum1 + sum2 + sum3
		}
	}
}

func resetAccumulatorRound13(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column+3 < columns; column += 4 {
			var sum0, sum1, sum2, sum3 float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				sum0 += a[row][inner] * b[inner][column]
				sum1 += a[row][inner] * b[inner][column+1]
				sum1 = 0
				sum2 += a[row][inner] * b[inner][column+2]
				sum3 += a[row][inner] * b[inner][column+3]
			}
			output[column] = sum0 + sum1 + sum2 + sum3
		}
	}
}

func pointerExposedAccumulatorRound13(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column+3 < columns; column += 4 {
			var sum0, sum1, sum2, sum3 float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				sum0 += a[row][inner] * b[inner][column]
				sum1 += a[row][inner] * b[inner][column+1]
				round13ResetFloat(&sum1)
				sum2 += a[row][inner] * b[inner][column+2]
				sum3 += a[row][inner] * b[inner][column+3]
			}
			output[column] = sum0 + sum1 + sum2 + sum3
		}
	}
}

func callableExposedAccumulatorRound13(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column+3 < columns; column += 4 {
			var sum0, sum1, sum2, sum3 float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				reset := func() { sum1 = 0 }
				sum0 += a[row][inner] * b[inner][column]
				sum1 += a[row][inner] * b[inner][column+1]
				reset()
				sum2 += a[row][inner] * b[inner][column+2]
				sum3 += a[row][inner] * b[inner][column+3]
			}
			output[column] = sum0 + sum1 + sum2 + sum3
		}
	}
}
