package ps6010

type ps6010Round11Mutator interface {
	mutateRound11(int)
}

type ps6010Round11Holder struct {
	values []float64
}

func (holder ps6010Round11Holder) mutateRound11(output int) {
	holder.values[0] = float64(output)
}

type ps6010Round11NamedHolder ps6010Round11Holder

func (holder ps6010Round11NamedHolder) mutateRound11Named(output int) {
	holder.values[0] = float64(output)
}

type ps6010Round11Wrapper struct {
	holder ps6010Round11Holder
}

func inlineConversionReceiver(values, weights []float64, out, n int) []float64 {
	dst := make([]float64, out)
	for o := 0; o < out; o++ {
		ps6010Round11Mutator(ps6010Round11Holder{values: values}).mutateRound11(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += values[i] * weights[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

func storedConversionMethodValue(values, weights []float64, out, n int) []float64 {
	dst := make([]float64, out)
	mutate := ps6010Round11Mutator(ps6010Round11Holder{values: values}).mutateRound11
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

func inlineNamedConversionReceiver(values, weights []float64, out, n int) []float64 {
	dst := make([]float64, out)
	for o := 0; o < out; o++ {
		ps6010Round11NamedHolder(ps6010Round11Holder{values: values}).mutateRound11Named(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += values[i] * weights[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

func nestedStructSelectorReceiver(values, weights []float64, out, n int) []float64 {
	dst := make([]float64, out)
	for o := 0; o < out; o++ {
		ps6010Round11Wrapper{holder: ps6010Round11Holder{values: values}}.holder.mutateRound11(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += values[i] * weights[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

func inlineMapIndexReceiver(values, weights []float64, out, n int) []float64 {
	dst := make([]float64, out)
	for o := 0; o < out; o++ {
		map[int]ps6010Round11Holder{0: {values: values}}[0].mutateRound11(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += values[i] * weights[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

func inlineArrayIndexReceiver(values, weights []float64, out, n int) []float64 {
	dst := make([]float64, out)
	for o := 0; o < out; o++ {
		[1]ps6010Round11Holder{{values: values}}[0].mutateRound11(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += values[i] * weights[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

func inlineSliceIndexReceiver(values, weights []float64, out, n int) []float64 {
	dst := make([]float64, out)
	for o := 0; o < out; o++ {
		[]ps6010Round11Holder{{values: values}}[0].mutateRound11(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += values[i] * weights[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

func directCompositeReceiver(values, weights []float64, out, n int) []float64 {
	dst := make([]float64, out)
	for o := 0; o < out; o++ {
		ps6010Round11Holder{values: values}.mutateRound11(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += values[i] * weights[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

func storedCompositeMethodValue(values, weights []float64, out, n int) []float64 {
	dst := make([]float64, out)
	mutate := (ps6010Round11Holder{values: values}).mutateRound11
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

// A value receiver containing only an array receives an independent copy.
// Mutating that copy must not suppress a valid diagnostic on the source array.
type ps6010Round11ValueMutator interface {
	mutateRound11Value(int)
}

type ps6010Round11ValueHolder struct {
	values [8]float64
}

func (holder ps6010Round11ValueHolder) mutateRound11Value(output int) {
	holder.values[0] = float64(output)
}

func valueOnlyConversionControl(values [8]float64, weights [64]float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round11ValueMutator(ps6010Round11ValueHolder{values: values}).mutateRound11Value(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += values[i] * weights[i*out+o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}
