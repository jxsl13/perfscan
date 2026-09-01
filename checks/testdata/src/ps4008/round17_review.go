package ps4008

func round17SetThenReset(target *int, inner int) {
	*target = inner
	*target = 0
}

func round17ResetThenSet(target *int, inner int) {
	*target = 0
	*target = inner
}

func round17InOrder(first, second func()) {
	first()
	second()
}

func round17ForwardInOrder(first, second func()) {
	round17InOrder(first, second)
}

func round17ConditionalReset(target *int, flag bool) {
	if flag {
		*target = 0
	}
}

func round17EarlyReset(target *int, flag bool) {
	if flag {
		return
	}
	*target = 0
}

func directHelperEndsResetRound17(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base := 0
				round17SetThenReset(&base, inner)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func directHelperEndsInnerRound17(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop`
				base := 0
				round17ResetThenSet(&base, inner)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func sequentialCallbacksEndResetRound17(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base := 0
				round17InOrder(
					func() { base = inner },
					func() { base = 0 },
				)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func sequentialCallbacksEndInnerRound17(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop`
				base := 0
				round17InOrder(
					func() { base = 0 },
					func() { base = inner },
				)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func nestedSequentialCallbacksEndResetRound17(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base := 0
				round17ForwardInOrder(
					func() { base = inner },
					func() { base = 0 },
				)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func conditionalResetRemainsConservativeRound17(a, b [][]float64, output []float64, rows, columns int, flag bool) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop`
				base := inner
				round17ConditionalReset(&base, flag)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func earlyResetRemainsConservativeRound17(a, b [][]float64, output []float64, rows, columns int, flag bool) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop`
				base := inner
				round17EarlyReset(&base, flag)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}
