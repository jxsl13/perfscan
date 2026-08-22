package ps3059

func parallelFor(n int, f func(int)) {
	for i := 0; i < n; i++ {
		f(i)
	}
}

// Every write lands through obase, derived from b: bandable, and the
// direct-name check (PS3034) is blind here.
func transpose(dst, src []float64, batch, out int) {
	for b := 0; b < batch; b++ { // want `every write in this serial nest lands through a base derived from outer variable b — bandable, and a fan-out helper exists in this package; check the band owns disjoint output, then gate with BOTH a bit-exact digest and -race`
		obase := b * out
		for j := 0; j < out; j++ {
			dst[obase+j] = src[j*batch+b]
		}
	}
}

// Direct-name writes are PS3034's finding, not this check's: silent here.
func directName(dst, src [][]float64, batch, d int) {
	for b := 0; b < batch; b++ {
		for j := 0; j < d; j++ {
			dst[b][j] = src[b][j]
		}
	}
}

// A write outside the derivation chain is not provably disjoint: silent.
func sharedWrite(dst, sum []float64, batch, out int) {
	for b := 0; b < batch; b++ {
		obase := b * out
		for j := 0; j < out; j++ {
			dst[obase+j] = 1
			sum[j] += 1
		}
	}
}

// Fans out already: silent.
func banded(dst, src []float64, batch, out int) {
	parallelFor(batch, func(b int) {
		obase := b * out
		for j := 0; j < out; j++ {
			dst[obase+j] = src[j*batch+b]
		}
	})
}
