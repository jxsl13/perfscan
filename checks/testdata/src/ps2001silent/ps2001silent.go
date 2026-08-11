package ps2001silent

type tensor struct{ data []float64 }

func Zeros(n int) *tensor { return &tensor{data: make([]float64, n)} }

func process(t *tensor) {}

// With an empty vocabulary PS2001 must stay silent even on this shape.
func perItem(batches [][]float64, n int) {
	for range batches {
		tmp := Zeros(n)
		process(tmp)
	}
}
