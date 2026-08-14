package ps2140

import "math"

// Vocabulary for this fixture: outputBufferElemTypes = {float64, Scalar}.

type Scalar float64

// --- POSITIVES: input-sized make, single full-overwrite loop, returned ---

// Range-loop form over a len(param)-sized buffer.
func SiLU(x []float64) []float64 {
	out := make([]float64, len(x)) // want `operation allocates an input-sized \[\]float64 result, fully overwrites it`
	for i := range out {
		out[i] = x[i] / (1 + math.Exp(-x[i]))
	}
	return out
}

// Canonical for-loop form: for i := 0; i < len(out); i++.
func Neg(x []float64) []float64 {
	out := make([]float64, len(x)) // want `operation allocates an input-sized \[\]float64 result`
	for i := 0; i < len(out); i++ {
		out[i] = -x[i]
	}
	return out
}

// Named element type in the vocabulary; size from an int parameter.
func Ramp(n int, base Scalar) []Scalar {
	out := make([]Scalar, n) // want `operation allocates an input-sized \[\]Scalar result`
	for i := range out {
		out[i] = base + Scalar(i)
	}
	return out
}

// --- GUARDS: none of the following may be reported ---

// An Into sibling already exists — the destination API is offered.
func Tanh(x []float64) []float64 {
	out := make([]float64, len(x))
	for i := range out {
		out[i] = math.Tanh(x[i])
	}
	return out
}

func TanhInto(dst, x []float64) {
	for i := range dst {
		dst[i] = math.Tanh(x[i])
	}
}

// A New* constructor: allocating a fresh value is its whole job.
func NewBuffer(n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = 0
	}
	return out
}

// Constant size: not derived from an input.
func Const(x []float64) []float64 {
	out := make([]float64, 8)
	for i := range out {
		out[i] = x[0]
	}
	return out
}

// A read of the buffer (out[0] on a RHS) — not a clean full overwrite.
func ReadFirst(x []float64) []float64 {
	out := make([]float64, len(x))
	for i := range out {
		out[i] = x[i]
	}
	sink := out[0]
	_ = sink
	return out
}

// += reads the zero value before writing.
func Accumulate(x []float64) []float64 {
	out := make([]float64, len(x))
	for i := range out {
		out[i] += x[i]
	}
	return out
}

// The buffer is passed to a call (it may be read there / escapes).
func Passed(x []float64) []float64 {
	out := make([]float64, len(x))
	fill(out)
	return out
}

func fill(s []float64) {
	for i := range s {
		s[i] = 1
	}
}

// Returns a slice of the buffer, not the buffer itself.
func Sliced(x []float64) []float64 {
	out := make([]float64, len(x))
	for i := range out {
		out[i] = x[i]
	}
	return out[1:]
}

// Element type not in the vocabulary.
func Iota(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

// Two distinct write loops — not a single overwrite pass.
func TwoLoops(x []float64) []float64 {
	out := make([]float64, len(x))
	for i := range out {
		out[i] = x[i]
	}
	for i := range out {
		out[i] = 0
	}
	return out
}

// The write index is a fixed slot, not the loop variable (partial write).
func WrongIndex(x []float64) []float64 {
	out := make([]float64, len(x))
	for i := range out {
		out[0] = x[i]
	}
	return out
}
