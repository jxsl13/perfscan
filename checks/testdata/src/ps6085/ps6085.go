package ps6085

var grid [2048][8]float32

func init() {
	for row := range grid {
		for lane := range grid[row] {
			grid[row][lane] = float32(row-8) + float32(lane)*0.25
		}
	}
}

func decodeRow(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	row := &grid[rowIndex&2047]
	var sum float32
	for lane := range 8 {
		sum += scale * (row[lane] + delta) // want "configured hot loop adds one of 2 exact float32 states"
	}
	return sum
}
