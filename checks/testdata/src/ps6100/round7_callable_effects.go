package ps6100

func round7Invoke(call func()) { call() }

func round7InvokeThroughAlias(call func()) {
	next := call
	next()
}

func round7RetainWithoutInvoke(call func()) { _ = call }

func round7DeferUntilHelperReturn(call func()) { defer call() }

func round7InvokeWrite(call func([]float64, int), values []float64, index int) {
	call(values, index)
}

func round7RecursiveInvoke(call func(), depth int) {
	if depth <= 0 {
		call()
		return
	}
	round7RecursiveInvoke(call, depth-1)
}

func round7HelperInvokedRebind(alpha []float64, left int) {
	alias := alpha
	stored := func() { alias = nil }
	round7InvokeThroughAlias(stored)
	for step := 0; step < 4; step++ {
		for index := range alias {
			if alias[index] > 0 {
				_ = index
			}
		}
		for index := range alias {
			if alias[index] < 1 {
				_ = index
			}
		}
		alpha[left]++
	}
}

func round7HelperUninvokedRebind(alpha []float64, left int) {
	alias := alpha
	stored := func() { alias = nil }
	round7RetainWithoutInvoke(stored)
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\)`
		for index := range alias {
			if alias[index] > 0 {
				_ = index
			}
		}
		for index := range alias {
			if alias[index] < 1 {
				_ = index
			}
		}
		alpha[left]++
	}
}

func round7NestedDeferredRebind(alpha []float64, left int) {
	alias := alpha
	stored := func() { defer func() { alias = nil }() }
	stored()
	for step := 0; step < 4; step++ {
		for index := range alias {
			if alias[index] > 0 {
				_ = index
			}
		}
		for index := range alias {
			if alias[index] < 1 {
				_ = index
			}
		}
		alpha[left]++
	}
}

func round7HelperDeferredRebind(alpha []float64, left int) {
	alias := alpha
	stored := func() { alias = nil }
	round7DeferUntilHelperReturn(stored)
	for step := 0; step < 4; step++ {
		for index := range alias {
			if alias[index] > 0 {
				_ = index
			}
		}
		for index := range alias {
			if alias[index] < 1 {
				_ = index
			}
		}
		alpha[left]++
	}
}

func round7OuterDeferredRebind(alpha []float64, left int) {
	alias := alpha
	stored := func() { alias = nil }
	defer stored()
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\)`
		for index := range alias {
			if alias[index] > 0 {
				_ = index
			}
		}
		for index := range alias {
			if alias[index] < 1 {
				_ = index
			}
		}
		alpha[left]++
	}
}

func round7StoredManyWrites(alpha []float64, left int) {
	stored := func() {
		alpha[0]++
		alpha[1]++
		alpha[2]++
		alpha[3]++
		alpha[4]++
	}
	for step := 0; step < 4; step++ {
		for index := range alpha {
			if alpha[index] > 0 {
				_ = index
			}
		}
		for index := range alpha {
			if alpha[index] < 1 {
				_ = index
			}
		}
		alpha[left]++
		stored()
	}
}

func round7StoredOneWrite(alpha []float64, left int) {
	stored := func() { alpha[0]++ }
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*2 bounded indexed mutation site\(s\) \(alpha\[0\]@[0-9]+, alpha\[left\]@[0-9]+\)`
		for index := range alpha {
			if alpha[index] > 0 {
				_ = index
			}
		}
		for index := range alpha {
			if alpha[index] < 1 {
				_ = index
			}
		}
		alpha[left]++
		stored()
	}
}

func round7StoredAsyncWrite(alpha []float64, left int) {
	stored := func() { alpha[0]++ }
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*2 bounded indexed mutation site\(s\).*goroutine callable may mutate or retain alpha@[0-9]+`
		for index := range alpha {
			if alpha[index] > 0 {
				_ = index
			}
		}
		for index := range alpha {
			if alpha[index] < 1 {
				_ = index
			}
		}
		alpha[left]++
		go stored()
	}
}

func round7StoredParameterWrite(alpha []float64, left, right int) {
	stored := func(values []float64, index int) { values[index]++ }
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*2 bounded indexed mutation site\(s\) \(alpha\[left\]@[0-9]+, alpha\[right\]@[0-9]+\)`
		for index := range alpha {
			if alpha[index] > 0 {
				_ = index
			}
		}
		for index := range alpha {
			if alpha[index] < 1 {
				_ = index
			}
		}
		alpha[left]++
		round7InvokeWrite(stored, alpha, right)
	}
}

type round7Slice []float64

func (alpha round7Slice) round7ManyWrites() {
	alpha[0]++
	alpha[1]++
	alpha[2]++
	alpha[3]++
	alpha[4]++
}

func (alpha round7Slice) round7OneWrite() { alpha[0]++ }

func (alpha round7Slice) round7WriteAt(index int) { alpha[index]++ }

func round7MethodValueManyWrites(alpha round7Slice, left int) {
	stored := alpha.round7ManyWrites
	for step := 0; step < 4; step++ {
		for index := range alpha {
			if alpha[index] > 0 {
				_ = index
			}
		}
		for index := range alpha {
			if alpha[index] < 1 {
				_ = index
			}
		}
		alpha[left]++
		stored()
	}
}

func round7MethodValueOneWrite(alpha round7Slice, left int) {
	stored := alpha.round7OneWrite
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*2 bounded indexed mutation site\(s\) \(alpha\[0\]@[0-9]+, alpha\[left\]@[0-9]+\)`
		for index := range alpha {
			if alpha[index] > 0 {
				_ = index
			}
		}
		for index := range alpha {
			if alpha[index] < 1 {
				_ = index
			}
		}
		alpha[left]++
		round7Invoke(stored)
	}
}

func round7MethodExpressionOneWrite(alpha round7Slice, left int) {
	stored := round7Slice.round7OneWrite
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*2 bounded indexed mutation site\(s\) \(alpha\[0\]@[0-9]+, alpha\[left\]@[0-9]+\)`
		for index := range alpha {
			if alpha[index] > 0 {
				_ = index
			}
		}
		for index := range alpha {
			if alpha[index] < 1 {
				_ = index
			}
		}
		alpha[left]++
		stored(alpha)
	}
}

func round7MethodValueArgument(alpha round7Slice, left, right int) {
	stored := alpha.round7WriteAt
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*2 bounded indexed mutation site\(s\) \(alpha\[left\]@[0-9]+, alpha\[right\]@[0-9]+\)`
		for index := range alpha {
			if alpha[index] > 0 {
				_ = index
			}
		}
		for index := range alpha {
			if alpha[index] < 1 {
				_ = index
			}
		}
		alpha[left]++
		stored(right)
	}
}

func round7MethodExpressionArgument(alpha round7Slice, left, right int) {
	stored := round7Slice.round7WriteAt
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*2 bounded indexed mutation site\(s\) \(alpha\[left\]@[0-9]+, alpha\[right\]@[0-9]+\)`
		for index := range alpha {
			if alpha[index] > 0 {
				_ = index
			}
		}
		for index := range alpha {
			if alpha[index] < 1 {
				_ = index
			}
		}
		alpha[left]++
		stored(alpha, right)
	}
}

func round7MethodValueReceiverSnapshot(alpha, other round7Slice, left int) {
	receiver := alpha
	stored := receiver.round7OneWrite
	receiver = other
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*2 bounded indexed mutation site\(s\) \(alpha\[0\]@[0-9]+, alpha\[left\]@[0-9]+\)`
		for index := range alpha {
			if alpha[index] > 0 {
				_ = index
			}
		}
		for index := range alpha {
			if alpha[index] < 1 {
				_ = index
			}
		}
		alpha[left]++
		stored()
	}
}

type round7State struct {
	alpha []float64
}

func (state *round7State) round7WriteAt(index int) { state.alpha[index]++ }

func round7PointerMethodValue(state *round7State, left, right int) {
	stored := state.round7WriteAt
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of state.alpha.*2 bounded indexed mutation site\(s\) \(state.alpha\[left\]@[0-9]+, state.alpha\[right\]@[0-9]+\)`
		for index := range state.alpha {
			if state.alpha[index] > 0 {
				_ = index
			}
		}
		for index := range state.alpha {
			if state.alpha[index] < 1 {
				_ = index
			}
		}
		state.alpha[left]++
		stored(right)
	}
}

func round7CallableSourceOverflow(alpha []float64, left, choice int) {
	stored := func() {}
	if choice == 1 {
		stored = func() {}
	} else if choice == 2 {
		stored = func() {}
	} else if choice == 3 {
		stored = func() {}
	} else if choice == 4 {
		stored = func() {}
	} else if choice == 5 {
		stored = func() {}
	} else if choice == 6 {
		stored = func() {}
	} else if choice == 7 {
		stored = func() {}
	} else if choice == 8 {
		stored = func() {}
	}
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*opaque function value may mutate or retain alpha@[0-9]+`
		for index := range alpha {
			if alpha[index] > 0 {
				_ = index
			}
		}
		for index := range alpha {
			if alpha[index] < 1 {
				_ = index
			}
		}
		alpha[left]++
		stored()
	}
}

func round7RecursiveCallableHazard(alpha []float64, left int) {
	stored := func() { alpha[0]++ }
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*2 bounded indexed mutation site\(s\).*opaque function value may mutate or retain alpha@[0-9]+`
		for index := range alpha {
			if alpha[index] > 0 {
				_ = index
			}
		}
		for index := range alpha {
			if alpha[index] < 1 {
				_ = index
			}
		}
		alpha[left]++
		round7RecursiveInvoke(stored, 1)
	}
}
