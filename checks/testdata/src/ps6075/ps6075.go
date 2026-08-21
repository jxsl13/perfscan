package ps6075

func blocked4x2(left, right, destination []float64, rows, depth, columns int) {
	for row := 0; row < rows; row += 4 {
		for column := 0; column < columns; column += 2 { // want `blocked4x2 hoists 8 reduction accumulators across depth while source right advances by columns\*8 bytes per p iteration and uses only 16 of each configured 128-byte cache line \(12\.5%\).*\(depth-1\)\*64 B destination store bytes avoided versus depth\*128 B source line bytes fetched for depth\*16 B useful source bytes.*tiling or packing`
			var acc00, acc01, acc10, acc11, acc20, acc21, acc30, acc31 float64
			for p := 0; p < depth; p++ {
				acc00 += left[(row+0)*depth+p] * right[p*columns+column]
				acc01 += left[(row+0)*depth+p] * right[p*columns+column+1]
				acc10 += left[(row+1)*depth+p] * right[p*columns+column]
				acc11 += left[(row+1)*depth+p] * right[p*columns+column+1]
				acc20 += left[(row+2)*depth+p] * right[p*columns+column]
				acc21 += left[(row+2)*depth+p] * right[p*columns+column+1]
				acc30 += left[(row+3)*depth+p] * right[p*columns+column]
				acc31 += left[(row+3)*depth+p] * right[p*columns+column+1]
			}
			destination[(row+0)*columns+column], destination[(row+0)*columns+column+1] = acc00, acc01
			destination[(row+1)*columns+column], destination[(row+1)*columns+column+1] = acc10, acc11
			destination[(row+2)*columns+column], destination[(row+2)*columns+column+1] = acc20, acc21
			destination[(row+3)*columns+column], destination[(row+3)*columns+column+1] = acc30, acc31
		}
	}
}

func exactQuarter(left, right, destination []float64, depth, columns int) {
	for column := 0; column < columns; column += 4 { // want `exactQuarter hoists 4 reduction accumulators across depth.*uses only 32 of each configured 128-byte cache line \(25\.0%\)`
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

func constantShape(left, right, destination []float64) {
	for column := 0; column < 16; column += 2 { // want `constantShape hoists 2 reduction accumulators across 8.*up to 112 B destination store bytes avoided versus 1024 B source line bytes fetched for 128 B useful source bytes`
		var acc0, acc1 float64
		for p := 0; p < 8; p++ {
			acc0 += left[p] * right[p*16+column]
			acc1 += left[p] * right[p*16+column+1]
		}
		destination[column], destination[column+1] = acc0, acc1
	}
}

func wideTile(left, right, destination []float64, depth, columns int) {
	for column := 0; column < columns; column += 5 {
		var acc0, acc1, acc2, acc3, acc4 float64
		for p := 0; p < depth; p++ {
			acc0 += left[p] * right[p*columns+column]
			acc1 += left[p] * right[p*columns+column+1]
			acc2 += left[p] * right[p*columns+column+2]
			acc3 += left[p] * right[p*columns+column+3]
			acc4 += left[p] * right[p*columns+column+4]
		}
		destination[column], destination[column+1] = acc0, acc1
		destination[column+2], destination[column+3] = acc2, acc3
		destination[column+4] = acc4
	}
}

func rowContiguous(left, right, destination []float64, depth, columns int) {
	for p := 0; p < depth; p++ {
		for column := 0; column < columns; column++ {
			destination[column] += left[p] * right[p*columns+column]
		}
	}
}

func accumulatorInsideReduction(left, right, destination []float64, depth, columns int) {
	for column := 0; column < columns; column++ {
		for p := 0; p < depth; p++ {
			accumulator := 0.0
			accumulator += left[p] * right[p*columns+column]
			destination[column] = accumulator
		}
	}
}

func noPostReductionStore(left, right []float64, depth, columns int) float64 {
	var result float64
	for column := 0; column < columns; column++ {
		accumulator := 0.0
		for p := 0; p < depth; p++ {
			accumulator += left[p] * right[p*columns+column]
		}
		result += accumulator
	}
	return result
}

func differentStride(left, right, destination []float64, depth, columns, leading int) {
	for column := 0; column < columns; column++ {
		accumulator := 0.0
		for p := 0; p < depth; p++ {
			accumulator += left[p] * right[p*leading+column]
		}
		destination[column] = accumulator
	}
}

func gappedTile(left, right, destination []float64, depth, columns int) {
	for column := 0; column < columns; column += 3 {
		var acc0, acc2 float64
		for p := 0; p < depth; p++ {
			acc0 += left[p] * right[p*columns+column]
			acc2 += left[p] * right[p*columns+column+2]
		}
		destination[column], destination[column+2] = acc0, acc2
	}
}

func nestedStorage(left []float64, right [][]float64, destination []float64, depth, columns int) {
	for column := 0; column < columns; column++ {
		accumulator := 0.0
		for p := 0; p < depth; p++ {
			accumulator += left[p] * right[p][column]
		}
		destination[column] = accumulator
	}
}

//perfscan:cache-locality-validated complete shape campaigns retained this tile.
func validated(left, right, destination []float64, depth, columns int) {
	for column := 0; column < columns; column++ {
		accumulator := 0.0
		for p := 0; p < depth; p++ {
			accumulator += left[p] * right[p*columns+column]
		}
		destination[column] = accumulator
	}
}

var _ = []any{
	blocked4x2, exactQuarter, constantShape, wideTile, rowContiguous,
	accumulatorInsideReduction, noPostReductionStore, differentStride,
	gappedTile, nestedStorage, validated,
}
