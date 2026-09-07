package ps6100

func repeatedDirect(alpha []float64, labels []int, left, right int, nextLeft, nextRight, limit float64, stop bool) {
	for step := 0; step < 64; step++ { // want `iterative loop repeats 2 full scans of alpha and derives 2 finite-state predicate bit\(s\) from per-element inputs alpha, labels; 2 bounded indexed mutation site\(s\).*prove reference-backed aliases cannot mutate alpha, labels.*continue@[0-9]+`
		for index := range alpha {
			if labels[index] > 0 && alpha[index] < limit {
				_ = index
			}
		}
		for index := 0; index < len(alpha); index++ {
			if labels[index] < 0 && alpha[index] > 0 {
				_ = index
			}
		}
		alpha[left] = nextLeft
		alpha[right] = nextRight
		if stop {
			continue
		}
	}
}

func isUpper[T ~float32 | ~float64](alpha []T, labels []int, index int, limit T) bool {
	return labels[index] > 0 && alpha[index] < limit
}

func forwardedUpper[T ~float32 | ~float64](alpha []T, labels []int, index int, limit T) bool {
	return isUpper(alpha, labels, index, limit)
}

func isLower[T ~float32 | ~float64](alpha []T, labels []int, index int, limit T) bool {
	return labels[index] < 0 && alpha[index] > -limit
}

func updatePair[T ~float32 | ~float64](alpha []T, left, right int, nextLeft, nextRight T) {
	alpha[left] = nextLeft
	alpha[right] = nextRight
}

func updateElementPointer(value *float64) {
	(*value)++
}

func identityPredicate(value bool) bool {
	return value
}

func scalarAt[T ~float32 | ~float64](values []T, index int) T {
	return values[index]
}

func recursivePredicate(value bool) bool {
	return recursivePredicate(value)
}

func repeatedHelpers[T ~float32 | ~float64](alpha []T, labels []int, left, right int, nextLeft, nextRight, limit T, done bool) {
	for !done { // want `iterative loop repeats 2 full scans of alpha and derives 2 finite-state predicate bit\(s\) from per-element inputs alpha, labels; 2 bounded indexed mutation site\(s\).*prove predicate/config dependencies remain invariant or rebuild all status entries when they change: limit`
		for index := range alpha {
			if forwardedUpper(alpha, labels, index, limit) {
				_ = index
			}
		}
		for index := range alpha {
			if isLower(alpha, labels, index, limit) {
				_ = index
			}
		}
		updatePair(alpha, left, right, nextLeft, nextRight)
		done = true
	}
}

func repeatedNestedAndScalarHelpers(alpha []float64, left int) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha and derives 2 finite-state predicate bit\(s\) from per-element inputs alpha; 1 bounded indexed mutation site\(s\)`
		for index := range alpha {
			if identityPredicate(identityPredicate(alpha[index] > 0)) {
				_ = index
			}
		}
		for index := range alpha {
			if scalarAt(alpha, index) < 1 {
				_ = index
			}
		}
		alpha[left]++
	}
}

func repeatedPointerMutationHelper(alpha []float64, left int) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\).*address exposing alpha escapes local proof@[0-9]+`
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
		updateElementPointer(&alpha[left])
	}
}

func repeatedStableAliasMutation(alpha []float64, left int) {
	alias := alpha
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
		alias[left]++
	}
}

func repeatedStableAliasScans(alpha []float64, left int) {
	alias := alpha
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

func repeatedStableAliasChain(alpha []float64, left int) {
	first := alpha
	second := first
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\)`
		for index := range second {
			if second[index] > 0 {
				_ = index
			}
		}
		for index := range second {
			if second[index] < 1 {
				_ = index
			}
		}
		alpha[left]++
	}
}

func reassignedSourceAfterAliasCapture(alpha, other []float64, left int) {
	alias := alpha
	alpha = other
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

func unstableAliasMutation(alpha, other []float64, left int) {
	alias := alpha
	alias = other
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
		alias[left]++
	}
}

func repeatedWithOpaqueHazard(alpha []float64, left int, next float64, mutate func([]float64)) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha and derives 2 finite-state predicate bit\(s\).*incremental refresh is unsafe until these bulk/escape hazards are resolved: opaque call may bulk-mutate or retain alpha@[0-9]+`
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
		alpha[left] = next
		mutate(alpha)
	}
}

func repeatedRangeValues(alpha []float64, left int) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha and derives 2 finite-state predicate bit\(s\) from per-element inputs alpha; 1 bounded indexed mutation site\(s\)`
		for _, value := range alpha {
			if value > 0 {
				_ = value
			}
		}
		for _, value := range alpha {
			if value < 1 {
				_ = value
			}
		}
		alpha[left]++
	}
}

func repeatedWithClosureHazard(alpha []float64, left int) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha and derives 2 finite-state predicate bit\(s\).*closure may mutate or retain alpha@[0-9]+`
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

func repeatedWithRebinding(alpha []float64, left int) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha and derives 2 finite-state predicate bit\(s\).*rebinding alpha invalidates cached membership@[0-9]+`
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
		alpha = alpha[:len(alpha)]
	}
}

type ps6100State struct {
	alpha []float64
}

func (state *ps6100State) update(index int) {
	state.alpha[index]++
}

func repeatedFieldWithMethodMutation(current *ps6100State, left int) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of current.alpha.*1 bounded indexed mutation site\(s\)`
		for index := range current.alpha {
			if current.alpha[index] > 0 {
				_ = index
			}
		}
		for index := range current.alpha {
			if current.alpha[index] < 1 {
				_ = index
			}
		}
		current.update(left)
	}
}

func repeatedFieldWithMethodExpressionMutation(current *ps6100State, left int) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of current.alpha.*1 bounded indexed mutation site\(s\)`
		for index := range current.alpha {
			if current.alpha[index] > 0 {
				_ = index
			}
		}
		for index := range current.alpha {
			if current.alpha[index] < 1 {
				_ = index
			}
		}
		(*ps6100State).update(current, left)
	}
}

func repeatedFieldWithPrefixEscape(current *ps6100State, left int, mutate func(*ps6100State)) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of current.alpha.*opaque call may bulk-mutate or retain current@[0-9]+`
		for index := range current.alpha {
			if current.alpha[index] > 0 {
				_ = index
			}
		}
		for index := range current.alpha {
			if current.alpha[index] < 1 {
				_ = index
			}
		}
		current.alpha[left]++
		mutate(current)
	}
}

func repeatedArrayWithAddressAlias(alpha [8]float64, left int) {
	alias := &alpha
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*address exposing alpha escapes local proof@[0-9]+`
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
		(*alias)[0]++
	}
}

func repeatedWithNestedBranches(alpha []float64, left int) {
	for step := 0; step < 4; step++ { // want `refresh every changed index before these exit/continue paths: continue@[0-9]+; initialize once`
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
		for nested := 0; nested < 2; nested++ {
			continue
		}
		switch step {
		case -1:
			break
		}
		continue
	}
}

func oneScanOnly(alpha []float64, left int) {
	for step := 0; step < 4; step++ {
		for index := range alpha {
			if alpha[index] > 0 {
				_ = index
			}
		}
		alpha[left]++
	}
}

func noRelevantMutation(alpha []float64) {
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
	}
}

func rebuiltEachIteration(source []float64, left int) {
	for step := 0; step < 4; step++ {
		alpha := append([]float64(nil), source...)
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

func rebuiltFieldEachIteration(source []float64, left int) {
	for step := 0; step < 4; step++ {
		state := struct{ alpha []float64 }{alpha: append([]float64(nil), source...)}
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

func tooManyMutationIndexes(alpha []float64, indexes [5]int) {
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
		alpha[indexes[0]]++
		alpha[indexes[1]]++
		alpha[indexes[2]]++
		alpha[indexes[3]]++
		alpha[indexes[4]]++
	}
}

func tooManyWritesAtSameIndex(alpha []float64, left int) {
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
		alpha[left]++
		alpha[left]++
		alpha[left]++
		alpha[left]++
	}
}

func tooManyShadowedMutationIndexes(alpha []float64) {
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
		{
			index := 0
			alpha[index]++
		}
		{
			index := 1
			alpha[index]++
		}
		{
			index := 2
			alpha[index]++
		}
		{
			index := 3
			alpha[index]++
		}
		{
			index := 4
			alpha[index]++
		}
	}
}

func tooManyMatrixRows(matrix [][]float64, rows [5]int) {
	for step := 0; step < 4; step++ {
		for row := range matrix {
			if len(matrix[row]) != 0 && matrix[row][0] > 0 {
				_ = row
			}
		}
		for row := range matrix {
			if len(matrix[row]) != 0 && matrix[row][0] < 1 {
				_ = row
			}
		}
		matrix[rows[0]][0]++
		matrix[rows[1]][0]++
		matrix[rows[2]][0]++
		matrix[rows[3]][0]++
		matrix[rows[4]][0]++
	}
}

func differentCollections(alpha, beta []float64, left int) {
	for step := 0; step < 4; step++ {
		for index := range alpha {
			if alpha[index] > 0 {
				_ = index
			}
		}
		for index := range beta {
			if beta[index] > 0 {
				_ = index
			}
		}
		alpha[left]++
	}
}

func partialScans(alpha []float64, left int) {
	for step := 0; step < 4; step++ {
		for index := 1; index < len(alpha); index++ {
			if alpha[index] > 0 {
				_ = index
			}
		}
		for index := 0; index+1 < len(alpha); index++ {
			if alpha[index] < 1 {
				_ = index
			}
		}
		alpha[left]++
	}
}

func mutationInsideScansOnly(alpha []float64) {
	for step := 0; step < 4; step++ {
		for index := range alpha {
			if alpha[index] > 0 {
				alpha[index]--
			}
		}
		for index := range alpha {
			if alpha[index] < 1 {
				alpha[index]++
			}
		}
	}
}

func bulkMutationLoop(alpha []float64, left int) {
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
		for index := range alpha {
			alpha[index]++
		}
	}
}

var predicateCalls int

func impurePredicate(value float64) bool {
	predicateCalls++
	return value > 0
}

func impurePredicateScans(alpha []float64, left int) {
	for step := 0; step < 4; step++ {
		for index := range alpha {
			if impurePredicate(alpha[index]) {
				_ = index
			}
		}
		for index := range alpha {
			if impurePredicate(alpha[index]) {
				_ = index
			}
		}
		alpha[left]++
	}
}

func recursivePredicateScans(alpha []float64, left int) {
	for step := 0; step < 4; step++ {
		for index := range alpha {
			if recursivePredicate(alpha[index] > 0) {
				_ = index
			}
		}
		for index := range alpha {
			if recursivePredicate(alpha[index] < 1) {
				_ = index
			}
		}
		alpha[left]++
	}
}

func receivingPredicateScans(alpha []float64, left int, predicates <-chan bool) {
	for step := 0; step < 4; step++ {
		for index := range alpha {
			if <-predicates && alpha[index] > 0 {
				_ = index
			}
		}
		for index := range alpha {
			if <-predicates && alpha[index] < 1 {
				_ = index
			}
		}
		alpha[left]++
	}
}

func mutuallyExclusiveScans(alpha []float64, left int) {
	for step := 0; step < 4; step++ {
		if step&1 == 0 {
			for index := range alpha {
				if alpha[index] > 0 {
					_ = index
				}
			}
		} else {
			for index := range alpha {
				if alpha[index] < 1 {
					_ = index
				}
			}
		}
		alpha[left]++
	}
}

func mutatingScanPlusBoundedWrite(alpha []float64, left int) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*loop-carried writes may bulk-mutate alpha@[0-9]+`
		for index := range alpha {
			if alpha[index] > 0 {
				alpha[index]--
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

func mutateDuringScan(alpha []float64, index int) {
	alpha[index]++
}

func helperMutatingScanPlusBoundedWrite(alpha []float64, left int) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*loop-carried writes may bulk-mutate alpha@[0-9]+`
		for index := range alpha {
			if alpha[index] > 0 {
				mutateDuringScan(alpha, index)
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

func scansInOneConditionalArm(alpha []float64, left int, enabled bool) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\)`
		if enabled {
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

func mutuallyExclusiveSwitchScans(alpha []float64, left, mode int) {
	for step := 0; step < 4; step++ {
		switch mode {
		case 0:
			for index := range alpha {
				if alpha[index] > 0 {
					_ = index
				}
			}
		default:
			for index := range alpha {
				if alpha[index] < 1 {
					_ = index
				}
			}
		}
		alpha[left]++
	}
}

func scansSeparatedByContinue(alpha []float64, left int, first bool) {
	for step := 0; step < 4; step++ {
		if first {
			for index := range alpha {
				if alpha[index] > 0 {
					_ = index
				}
			}
			continue
		}
		for index := range alpha {
			if alpha[index] < 1 {
				_ = index
			}
		}
		alpha[left]++
	}
}

func fallthroughScansCanCoexecute(alpha []float64, left, mode int) {
	for step := 0; step < 4; step++ { // want `iterative loop repeats 2 full scans of alpha.*1 bounded indexed mutation site\(s\)`
		switch mode {
		case 0:
			for index := range alpha {
				if alpha[index] > 0 {
					_ = index
				}
			}
			fallthrough
		default:
			for index := range alpha {
				if alpha[index] < 1 {
					_ = index
				}
			}
		}
		alpha[left]++
	}
}

func modifiedCountIndexes(alpha []float64, left int) {
	for step := 0; step < 4; step++ {
		for index := 0; index < len(alpha); index++ {
			index++
			if index < len(alpha) && alpha[index] > 0 {
				_ = index
			}
		}
		for index := 0; index < len(alpha); index++ {
			index += 0
			if alpha[index] < 1 {
				_ = index
			}
		}
		alpha[left]++
	}
}

func earlyBreakScans(alpha []float64, left int) {
	for step := 0; step < 4; step++ {
		for index := range alpha {
			if alpha[index] > 0 {
				break
			}
		}
		for index := range alpha {
			if alpha[index] < 1 {
				break
			}
		}
		alpha[left]++
	}
}

func labeledEarlyBreakScans(alpha []float64, left int) {
	for step := 0; step < 4; step++ {
	firstScan:
		for index := range alpha {
			if alpha[index] > 0 {
				break firstScan
			}
		}
	secondScan:
		for index := range alpha {
			if alpha[index] < 1 {
				break secondScan
			}
		}
		alpha[left]++
	}
}

func overwrittenRangeValues(alpha []float64, left int) {
	for step := 0; step < 4; step++ {
		for _, value := range alpha {
			value = 0
			if value > 0 {
				_ = value
			}
		}
		for _, value := range alpha {
			value = 1
			if value < 1 {
				_ = value
			}
		}
		alpha[left]++
	}
}

func aliasedCountIndexes(alpha []float64, left int) {
	var first, second int
	firstPointer, secondPointer := &first, &second
	for step := 0; step < 4; step++ {
		for first = 0; first < len(alpha); first++ {
			(*firstPointer)++
			if first < len(alpha) && alpha[first] > 0 {
				_ = first
			}
		}
		for second = 0; second < len(alpha); second++ {
			(*secondPointer)++
			if second < len(alpha) && alpha[second] < 1 {
				_ = second
			}
		}
		alpha[left]++
	}
}
