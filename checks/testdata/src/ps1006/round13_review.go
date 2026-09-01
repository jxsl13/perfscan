package ps1006

func round13Identity(f func()) func() { return f }

func round13Miss(p *int) bool {
	*p = 0
	return false
}

func round13SetStrideAndMiss(p *int, row, columns int) bool {
	*p = row * columns
	return false
}

func round13ResetFloat(p *float64) { *p = 0 }

// Assigning one lane from the preceding lane couples all four names into one
// serial recurrence; it is not an independent register tile.
func coupledAccumulatorRound13(a, weights, output []float64, taps, channels int) {
	for column := 0; column+3 < channels; column += 4 {
		var sum0, sum1, sum2, sum3 float64
		for tap := 0; tap < taps; tap++ {
			base := tap * channels
			sum0 += a[base+column] * weights[tap] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			sum1 = sum0
			sum1 += a[base+column+1] * weights[tap]
			sum2 = sum1
			sum2 += a[base+column+2] * weights[tap]
			sum3 = sum2
			sum3 += a[base+column+3] * weights[tap]
		}
		output[column], output[column+1], output[column+2], output[column+3] = sum0, sum1, sum2, sum3
	}
}

// A callable returned by an opaque factory can mutate the stride base, so it
// must conservatively invalidate the apparent tile.
func factoryCallableRound13(a, weights, output []float64, taps, channels int) {
	for column := 0; column+3 < channels; column += 4 {
		var sum0, sum1, sum2, sum3 float64
		base := 0
		pointer := &base
		mutate := round13Identity(func() { *pointer = 0 })
		for tap := 0; tap < taps; tap++ {
			base = tap * channels
			sum0 += a[base+column] * weights[tap] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			mutate()
			sum1 += a[base+column+1] * weights[tap]
			sum2 += a[base+column+2] * weights[tap]
			sum3 += a[base+column+3] * weights[tap]
		}
		output[column], output[column+1], output[column+2], output[column+3] = sum0, sum1, sum2, sum3
	}
}

func assertedCallableRound13(a, weights, output []float64, taps, channels int) {
	for column := 0; column+3 < channels; column += 4 {
		var sum0, sum1, sum2, sum3 float64
		base := 0
		pointer := &base
		mutate := any(func() { *pointer = 0 }).(func())
		for tap := 0; tap < taps; tap++ {
			base = tap * channels
			sum0 += a[base+column] * weights[tap] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			mutate()
			sum1 += a[base+column+1] * weights[tap]
			sum2 += a[base+column+2] * weights[tap]
			sum3 += a[base+column+3] * weights[tap]
		}
		output[column], output[column+1], output[column+2], output[column+3] = sum0, sum1, sum2, sum3
	}
}

// Case expressions execute in source order until one matches. The mutation in
// the first non-matching expression therefore reaches the selected body.
func priorCaseExpressionStrideRound13(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := 0
			switch {
			case round13SetStrideAndMiss(&base, row, columns):
			case true:
				sum += a[base+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			}
		}
		output[column] = sum
	}
}

// The prior expression definitely resets base and has no serial input. The
// selected case is value-only and must remain a negative control.
func priorCaseExpressionResetRound13(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			switch {
			case round13Miss(&base):
			case true:
				sum += a[base+column]
			}
		}
		output[column] = sum
	}
}

// A later expression in the same case list is unreachable after true.
func unreachableLaterCaseExpressionStrideRound13(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			switch {
			case true, round13Miss(&base):
				sum += a[base+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			}
		}
		output[column] = sum
	}
}

// Merely creating closures does not execute either body and must not hide the
// genuine four-lane register tile.
func dormantClosuresRound13(a, weights, output []float64, taps, channels int) {
	for column := 0; column+3 < channels; column += 4 {
		var sum0, sum1, sum2, sum3 float64
		for tap := 0; tap < taps; tap++ {
			base := tap * channels
			_ = func() { channels = 1 }
			_ = func() float64 { return a[0] }
			dormant := func() { channels = 2 }
			_ = dormant
			wrapped := any(func() { channels = 3 })
			_ = wrapped
			holder := struct{ call func() }{call: func() { channels = 4 }}
			_ = holder
			sum0 += a[base+column] * weights[tap]
			sum1 += a[base+column+1] * weights[tap]
			sum2 += a[base+column+2] * weights[tap]
			sum3 += a[base+column+3] * weights[tap]
		}
		output[column], output[column+1], output[column+2], output[column+3] = sum0, sum1, sum2, sum3
	}
}

// A per-iteration reset is not a loop-carried independent accumulator.
func resetAccumulatorRound13(a, weights, output []float64, taps, channels int) {
	for column := 0; column+3 < channels; column += 4 {
		var sum0, sum1, sum2, sum3 float64
		for tap := 0; tap < taps; tap++ {
			base := tap * channels
			sum0 += a[base+column] * weights[tap] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			sum1 += a[base+column+1] * weights[tap]
			sum1 = 0
			sum2 += a[base+column+2] * weights[tap]
			sum3 += a[base+column+3] * weights[tap]
		}
		output[column], output[column+1], output[column+2], output[column+3] = sum0, sum1, sum2, sum3
	}
}

func pointerExposedAccumulatorRound13(a, weights, output []float64, taps, channels int) {
	for column := 0; column+3 < channels; column += 4 {
		var sum0, sum1, sum2, sum3 float64
		for tap := 0; tap < taps; tap++ {
			base := tap * channels
			sum0 += a[base+column] * weights[tap] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			sum1 += a[base+column+1] * weights[tap]
			round13ResetFloat(&sum1)
			sum2 += a[base+column+2] * weights[tap]
			sum3 += a[base+column+3] * weights[tap]
		}
		output[column], output[column+1], output[column+2], output[column+3] = sum0, sum1, sum2, sum3
	}
}

func callableExposedAccumulatorRound13(a, weights, output []float64, taps, channels int) {
	for column := 0; column+3 < channels; column += 4 {
		var sum0, sum1, sum2, sum3 float64
		for tap := 0; tap < taps; tap++ {
			base := tap * channels
			reset := func() { sum1 = 0 }
			sum0 += a[base+column] * weights[tap] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			sum1 += a[base+column+1] * weights[tap]
			reset()
			sum2 += a[base+column+2] * weights[tap]
			sum3 += a[base+column+3] * weights[tap]
		}
		output[column], output[column+1], output[column+2], output[column+3] = sum0, sum1, sum2, sum3
	}
}

// Adjacent control: four uncoupled accumulators remain a valid tile.
func independentAccumulatorControlRound13(a, weights, output []float64, taps, channels int) {
	for column := 0; column+3 < channels; column += 4 {
		var sum0, sum1, sum2, sum3 float64
		for tap := 0; tap < taps; tap++ {
			base := tap * channels
			sum0 += a[base+column] * weights[tap]
			sum1 += a[base+column+1] * weights[tap]
			sum2 += a[base+column+2] * weights[tap]
			sum3 += a[base+column+3] * weights[tap]
		}
		output[column], output[column+1], output[column+2], output[column+3] = sum0, sum1, sum2, sum3
	}
}
