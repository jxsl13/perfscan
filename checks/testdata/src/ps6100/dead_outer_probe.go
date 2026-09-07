package ps6100

func deadOuterProbe(alpha []float64, labels []int, left int) {
	for false {
		for index := range alpha {
			if alpha[index] > 0 {
				labels[index] = 1
			}
		}
		alpha[left] = -alpha[left]
		for index := range alpha {
			if alpha[index] > 0 {
				labels[index] = 1
			}
		}
	}
}

func zeroTripCountingOuter(alpha []float64, labels []int, left int) {
	for step := 0; step < 0; step++ {
		for index := range alpha {
			if alpha[index] > 0 {
				labels[index] = 1
			}
		}
		alpha[left] = -alpha[left]
		for index := range alpha {
			if alpha[index] > 0 {
				labels[index] = 1
			}
		}
	}
}

func emptyRangeOuter(alpha []float64, labels []int, left int) {
	for range [0]struct{}{} {
		for index := range alpha {
			if alpha[index] > 0 {
				labels[index] = 1
			}
		}
		alpha[left] = -alpha[left]
		for index := range alpha {
			if alpha[index] > 0 {
				labels[index] = 1
			}
		}
	}
}

func unreachableEnclosingBranch(alpha []float64, labels []int, left int) {
	if false {
		for step := 0; step < 2; step++ {
			for index := range alpha {
				if alpha[index] > 0 {
					labels[index] = 1
				}
			}
			alpha[left] = -alpha[left]
			for index := range alpha {
				if alpha[index] > 0 {
					labels[index] = 1
				}
			}
		}
	}
}

func unreachableEnclosingElse(alpha []float64, labels []int, left int) {
	if true {
		return
	} else {
		for step := 0; step < 2; step++ {
			for index := range alpha {
				if alpha[index] > 0 {
					labels[index] = 1
				}
			}
			alpha[left] = -alpha[left]
			for index := range alpha {
				if alpha[index] > 0 {
					labels[index] = 1
				}
			}
		}
	}
}

func reachableConstantBranch(alpha []float64, labels []int, left int) {
	if true {
		for step := 0; step < 2; step++ { // want "iterative loop repeats"
			for index := range alpha {
				if alpha[index] > 0 {
					labels[index] = 1
				}
			}
			alpha[left] = -alpha[left]
			for index := range alpha {
				if alpha[index] > 0 {
					labels[index] = 1
				}
			}
		}
	}
}

func unreachableAfterReturn(alpha []float64, labels []int, left int) {
	return
	for step := 0; step < 2; step++ {
		for index := range alpha {
			if alpha[index] > 0 {
				labels[index] = 1
			}
		}
		alpha[left] = -alpha[left]
		for index := range alpha {
			if alpha[index] > 0 {
				labels[index] = 1
			}
		}
	}
}

func unreachableSwitchCase(alpha []float64, labels []int, left int) {
	switch 0 {
	case 1:
		for step := 0; step < 2; step++ {
			for index := range alpha {
				if alpha[index] > 0 {
					labels[index] = 1
				}
			}
			alpha[left] = -alpha[left]
			for index := range alpha {
				if alpha[index] > 0 {
					labels[index] = 1
				}
			}
		}
	}
}

func reachableSwitchCase(alpha []float64, labels []int, left int) {
	switch 0 {
	case 0:
		for step := 0; step < 2; step++ { // want "iterative loop repeats"
			for index := range alpha {
				if alpha[index] > 0 {
					labels[index] = 1
				}
			}
			alpha[left] = -alpha[left]
			for index := range alpha {
				if alpha[index] > 0 {
					labels[index] = 1
				}
			}
		}
	}
}

func reachableFallthroughCase(alpha []float64, labels []int, left int) {
	switch 0 {
	case 0:
		fallthrough
	case 1:
		for step := 0; step < 2; step++ { // want "iterative loop repeats"
			for index := range alpha {
				if alpha[index] > 0 {
					labels[index] = 1
				}
			}
			alpha[left] = -alpha[left]
			for index := range alpha {
				if alpha[index] > 0 {
					labels[index] = 1
				}
			}
		}
	}
}

func nilSliceRangeOuter(alpha []float64, labels []int, left int) {
	for range []int(nil) {
		for index := range alpha {
			if alpha[index] > 0 {
				labels[index] = 1
			}
		}
		alpha[left] = -alpha[left]
		for index := range alpha {
			if alpha[index] > 0 {
				labels[index] = 1
			}
		}
	}
}
