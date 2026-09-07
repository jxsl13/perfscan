package ps6100

func round4MutateMany(alpha []float64, left int) bool {
	alpha[left]++
	alpha[0]++
	alpha[1]++
	alpha[2]++
	alpha[3]++
	return true
}

func round4MutateOne(alpha []float64, left int) bool {
	alpha[left]++
	return true
}

func round4ObserveIndex(*int) bool { return true }

func deadOuterAfterBuiltinPanic(alpha []float64, left int) {
	panic("unreachable scans")
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
	}
}

func reachableOuterAfterShadowedPanic(alpha []float64, left int) {
	panic := func(any) {}
	panic("returns")
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\)`
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

func deadFallthroughAfterBuiltinPanic(alpha []float64, left int) {
	switch 0 {
	case 0:
		panic("fallthrough is unreachable")
		fallthrough
	default:
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
		}
	}
}

func reachableFallthroughAfterShadowedPanic(alpha []float64, left int) {
	panic := func(any) {}
	switch 0 {
	case 0:
		panic("returns")
		fallthrough
	default:
		for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\)`
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
}

func incompleteScansAfterBuiltinPanic(alpha []float64, left int) {
	for step := 0; step < 4; step++ {
		for index := range alpha {
			if alpha[index] > 0 {
				_ = index
			}
			panic("scan cannot complete")
		}
		for index := range alpha {
			if alpha[index] < 1 {
				_ = index
			}
			panic("scan cannot complete")
		}
		alpha[left]++
	}
}

func completeScansAfterShadowedPanic(alpha []float64, left int) {
	panic := func(any) {}
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\)`
		for index := range alpha {
			if alpha[index] > 0 {
				_ = index
			}
			panic("returns")
		}
		for index := range alpha {
			if alpha[index] < 1 {
				_ = index
			}
			panic("returns")
		}
		alpha[left]++
	}
}

func deadShortCircuitHelperMutation(alpha []float64, left int) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\)`
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
		if false && round4MutateMany(alpha, left) {
		}
	}
}

func reachableShortCircuitHelperMutation(alpha []float64, left int) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\)`
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
		if true && round4MutateOne(alpha, left) {
		}
	}
}

func reachableLeftOperandHelperMutation(alpha []float64, left int) {
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
		if round4MutateMany(alpha, left) && false {
		}
	}
}

func deadNestedOrHelperMutation(alpha []float64, left int) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\)`
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
		if !false || (true && round4MutateMany(alpha, left)) {
		}
	}
}

func reachableOrHelperMutation(alpha []float64, left int) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\)`
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
		if !true || round4MutateOne(alpha, left) {
		}
	}
}

func deadShortCircuitIndexAddress(alpha []float64, left int) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\)`
		for index := range alpha {
			if alpha[index] > 0 {
				_ = index
			}
			if false && round4ObserveIndex(&index) {
			}
		}
		for index := range alpha {
			if alpha[index] < 1 {
				_ = index
			}
			if false && round4ObserveIndex(&index) {
			}
		}
		alpha[left]++
	}
}

func reachableIndexAddress(alpha []float64, left int) {
	for step := 0; step < 4; step++ {
		for index := range alpha {
			if alpha[index] > 0 {
				_ = index
			}
			if true && round4ObserveIndex(&index) {
			}
		}
		for index := range alpha {
			if alpha[index] < 1 {
				_ = index
			}
			if true && round4ObserveIndex(&index) {
			}
		}
		alpha[left]++
	}
}

func deadCompoundAliasRebind(alpha []float64, left int, unknown bool) {
	alias := alpha
	if false && unknown {
		alias = nil
	}
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

func reachableAliasRebind(alpha []float64, left int) {
	alias := alpha
	if true {
		alias = nil
	}
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

func deadCompoundAliasAddress(alpha []float64, left int, unknown bool) {
	alias := alpha
	if false && unknown {
		_ = &alias
	}
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

func reachableAliasAddress(alpha []float64, left int) {
	alias := alpha
	_ = &alias
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

func deadAliasEffectsAfterBuiltinPanic(alpha []float64, left int, stop bool) {
	alias := alpha
	if stop {
		panic("later alias effects are unreachable")
		alias = nil
		_ = &alias
	}
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

func reachableAliasEffectsAfterShadowedPanic(alpha []float64, left int, stop bool) {
	panic := func(any) {}
	alias := alpha
	if stop {
		panic("returns")
		alias = nil
		_ = &alias
	}
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

func deadHelperAfterBuiltinPanic(alpha []float64, left int, stop bool) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\)`
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
		if stop {
			panic("later helper is unreachable")
			round4MutateMany(alpha, left)
		}
	}
}

func reachableHelperAfterShadowedPanic(alpha []float64, left int, stop bool) {
	panic := func(any) {}
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
		if stop {
			panic("returns")
			round4MutateMany(alpha, left)
		}
	}
}
