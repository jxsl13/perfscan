package ps2001

type tensor struct{ data []float64 }

func New(n int) *tensor   { return &tensor{data: make([]float64, n)} }
func Zeros(n int) *tensor { return &tensor{data: make([]float64, n)} }

func process(t *tensor) {}

func perItem(batches [][]float64, n int) {
	for range batches {
		tmp := Zeros(n) // want `allocator Zeros called inside a loop allocates every iteration; hoist and reuse the buffer or take scratch from a pool`
		process(tmp)
	}
}

func hoisted(batches [][]float64, n int) {
	tmp := New(n)
	for range batches {
		process(tmp)
	}
}
