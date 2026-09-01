package ps1006

import "ps919helper"

func round14SetStrideAndTrue(target *int, row, columns int) bool {
	*target = row * columns
	return true
}

func round14InvokeBool(callback func()) bool {
	callback()
	return true
}

// A package-qualified assignment writes the imported Var, not its PkgName.
func importedDirectStrideMutationRound14(a, weights, output []float64, taps, channels int) {
	for column := 0; column+3 < channels; column += 4 {
		var sum0, sum1, sum2, sum3 float64
		for tap := 0; tap < taps; tap++ {
			sum0 += a[tap*ps919helper.MutableStride+column] * weights[tap] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			ps919helper.MutableStride = channels + 1
			sum1 += a[tap*ps919helper.MutableStride+column+1] * weights[tap]
			sum2 += a[tap*ps919helper.MutableStride+column+2] * weights[tap]
			sum3 += a[tap*ps919helper.MutableStride+column+3] * weights[tap]
		}
		output[column] = sum0 + sum1 + sum2 + sum3
	}
}

// An opaque call can mutate a package variable in the imported package.
func importedHelperStrideMutationRound14(a, weights, output []float64, taps, channels int) {
	for column := 0; column+3 < channels; column += 4 {
		var sum0, sum1, sum2, sum3 float64
		for tap := 0; tap < taps; tap++ {
			sum0 += a[tap*ps919helper.MutableStride+column] * weights[tap] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			ps919helper.ChangeStride()
			sum1 += a[tap*ps919helper.MutableStride+column+1] * weights[tap]
			sum2 += a[tap*ps919helper.MutableStride+column+2] * weights[tap]
			sum3 += a[tap*ps919helper.MutableStride+column+3] * weights[tap]
		}
		output[column] = sum0 + sum1 + sum2 + sum3
	}
}

// Immutable imported inputs remain eligible for the four-lane proof.
func importedImmutableStrideControlRound14(a, weights, output []float64, taps, channels int) {
	for column := 0; column+3 < channels; column += 4 {
		var sum0, sum1, sum2, sum3 float64
		for tap := 0; tap < taps; tap++ {
			sum0 += a[tap*ps919helper.ImmutableStride+column] * weights[tap]
			sum1 += a[tap*ps919helper.ImmutableStride+column+1] * weights[tap]
			sum2 += a[tap*ps919helper.ImmutableStride+column+2] * weights[tap]
			sum3 += a[tap*ps919helper.ImmutableStride+column+3] * weights[tap]
		}
		output[column] = sum0 + sum1 + sum2 + sum3
	}
}

func ifConditionCreatesStrideRound14(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := 0
			if round14SetStrideAndTrue(&base, row, columns) {
				sum += a[base+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			}
		}
		output[column] = sum
	}
}

func callbackConditionCreatesStrideRound14(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := 0
			callback := func() { base = row * columns }
			if round14InvokeBool(callback) {
				sum += a[base+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			}
		}
		output[column] = sum
	}
}

func immediateCallbackConditionCreatesStrideRound14(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := 0
			if func() bool {
				base = row * columns
				return true
			}() {
				sum += a[base+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
			}
		}
		output[column] = sum
	}
}

// Converting a function value does not execute its body.
func dormantConditionClosureControlRound14(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := 0
			if any(func() { base = row * columns }) != nil {
				sum += a[base+column]
			}
		}
		output[column] = sum
	}
}

func conditionShortCircuitControlsRound14(a, weights, output []float64, taps, channels int) {
	for column := 0; column+3 < channels; column += 4 {
		var sum0, sum1, sum2, sum3 float64
		for tap := 0; tap < taps; tap++ {
			base := tap * channels
			if false && round14InvokeBool(func() { base = 0 }) {
				base = 0
			}
			if true || round14InvokeBool(func() { sum1 = 0 }) {
			}
			sum0 += a[base+column] * weights[tap]
			sum1 += a[base+column+1] * weights[tap]
			sum2 += a[base+column+2] * weights[tap]
			sum3 += a[base+column+3] * weights[tap]
		}
		output[column] = sum0 + sum1 + sum2 + sum3
	}
}

func falseConditionControlRound14(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			if false {
				sum += a[row*columns+column]
			}
		}
		output[column] = sum
	}
}

func unrelatedCallbackConditionControlRound14(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base, unrelated := 0, 0
			if round14InvokeBool(func() { unrelated = row * columns }) {
				sum += a[base+column]
			}
			output[0] += float64(unrelated)
		}
		output[column] += sum
	}
}
