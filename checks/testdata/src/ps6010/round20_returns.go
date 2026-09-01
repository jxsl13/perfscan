package ps6010

import "ps6010returnhelper"

func ps6010Round20MutatingFactory() func(int) {
	return ps6010Round17MutateGlobalMiddle
}

func ps6010Round20PureFactory() func(int) {
	return ps6010Round19PureCallback
}

func ps6010Round20MiddleFactory() func(int) {
	return ps6010Round20MutatingFactory()
}

func ps6010Round20RecursiveFactory() func(int) {
	if false {
		return ps6010Round20RecursiveFactory()
	}
	return ps6010Round17MutateGlobalMiddle
}

func ps6010Round20NamedFactory() (callback func(int)) {
	callback = ps6010Round17MutateGlobalMiddle
	return
}

func ps6010Round20DeferredCallback(output int) {
	defer ps6010Round17MutateGlobalMiddle(output)
}

func ps6010Round20DeferredFactory() func(int) {
	return ps6010Round20DeferredCallback
}

func ps6010Round20ClosureFactory() func(int) {
	return func(output int) {
		ps6010Round17GlobalSlice[0] = float64(output)
	}
}

func ps6010Round20CapturingFactory(values []float64) func(int) {
	return func(output int) {
		values[0] = float64(output)
	}
}

func ps6010Round20BodylessFactory() func(int)

func ps6010Round20MutatingSecondFactory() (func(int), func(int)) {
	return ps6010Round19PureCallback, ps6010Round17MutateGlobalMiddle
}

func ps6010Round20MutatingBoxFactory() ps6010returnhelper.CallbackBox {
	return ps6010returnhelper.CallbackBox{Callback: ps6010Round17MutateGlobalMiddle}
}

type ps6010Round20FactoryWorker struct{}

type ps6010Round20CapturedWorker struct {
	values []float64
}

func (ps6010Round20FactoryWorker) mutatingFactory() func(int) {
	return ps6010Round17MutateGlobalMiddle
}

func (ps6010Round20FactoryWorker) pureFactory() func(int) {
	return ps6010Round19PureCallback
}

func ps6010Round20ReturnedMethodFactory() func(int) {
	return ps6010Round20FactoryWorker{}.returnedMutation
}

func (ps6010Round20FactoryWorker) returnedMutation(output int) {
	ps6010Round17GlobalSlice[0] = float64(output)
}

func (worker ps6010Round20CapturedWorker) mutate(output int) {
	worker.values[0] = float64(output)
}

func ps6010Round20CapturedMethodFactory(values []float64) func(int) {
	return ps6010Round20CapturedWorker{values: values}.mutate
}

func importedReturnedCallbackRound20(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010returnhelper.CallFactory(ps6010Round20MutatingFactory, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func importedGenericReturnedCallbackRound20(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010returnhelper.CallFactoryGeneric[func() func(int)](ps6010Round20MutatingFactory, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func importedStoredReturnedCallbackRound20(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	var factory func() func(int)
	factory = ps6010Round20MutatingFactory
	for o := 0; o < out; o++ {
		ps6010returnhelper.CallFactory(factory, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func importedFieldReturnedCallbackRound20(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	holder := struct{ factory func() func(int) }{factory: ps6010Round20MutatingFactory}
	for o := 0; o < out; o++ {
		ps6010returnhelper.CallFactory(holder.factory, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func importedMethodReturnedCallbackRound20(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	factory := ps6010Round20FactoryWorker{}.mutatingFactory
	for o := 0; o < out; o++ {
		ps6010returnhelper.CallFactory(factory, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func importedInterfaceReturnedCallbackRound20(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	var dynamic any = ps6010Round20MutatingFactory
	factory := dynamic.(func() func(int))
	for o := 0; o < out; o++ {
		ps6010returnhelper.CallFactory(factory, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func importedTransitiveReturnedCallbackRound20(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010returnhelper.CallFactory(ps6010Round20MiddleFactory, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func importedRecursiveReturnedCallbackRound20(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010returnhelper.CallFactory(ps6010Round20RecursiveFactory, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func importedNamedReturnedCallbackRound20(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010returnhelper.CallFactory(ps6010Round20NamedFactory, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func importedAggregateReturnedCallbackRound20(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010returnhelper.CallBoxFactory(ps6010Round20MutatingBoxFactory, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func importedPureReturnedCallbackControlRound20(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010returnhelper.CallFactory(ps6010Round20PureFactory, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}

func importedPureStoredReturnedCallbackControlRound20(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	var factory func() func(int)
	factory = ps6010Round20PureFactory
	for o := 0; o < out; o++ {
		ps6010returnhelper.CallFactory(factory, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}

func importedPureMethodReturnedCallbackControlRound20(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	factory := ps6010Round20FactoryWorker{}.pureFactory
	for o := 0; o < out; o++ {
		ps6010returnhelper.CallFactory(factory, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}

func importedDeferredReturnedCallbackRound20(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010returnhelper.CallFactory(ps6010Round20DeferredFactory, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func importedClosureReturnedCallbackRound20(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010returnhelper.CallFactory(ps6010Round20ClosureFactory, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func importedCapturedReturnedCallbackRound20(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	factory := func() func(int) { return ps6010Round20CapturingFactory(a) }
	for o := 0; o < out; o++ {
		ps6010returnhelper.CallFactory(factory, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func importedBodylessReturnedCallbackRound20(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010returnhelper.CallFactory(ps6010Round20BodylessFactory, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func importedReturnedMethodCallbackRound20(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010returnhelper.CallFactory(ps6010Round20ReturnedMethodFactory, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func importedCapturedMethodCallbackRound20(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	factory := func() func(int) { return ps6010Round20CapturedMethodFactory(a) }
	for o := 0; o < out; o++ {
		ps6010returnhelper.CallFactory(factory, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func importedTupleReturnedCallbackRound20(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010returnhelper.CallSecondFactory(ps6010Round20MutatingSecondFactory, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func importedLateDeclaredReturnedCallbackRound20(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010returnhelper.CallFactory(ps6010Round20LateFactory, o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func ps6010Round20LateFactory() func(int) {
	return ps6010Round17MutateGlobalMiddle
}

func localPureReturnedCallbackControlRound20(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		callback := ps6010Round20PureFactory()
		callback(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}

func ps6010Round21ControlPureCallback(int) {}

func ps6010Round21MixedPureCallback(int) {}

func ps6010Round21FieldPureCallback(int) {}

func ps6010Round21PurePair() (func(int), func(int)) {
	return ps6010Round21ControlPureCallback, ps6010Round21ControlPureCallback
}

func ps6010Round21MutatingSecondPair() (func(int), func(int)) {
	return ps6010Round21MixedPureCallback, ps6010Round17MutateGlobalMiddle
}

func ps6010Round21MutatingFirstPair() (func(int), func(int)) {
	return ps6010Round17MutateGlobalMiddle, ps6010Round21MixedPureCallback
}

func ps6010Round21NamedPair() (first func(int), second func(int)) {
	first = ps6010Round21MixedPureCallback
	second = ps6010Round17MutateGlobalMiddle
	return
}

func ps6010Round21GenericPair[F ~func(int)](first, second F) (F, F) {
	return first, second
}

type ps6010Round21PairBox struct {
	first  func(int)
	second func(int)
}

type ps6010Round21MethodCarrier struct {
	callback func(int)
}

func (carrier ps6010Round21MethodCarrier) invoke(extra func(int)) {
	_ = extra
	ps6010returnhelper.InvokeCallback(carrier.callback, 0)
}

func ps6010Round21FieldPair() (func(int), func(int)) {
	box := ps6010Round21PairBox{
		first:  ps6010Round21FieldPureCallback,
		second: ps6010Round17MutateGlobalMiddle,
	}
	return box.first, box.second
}

func ps6010Round21MethodPair() (ps6010Round21MethodCarrier, func(int)) {
	return ps6010Round21MethodCarrier{callback: ps6010Round17MutateGlobalMiddle}, ps6010Round21MixedPureCallback
}

func ps6010Round21ForwardFirst(first, second func(int)) {
	_ = second
	ps6010returnhelper.InvokeCallback(first, 0)
}

func ps6010Round21ForwardSecond(first, second func(int)) {
	_ = first
	ps6010returnhelper.InvokeCallbackGeneric[func(int)](second, 0)
}

func ps6010Round21ForwardFixedVariadic(first func(int), rest ...func(int)) {
	_ = first
	ps6010returnhelper.InvokeCallback(rest[0], 0)
}

func ps6010Round21ForwardVariadic(callbacks ...func(int)) {
	ps6010returnhelper.InvokeCallback(callbacks[len(callbacks)-1], 0)
}

func ps6010Round21ForwardSecondControl(first, second func(int)) {
	_ = first
	ps6010returnhelper.InvokeCallback(second, 0)
}

func ps6010Round21ForwardFixedVariadicControl(first func(int), rest ...func(int)) {
	_ = first
	ps6010returnhelper.InvokeCallback(rest[0], 0)
}

func expandedMutatingSecondTupleRound21(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round21ForwardSecond(ps6010Round21MutatingSecondPair())
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func expandedMutatingFirstTupleRound21(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round21ForwardFirst(ps6010Round21MutatingFirstPair())
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func expandedNamedTupleRound21(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round21ForwardSecond(ps6010Round21NamedPair())
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func expandedGenericTupleRound21(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round21ForwardSecond(ps6010Round21GenericPair[func(int)](
			ps6010Round21MixedPureCallback,
			ps6010Round17MutateGlobalMiddle,
		))
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func expandedInferredGenericTupleRound21(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round21ForwardSecond(ps6010Round21GenericPair(
			ps6010Round21MixedPureCallback,
			ps6010Round17MutateGlobalMiddle,
		))
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func expandedFieldTupleRound21(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round21ForwardSecond(ps6010Round21FieldPair())
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func expandedMethodExpressionTupleRound21(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	invoke := ps6010Round21MethodCarrier.invoke
	for o := 0; o < out; o++ {
		invoke(ps6010Round21MethodPair())
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func expandedFixedVariadicTupleRound21(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round21ForwardFixedVariadic(ps6010Round21MutatingSecondPair())
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func expandedVariadicTupleRound21(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round21ForwardVariadic(ps6010Round21MutatingSecondPair())
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func expandedPureTupleControlRound21(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round21ForwardSecondControl(ps6010Round21PurePair())
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}

func ordinaryPureArgumentsControlRound21(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round21ForwardSecondControl(ps6010Round21ControlPureCallback, ps6010Round21ControlPureCallback)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}

func expandedPureVariadicControlRound21(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round21ForwardFixedVariadicControl(ps6010Round21PurePair())
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}
