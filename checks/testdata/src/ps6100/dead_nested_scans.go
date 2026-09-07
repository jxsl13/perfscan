package ps6100

func deadNestedIfScans(alpha []float64, left int) {
	for step := 0; step < 4; step++ {
		if false {
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
		}
		alpha[left]++
	}
}

func reachableNestedIfScans(alpha []float64, left int) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\)`
		if true {
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
		}
		alpha[left]++
	}
}

func deadNestedSwitchScans(alpha []float64, left int) {
	for step := 0; step < 4; step++ {
		switch 0 {
		case 1:
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
		}
		alpha[left]++
	}
}

func reachableNestedSwitchScans(alpha []float64, left int) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\)`
		switch 0 {
		case 0:
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
		}
		alpha[left]++
	}
}

func unreachableDefaultAfterMatchingReturn(alpha []float64, left int) {
	for step := 0; step < 4; step++ {
		switch 0 {
		case 0:
			return
		default:
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
		}
		alpha[left]++
	}
}

func reachableDefaultScans(alpha []float64, left int) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\)`
		switch 0 {
		case 1:
			return
		default:
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
		}
		alpha[left]++
	}
}

func unreachableFallthroughChainScans(alpha []float64, left int) {
	for step := 0; step < 4; step++ {
		switch 0 {
		case 1:
			fallthrough
		case 2:
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
		}
		alpha[left]++
	}
}

func reachableFallthroughChainScans(alpha []float64, left int) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\)`
		switch 0 {
		case 0:
			fallthrough
		case 1:
			fallthrough
		default:
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
		}
		alpha[left]++
	}
}
