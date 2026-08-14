package ps2140silent

import "math"

// This is a textbook PS2140 match, but the test runs it with an EMPTY
// outputBufferElemTypes vocabulary, so the check must stay silent. The fixture
// deliberately carries no expectation comments. (A silent zero here is
// correct, not a missed finding.)

func SiLU(x []float64) []float64 {
	out := make([]float64, len(x))
	for i := range out {
		out[i] = x[i] / (1 + math.Exp(-x[i]))
	}
	return out
}
