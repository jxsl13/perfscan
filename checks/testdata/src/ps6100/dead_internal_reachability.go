package ps6100

func round3Mutate(alpha []float64, left int) {
	alpha[left]++
}

func deadNestedIfPredicates(alpha []float64, left int) {
	for step := 0; step < 4; step++ {
		for index := range alpha {
			if false {
				if alpha[index] > 0 {
					_ = index
				}
			}
		}
		for index := range alpha {
			if true {
				if false {
					if alpha[index] < 1 {
						_ = index
					}
				}
			}
		}
		alpha[left]++
	}
}

func reachableNestedIfPredicates(alpha []float64, left int) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\)`
		for index := range alpha {
			if true {
				if alpha[index] > 0 {
					_ = index
				}
			}
		}
		for index := range alpha {
			if true {
				if alpha[index] < 1 {
					_ = index
				}
			}
		}
		alpha[left]++
	}
}

func deadNestedSwitchPredicates(alpha []float64, left int) {
	for step := 0; step < 4; step++ {
		for index := range alpha {
			switch 0 {
			case 1:
				if alpha[index] > 0 {
					_ = index
				}
			}
		}
		for index := range alpha {
			switch 0 {
			case 1:
				if alpha[index] < 1 {
					_ = index
				}
			}
		}
		alpha[left]++
	}
}

func reachableNestedSwitchPredicates(alpha []float64, left int) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\)`
		for index := range alpha {
			switch 0 {
			case 0:
				if alpha[index] > 0 {
					_ = index
				}
			}
		}
		for index := range alpha {
			switch 0 {
			case 0:
				if alpha[index] < 1 {
					_ = index
				}
			}
		}
		alpha[left]++
	}
}

func deadPredicatesAfterContinue(alpha []float64, left int) {
	for step := 0; step < 4; step++ {
		for index := range alpha {
			continue
			if alpha[index] > 0 {
				_ = index
			}
		}
		for index := range alpha {
			continue
			if alpha[index] < 1 {
				_ = index
			}
		}
		alpha[left]++
	}
}

func reachablePredicatesBeforeContinue(alpha []float64, left int) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\)`
		for index := range alpha {
			if alpha[index] > 0 {
				_ = index
			}
			continue
		}
		for index := range alpha {
			if alpha[index] < 1 {
				_ = index
			}
			continue
		}
		alpha[left]++
	}
}

func deadNestedIfMutation(alpha []float64, left int) {
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
		if true {
			if false {
				alpha[left]++
			}
		}
	}
}

func reachableNestedIfMutation(alpha []float64, left int) {
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
		if true {
			alpha[left]++
		}
	}
}

func deadNestedSwitchMutation(alpha []float64, left int) {
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
		switch 0 {
		case 1:
			alpha[left]++
		}
	}
}

func reachableNestedSwitchMutation(alpha []float64, left int) {
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
		switch 0 {
		case 0:
			alpha[left]++
		}
	}
}

func deadMutationAfterContinue(alpha []float64, left int) {
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
		continue
		alpha[left]++
	}
}

func reachableMutationBeforeContinue(alpha []float64, left int) {
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
		continue
	}
}

func deadHelperMutation(alpha []float64, left int) {
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
		if false {
			round3Mutate(alpha, left)
		}
	}
}

func reachableHelperMutation(alpha []float64, left int) {
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
		round3Mutate(alpha, left)
	}
}

func deadOpaqueHazard(alpha []float64, left int, mutate func([]float64)) {
	for step := 0; step < 4; step++ { // want `prove reference-backed aliases cannot mutate alpha outside the listed refresh sites; initialize once`
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
		if false {
			mutate(alpha)
		}
	}
}

func reachableOpaqueHazard(alpha []float64, left int, mutate func([]float64)) {
	for step := 0; step < 4; step++ { // want `opaque call may bulk-mutate or retain alpha@[0-9]+`
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
		if true {
			mutate(alpha)
		}
	}
}

func deadClosureHazard(alpha []float64, left int) {
	for step := 0; step < 4; step++ { // want `prove reference-backed aliases cannot mutate alpha outside the listed refresh sites; initialize once`
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
		if false {
			deferred := func() { alpha[0]++ }
			_ = deferred
		}
	}
}

func reachableClosureHazard(alpha []float64, left int) {
	for step := 0; step < 4; step++ { // want `closure may mutate or retain alpha@[0-9]+`
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
		deferred := func() { alpha[0]++ }
		_ = deferred
	}
}

func unreachableSequencedFallthroughScans(alpha []float64, left int) {
	switch 0 {
	case 0:
		if true {
			return
		}
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

func reachableSequencedFallthroughScans(alpha []float64, left int) {
	switch 0 {
	case 0:
		if false {
			return
		}
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

func maybeReachableSequencedFallthroughScans(alpha []float64, left int, stop bool) {
	switch 0 {
	case 0:
		if stop {
			return
		}
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
