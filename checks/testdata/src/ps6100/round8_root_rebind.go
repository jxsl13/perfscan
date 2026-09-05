package ps6100

type round8Slice []float64

func (values round8Slice) round8WriteZero() { values[0]++ }

func round8DirectRootRebind(alpha, other []float64, left int) {
	alpha = other
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\) \(alpha\[left\]@[0-9]+\)`
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
	}
}

func round8SelfReslicedRoot(alpha []float64, left int) {
	alpha = alpha[1:]
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\) \(alpha\[left\]@[0-9]+\)`
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
	}
}

func round8MethodValueAfterReslice(alpha round8Slice, left int) {
	alpha = alpha[1:]
	stored := alpha.round8WriteZero
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

func round8StoredMethodKeepsPreRebindReceiver(alpha, other, replacement round8Slice, left int) {
	alpha = other
	stored := other.round8WriteZero
	other = replacement
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

func round8DirectMethodUsesPostRebindReceiver(alpha, other, replacement round8Slice, left int) {
	alpha = other
	other = replacement
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\) \(alpha\[left\]@[0-9]+\)`
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
		other.round8WriteZero()
	}
}

func round8TupleRootRebind(alpha, other []float64, left int) {
	alpha, other = other, alpha
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\) \(alpha\[left\]@[0-9]+\)`
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
		other[0]++
		other[1]++
		other[2]++
		other[3]++
		other[4]++
	}
}

func round8LocalRootRebind(alpha, other []float64, left int) {
	current := alpha
	current = other
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of current.*1 bounded indexed mutation site\(s\) \(current\[left\]@[0-9]+\)`
		for index := range current {
			if current[index] > 0 {
				_ = index
			}
		}
		for index := range current {
			if current[index] < 1 {
				_ = index
			}
		}
		current[left]++
	}
}

func round8OuterInitializerRebind(alpha, other []float64, left int) {
	step := 0
	for alpha = other; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\) \(alpha\[left\]@[0-9]+\)`
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
	}
}

func round8PredicateInputRebind(indexes []int, flags, other []bool, left int) {
	flags = other
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of indexes.*1 bounded indexed mutation site\(s\) \(flags\[left\]@[0-9]+\)`
		for index := range indexes {
			if flags[index] {
				_ = index
			}
		}
		for index := range indexes {
			if !flags[index] {
				_ = index
			}
		}
		flags[left] = !flags[left]
	}
}

type round8State struct {
	alpha []float64
}

func round8PointerFieldRootRebind(state, other *round8State, left int) {
	state = other
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of state.alpha.*1 bounded indexed mutation site\(s\) \(state.alpha\[left\]@[0-9]+\)`
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
	}
}

func round8SourceReboundAfterSnapshot(alpha, other, replacement []float64, left int) {
	alpha = other
	other = replacement
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\) \(alpha\[left\]@[0-9]+\)`
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
		other[0]++
		other[1]++
		other[2]++
		other[3]++
		other[4]++
		alpha[left]++
	}
}

func round8UnchangedSourceAlias(alpha, other []float64, left int) {
	alpha = other
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
		other[0]++
	}
}

func round8UnchangedSourceManyWrites(alpha, other []float64, left int) {
	alpha = other
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
		other[0]++
		other[1]++
		other[2]++
		other[3]++
		other[4]++
	}
}

func round8BranchAmbiguousRoot(alpha, other, replacement []float64, left int, choose bool) {
	if choose {
		alpha = other
	} else {
		alpha = replacement
	}
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\) \(alpha\[left\]@[0-9]+\).*branch-ambiguous pre-loop rebind may alias alpha@[0-9]+`
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
		other[0]++
		alpha[left]++
	}
}

func round8BranchAmbiguousManyWrites(alpha, other, replacement []float64, left int, choose bool) {
	if choose {
		alpha = other
	} else {
		alpha = replacement
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
		other[0]++
		other[1]++
		other[2]++
		other[3]++
		other[4]++
	}
}
