package ps1006

func round13P1NoopInt(_ *int) bool { return false }

func round13P1ResetInt(pointer *int) bool {
	*pointer = 0
	return false
}

func round13P1Invoke(callback func()) { callback() }

// Constants in a tagged boolean switch compare against the tag; `case true`
// does not make the following `case false` unreachable.
func taggedBoolSwitchRound13P1(a, output []float64, rows, columns int, flag bool) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			switch flag {
			case true:
			case false:
				sum += a[base+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			}
		}
		output[column] = sum
	}
}

// Taking an address is only a may-write. A no-op callee leaves the original
// row*columns dependency live in the selected case.
func noopCasePointerRound13P1(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			switch {
			case round13P1NoopInt(&base):
			case true:
				sum += a[base+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			}
		}
		output[column] = sum
	}
}

// A declared helper can invoke a function-valued argument which directly
// captures and mutates a derived stride local.
func callbackArgumentRound13P1(a, weights, output []float64, taps, channels int) {
	for column := 0; column+3 < channels; column += 4 {
		var sum0, sum1, sum2, sum3 float64
		base := 0
		for tap := 0; tap < taps; tap++ {
			base = tap * channels
			mutate := func() { base = 0 }
			sum0 += a[base+column] * weights[tap] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			round13P1Invoke(mutate)
			sum1 += a[base+column+1] * weights[tap]
			sum2 += a[base+column+2] * weights[tap]
			sum3 += a[base+column+3] * weights[tap]
		}
		output[column] = sum0 + sum1 + sum2 + sum3
	}
}

// Control: both tagged-switch cases are reachable, but neither access has a
// multiplied high-stride inner-loop term.
func taggedBoolContiguousControlRound13P1(a, output []float64, rows int, flag bool) {
	var sum float64
	for row := 0; row < rows; row++ {
		switch flag {
		case true:
			sum += a[row]
		case false:
			sum += a[row+1]
		}
	}
	output[0] = sum
}

// Control: the exact straight-line helper body proves that base is reset.
func provenResetCaseControlRound13P1(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			switch {
			case round13P1ResetInt(&base):
			case true:
				sum += a[base+column]
			}
		}
		output[column] = sum
	}
}

// Control: callback effects on an unrelated scalar do not invalidate a tile.
func unrelatedCallbackControlRound13P1(a, weights, output []float64, taps, channels int) {
	for column := 0; column+3 < channels; column += 4 {
		var sum0, sum1, sum2, sum3 float64
		unrelated := 0
		for tap := 0; tap < taps; tap++ {
			base := tap * channels
			callback := func() { unrelated++ }
			round13P1Invoke(callback)
			sum0 += a[base+column] * weights[tap]
			sum1 += a[base+column+1] * weights[tap]
			sum2 += a[base+column+2] * weights[tap]
			sum3 += a[base+column+3] * weights[tap]
		}
		output[column] = sum0 + sum1 + sum2 + sum3 + float64(unrelated)
	}
}
