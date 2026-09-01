package ps4008

func round15NoopIntPointer(*int) {}

func round15ResetIntPointer(value *int) { *value = 0 }

func round15NoopVoid() {}

func round15InvokeBool(callback func()) bool {
	callback()
	return true
}

// A pointer alias passed to a proved no-op is exposure, not an overwrite.
func pointerAliasNoopRound15(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := inner
				pointer := &base
				round15NoopIntPointer(pointer)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

// Control: the same alias passed to a proved reset really is overwritten.
func pointerAliasResetControlRound15(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base := inner
				pointer := &base
				round15ResetIntPointer(pointer)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

// A typed function-value alias of a declared no-op has known empty effects.
func declaredFunctionAliasNoopRound15(a, b [][]float64, output []float64, rows, columns int) {
	call := round15NoopVoid
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := inner
				call()
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

// Control: a declared no-op alias must not invalidate a complete register tile.
func declaredFunctionAliasTileControlRound15(a, b [][]float64, output []float64, rows, columns int) {
	call := round15NoopVoid
	for row := 0; row < rows; row++ {
		for column := 0; column+3 < columns; column += 4 {
			var sum0, sum1, sum2, sum3 float64
			for inner := 0; inner < rows; inner++ {
				call()
				sum0 += a[row][inner] * b[inner][column]
				sum1 += a[row][inner] * b[inner][column+1]
				sum2 += a[row][inner] * b[inner][column+2]
				sum3 += a[row][inner] * b[inner][column+3]
			}
			output[column] = sum0 + sum1 + sum2 + sum3
		}
	}
}

// Impossible callback writes in stored and immediately-invoked closures are inert.
func unreachableCallableWritesRound15(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := inner
				callback := func() {
					if false {
						base = 0
					}
				}
				callback()
				(func() {
					if true || round15InvokeBool(func() { base = 0 }) {
					}
				})()
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}
