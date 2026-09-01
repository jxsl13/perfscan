package ps1006

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
func round16Invoke(callback func())                            { callback() }
func round16DiscardVariadic(...func())                         {}
func round16InvokeOneVariadic(callbacks ...func())             { callbacks[0]() }
func round16SetStride(target *int, row, columns int)           { *target = row * columns }
func round16ResetIgnoringInputs(target *int, row, columns int) { *target = 0 }
func round16ResetGlobal()                                      { round16GlobalBase = 0 }
func round16SetGlobal(row, columns int)                        { round16GlobalBase = row * columns }
func round16NestedSetStride(target *int, row, columns int) {
	apply := func() { *target = row * columns }
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

func methodExpressionRound16(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := round16Offset(row * columns)
			(*round16Offset).noop(&base)
			sum += a[int(base)+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		output[column] = sum
	}
}

func methodExpressionResetControlRound16(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := round16Offset(row * columns)
			(*round16Offset).reset(&base)
			sum += a[int(base)+column]
		}
		output[column] = sum
	}
}

func methodExpressionAliasRound16(a, output []float64, rows, columns int) {
	call := (*round16Offset).noop
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := round16Offset(row * columns)
			call(&base)
			sum += a[int(base)+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		output[column] = sum
	}
}

func genericInstancesRound16(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			round16GenericNoop[int](&base)
			sum += a[base+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		output[column] = sum
	}
}

func genericIndexListRound16(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			round16GenericNoop2[int, string](&base)
			sum += a[base+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		output[column] = sum
	}
}

func genericResetControlRound16(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			round16GenericReset[int](&base)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func deferTimingRound16(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			defer round16GenericReset(&base)
			sum += a[base+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		output[column] = sum
	}
}

func callbackDeferResetControlRound16(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			callback := func() { defer round16GenericReset(&base) }
			callback()
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func callbackInvocationModesRound16(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			round16DiscardCallback(func() { base = 0 })
			sum += a[base+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		output[column] = sum
	}
}

func conditionalCallbackRound16(a, output []float64, rows, columns int, flag bool) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			round16MaybeInvoke(func() { base = 0 }, flag)
			sum += a[base+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		output[column] = sum
	}
}

func invokedCallbackResetControlRound16(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			round16Invoke(func() { base = 0 })
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func ordinaryCallsCreateStrideRound16(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := 0
			callback := func() { base = row * columns }
			callback()
			sum += a[base+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		output[column] = sum
	}
}

func ordinaryHelperCreatesStrideRound16(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := 0
			round16SetStride(&base, row, columns)
			sum += a[base+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		output[column] = sum
	}
}

func unusedDerivedArgumentsControlRound16(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			round16ResetIgnoringInputs(&base, row, columns)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func methodValuesRound16(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum0, sum1 float64
		for row := 0; row < rows; row++ {
			base0 := round16Offset(row * columns)
			noop := base0.noop
			noop()
			sum0 += a[int(base0)+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			base1 := round16Offset(row * columns)
			reset := base1.reset
			reset()
			sum1 += a[int(base1)+column]
		}
		output[column] = sum0 + sum1
	}
}

func goAndVariadicRound16(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			go round16GenericReset(&base)
			sum += a[base+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		output[column] = sum
	}
}

func variadicDiscardRound16(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			round16DiscardVariadic(func() { base = 0 })
			sum += a[base+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		output[column] = sum
	}
}

func variadicInvokeControlRound16(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			round16InvokeOneVariadic(func() { base = 0 })
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func variadicFirstArgumentInvokedControlRound16(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			round16InvokeOneVariadic(func() { base = 0 }, func() {})
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func variadicSecondArgumentNotInvokedRound16(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			round16InvokeOneVariadic(func() {}, func() { base = 0 })
			sum += a[base+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		output[column] = sum
	}
}

func packageGlobalResetControlRound16(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			round16GlobalBase = row * columns
			round16ResetGlobal()
			sum += a[round16GlobalBase+column]
		}
		output[column] = sum
	}
}

func packageGlobalCreatesStrideRound16(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			round16GlobalBase = 0
			round16SetGlobal(row, columns)
			sum += a[round16GlobalBase+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		output[column] = sum
	}
}

func nestedHelperCreatesStrideRound16(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := 0
			round16NestedSetStride(&base, row, columns)
			sum += a[base+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		output[column] = sum
	}
}

func nestedHelperResetControlRound16(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			round16NestedReset(&base)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func recursiveHelperNoopRound16(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			round16RecursiveNoop(&base, 1)
			sum += a[base+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		output[column] = sum
	}
}
