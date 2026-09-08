package ps6112aligned

func schedule(rows int) {
	bands := (rows + bandRows - 1) / bandRows
	for task := range bands {
		start := task * bandRows
		count := min(bandRows, rows-start)
		band(start, count)
	}
}

func band(start, count int)            { kernelEntry(nil, start, count) }
func scalarTail(_ []float32, _, _ int) {}
