package ps6010

import (
	"sort"
	"unsafe"
)

func ps6010Round19Invoke(callback func(int), output int) { callback(output) }

func ps6010Round19InvokePure(callback func(int), output int) { callback(output) }

func ps6010Round19InvokeMiddle(callback func(int), output int) {
	ps6010Round19InvokeLeaf(callback, output)
}

func ps6010Round19InvokeLeaf(callback func(int), output int) { callback(output) }

func ps6010Round19InvokeRecursive(callback func(int), output int) {
	if output < 0 {
		ps6010Round19InvokeRecursive(callback, output+1)
	}
	callback(output)
}

func ps6010Round19InvokeDeferred(callback func(int), output int) { defer callback(output) }

func ps6010Round19PureCallback(int) {}

func ps6010Round19ImportedCallback(output int) bool {
	ps6010Round17GlobalSlice[0] = float64(output)
	return false
}

func ps6010Round19PurePredicate(int) bool { return false }

func ps6010Round19GenericCallback[T ~int](output T) {
	ps6010Round17GlobalSlice[0] = float64(output)
}

var ps6010Round19Raw uintptr

func ps6010Round19UnsafeCallback(output int) {
	*(*float64)(unsafe.Pointer(ps6010Round19Raw)) = float64(output)
}

func ps6010Round19CallbackPair() (bool, func(int)) {
	return true, ps6010Round17MutateGlobalMiddle
}

type ps6010Round19Worker struct{}

func (ps6010Round19Worker) mutate(output int) {
	ps6010Round17GlobalSlice[0] = float64(output)
}

func (ps6010Round19Worker) pure(int) {}

type ps6010Round19Mutator interface{ mutate(int) }

func ps6010Round19InvokeGenericMethod[T ps6010Round19Mutator](value T, output int) {
	value.mutate(output)
}

type ps6010Round19CallbackHolder struct{ callback func(int) }

func (holder ps6010Round19CallbackHolder) invoke(output int) {
	holder.callback(output)
}

type ps6010Round19Sort struct{}

func (ps6010Round19Sort) Len() int           { return 1 }
func (ps6010Round19Sort) Less(int, int) bool { return false }
func (ps6010Round19Sort) Swap(left, right int) {
	ps6010Round17GlobalSlice[0] = float64(left + right)
}

func storedCallableFieldRound19(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	holder := struct{ mutate func(int) }{mutate: ps6010Round17MutateGlobalMiddle}
	for o := 0; o < out; o++ {
		holder.mutate(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func separatelyAssignedCallableRound19(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	var mutate func(int)
	mutate = ps6010Round17MutateGlobalMiddle
	for o := 0; o < out; o++ {
		mutate(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func reverseAliasChainRound19(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	var first, second func(int)
	first = second
	second = ps6010Round17MutateGlobalMiddle
	for o := 0; o < out; o++ {
		first(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func storedClosureRound19(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	holder := struct{ mutate func(int) }{
		mutate: func(output int) { ps6010Round17GlobalSlice[0] = float64(output) },
	}
	for o := 0; o < out; o++ {
		holder.mutate(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func namedCallbackArgumentRound19(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round19Invoke(ps6010Round17MutateGlobalMiddle, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func transitiveCallbackArgumentRound19(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round19InvokeMiddle(ps6010Round17MutateGlobalMiddle, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func recursiveCallbackArgumentRound19(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round19InvokeRecursive(ps6010Round17MutateGlobalMiddle, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func deferredCallbackArgumentRound19(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round19InvokeDeferred(ps6010Round17MutateGlobalMiddle, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func genericCallbackArgumentRound19(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round19Invoke(ps6010Round19GenericCallback[int], o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func unsafeCallbackArgumentRound19(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round19Invoke(ps6010Round19UnsafeCallback, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func bodylessCallbackArgumentRound19(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round19Invoke(ps6010Round17AssemblyLike, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func tupleCallbackRound19(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		_, callback := ps6010Round19CallbackPair()
		callback(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func storedMethodValueRound19(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	holder := struct{ mutate func(int) }{mutate: ps6010Round19Worker{}.mutate}
	for o := 0; o < out; o++ {
		holder.mutate(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func storedMethodExpressionRound19(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	mutate := ps6010Round19Worker.mutate
	worker := ps6010Round19Worker{}
	for o := 0; o < out; o++ {
		mutate(worker, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func callbackReceiverMethodRound19(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	holder := ps6010Round19CallbackHolder{callback: ps6010Round17MutateGlobalMiddle}
	for o := 0; o < out; o++ {
		holder.invoke(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func callbackReceiverMethodExpressionRound19(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	holder := ps6010Round19CallbackHolder{callback: ps6010Round17MutateGlobalMiddle}
	invoke := ps6010Round19CallbackHolder.invoke
	for o := 0; o < out; o++ {
		invoke(holder, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func genericMethodDispatchRound19(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	worker := ps6010Round19Worker{}
	for o := 0; o < out; o++ {
		ps6010Round19InvokeGenericMethod(worker, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func interfaceMethodAliasRound19(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	var dynamic ps6010Round19Mutator = ps6010Round19Worker{}
	mutate := dynamic.mutate
	for o := 0; o < out; o++ {
		mutate(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func importedInterfaceDispatchRound19(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	var dynamic sort.Interface = ps6010Round19Sort{}
	for o := 0; o < out; o++ {
		dynamic.Swap(o, 0)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func importedHigherOrderCallRound19(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		_ = sort.Search(out, ps6010Round19ImportedCallback)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func goCallbackRound19(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		go ps6010Round19Invoke(ps6010Round17MutateGlobalMiddle, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func channelCallbackRound19(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	callbacks := make(chan func(int), 1)
	callbacks <- ps6010Round17MutateGlobalMiddle
	for o := 0; o < out; o++ {
		callback := <-callbacks
		callback(o)
		callbacks <- callback
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func selectCallbackRound19(a, weights []float64, callbacks <-chan func(int), out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		var callback func(int)
		select {
		case callback = <-callbacks:
		default:
			callback = ps6010Round17MutateGlobalMiddle
		}
		if callback != nil {
			callback(o)
		}
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func typeSwitchCallbackRound19(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	var dynamic any = ps6010Round17MutateGlobalMiddle
	for o := 0; o < out; o++ {
		switch callback := dynamic.(type) {
		case func(int):
			callback(o)
		}
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func pureCallbackControlRound19(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round19InvokePure(ps6010Round19PureCallback, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}

func pureMethodExpressionControlRound19(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	pure := ps6010Round19Worker.pure
	worker := ps6010Round19Worker{}
	for o := 0; o < out; o++ {
		pure(worker, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}

func pureImportedCallbackControlRound19(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		_ = sort.Search(out, ps6010Round19PurePredicate)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}
