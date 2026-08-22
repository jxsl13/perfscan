package ps2007

type biasser struct{}

func (biasser) Bias(q, k int) []float64 { return make([]float64, q*k) }

func use(float64) {}

func oneRow(b biasser, pos, kk, heads, k, h int) {
	full := b.Bias(pos+1, pos+1) // want `Bias is called with pos \+ 1 twice, building a square result, and pos then indexes it — an N×N object materialized to consume one row; compute the row directly from the callee's per-element rule \(verify bit-identity for ±Inf/NaN/-0\)`
	use(full[(pos*kk+k)*heads+h])
}

// Derivation chain: fs is derived from full and indexed by pos.
func derivedChain(b biasser, pos int) {
	full := b.Bias(pos+1, pos+1) // want `Bias is called with pos \+ 1 twice, building a square result, and pos then indexes it — an N×N object materialized to consume one row; compute the row directly from the callee's per-element rule \(verify bit-identity for ±Inf/NaN/-0\)`
	fs := full[:]
	use(fs[pos])
}

// The position bounds a loop: the square result is walked in full, silent.
func consumedWhole(b biasser, dseq int) float64 {
	mask := b.Bias(dseq, dseq)
	s := 0.0
	for i := 0; i < dseq; i++ {
		s += mask[i]
	}
	return s
}

// Constant sizes are a fixed small shape, silent.
func fixedShape(b biasser) []float64 {
	return b.Bias(2, 2)
}

// Different size arguments: a genuine rectangular build, silent.
func rectangular(b biasser, q, k int) []float64 {
	return b.Bias(q, k)
}
