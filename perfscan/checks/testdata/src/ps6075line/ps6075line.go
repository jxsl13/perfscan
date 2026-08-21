package ps6075line

// Four float64 values use 32 bytes: one quarter of an Apple 128-byte line but
// one half of the configured 64-byte line in this fixture, so it stays silent.
func halfLine(left, right, destination []float64, depth, columns int) {
	for column := 0; column < columns; column += 4 {
		var acc0, acc1, acc2, acc3 float64
		for p := 0; p < depth; p++ {
			acc0 += left[p] * right[p*columns+column]
			acc1 += left[p] * right[p*columns+column+1]
			acc2 += left[p] * right[p*columns+column+2]
			acc3 += left[p] * right[p*columns+column+3]
		}
		destination[column], destination[column+1] = acc0, acc1
		destination[column+2], destination[column+3] = acc2, acc3
	}
}
