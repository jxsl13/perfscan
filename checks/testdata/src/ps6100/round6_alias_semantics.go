package ps6100

func round6StoredUninvoked(alpha []float64, left int) {
	alias := alpha
	stored := func() { alias = nil }
	_ = stored
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

func round6StoredInvoked(alpha []float64, left int) {
	alias := alpha
	stored := func() { alias = nil }
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

func round6StoredInvokedThroughAlias(alpha []float64, left int) {
	alias := alpha
	stored := func() { alias = nil }
	invoke := stored
	invoke()
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

func round6StoredLiveLeft(alpha []float64, left int) {
	alias := alpha
	stored := func() bool { alias = nil; return false }
	if stored() && false {
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

func round6StoredDeadRight(alpha []float64, left int) {
	alias := alpha
	stored := func() bool { alias = nil; return false }
	if true || stored() {
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

func round6StoredDeferred(alpha []float64, left int) {
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

func round6StoredAsync(alpha []float64, left int) {
	alias := alpha
	stored := func() { alias = nil }
	go stored()
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

func round6SwapOne(alpha, other []float64, left int) {
	first, second := alpha, other
	first, second = second, first
	second[left]++
}

func round6SwappedAliasOneWrite(alpha, other []float64, left int) {
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
		round6SwapOne(alpha, other, left)
	}
}

func round6SwapMany(alpha, other []float64, left int) {
	first, second := alpha, other
	first, second = second, first
	second[left]++
	second[0]++
	second[1]++
	second[2]++
	second[3]++
}

func round6SwappedAliasManyWrites(alpha, other []float64, left int) {
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
		round6SwapMany(alpha, other, left)
	}
}

func round6ResliceOne(alpha []float64) {
	alias := alpha[1:]
	alias[0]++
}

func round6ReslicedAliasIndex(alpha []float64) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\) \(alpha\[1\]@[0-9]+\)`
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
		round6ResliceOne(alpha)
	}
}

func round6NestedResliceOne(alpha []float64) {
	first := alpha[1:]
	second := first[2:]
	second[0]++
}

func round6NestedReslicedAliasIndex(alpha []float64) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\) \(alpha\[3\]@[0-9]+\)`
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
		round6NestedResliceOne(alpha)
	}
}

func round6SelfResliceOne(alpha []float64) {
	alias := alpha[1:]
	alias = alias[1:]
	alias[0]++
}

func round6SelfReslicedAliasIndex(alpha []float64) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\) \(alpha\[2\]@[0-9]+\)`
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
		round6SelfResliceOne(alpha)
	}
}

func round6ResliceOther(other []float64) {
	alias := other[1:]
	alias[0]++
}

func round6UnrelatedReslicedAlias(alpha, other []float64) {
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
		round6ResliceOther(other)
	}
}
