package ps6100

func round7ReviewInvoke(call func()) { call() }

func round7ReviewCapturedAliasFive(alpha, other []float64, left int) {
	alias := other
	stored := func() {
		alias[0]++
		alias[1]++
		alias[2]++
		alias[3]++
		alias[4]++
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
		alias = alpha
		round7ReviewInvoke(stored)
	}
}

func round7ReviewCapturedResliceFive(alpha, other []float64, left int) {
	alias := other
	stored := func() {
		alias[0]++
		alias[1]++
		alias[2]++
		alias[3]++
		alias[4]++
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
		alias = alpha[1:]
		stored()
	}
}

func round7ReviewOuterTupleAliasFive(alpha, other []float64, left int) {
	first, second := alpha, other
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
		first, second = second, first
		second[0]++
		second[1]++
		second[2]++
		second[3]++
		second[4]++
		first, second = second, first
	}
}

func round7ReviewTargetAssignedAfterLoop(alpha []float64, left int) {
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
	stored = func() {
		alpha[0]++
		alpha[1]++
		alpha[2]++
		alpha[3]++
		alpha[4]++
	}
}

func round7ReviewTargetAssignedAfterLoopThroughWrapper(alpha []float64, left int) {
	stored := func() { alpha[0]++ }
	wrapper := func() { stored() }
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
		wrapper()
	}
	stored = func() {
		alpha[0]++
		alpha[1]++
		alpha[2]++
		alpha[3]++
		alpha[4]++
	}
}

func round7ReviewTargetAssignedInsideLoop(alpha []float64, left int) {
	stored := func() { alpha[0]++ }
	wrapper := func() { stored() }
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
		wrapper()
		stored = func() {
			alpha[0]++
			alpha[1]++
			alpha[2]++
			alpha[3]++
			alpha[4]++
		}
	}
}

func round7ReviewCallableTupleSnapshot(alpha []float64, left int) {
	first := func() { alpha[0]++ }
	second := func() {
		alpha[0]++
		alpha[1]++
		alpha[2]++
		alpha[3]++
		alpha[4]++
	}
	first, second = second, first
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
		second()
	}
	_ = first
}

func round7ReviewOuterTupleAliasOne(alpha, other []float64, left int) {
	first, second := alpha, other
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
		first, second = second, first
		second[0]++
		first, second = second, first
	}
}
