package ps6103

import ops "ps6103ops"

const steps = 5

func consume(*ops.Tensor) {}

func fixedSibling(left, right *ops.Tensor) {
	for step := 0; step < steps; step++ {
		product := ops.MatMul(left, right) // want `fixed 5-step loop repeatedly creates short-lived ops.MatMul result product; the type-compatible MatMulInto sibling can preserve optimized dispatch`
		consume(product)
	}
}

func fixedWrappedSibling(left, right *ops.Tensor) {
	for step := 0; step < steps; step++ {
		product := ops.WrappedMatMul(left, right) // want `fixed 5-step loop repeatedly creates short-lived ops.WrappedMatMul result product; the type-compatible WrappedMatMulInto sibling can preserve optimized dispatch`
		consume(product)
	}
}

func destinationLast(left, right *ops.Tensor) {
	for range 5 {
		product := ops.LastDestination(left, right) // want `fixed 5-step loop repeatedly creates short-lived ops.LastDestination result product; the type-compatible LastDestinationInto sibling can preserve optimized dispatch`
		consume(product)
	}
}

func fixedError(left, right *ops.Tensor) error {
	for step := 0; step < 5; step++ {
		product, err := ops.WithError(left, right) // want `fixed 5-step loop repeatedly creates short-lived ops.WithError result product; the type-compatible WithErrorInto sibling can preserve optimized dispatch`
		if err != nil {
			return err
		}
		consume(product)
	}
	return nil
}

func fixedWrappedError(left, right *ops.Tensor) error {
	for step := 0; step < 5; step++ {
		product, err := ops.WrappedWithError(left, right) // want `fixed 5-step loop repeatedly creates short-lived ops.WrappedWithError result product; the type-compatible WrappedWithErrorInto sibling can preserve optimized dispatch`
		if err != nil {
			return err
		}
		consume(product)
	}
	return nil
}

func fixedStoredError(left, right *ops.Tensor) error {
	for step := 0; step < 5; step++ {
		product, err := ops.StoredWithError(left, right) // want `fixed 5-step loop repeatedly creates short-lived ops.StoredWithError result product; the type-compatible StoredWithErrorInto sibling can preserve optimized dispatch`
		if err != nil {
			return err
		}
		consume(product)
	}
	return nil
}

func fixedMethod(backend *ops.Backend, left, right *ops.Tensor) {
	for range 5 {
		product := backend.Execute(left, right) // want `fixed 5-step loop repeatedly creates short-lived backend.Execute result product; the type-compatible ExecuteInto sibling can preserve optimized dispatch`
		consume(product)
	}
}

func unrelatedPromotedSibling(backend *ops.Mixed, input *ops.Tensor) {
	for range 5 {
		result := backend.Run(input)
		consume(result)
	}
}

func fixedArray(left, right *ops.Tensor) {
	for range [3]struct{}{} {
		product := ops.MatMul(left, right) // want `fixed 3-step loop repeatedly creates short-lived ops.MatMul result product`
		consume(product)
	}
}

func fixedGeneric(left *ops.Tensor) {
	for range 4 {
		product := ops.Generic(left) // want `fixed 4-step loop repeatedly creates short-lived ops.Generic result product; the type-compatible GenericInto sibling can preserve optimized dispatch`
		consume(product)
	}
}

func fixedGenericLayer(left *ops.Tensor) {
	for range 4 {
		product := ops.GenericLayer(left) // want `fixed 4-step loop repeatedly creates short-lived ops.GenericLayer result product; the type-compatible GenericLayerInto sibling can preserve optimized dispatch`
		consume(product)
	}
}

func fixedGenericMethod(backend *ops.GenericBackend[ops.Tensor], left *ops.Tensor) {
	for range 4 {
		product := backend.Process(left) // want `fixed 4-step loop repeatedly creates short-lived backend.Process result product; the type-compatible ProcessInto sibling can preserve optimized dispatch`
		consume(product)
	}
}

func outerTemporary(left, right *ops.Tensor) {
	var product *ops.Tensor
	for range 4 {
		product = ops.MatMul(left, right) // want `fixed 4-step loop repeatedly creates short-lived ops.MatMul result product; the type-compatible MatMulInto sibling can preserve optimized dispatch`
		consume(product)
	}
}

func oneShot(left, right *ops.Tensor) {
	product := ops.MatMul(left, right)
	consume(product)
}

func oneIteration(left, right *ops.Tensor) {
	for step := 0; step < 1; step++ {
		product := ops.MatMul(left, right)
		consume(product)
	}
}

func variableCount(left, right *ops.Tensor, count int) {
	for step := 0; step < count; step++ {
		product := ops.MatMul(left, right)
		consume(product)
	}
}

func indexDependent(left, right []*ops.Tensor) {
	for step := 0; step < 5; step++ {
		product := ops.MatMul(left[step], right[step])
		consume(product)
	}
}

func derivedIndex(left, right []*ops.Tensor) {
	for step := 0; step < 5; step++ {
		selected := step
		product := ops.MatMul(left[selected], right[selected])
		consume(product)
	}
}

func earlierReturn(left, right *ops.Tensor, stop bool) {
	for step := 0; step < 5; step++ {
		if stop {
			return
		}
		product := ops.MatMul(left, right)
		consume(product)
	}
}

func effectfulReceiver(next func() *ops.Backend, left, right *ops.Tensor) {
	for step := 0; step < 5; step++ {
		product := next().Execute(left, right)
		consume(product)
	}
}

func goCapture(left, right *ops.Tensor) {
	for range 5 {
		product := ops.MatMul(left, right)
		go consume(product)
	}
}

func deferCapture(left, right *ops.Tensor) {
	for range 5 {
		product := ops.MatMul(left, right)
		defer consume(product)
	}
}

func conditional(left, right *ops.Tensor, enabled bool) {
	for step := 0; step < 5; step++ {
		if enabled {
			product := ops.MatMul(left, right)
			consume(product)
		}
	}
}

func retained(left, right *ops.Tensor) []*ops.Tensor {
	var results []*ops.Tensor
	for step := 0; step < 5; step++ {
		product := ops.MatMul(left, right)
		results = append(results, product)
	}
	return results
}

func closureCapture(left, right *ops.Tensor) {
	for step := 0; step < 5; step++ {
		product := ops.MatMul(left, right)
		func() { consume(product) }()
	}
}

func opaqueWithoutSibling(left, right *ops.Tensor) {
	for step := 0; step < 5; step++ {
		product := ops.Opaque(left, right)
		consume(product)
	}
}

func incompatibleSibling(input *ops.Tensor) {
	for step := 0; step < 5; step++ {
		product := ops.Map(input)
		consume(product)
	}
}

func wrongIntoResults(input *ops.Tensor) {
	for step := 0; step < 5; step++ {
		result := ops.WrongResults(input)
		consume(result)
	}
}

func nonAllocatingSibling(input *ops.Tensor) {
	for step := 0; step < 5; step++ {
		result := ops.Borrow(input)
		consume(result)
	}
}

func constructor(input *ops.Tensor) {
	for step := 0; step < 5; step++ {
		result := ops.NewTensor(input)
		consume(result)
	}
}

func multiData(left, right *ops.Tensor) {
	for step := 0; step < 5; step++ {
		first, second := ops.Pair(left, right)
		consume(first)
		consume(second)
	}
}

func earlyExit(left, right *ops.Tensor, stop bool) {
	for step := 0; step < 5; step++ {
		product := ops.MatMul(left, right)
		consume(product)
		if stop {
			break
		}
	}
}

func nestedCall(left, right *ops.Tensor, selectTensor func(*ops.Tensor) *ops.Tensor) {
	for step := 0; step < 5; step++ {
		product := ops.MatMul(selectTensor(left), right)
		consume(product)
	}
}
