package ps1003

func predictBatch(rows [][]float64) []float64 { return make([]float64, len(rows)) }

func perRow(rows [][]float64) []float64 {
	out := make([]float64, 0, len(rows))
	for _, row := range rows {
		out = append(out, predictBatch([][]float64{row})[0]) // want `batch API predictBatch called with a single-element slice literal in a loop; batch the items once outside the loop or use a single-item API`
	}
	return out
}

func batched(rows [][]float64) []float64 {
	return predictBatch(rows)
}

func onceOutsideLoop(row []float64) []float64 {
	return predictBatch([][]float64{row})
}
