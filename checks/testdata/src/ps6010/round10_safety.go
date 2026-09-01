package ps6010

type ps6010LeftHolder struct{ values []float64 }
type ps6010RightHolder struct{ other []float64 }

// The two non-convertible aggregate parameters can still retain slice headers
// whose backing arrays overlap. A write through right therefore invalidates
// the apparently invariant left container.
func distinctStructSliceAlias(left ps6010LeftHolder, right ps6010RightHolder, w []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		right.other[0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += left.values[i] * w[o]
		}
		dst[o] = acc
	}
	return dst
}

type ps6010NestedLeft struct {
	layers [1]struct{ values []float64 }
}

type ps6010NestedRight struct {
	layers [2]struct{ other []float64 }
}

func nestedArrayStructAlias(left ps6010NestedLeft, right ps6010NestedRight, w []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		right.layers[0].other[0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += left.layers[0].values[i] * w[o]
		}
		dst[o] = acc
	}
	return dst
}

type ps6010MapLeft struct{ values map[int][]float64 }
type ps6010MapRight struct{ other map[string][]float64 }

func nestedMapValueAlias(left ps6010MapLeft, right ps6010MapRight, w []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		right.other["row"][0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += left.values[0][i] * w[o]
		}
		dst[o] = acc
	}
	return dst
}

type ps6010PointerLeft struct{ values []float64 }
type ps6010PointerRight struct{ other []float64 }
type ps6010PointerLeftRoot struct{ leaf *ps6010PointerLeft }
type ps6010PointerRightRoot struct{ leaf *ps6010PointerRight }

func nestedPointerAlias(left ps6010PointerLeftRoot, right ps6010PointerRightRoot, w []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		right.leaf.other[0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += left.leaf.values[i] * w[o]
		}
		dst[o] = acc
	}
	return dst
}

type ps6010InterfaceLeft struct{ values any }
type ps6010InterfaceRight struct{ other any }

func nestedInterfaceAlias(left ps6010InterfaceLeft, right ps6010InterfaceRight, w []float64, out, n int) [8]float64 {
	var dst [8]float64
	leftValues := left.values.([]float64)
	rightValues := right.other.([]float64)
	for o := 0; o < out; o++ {
		rightValues[0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += leftValues[i] * w[o]
		}
		dst[o] = acc
	}
	return dst
}

type ps6010MethodHolder struct{ values []float64 }

func (holder ps6010MethodHolder) mutate(output int) {
	holder.values[0] = float64(output)
}

func (holder *ps6010MethodHolder) mutatePointer(output int) {
	holder.values[0] = float64(output)
}

func storedValueMethodAlias(a, w []float64, out, n int) [8]float64 {
	var dst [8]float64
	holder := ps6010MethodHolder{values: a}
	mutate := holder.mutate
	for o := 0; o < out; o++ {
		mutate(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[o]
		}
		dst[o] = acc
	}
	return dst
}

// An address-of composite has no named source root of its own, but the pointer
// still retains the slice stored in the literal. A stored pointer-receiver
// method value can mutate that hidden descendant.
func storedPointerMethodAlias(a, w []float64, out, n int) [8]float64 {
	var dst [8]float64
	holder := &ps6010MethodHolder{values: a}
	mutate := holder.mutatePointer
	for o := 0; o < out; o++ {
		mutate(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[o]
		}
		dst[o] = acc
	}
	return dst
}

func immediatePointerMethodAlias(a, w []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		(&ps6010MethodHolder{values: a}).mutatePointer(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[o]
		}
		dst[o] = acc
	}
	return dst
}

func callbackParameterAlias(a, w []float64, mutate func(int), out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		mutate(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[o]
		}
		dst[o] = acc
	}
	return dst
}

type ps6010CallbackBox struct{ callback func(int) }

func callbackFieldAlias(a, w []float64, out, n int) [8]float64 {
	var dst [8]float64
	holder := ps6010MethodHolder{values: a}
	box := ps6010CallbackBox{callback: holder.mutate}
	for o := 0; o < out; o++ {
		box.callback(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[o]
		}
		dst[o] = acc
	}
	return dst
}

func callbackInterfaceAlias(a, w []float64, out, n int) [8]float64 {
	var dst [8]float64
	holder := ps6010MethodHolder{values: a}
	var callback any = holder.mutate
	for o := 0; o < out; o++ {
		callback.(func(int))(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[o]
		}
		dst[o] = acc
	}
	return dst
}

// A method expression receives the value explicitly; its argument provenance
// must continue to invalidate the captured slice field.
func methodExpressionAlias(a, w []float64, out, n int) [8]float64 {
	var dst [8]float64
	holder := ps6010MethodHolder{values: a}
	mutate := ps6010MethodHolder.mutate
	for o := 0; o < out; o++ {
		mutate(holder, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[o]
		}
		dst[o] = acc
	}
	return dst
}

// Separately copied value-only aggregates cannot share storage. Keep the safe
// diagnostic even though the sibling aggregate is mutated.
type ps6010ValueLeft struct{ values [8]float64 }
type ps6010ValueRight struct{ other [8]float64 }

func valueOnlyNestedAggregateControl(left ps6010ValueLeft, right ps6010ValueRight, w []float64, out, n int) [8]float64 {
	var dst [8]float64
valueAggregateRound10Loop:
	for o := 0; o < out; o++ {
		if o < 0 {
			continue valueAggregateRound10Loop
		}
		right.other[0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += left.values[i] * w[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}
