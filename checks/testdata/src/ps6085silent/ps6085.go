package ps6085silent

var grid [16][8]float32

func decodeRow(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	row := &grid[rowIndex&15]
	var sum float32
	for lane := range 8 {
		sum += scale * (row[lane] + delta)
	}
	return sum
}
