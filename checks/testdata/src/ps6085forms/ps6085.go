package ps6085forms

var (
	directGrid  [4][8]float32
	valueGrid   [4][8]float32
	float64Grid [4][8]float64
	methodGrid  [4][8]float32
	countedGrid [4][8]float32
)

func direct(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	var sum float32
	for lane := range 8 {
		sum += scale * (directGrid[rowIndex&3][lane] + delta) // want "configured hot loop adds one of 2 exact float32 states"
	}
	return sum
}

func byValue(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	row := valueGrid[rowIndex&3]
	var sum float32
	for lane := range 8 {
		sum += scale * (row[lane] + delta) // want "configured hot loop adds one of 2 exact float32 states"
	}
	return sum
}

func float64Row(rowIndex int, negative bool, scale float64) float64 {
	delta := float64(0.125)
	if negative {
		delta = float64(-0.125)
	}
	row := &float64Grid[rowIndex&3]
	var sum float64
	for lane := range 8 {
		sum += scale * (row[lane] + delta) // want "configured hot loop adds one of 2 exact float64 states"
	}
	return sum
}

type decoder struct{}

func (decoder) methodRow(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	row := methodGrid[rowIndex&3]
	var sum float32
	for lane := range 8 {
		sum += scale * (row[lane] + delta) // want "configured hot loop adds one of 2 exact float32 states"
	}
	return sum
}

func counted(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	row := &countedGrid[rowIndex&3]
	var sum float32
	for lane := 0; lane < 8; lane++ {
		sum += scale * (row[lane] + delta) // want "configured hot loop adds one of 2 exact float32 states"
	}
	return sum
}
