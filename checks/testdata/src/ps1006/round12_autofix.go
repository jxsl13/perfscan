package ps1006

// Each shape below remains diagnosable, but slice bounds/overlap are runtime
// properties, so no loop-interchange edit may be attached.
func negativeColumnsMustStayAdvisoryRound12(a, out []float64, rows, cols int) {
	for c := 0; c < cols; c++ {
		sum := 0.0
		for r := 0; r < rows; r++ {
			sum += a[r*cols+c] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = sum
	}
}

func sourcePanicStoreOrderMustStayAdvisoryRound12(a, out []float64, rows, cols int) {
	for c := 0; c < cols; c++ {
		sum := 0.0
		for r := 0; r < rows; r++ {
			sum += a[r*cols+c] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = sum
	}
}

func sharedBackingMustStayAdvisoryRound12(a, out []float64, rows, cols int) {
	for c := 0; c < cols; c++ {
		sum := 0.0
		for r := 0; r < rows; r++ {
			sum += a[r*cols+c] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		out[c] = sum
	}
}
