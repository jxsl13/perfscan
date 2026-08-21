package ps6076silent

func parallelFor(n int, body func(lo, hi int)) { body(0, n) }

func packedPerBand(source []float64, rows int) {
	parallelFor(rows, func(lo, hi int) {
		packed := make([]float64, rows)
		copy(packed, source)
		_, _, _ = lo, hi, packed
	})
}
