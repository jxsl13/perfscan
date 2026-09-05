package ps1006_zero_column

const columns = 0
const rows = 1 << 30

// The original outer loop is zero-trip regardless of rows. Interchanging it
// would execute the row loop and turn this constant-time no-op into O(rows).
func zeroColumnReduction(a [0]float64) (output [0]float64) {
	for column := 0; column < columns; column++ {
		sum := 0.0
		for row := 0; row < rows; row++ {
			sum += a[row*columns+column] // want `the inner loop variable is the multiplied \(high-stride\) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O\(width\) scratch, otherwise interchange or gather into contiguous scratch`
		}
		output[column] = sum
	}
	return output
}
