package ps6112silent

const bandRows = 30

func schedule(rows int) {
	bands := (rows + bandRows - 1) / bandRows
	for task := range bands {
		start := task * bandRows
		count := min(bandRows, rows-start)
		band(start, count)
	}
}

func band(start, count int) { kernelEntry(nil, start, count) }

func kernelEntry(data []float32, lo, hi int) {
	i := lo
	for ; i+3 < hi; i += 4 {
		tile4(data, i)
	}
	if i < hi {
		scalarTail(data, i, hi)
	}
}

func tile4(_ []float32, _ int)         {}
func scalarTail(_ []float32, _, _ int) {}
