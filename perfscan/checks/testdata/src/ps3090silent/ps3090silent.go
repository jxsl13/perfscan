package ps3090silent

// A textbook capturing-closure-to-fan-out-helper match, but the test runs it
// with an EMPTY fanOutHelpers vocabulary, so the check must stay silent. The
// fixture deliberately carries no expectation comments.

func parallelFor(n int, body func(lo, hi int)) { body(0, n) }

func SiLU(dst, src []float64) {
	parallelFor(len(dst), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			dst[i] = src[i]
		}
	})
}
