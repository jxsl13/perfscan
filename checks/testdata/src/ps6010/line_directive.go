package ps6010

func duplicateLineDirectiveFixes(a [4]float64, w [16]float64, out, n int) ([4]float64, [4]float64) {
	var first, second [4]float64
//line generated.go:10:2
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += a[i] * w[i*out+o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)` `this operand does not vary with the output index p but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		first[o] = sum
	}
//line generated.go:10:2
	for p := 0; p < out; p++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += a[i] * w[i*out+p]
		}
		second[p] = sum
	}
	return first, second
}
