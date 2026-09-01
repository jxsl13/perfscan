package ps4008

func round14SetIndexAndTrue(target *int, value int) bool {
	*target = value
	return true
}

func round14InvokeBool(callback func()) bool {
	callback()
	return true
}

func ifConditionCreatesDotIndexRound14(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := 0
				if round14SetIndexAndTrue(&base, inner) {
					sum += a[row][base] * b[base][column]
				}
			}
			output[column] = sum
		}
	}
}

func callbackConditionCreatesDotIndexRound14(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := 0
				callback := func() { base = inner }
				if round14InvokeBool(callback) {
					sum += a[row][base] * b[base][column]
				}
			}
			output[column] = sum
		}
	}
}

func immediateCallbackConditionCreatesDotIndexRound14(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := 0
				if func() bool {
					base = inner
					return true
				}() {
					sum += a[row][base] * b[base][column]
				}
			}
			output[column] = sum
		}
	}
}

// Converting a function value does not execute its body.
func dormantConditionClosureControlRound14(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base := 0
				if any(func() { base = inner }) != nil {
					sum += a[row][base] * b[base][column]
				}
			}
			output[column] = sum
		}
	}
}

func falseConditionControlRound14(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				if false {
					sum += a[row][inner] * b[inner][column]
				}
			}
			output[column] = sum
		}
	}
}

func shortCircuitCallbackTileControlRound14(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column+3 < columns; column += 4 {
			var sum0, sum1, sum2, sum3 float64
			for inner := 0; inner < rows; inner++ {
				if true || round14InvokeBool(func() { sum1 = 0 }) {
				}
				sum0 += a[row][inner] * b[inner][column]
				sum1 += a[row][inner] * b[inner][column+1]
				sum2 += a[row][inner] * b[inner][column+2]
				sum3 += a[row][inner] * b[inner][column+3]
			}
			output[column] = sum0 + sum1 + sum2 + sum3
		}
	}
}

func activeCallbackInvalidatesTileRound14(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column+3 < columns; column += 4 {
			var sum0, sum1, sum2, sum3 float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				if round14InvokeBool(func() { sum1 = 0 }) {
				}
				sum0 += a[row][inner] * b[inner][column]
				sum1 += a[row][inner] * b[inner][column+1]
				sum2 += a[row][inner] * b[inner][column+2]
				sum3 += a[row][inner] * b[inner][column+3]
			}
			output[column] = sum0 + sum1 + sum2 + sum3
		}
	}
}

func unrelatedCallbackTileControlRound14(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column+3 < columns; column += 4 {
			var sum0, sum1, sum2, sum3 float64
			unrelated := 0
			for inner := 0; inner < rows; inner++ {
				if round14InvokeBool(func() { unrelated = inner }) {
				}
				sum0 += a[row][inner] * b[inner][column]
				sum1 += a[row][inner] * b[inner][column+1]
				sum2 += a[row][inner] * b[inner][column+2]
				sum3 += a[row][inner] * b[inner][column+3]
			}
			output[column] = sum0 + sum1 + sum2 + sum3 + float64(unrelated)
		}
	}
}
