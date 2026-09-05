package ps4008

type round16Offset int

var round16GlobalBase int

func (*round16Offset) noop()        {}
func (value *round16Offset) reset() { *value = 0 }

func round16GenericNoop[T any](*T)         {}
func round16GenericReset[T ~int](value *T) { *value = 0 }
func round16GenericNoop2[T, U any](*T)     {}

func round16DiscardCallback(func()) {}
func round16MaybeInvoke(callback func(), flag bool) {
	if flag {
		callback()
	}
}
func round16Invoke(callback func())                    { callback() }
func round16DiscardVariadic(...func())                 {}
func round16InvokeOneVariadic(callbacks ...func())     { callbacks[0]() }
func round16SetInner(target *int, inner int)           { *target = inner }
func round16ResetIgnoringInner(target *int, inner int) { *target = 0 }
func round16ResetGlobal()                              { round16GlobalBase = 0 }
func round16SetGlobal(inner int)                       { round16GlobalBase = inner }
func round16NestedSetInner(target *int, inner int) {
	apply := func() { *target = inner }
	apply()
}
func round16NestedReset(target *int) {
	apply := func() { *target = 0 }
	apply()
}
func round16RecursiveNoop(target *int, depth int) {
	if depth > 0 {
		round16RecursiveNoop(target, depth-1)
	}
}

func methodExpressionRound16(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := round16Offset(inner)
				(*round16Offset).noop(&base)
				sum += a[row][int(base)] * b[int(base)][column]
			}
			output[column] = sum
		}
	}
}

func methodExpressionResetControlRound16(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base := round16Offset(inner)
				(*round16Offset).reset(&base)
				sum += a[row][int(base)] * b[int(base)][column]
			}
			output[column] = sum
		}
	}
}

func methodExpressionAliasRound16(a, b [][]float64, output []float64, rows, columns int) {
	call := (*round16Offset).noop
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := round16Offset(inner)
				call(&base)
				sum += a[row][int(base)] * b[int(base)][column]
			}
			output[column] = sum
		}
	}
}

func genericInstanceRound16(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := inner
				round16GenericNoop[int](&base)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func genericIndexListRound16(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := inner
				round16GenericNoop2[int, string](&base)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func genericResetControlRound16(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base := inner
				round16GenericReset[int](&base)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func deferTimingRound16(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := inner
				defer round16GenericReset(&base)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func callbackDeferResetControlRound16(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base := inner
				callback := func() { defer round16GenericReset(&base) }
				callback()
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func discardedCallbackRound16(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := inner
				round16DiscardCallback(func() { base = 0 })
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func conditionalCallbackRound16(a, b [][]float64, output []float64, rows, columns int, flag bool) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := inner
				round16MaybeInvoke(func() { base = 0 }, flag)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func invokedCallbackResetControlRound16(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base := inner
				round16Invoke(func() { base = 0 })
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func callbackCreatesInnerRound16(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := 0
				callback := func() { base = inner }
				callback()
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func helperCreatesInnerRound16(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := 0
				round16SetInner(&base, inner)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func unusedDerivedArgumentControlRound16(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base := inner
				round16ResetIgnoringInner(&base, inner)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func methodValueNoopRound16(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := round16Offset(inner)
				noop := base.noop
				noop()
				sum += a[row][int(base)] * b[int(base)][column]
			}
			output[column] = sum
		}
	}
}

func methodValueResetControlRound16(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base := round16Offset(inner)
				reset := base.reset
				reset()
				sum += a[row][int(base)] * b[int(base)][column]
			}
			output[column] = sum
		}
	}
}

func goResetRound16(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := inner
				go round16GenericReset(&base)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func variadicDiscardRound16(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := inner
				round16DiscardVariadic(func() { base = 0 })
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func variadicInvokeControlRound16(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base := inner
				round16InvokeOneVariadic(func() { base = 0 })
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func variadicFirstArgumentInvokedControlRound16(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base := inner
				round16InvokeOneVariadic(func() { base = 0 }, func() {})
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func variadicSecondArgumentNotInvokedRound16(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := inner
				round16InvokeOneVariadic(func() {}, func() { base = 0 })
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func packageGlobalResetControlRound16(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				round16GlobalBase = inner
				round16ResetGlobal()
				sum += a[row][round16GlobalBase] * b[round16GlobalBase][column]
			}
			output[column] = sum
		}
	}
}

func packageGlobalCreatesInnerRound16(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				round16GlobalBase = 0
				round16SetGlobal(inner)
				sum += a[row][round16GlobalBase] * b[round16GlobalBase][column]
			}
			output[column] = sum
		}
	}
}

func nestedHelperCreatesInnerRound16(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := 0
				round16NestedSetInner(&base, inner)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func nestedHelperResetControlRound16(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base := inner
				round16NestedReset(&base)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func recursiveHelperNoopRound16(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain \(reassociation is not bit-identical — gate with a tolerance oracle\)`
				base := inner
				round16RecursiveNoop(&base, 1)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}
