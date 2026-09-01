package ps6010

// A slice parameter may point into a package array. Delaying these stores
// changes subsequent reads when the caller passes ps6010GlobalArray[:].
func globalArrayDestination(a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o]
		}
		ps6010GlobalArray[o] = acc
	}
}

func callGlobalArrayDestination(w []float64) {
	globalArrayDestination(ps6010GlobalArray[:], w, 4, 1)
}

func globalNamedArrayDestination(a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o]
		}
		ps6010GlobalNamedArray[o] = acc
	}
}

func callGlobalNamedArrayDestination(w []float64) {
	globalNamedArrayDestination(ps6010GlobalNamedArray[:], w, 4, 1)
}

func globalArrayFieldDestination(a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o]
		}
		ps6010GlobalArrayField.values[o] = acc
	}
}

func globalNestedArrayElementDestination(a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o]
		}
		ps6010GlobalArrayElements[0][o] = acc
	}
}

func localArraySliceAlias(w []float64, out, n int) [8]float64 {
	var dst [8]float64
	a := dst[:]
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

type ps6010SendChannel chan<- []float64
type ps6010ReceiveChannel <-chan []float64

func directionalChannelTransfer(send ps6010SendChannel, receive ps6010ReceiveChannel, w []float64, out, n int) []float64 {
	dst := make([]float64, out)
	send <- dst
	a := <-receive
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

func commaOKChannelTransfer(channel chan []float64, w []float64, out, n int) []float64 {
	dst := make([]float64, out)
	channel <- dst
	a, ok := <-channel
	_ = ok
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

func rangedChannelTransfer(channel <-chan []float64, w []float64, out, n int) {
	for a := range channel {
		var dst [8]float64
		for o := 0; o < out; o++ {
			acc := 0.0
			for i := 0; i < n; i++ {
				acc += a[i] * w[i*out+o]
			}
			dst[o] = acc
		}
		return
	}
}

func callbackChannelTransfer(channel chan []float64, w []float64, out, n int) []float64 {
	dst := make([]float64, out)
	send := func(values []float64) { channel <- values }
	send(dst)
	a := <-channel
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

func helperChannelTransfer(channel chan []float64, w []float64, out, n int) []float64 {
	dst := make([]float64, out)
	sendPS6010SliceFromOtherFile(channel, dst)
	a := <-channel
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o]
		}
		dst[o] = acc
	}
	return dst
}

func valueOnlyChannelControl(channel chan int, a, w []float64, out, n int) [8]float64 {
	channel <- n
	n = <-channel
	var dst [8]float64
valueChannelLoop:
	for o := 0; o < out; o++ {
		if o < 0 {
			continue valueChannelLoop
		}
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}

type ps6010AggregateLeaf struct {
	values []float64
}

type ps6010NestedAggregate struct {
	leaf ps6010AggregateLeaf
}

func mutatePS6010NestedAggregate(holder *ps6010NestedAggregate, output int) {
	holder.leaf.values[0] = float64(output)
}

func nestedStructAggregateAlias(a, w []float64, out, n int) [8]float64 {
	var dst [8]float64
	holder := ps6010NestedAggregate{leaf: ps6010AggregateLeaf{values: a}}
	for o := 0; o < out; o++ {
		mutatePS6010NestedAggregate(&holder, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[o]
		}
		dst[o] = acc
	}
	return dst
}

func fieldStoreAggregateAlias(a, w []float64, out, n int) [8]float64 {
	var dst [8]float64
	var holder ps6010NestedAggregate
	holder.leaf.values = a
	for o := 0; o < out; o++ {
		mutatePS6010NestedAggregate(&holder, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[o]
		}
		dst[o] = acc
	}
	return dst
}

func invokePS6010Callback(callback func()) { callback() }

func callbackStructAggregateAlias(a, w []float64, out, n int) [8]float64 {
	var dst [8]float64
	holder := ps6010NestedAggregate{leaf: ps6010AggregateLeaf{values: a}}
	for o := 0; o < out; o++ {
		invokePS6010Callback(func() { holder.leaf.values[0] = float64(o) })
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[o]
		}
		dst[o] = acc
	}
	return dst
}

func nestedSliceAggregateAlias(a, w []float64, out, n int) [8]float64 {
	var dst [8]float64
	rows := [][]float64{a}
	for o := 0; o < out; o++ {
		rows[0][0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[o]
		}
		dst[o] = acc
	}
	return dst
}

func elementStoreAggregateAlias(a, w []float64, out, n int) [8]float64 {
	var dst [8]float64
	rows := make([][]float64, 1)
	rows[0] = a
	for o := 0; o < out; o++ {
		rows[0][0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[o]
		}
		dst[o] = acc
	}
	return dst
}

func arrayAggregateAlias(a, w []float64, out, n int) [8]float64 {
	var dst [8]float64
	holders := [1]ps6010AggregateLeaf{{values: a}}
	for o := 0; o < out; o++ {
		holders[0].values[0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[o]
		}
		dst[o] = acc
	}
	return dst
}

func mapAggregateAlias(a, w []float64, out, n int) [8]float64 {
	var dst [8]float64
	holder := map[int][]float64{0: a}
	for o := 0; o < out; o++ {
		holder[0][0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[o]
		}
		dst[o] = acc
	}
	return dst
}

func interfaceAggregateAlias(a, w []float64, out, n int) [8]float64 {
	var dst [8]float64
	var holder any = a
	alias := holder.([]float64)
	for o := 0; o < out; o++ {
		alias[0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[o]
		}
		dst[o] = acc
	}
	return dst
}

func packageGlobalAggregateEscape(a, w []float64, out, n int) [8]float64 {
	var dst [8]float64
	ps6010EscapedSlice = a
	for o := 0; o < out; o++ {
		mutatePS6010EscapedSlice(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[o]
		}
		dst[o] = acc
	}
	return dst
}

func valueOnlyAggregateControl(a, w []float64, out, n int) [8]float64 {
	var dst [8]float64
	holder := struct{ value int }{value: n}
valueAggregateLoop:
	for o := 0; o < out; o++ {
		if o < 0 {
			continue valueAggregateLoop
		}
		holder.value = o
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}
