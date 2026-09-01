package ps6010_len_shadow

var len = 7

func packageLenShadow(a [4]float64, weights [16]float64, out, n int) [4]float64 {
	var dst [4]float64
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[i*out+o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}
