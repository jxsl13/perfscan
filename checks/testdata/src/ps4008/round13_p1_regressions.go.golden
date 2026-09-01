package ps4008

func round13P1NoopInt(_ *int) bool { return false }

func round13P1ResetInt(value *int) bool {
	*value = 0
	return false
}

func round13P1Invoke(callback func()) { callback() }

func round13P1NoopFloat(_ *float64) {}

func round13P1ResetAndReturn(value *float64, replacement float64) float64 {
	*value = 0
	return replacement
}

func taggedBoolSwitchRound13P1(a, b [][]float64, output []float64, rows, columns int, flag bool) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				switch flag {
				case true:
				case false:
					sum += a[row][inner] * b[inner][column]
				}
			}
			output[column] = sum
		}
	}
}

func taggedBoolCompleteTileControlRound13P1(a, b [][]float64, output []float64, rows, columns int, flag bool) {
	for row := 0; row < rows; row++ {
		for column := 0; column+3 < columns; column += 4 {
			var sum0, sum1, sum2, sum3 float64
			for inner := 0; inner < rows; inner++ {
				switch flag {
				case true:
					sum0 += a[row][inner] * b[inner][column]
					sum1 += a[row][inner] * b[inner][column+1]
					sum2 += a[row][inner] * b[inner][column+2]
					sum3 += a[row][inner] * b[inner][column+3]
				case false:
					sum0 += a[row][inner] * b[inner][column]
					sum1 += a[row][inner] * b[inner][column+1]
					sum2 += a[row][inner] * b[inner][column+2]
					sum3 += a[row][inner] * b[inner][column+3]
				}
			}
			output[column] = sum0 + sum1 + sum2 + sum3
		}
	}
}

func noopCasePointerRound13P1(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := inner
				switch {
				case round13P1NoopInt(&base):
				case true:
					sum += a[row][base] * b[base][column]
				}
			}
			output[column] = sum
		}
	}
}

func provenResetCaseControlRound13P1(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base := inner
				switch {
				case round13P1ResetInt(&base):
				case true:
					sum += a[row][base] * b[base][column]
				}
			}
			output[column] = sum
		}
	}
}

func callbackArgumentRound13P1(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column+3 < columns; column += 4 {
			var sum0, sum1, sum2, sum3 float64
			base := 0
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base = inner
				mutate := func() { base = 0 }
				sum0 += a[row][base] * b[base][column]
				round13P1Invoke(mutate)
				sum1 += a[row][base] * b[base][column+1]
				sum2 += a[row][base] * b[base][column+2]
				sum3 += a[row][base] * b[base][column+3]
			}
			output[column] = sum0 + sum1 + sum2 + sum3
		}
	}
}

func unrelatedCallbackControlRound13P1(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column+3 < columns; column += 4 {
			var sum0, sum1, sum2, sum3 float64
			base := 0
			unrelated := 0
			for inner := 0; inner < rows; inner++ {
				base = inner
				callback := func() { unrelated++ }
				round13P1Invoke(callback)
				sum0 += a[row][base] * b[base][column]
				sum1 += a[row][base] * b[base][column+1]
				sum2 += a[row][base] * b[base][column+2]
				sum3 += a[row][base] * b[base][column+3]
			}
			output[column] = sum0 + sum1 + sum2 + sum3 + float64(unrelated)
		}
	}
}

func candidateRHSResetRound13P1(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column+3 < columns; column += 4 {
			var sum0, sum1, sum2, sum3 float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				sum0 += round13P1ResetAndReturn(&sum0, a[row][inner]) * b[inner][column]
				sum1 += a[row][inner] * b[inner][column+1]
				sum2 += a[row][inner] * b[inner][column+2]
				sum3 += a[row][inner] * b[inner][column+3]
			}
			output[column] = sum0 + sum1 + sum2 + sum3
		}
	}
}

func candidatePureControlRound13P1(a, b [][]float64, output []float64, rows, columns int) {
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

func invalidActiveBranchRound13P1(a, b [][]float64, output []float64, rows, columns int, flag bool) {
	for row := 0; row < rows; row++ {
		for column := 0; column+3 < columns; column += 4 {
			var sum0, sum1, sum2, sum3, serial float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				if flag {
					sum0 += a[row][inner] * b[inner][column]
					sum1 += a[row][inner] * b[inner][column+1]
					sum2 += a[row][inner] * b[inner][column+2]
					sum3 += a[row][inner] * b[inner][column+3]
				} else {
					serial += a[row][inner] * b[inner][column]
					round13P1NoopFloat(&serial)
				}
			}
			output[column] = sum0 + sum1 + sum2 + sum3 + serial
		}
	}
}

func inactiveBranchControlRound13P1(a, b [][]float64, output []float64, rows, columns int, flag bool) {
	for row := 0; row < rows; row++ {
		for column := 0; column+3 < columns; column += 4 {
			var sum0, sum1, sum2, sum3 float64
			for inner := 0; inner < rows; inner++ {
				if flag {
					sum0 += a[row][inner] * b[inner][column]
					sum1 += a[row][inner] * b[inner][column+1]
					sum2 += a[row][inner] * b[inner][column+2]
					sum3 += a[row][inner] * b[inner][column+3]
				}
			}
			output[column] = sum0 + sum1 + sum2 + sum3
		}
	}
}
