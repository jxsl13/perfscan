package ps6010

type ps6010Round15NamedSlice []float64
type ps6010Round15NamedPointer *float64

type ps6010Round15ElementHolder struct {
	element *float64
}

type ps6010Round15ArrayHolder struct {
	array *[8]float64
}

func elementPointerAliasRound15(a []float64, pointer *float64, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		*pointer = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func namedElementPointerAliasRound15(a ps6010Round15NamedSlice, pointer ps6010Round15NamedPointer, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		*pointer = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func selectedElementPointerAliasRound15(a []float64, holder ps6010Round15ElementHolder, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		*holder.element = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func arrayAndElementPointersRound15(a *[8]float64, pointer *float64, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		*pointer = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func selectedArrayAndElementPointersRound15(holder ps6010Round15ArrayHolder, pointer *float64, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		*pointer = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += holder.array[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func freshElementPointerControlRound15(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	pointer := new(float64)
	for o := 0; o < out; o++ {
		*pointer = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}

func incompatibleElementPointerControlRound15(a, weights []float64, pointer *int, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		*pointer = o
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}

func capturedReceiveAliasRound15(a []float64, channel <-chan []float64, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	func() {
		alias := <-channel
		for o := 0; o < out; o++ {
			alias[0] = float64(o)
			acc := 0.0
			for i := 0; i < n; i++ {
				acc += a[i] * weights[o]
			}
			dst[o] = acc
		}
	}()
	return dst
}

func nestedCapturedReceiveAliasRound15(a []float64, channel <-chan []float64, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	func() {
		func() {
			alias := <-channel
			for o := 0; o < out; o++ {
				alias[0] = float64(o)
				acc := 0.0
				for i := 0; i < n; i++ {
					acc += a[i] * weights[o]
				}
				dst[o] = acc
			}
		}()
	}()
	return dst
}

func capturedLocalReceiveAliasRound15(a []float64, channel <-chan []float64, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	local := a
	func() {
		alias := <-channel
		for o := 0; o < out; o++ {
			alias[0] = float64(o)
			acc := 0.0
			for i := 0; i < n; i++ {
				acc += local[i] * weights[o]
			}
			dst[o] = acc
		}
	}()
	return dst
}

func incompatibleCapturedReceiveControlRound15(a, weights []float64, channel <-chan []int, out, n int) [8]float64 {
	var dst [8]float64
	func() {
		alias := <-channel
		for o := 0; o < out; o++ {
			alias[0] = o
			acc := 0.0
			for i := 0; i < n; i++ {
				acc += a[i] * weights[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
			}
			dst[o] = acc
		}
	}()
	return dst
}
