package ps6100

type round5Units []struct{}

type round5AliasSlice []float64

type round5AliasMutator struct{}

func round5UnknownSize() int { return 0 }

func round5AliasMany(alpha []float64, index int) {
	first := alpha
	second := first
	second[index]++
	second[0]++
	second[1]++
	second[2]++
	second[3]++
}

func round5AliasOne(alpha []float64, index int) {
	alias := alpha
	alias[index]++
}

func round5AliasRebindAway(alpha, other []float64, index int) {
	alias := alpha
	alias = other
	alias[index]++
}

func round5AliasBranch(alpha, other []float64, index int, useOther bool) {
	alias := alpha
	if useOther {
		alias = other
	}
	alias[index]++
}

func round5Identity(alpha []float64) []float64 { return alpha }

func round5AliasCallResult(alpha []float64, index int) {
	alias := alpha
	alias = round5Identity(alias)
	alias[index]++
}

func round5AliasFreshStorage(alpha []float64, index int) {
	alias := alpha
	alias = make([]float64, len(alpha))
	alias[index]++
}

func round5GenericAliasMany[Slice ~[]float64](alpha Slice, index int) {
	alias := alpha
	alias[index]++
	alias[0]++
	alias[1]++
	alias[2]++
	alias[3]++
}

func round5GenericAliasOne[Slice ~[]float64](alpha Slice, index int) {
	alias := alpha
	alias[index]++
}

func (round5AliasMutator) aliasMany(alpha []float64, index int) {
	alias := alpha
	alias[index]++
	alias[0]++
	alias[1]++
	alias[2]++
	alias[3]++
}

func (round5AliasMutator) aliasOne(alpha []float64, index int) {
	alias := alpha
	alias[index]++
}

func round5PointerAliasMany(alpha *[]float64, index int) {
	alias := alpha
	(*alias)[index]++
	(*alias)[0]++
	(*alias)[1]++
	(*alias)[2]++
	(*alias)[3]++
}

func round5PointerAliasOne(alpha *[]float64, index int) {
	alias := alpha
	(*alias)[index]++
}

func round5MapAliasMany(labels map[int]float64, index int) {
	alias := labels
	alias[index]++
	alias[0]++
	alias[1]++
	alias[2]++
	alias[3]++
}

func round5MapAliasOne(labels map[int]float64, index int) {
	alias := labels
	alias[index]++
}

func zeroTripMakeSlice(alpha []float64, left int) {
	for range make([]struct{}, 0) {
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

func zeroTripMakeSliceWithEvaluatedCapacity(alpha []float64, left int) {
	for range make([]struct{}, 0, round5UnknownSize()) {
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

func nonzeroMakeSlice(alpha []float64, left int) {
	for range make([]struct{}, 1) { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\)`
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

func unknownMakeSlice(alpha []float64, left, size int) {
	for range make([]struct{}, size) { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\)`
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

func zeroTripNamedMakeSlice(alpha []float64, left int) {
	for range make(round5Units, 0) {
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

func zeroTripConvertedString(alpha []float64, left int) {
	for range string(make([]byte, 0)) {
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

func nonzeroConvertedString(alpha []float64, left int) {
	for range string(make([]byte, 1)) { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\)`
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

func zeroTripFreshMap(alpha []float64, left int) {
	for range make(map[int]struct{}, round5UnknownSize()) {
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

func zeroTripFreshChannel(alpha []float64, left int) {
	for range make(chan struct{}, round5UnknownSize()) {
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

func immediatelyInvokedAliasRebind(alpha []float64, left int) {
	alias := alpha
	(func() { alias = nil })()
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

func shortCircuitedLiteralRebind(alpha []float64, left int) {
	alias := alpha
	if true || func() bool { alias = nil; return true }() {
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

func storedUninvokedLiteralRebind(alpha []float64, left int) {
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

func deferredLiteralRebind(alpha []float64, left int) {
	alias := alpha
	defer func() { alias = nil }()
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

func asynchronousLiteralRebind(alpha []float64, left int) {
	alias := alpha
	go func() { alias = nil }()
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

func invokedLiteralParameterRebind(alpha []float64, left int) {
	alias := alpha
	func(local []float64) { local = nil }(alias)
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

func invokedLiteralCapturedMutationHazard(alpha []float64, left int, mutate func([]float64, int)) {
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
		func() { mutate(alpha, left) }()
	}
}

func invokedLiteralArgumentHazard(alpha []float64, left int, mutate func([]float64, int)) {
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
		func(local []float64) { mutate(local, left) }(alpha)
	}
}

func helperAliasChainMany(alpha []float64, left int) {
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
		round5AliasMany(alpha, left)
	}
}

func helperAliasOneWrite(alpha []float64, left int) {
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
		round5AliasOne(alpha, left)
	}
}

func helperAliasDefiniteRebind(alpha, other []float64, left int) {
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
		round5AliasRebindAway(alpha, other, left)
	}
}

func helperAliasBranchHazard(alpha, other []float64, left int, useOther bool) {
	for step := 0; step < 4; step++ { // want `helper alias flow may mutate alpha@[0-9]+`
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
		round5AliasBranch(alpha, other, left, useOther)
	}
}

func helperAliasCallResultHazard(alpha []float64, left int) {
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\)`
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
		round5AliasCallResult(alpha, left)
	}
}

func helperAliasFreshStorage(alpha []float64, left int) {
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
		round5AliasFreshStorage(alpha, left)
	}
}

func helperGenericAliasMany(alpha round5AliasSlice, left int) {
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
		round5GenericAliasMany(alpha, left)
	}
}

func helperGenericAliasOne(alpha round5AliasSlice, left int) {
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
		round5GenericAliasOne(alpha, left)
	}
}

func helperMethodAliasMany(alpha []float64, left int) {
	var mutator round5AliasMutator
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
		mutator.aliasMany(alpha, left)
	}
}

func helperMethodExpressionAliasMany(alpha []float64, left int) {
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
		round5AliasMutator.aliasMany(round5AliasMutator{}, alpha, left)
	}
}

func helperMethodAliasOne(alpha []float64, left int) {
	var mutator round5AliasMutator
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
		mutator.aliasOne(alpha, left)
	}
}

func helperMethodExpressionAliasOne(alpha []float64, left int) {
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
		round5AliasMutator.aliasOne(round5AliasMutator{}, alpha, left)
	}
}

func helperPointerAliasMany(alpha []float64, left int) {
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
		round5PointerAliasMany(&alpha, left)
	}
}

func helperPointerAliasOne(alpha []float64, left int) {
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
		round5PointerAliasOne(&alpha, left)
	}
}

func helperMapAliasMany(alpha []float64, labels map[int]float64, left int) {
	for step := 0; step < 4; step++ {
		for index := range alpha {
			if labels[index] > 0 {
				_ = index
			}
		}
		for index := range alpha {
			if labels[index] < 1 {
				_ = index
			}
		}
		round5MapAliasMany(labels, left)
	}
}

func helperMapAliasOne(alpha []float64, labels map[int]float64, left int) {
	for step := 0; step < 4; step++ {
		for index := range alpha {
			if labels[index] > 0 {
				_ = index
			}
		}
		for index := range alpha {
			if labels[index] < 1 {
				_ = index
			}
		}
		round5MapAliasOne(labels, left)
	}
}

func unrelatedHelperMapAliasWrites(alpha []float64, labels map[int]float64, left int) {
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
		round5MapAliasMany(labels, left)
	}
}
