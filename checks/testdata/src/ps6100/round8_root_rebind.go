package ps6100

type round8Slice []float64

func (values round8Slice) round8WriteZero() { values[0]++ }

func (values round8Slice) round8WriteFive() {
	values[0]++
	values[1]++
	values[2]++
	values[3]++
	values[4]++
}

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

type round8MethodState struct {
	alpha round8Slice
}

func round8FieldHeaderRebind(state *round8MethodState, other round8Slice) {
	state.alpha = other
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of state.alpha.*1 bounded indexed mutation site\(s\) \(state.alpha\[0\]@[0-9]+\)`
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
		other[0]++
	}
}

func round8DereferencedHeaderRebind(slot *round8Slice, other round8Slice) {
	*slot = other
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of slot.*1 bounded indexed mutation site\(s\) \(slot\[0\]@[0-9]+\)`
		for index := range *slot {
			if (*slot)[index] > 0 {
				_ = index
			}
		}
		for index := range *slot {
			if (*slot)[index] < 1 {
				_ = index
			}
		}
		other[0]++
	}
}

func round8StoredFieldReceiverSnapshot(state *round8MethodState, replacement round8Slice) {
	stored := state.alpha.round8WriteZero
	state.alpha = replacement
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of state.alpha.*1 bounded indexed mutation site\(s\) \(state.alpha\[1\]@[0-9]+\)`
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
		stored()
		state.alpha[1]++
	}
}

func round8DirectOldFieldReceiverSnapshot(state *round8MethodState, replacement round8Slice) {
	old := state.alpha
	state.alpha = replacement
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of state.alpha.*1 bounded indexed mutation site\(s\) \(state.alpha\[1\]@[0-9]+\)`
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
		old.round8WriteZero()
		state.alpha[1]++
	}
}

func round8StoredOldFieldFive(state *round8MethodState, replacement round8Slice) {
	stored := state.alpha.round8WriteFive
	state.alpha = replacement
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of state.alpha.*1 bounded indexed mutation site\(s\) \(state.alpha\[0\]@[0-9]+\)`
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
		stored()
		state.alpha[0]++
	}
}

func round8DirectCurrentFieldReceiver(state *round8MethodState, replacement round8Slice, left int) {
	state.alpha = replacement
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of state.alpha.*2 bounded indexed mutation site\(s\) \(state.alpha\[0\]@[0-9]+, state.alpha\[left\]@[0-9]+\)`
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
		state.alpha.round8WriteZero()
		state.alpha[left]++
	}
}

func round8SelfResliceSourceAlias(alpha []float64) {
	original := alpha
	alpha = alpha[1:]
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\) \(alpha\[0\]@[0-9]+\)`
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
		original[1]++
	}
}

func round8OtherResliceSourceAlias(alpha, other []float64) {
	alpha = other[1:]
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\) \(alpha\[0\]@[0-9]+\)`
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
		other[1]++
	}
}

func round8SelfResliceSourceMany(alpha []float64) {
	original := alpha
	alpha = alpha[1:]
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
		alpha[0]++
		original[1]++
		original[2]++
		original[3]++
		original[4]++
		original[5]++
	}
}

func round8SelfResliceBeforeViewWrite(alpha []float64) {
	original := alpha
	alpha = alpha[1:]
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\) \(alpha\[0\]@[0-9]+\)`
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
		original[0]++
		alpha[0]++
	}
}

func round8EmptyMakeRebind(alpha []float64) {
	alpha = make([]float64, 0)
	for step := 0; step < 4 && len(alpha) > 0; step++ {
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
		alpha[0]++
	}
}

func round8NilRebind(alpha []float64) {
	alpha = nil
	for step := 0; step < 4 && len(alpha) > 0; step++ {
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
		alpha[0]++
	}
}

func round8ReturningRebindBranch(alpha, other []float64, stop bool) {
	if stop {
		alpha = other
		return
	}
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\) \(alpha\[0\]@[0-9]+\)`
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
		alpha[0]++
	}
}

func round8EnclosingLoopRebind(alpha, other []float64) {
	for round := 0; round < 2; round++ {
		alpha = other
		for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\) \(alpha\[0\]@[0-9]+\)`
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
			alpha[0]++
		}
	}
}

func round8TupleFieldHeaderRebind(state *round8MethodState, other round8Slice, left int) {
	state.alpha, other = other, state.alpha
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
		other[0]++
		other[1]++
		other[2]++
		other[3]++
		other[4]++
	}
}

func round8BranchAmbiguousFieldHeader(state *round8MethodState, other, replacement round8Slice, left int, choose bool) {
	if choose {
		state.alpha = other
	} else {
		state.alpha = replacement
	}
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of state.alpha.*1 bounded indexed mutation site\(s\) \(state.alpha\[left\]@[0-9]+\).*branch-ambiguous pre-loop rebind may alias state.alpha@[0-9]+`
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

func round8ResliceSourceSnapshot(alpha, other, replacement []float64, left int) {
	alpha = other[1:]
	retained := other
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
		other[0]++
		other[1]++
		other[2]++
		other[3]++
		other[4]++
		retained[1]++
		alpha[left]++
	}
}

func round8EmptyFieldRebind(state *round8MethodState) {
	state.alpha = make(round8Slice, 0)
	for step := 0; step < 4 && len(state.alpha) > 0; step++ {
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
		state.alpha[0]++
	}
}

func round8NilDereferenceRebind(slot *round8Slice) {
	*slot = nil
	for step := 0; step < 4 && len(*slot) > 0; step++ {
		for index := range *slot {
			if (*slot)[index] > 0 {
				_ = index
			}
		}
		for index := range *slot {
			if (*slot)[index] < 1 {
				_ = index
			}
		}
		(*slot)[0]++
	}
}

func round8NestedSelfResliceSourceAlias(alpha []float64) {
	original := alpha
	alpha = alpha[1:]
	alpha = alpha[2:]
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\) \(alpha\[0\]@[0-9]+\)`
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
		original[3]++
	}
}
