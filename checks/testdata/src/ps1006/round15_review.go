package ps1006

func round15NoopIntPointer(*int) {}

func round15ResetIntPointer(value *int) { *value = 0 }

func round15NoopVoid() {}

func round15InvokeBool(callback func()) bool {
	callback()
	return true
}

var round15MutableStride = 8

func round15MutateStride() bool {
	round15MutableStride++
	return true
}

// A pointer alias passed to a proved no-op is exposure, not an overwrite.
func pointerAliasNoopRound15(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			pointer := &base
			round15NoopIntPointer(pointer)
			sum += a[base+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		output[column] = sum
	}
}

// Control: the same alias passed to a proved reset really is overwritten.
func pointerAliasResetControlRound15(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			pointer := &base
			round15ResetIntPointer(pointer)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

// A typed function-value alias of a declared no-op has known empty effects.
func declaredFunctionAliasNoopRound15(a, output []float64, rows, columns int) {
	call := round15NoopVoid
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			call()
			sum += a[base+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		output[column] = sum
	}
}

// Control: a declared no-op alias must not invalidate a complete register tile.
func declaredFunctionAliasTileControlRound15(a, weights, output []float64, taps, channels int) {
	call := round15NoopVoid
	for column := 0; column+3 < channels; column += 4 {
		var sum0, sum1, sum2, sum3 float64
		for tap := 0; tap < taps; tap++ {
			base := tap * channels
			call()
			sum0 += a[base+column] * weights[tap]
			sum1 += a[base+column+1] * weights[tap]
			sum2 += a[base+column+2] * weights[tap]
			sum3 += a[base+column+3] * weights[tap]
		}
		output[column] = sum0 + sum1 + sum2 + sum3
	}
}

// Impossible callback writes in stored and immediately-invoked closures are inert.
func unreachableCallableWritesRound15(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
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
			sum += a[base+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		output[column] = sum
	}
}

// Control: an unreachable package-global mutator cannot destabilize the stride.
func shortCircuitPackageGlobalControlRound15(a, weights, output []float64, taps, channels int) {
	for column := 0; column+3 < channels; column += 4 {
		var sum0, sum1, sum2, sum3 float64
		for tap := 0; tap < taps; tap++ {
			if false && round15MutateStride() {
			}
			if true || round15MutateStride() {
			}
			sum0 += a[tap*round15MutableStride+column] * weights[tap]
			sum1 += a[tap*round15MutableStride+column+1] * weights[tap]
			sum2 += a[tap*round15MutableStride+column+2] * weights[tap]
			sum3 += a[tap*round15MutableStride+column+3] * weights[tap]
		}
		output[column] = sum0 + sum1 + sum2 + sum3
	}
}

func conditionalCallableWriteRound15(a, output []float64, rows, columns int, flag bool) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			callback := func() {
				if flag {
					base = 0
				}
			}
			callback()
			sum += a[base+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		output[column] = sum
	}
}

func returnAndGotoReachabilityRound15(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum0, sum1 float64
		for row := 0; row < rows; row++ {
			base0 := row * columns
			returning := func() {
				return
				base0 = 0
			}
			returning()
			sum0 += a[base0+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`

			base1 := row * columns
			jumping := func() {
				goto done
				base1 = 0
			done:
			}
			jumping()
			sum1 += a[base1+column]
		}
		output[column] = sum0 + sum1
	}
}

func gotoSkipsWriteRound15(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			callback := func() {
				goto done
				base = 0
			done:
			}
			callback()
			sum += a[base+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		output[column] = sum
	}
}

// Control: every reachable path jumps through the overwrite.
func gotoOverwriteControlRound15(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			callback := func() {
				goto reset
			reset:
				base = 0
			}
			callback()
			sum += a[base+column]
		}
		output[column] = sum
	}
}
