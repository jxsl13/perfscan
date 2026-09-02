package ps6010

import "ps6010returnhelper"

type ps6010Round23Carrier struct {
	values []float64
}

func (carrier ps6010Round23Carrier) mutate(_ int) {
	carrier.values[0]++
}

func ps6010Round23MutateExpanded(_ int, values []float64) {
	values[0]++
}

func ps6010Round23MutateFixedVariadic(_ int, values ...[]float64) {
	values[len(values)-1][0]++
}

type ps6010Round23NamedSlice []float64

type ps6010Round23NamedPointer *float64

func ps6010Round23MutateNamed(_ int, values ps6010Round23NamedSlice) {
	values[0]++
}

func ps6010Round23MutatePointer(_ int, value ps6010Round23NamedPointer) {
	(*value)++
}

type ps6010Round23GenericHolder[T any] struct {
	value T
}

type ps6010Round23CallbackOnly struct {
	callback func(int)
}

func ps6010Round23MutateGeneric[T ~[]float64](_ int, holder ps6010Round23GenericHolder[T]) {
	holder.value[0]++
}

func ps6010Round23InvokeCallbackOnly(_ int, holder ps6010Round23CallbackOnly) {
	holder.callback(0)
}

func expandedStorageParameterRound23(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	pair := func() (int, []float64) { return 0, a }
	for o := 0; o < out; o++ {
		ps6010Round23MutateExpanded(pair())
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func expandedStorageMethodReceiverRound23(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	pair := func() (ps6010Round23Carrier, int) {
		return ps6010Round23Carrier{values: a}, 0
	}
	invoke := ps6010Round23Carrier.mutate
	for o := 0; o < out; o++ {
		invoke(pair())
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func expandedStorageNamedSliceRound23(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	pair := func() (int, ps6010Round23NamedSlice) {
		return 0, ps6010Round23NamedSlice(a)
	}
	for o := 0; o < out; o++ {
		ps6010Round23MutateNamed(pair())
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func expandedStorageGenericHolderRound23(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	pair := func() (int, ps6010Round23GenericHolder[[]float64]) {
		return 0, ps6010Round23GenericHolder[[]float64]{value: a}
	}
	for o := 0; o < out; o++ {
		ps6010Round23MutateGeneric(pair())
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func expandedStorageNamedPointerRound23(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	pair := func() (int, ps6010Round23NamedPointer) {
		return 0, ps6010Round23NamedPointer(&a[0])
	}
	for o := 0; o < out; o++ {
		ps6010Round23MutatePointer(pair())
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func expandedStorageVariadicRound23(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	pair := func() (int, []float64) { return 0, a }
	for o := 0; o < out; o++ {
		ps6010Round23MutateFixedVariadic(pair())
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func expandedStorageImportedRound23(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	pair := func() (int, []float64) { return 0, a }
	for o := 0; o < out; o++ {
		ps6010returnhelper.MutateExpandedSlice(pair())
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func expandedStorageImportedVariadicRound23(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	pair := func() (int, []float64) { return 0, a }
	for o := 0; o < out; o++ {
		ps6010returnhelper.MutateExpandedVariadic(pair())
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func expandedStorageImportedMethodRound23(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	pair := func() (ps6010returnhelper.ExpandedCarrier, int) {
		return ps6010returnhelper.ExpandedCarrier{Values: a}, 0
	}
	invoke := ps6010returnhelper.ExpandedCarrier.MutateExpanded
	for o := 0; o < out; o++ {
		invoke(pair())
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func expandedStorageIncompatibleControlRound23(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	ints := []int{1}
	pair := func() (int, []int) { return 0, ints }
	mutate := func(_ int, values []int) { values[0]++ }
	for o := 0; o < out; o++ {
		mutate(pair())
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}

func expandedCallableOnlyControlRound23(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	pair := func() (int, ps6010Round23CallbackOnly) {
		return 0, ps6010Round23CallbackOnly{callback: ps6010Round21ControlPureCallback}
	}
	for o := 0; o < out; o++ {
		ps6010Round23InvokeCallbackOnly(pair())
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}
