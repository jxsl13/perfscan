package ps1006

func round17SetThenReset(target *int, row, columns int) {
	*target = row * columns
	*target = 0
}

func round17ResetThenSet(target *int, row, columns int) {
	*target = 0
	*target = row * columns
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

func directHelperEndsResetRound17(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := 0
			round17SetThenReset(&base, row, columns)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func directHelperEndsStrideRound17(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := 0
			round17ResetThenSet(&base, row, columns)
			sum += a[base+column] // want `inner loop variable`
		}
		output[column] = sum
	}
}

func sequentialCallbacksEndResetRound17(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := 0
			round17InOrder(
				func() { base = row * columns },
				func() { base = 0 },
			)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func sequentialCallbacksEndStrideRound17(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := 0
			round17InOrder(
				func() { base = 0 },
				func() { base = row * columns },
			)
			sum += a[base+column] // want `inner loop variable`
		}
		output[column] = sum
	}
}

func nestedSequentialCallbacksEndResetRound17(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := 0
			round17ForwardInOrder(
				func() { base = row * columns },
				func() { base = 0 },
			)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func conditionalResetRemainsConservativeRound17(a, output []float64, rows, columns int, flag bool) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			round17ConditionalReset(&base, flag)
			sum += a[base+column] // want `inner loop variable`
		}
		output[column] = sum
	}
}

func earlyResetRemainsConservativeRound17(a, output []float64, rows, columns int, flag bool) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			round17EarlyReset(&base, flag)
			sum += a[base+column] // want `inner loop variable`
		}
		output[column] = sum
	}
}
