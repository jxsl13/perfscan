package ps6010

type ps6010Round13Holder struct {
	values []float64
}

func (holder ps6010Round13Holder) mutateRound13(output int) {
	holder.values[0] = float64(output)
}

type ps6010Round13NamedHolders []ps6010Round13Holder

func rangeValueReceiverRound13(values, weights []float64, out, n int) []float64 {
	dst := make([]float64, out)
	holders := ps6010Round13NamedHolders{{values: values}}
	for _, holder := range holders {
		for o := 0; o < out; o++ {
			holder.mutateRound13(o)
			acc := 0.0
			for i := 0; i < n; i++ {
				acc += values[i] * weights[i*out+o]
			}
			dst[o] = acc
		}
	}
	return dst
}

func rangeSliceValueRound13(values, weights []float64, out, n int) []float64 {
	dst := make([]float64, out)
	rows := [][]float64{values}
	for _, row := range rows {
		for o := 0; o < out; o++ {
			row[0] = float64(o)
			acc := 0.0
			for i := 0; i < n; i++ {
				acc += values[i] * weights[i*out+o]
			}
			dst[o] = acc
		}
	}
	return dst
}

func rangeMapKeyRound13(values, weights []float64, out, n int) []float64 {
	dst := make([]float64, out)
	entries := map[*ps6010Round13Holder]int{{values: values}: 1}
	for holder := range entries {
		for o := 0; o < out; o++ {
			holder.mutateRound13(o)
			acc := 0.0
			for i := 0; i < n; i++ {
				acc += values[i] * weights[i*out+o]
			}
			dst[o] = acc
		}
	}
	return dst
}

func typeSwitchValueReceiverRound13(values, weights []float64, out, n int) []float64 {
	dst := make([]float64, out)
	var boxed any = ps6010Round13Holder{values: values}
	switch holder := boxed.(type) {
	case ps6010Round13Holder:
		for o := 0; o < out; o++ {
			holder.mutateRound13(o)
			acc := 0.0
			for i := 0; i < n; i++ {
				acc += values[i] * weights[i*out+o]
			}
			dst[o] = acc
		}
	}
	return dst
}

func inlineChannelReceiveRound13(ch <-chan ps6010Round13Holder, values, weights []float64, out, n int) []float64 {
	dst := make([]float64, out)
	for o := 0; o < out; o++ {
		(<-ch).mutateRound13(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += values[i] * weights[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

func channelRangeReceiverRound13(ch <-chan ps6010Round13Holder, values, weights []float64, out, n int) []float64 {
	dst := make([]float64, out)
	for holder := range ch {
		for o := 0; o < out; o++ {
			holder.mutateRound13(o)
			acc := 0.0
			for i := 0; i < n; i++ {
				acc += values[i] * weights[i*out+o]
			}
			dst[o] = acc
		}
	}
	return dst
}

func iteratorRangeReceiverRound13(iterator func(func(ps6010Round13Holder) bool), values, weights []float64, out, n int) []float64 {
	dst := make([]float64, out)
	for holder := range iterator {
		for o := 0; o < out; o++ {
			holder.mutateRound13(o)
			acc := 0.0
			for i := 0; i < n; i++ {
				acc += values[i] * weights[i*out+o]
			}
			dst[o] = acc
		}
	}
	return dst
}

type ps6010Round13ValueHolder struct {
	values [8]float64
}

func (holder ps6010Round13ValueHolder) mutateRound13Value(output int) {
	holder.values[0] = float64(output)
}

func rangeValueOnlyControlRound13(values [8]float64, weights [64]float64, out, n int) [8]float64 {
	var dst [8]float64
	holders := [1]ps6010Round13ValueHolder{{values: values}}
	for _, holder := range holders {
		for o := 0; o < out; o++ {
			holder.mutateRound13Value(o)
			acc := 0.0
			for i := 0; i < n; i++ {
				acc += values[i] * weights[i*out+o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
			}
			dst[o] = acc
		}
	}
	return dst
}

func typeSwitchValueOnlyControlRound13(values [8]float64, weights [64]float64, out, n int) [8]float64 {
	var dst [8]float64
	var boxed any = ps6010Round13ValueHolder{values: values}
	switch holder := boxed.(type) {
	case ps6010Round13ValueHolder:
		for o := 0; o < out; o++ {
			holder.mutateRound13Value(o)
			acc := 0.0
			for i := 0; i < n; i++ {
				acc += values[i] * weights[i*out+o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
			}
			dst[o] = acc
		}
	}
	return dst
}
