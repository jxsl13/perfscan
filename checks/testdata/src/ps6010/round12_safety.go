package ps6010

type ps6010Round12Mutator interface {
	mutateRound12(int)
}

type ps6010Round12Holder struct {
	values []float64
}

func (holder ps6010Round12Holder) mutateRound12(output int) {
	holder.values[0] = float64(output)
}

type ps6010Round12Holders []ps6010Round12Holder

type ps6010Round12Nested struct {
	holders [1]ps6010Round12Holder
}

// append copies a reference-bearing element into the result. A stored method
// value loaded through that result can therefore mutate values.
func appendStoredReceiverRound12(values, weights []float64, out, n int) []float64 {
	dst := make([]float64, out)
	holders := append([]ps6010Round12Holder(nil), ps6010Round12Holder{values: values})
	mutate := holders[0].mutateRound12
	for o := 0; o < out; o++ {
		mutate(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += values[i] * weights[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

// A variadic append from a named slice copies its reference-bearing elements.
func appendNamedVariadicRound12(values, weights []float64, out, n int) []float64 {
	dst := make([]float64, out)
	source := ps6010Round12Holders{{values: values}}
	holders := append(ps6010Round12Holders(nil), source...)
	mutate := holders[0].mutateRound12
	for o := 0; o < out; o++ {
		mutate(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += values[i] * weights[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

// Interface elements retain their recursively reference-bearing dynamic value.
func appendInterfaceReceiverRound12(values, weights []float64, out, n int) []float64 {
	dst := make([]float64, out)
	holders := append([]any(nil), ps6010Round12Holder{values: values})
	mutate := holders[0].(ps6010Round12Holder).mutateRound12
	for o := 0; o < out; o++ {
		mutate(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += values[i] * weights[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

// copy transfers reference-bearing elements even when the source is inline.
func copyInlineReceiverRound12(values, weights []float64, out, n int) []float64 {
	dst := make([]float64, out)
	holders := make([]ps6010Round12Holder, 1)
	copy(holders, []ps6010Round12Holder{{values: values}})
	mutate := holders[0].mutateRound12
	for o := 0; o < out; o++ {
		mutate(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += values[i] * weights[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

// Named slices and nested array/struct elements keep the same descendants.
func copyNamedNestedRound12(values, weights []float64, out, n int) []float64 {
	dst := make([]float64, out)
	source := []ps6010Round12Nested{{holders: [1]ps6010Round12Holder{{values: values}}}}
	target := make([]ps6010Round12Nested, 1)
	copy(target[:], source[:])
	mutate := target[0].holders[0].mutateRound12
	for o := 0; o < out; o++ {
		mutate(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += values[i] * weights[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

type ps6010Round12ValueHolder struct {
	values [8]float64
}

func (holder ps6010Round12ValueHolder) mutateRound12Value(output int) {
	holder.values[0] = float64(output)
}

// append and copy duplicate value-only elements. Mutating the copies cannot
// affect the source array, so both otherwise canonical operands remain valid.
func appendValueOnlyControlRound12(values [8]float64, weights [64]float64, out, n int) [8]float64 {
	var dst [8]float64
	holders := append([]ps6010Round12ValueHolder(nil), ps6010Round12ValueHolder{values: values})
	for o := 0; o < out; o++ {
		holders[0].mutateRound12Value(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += values[i] * weights[i*out+o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}

func copyValueOnlyControlRound12(values [8]float64, weights [64]float64, out, n int) [8]float64 {
	var dst [8]float64
	holders := make([]ps6010Round12ValueHolder, 1)
	copy(holders, []ps6010Round12ValueHolder{{values: values}})
	for o := 0; o < out; o++ {
		holders[0].mutateRound12Value(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += values[i] * weights[i*out+o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}
