package ps6112routernegative

func schedule(rows int) {
	bands := (rows + bandRows - 1) / bandRows
	for task := range bands {
		start := task * bandRows
		count := min(bandRows, rows-start)
		band(start, count)
	}
}

func fakeSchedule(rows int) {
	bands := (rows + bandRows - 1) / bandRows
	for task := range bands {
		start := task * bandRows
		count := min(bandRows, rows-start)
		fakeBand(start, count)
	}
}

func band(start, count int)            { kernelEntry(nil, start, count) }
func fakeBand(start, count int)        { fakeEntry(nil, start, count) }
func scalarTail(_ []float32, _, _ int) {}
