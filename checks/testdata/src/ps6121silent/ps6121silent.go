package ps6121silent

func parallelFor(n int, body func(int, int)) { body(0, n) }
func concrete(x, y float64) float64          { return x + y }
func helper(out []float64, fn func(float64, float64) float64) {
	parallelFor(len(out), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			out[i] = fn(out[i], 1)
		}
	})
}
func caller(out []float64) { helper(out, concrete) }
