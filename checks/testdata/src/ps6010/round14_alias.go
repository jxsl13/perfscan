package ps6010

type ps6010Round14NamedSlice []float64
type ps6010Round14NamedArray [8]float64

type ps6010Round14PointerHolder struct {
	array *[8]float64
}

func sliceAndArrayPointerAliasRound14(a []float64, pointer *[8]float64, weights []float64, dst []float64, out, n int) []float64 {
	for o := 0; o < out; o++ {
		pointer[0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

func namedSliceAndArrayPointerAliasRound14(pointer *ps6010Round14NamedArray, a ps6010Round14NamedSlice, weights []float64, dst []float64, out, n int) []float64 {
	for o := 0; o < out; o++ {
		pointer[0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

func selectedArrayPointerAliasRound14(a []float64, holder ps6010Round14PointerHolder, weights []float64, dst []float64, out, n int) []float64 {
	for o := 0; o < out; o++ {
		holder.array[0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

func freshArrayPointerControlRound14(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	pointer := new([8]float64)
	for o := 0; o < out; o++ {
		pointer[0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[i*out+o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}

func incompatibleArrayPointerControlRound14(a, weights []float64, pointer *[8]int, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		pointer[0] = o
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[i*out+o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}
