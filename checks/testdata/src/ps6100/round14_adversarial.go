package ps6100

func round14SetAndReturn(pointer *int) int { *pointer = 4; return 0 }
func round14SetAndReturnBool(pointer *int) bool {
	*pointer = 4
	return true
}
func round14Consume(int) {}

func round14HelperInAssignment(other []float64) {
	low := 2
	_ = round14SetAndReturn(&low)
	alpha := other[low:]
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\).*alpha\[0\].*alpha\[1\]`
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
		other[4]++
		alpha[1]++
	}
}

func round14HelperInArgument(other []float64) {
	low := 2
	round14Consume(round14SetAndReturn(&low))
	alpha := other[low:]
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\).*alpha\[0\].*alpha\[1\]`
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
		other[4]++
		alpha[1]++
	}
}

func round14HelperInCompositeLiteral(other []float64) {
	low := 2
	_ = []int{round14SetAndReturn(&low)}
	alpha := other[low:]
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\).*alpha\[0\].*alpha\[1\]`
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
		other[4]++
		alpha[1]++
	}
}

func round14ConditionalBoundCall(other []float64, flag bool) {
	low := 2
	_ = flag && round14SetAndReturnBool(&low)
	alpha := other[low:]
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\).*pre-loop slice bounds prevent exact alias offset proof`
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
		other[4]++
		alpha[1]++
	}
}

func round14IndirectIncrement(other []float64) {
	low := 2
	pointer := &low
	(*pointer)++
	(*pointer)++
	alpha := other[low:]
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\).*alpha\[0\].*alpha\[1\]`
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
		other[4]++
		alpha[1]++
	}
}

func round14IndirectExactRange(other []float64) {
	low := 2
	pointer := &low
	for _, *pointer = range []int{4} {
	}
	alpha := other[low:]
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\).*alpha\[0\].*alpha\[1\]`
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
		other[4]++
		alpha[1]++
	}
}

func round14EmptyRangePreservesBound(other []float64) {
	low := 2
	for _, low = range []int{} {
	}
	alpha := other[low:]
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\).*alpha\[0\].*alpha\[1\]`
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
		other[2]++
		alpha[1]++
	}
}

type round14Deep struct {
	nested struct{ rows [1][]float64 }
}

func round14MutateDeep(value round14Deep) { value.nested.rows[0][0]++ }

func round14DeepReference(value round14Deep) {
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\).*value\.nested\.rows\[0\]\[0\].*value\.nested\.rows\[0\]\[1\]`
		for index := range value.nested.rows[0] {
			if value.nested.rows[0][index] > 0 {
				_ = index
			}
		}
		for index := range value.nested.rows[0] {
			if value.nested.rows[0][index] < 1 {
				_ = index
			}
		}
		round14MutateDeep(value)
		value.nested.rows[0][1]++
	}
}

type round14MapBox struct{ values map[int]float64 }

func round14MutateMap(value round14MapBox) { value.values[0]++ }

// Map iteration is deliberately not treated as an exact full-slice scan.
func round14MapReference(value round14MapBox) {
	for step := 0; step < 4; step++ {
		for index := range value.values {
			if value.values[index] > 0 {
				_ = index
			}
		}
		for index := range value.values {
			if value.values[index] < 1 {
				_ = index
			}
		}
		round14MutateMap(value)
		value.values[1]++
	}
}

type round14ArrayBox struct{ values [4]float64 }

func round14MutateArrayPointer(value *round14ArrayBox) { value.values[0]++ }

func round14ArrayPointerReference(value *round14ArrayBox) {
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\).*value\.values\[0\].*value\.values\[1\]`
		for index := range value.values {
			if value.values[index] > 0 {
				_ = index
			}
		}
		for index := range value.values {
			if value.values[index] < 1 {
				_ = index
			}
		}
		round14MutateArrayPointer(value)
		value.values[1]++
	}
}

type round14IndexedLeaf struct{ values []float64 }
type round14IndexedBox struct{ items [2]*round14IndexedLeaf }

func round14IndexedBump(value *round14IndexedLeaf, index int) { value.values[index]++ }
func (value *round14IndexedLeaf) round14Bump(index int)       { value.values[index]++ }

func round14IndexedNestedPointer(value *round14IndexedBox) {
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\).*value\.items\[0\]\.values\[0\].*value\.items\[0\]\.values\[1\]`
		for index := range value.items[0].values {
			if value.items[0].values[index] > 0 {
				_ = index
			}
		}
		for index := range value.items[0].values {
			if value.items[0].values[index] < 1 {
				_ = index
			}
		}
		round14IndexedBump(value.items[0], 0)
		value.items[0].values[1]++
	}
}

func round14DifferentIndexedPointer(value *round14IndexedBox) {
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\) \(value\.items\[0\]\.values\[1\]@[0-9]+\)`
		for index := range value.items[0].values {
			if value.items[0].values[index] > 0 {
				_ = index
			}
		}
		for index := range value.items[0].values {
			if value.items[0].values[index] < 1 {
				_ = index
			}
		}
		round14IndexedBump(value.items[1], 0)
		value.items[0].values[1]++
	}
}

func round14StoredIndexedReceiverAfterReplacement(value *round14IndexedBox, replacement []float64) {
	bump := value.items[0].round14Bump
	value.items[0] = &round14IndexedLeaf{replacement}
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\) \(value\.items\[0\]\.values\[1\]@[0-9]+\)`
		for index := range value.items[0].values {
			if value.items[0].values[index] > 0 {
				_ = index
			}
		}
		for index := range value.items[0].values {
			if value.items[0].values[index] < 1 {
				_ = index
			}
		}
		bump(0)
		value.items[0].values[1]++
	}
}

type round14ReceiverLeaf struct{ values []float64 }
type round14ReceiverInner struct{ *round14ReceiverLeaf }
type round14ReceiverOuter struct{ *round14ReceiverInner }

func (value *round14ReceiverLeaf) round14ReceiverBump(index int) { value.values[index]++ }
func (value *round14ReceiverOuter) round14Replace(replacement []float64) {
	value.round14ReceiverInner = &round14ReceiverInner{&round14ReceiverLeaf{replacement}}
}
func (value *round14ReceiverOuter) round14NoReplacement(replacement []float64) {
	unused := func() {
		value.round14ReceiverInner = &round14ReceiverInner{&round14ReceiverLeaf{replacement}}
	}
	_ = unused
}
func (value *round14ReceiverOuter) round14FalseReplacement(replacement []float64) {
	if false {
		value.round14ReceiverInner = &round14ReceiverInner{&round14ReceiverLeaf{replacement}}
	}
}
func (value *round14ReceiverOuter) round14ReplaceViaHelper(replacement []float64) {
	value.round14Replace(replacement)
}

func round14DirectReplacingMethod(value *round14ReceiverOuter, replacement []float64) {
	bump := value.round14ReceiverBump
	value.round14Replace(replacement)
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\)`
		for index := range value.values {
			if value.values[index] > 0 {
				_ = index
			}
		}
		for index := range value.values {
			if value.values[index] < 1 {
				_ = index
			}
		}
		bump(0)
		value.values[1]++
	}
}

func round14StoredReplacingMethod(value *round14ReceiverOuter, replacement []float64) {
	bump := value.round14ReceiverBump
	replace := value.round14Replace
	replace(replacement)
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\)`
		for index := range value.values {
			if value.values[index] > 0 {
				_ = index
			}
		}
		for index := range value.values {
			if value.values[index] < 1 {
				_ = index
			}
		}
		bump(0)
		value.values[1]++
	}
}

func round14ClosureReplacing(value *round14ReceiverOuter, replacement []float64) {
	bump := value.round14ReceiverBump
	replace := func() { value.round14ReceiverInner = &round14ReceiverInner{&round14ReceiverLeaf{replacement}} }
	replace()
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\)`
		for index := range value.values {
			if value.values[index] > 0 {
				_ = index
			}
		}
		for index := range value.values {
			if value.values[index] < 1 {
				_ = index
			}
		}
		bump(0)
		value.values[1]++
	}
}

func round14UncalledNestedReplacement(value *round14ReceiverOuter, replacement []float64) {
	bump := value.round14ReceiverBump
	value.round14NoReplacement(replacement)
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\)`
		for index := range value.values {
			if value.values[index] > 0 {
				_ = index
			}
		}
		for index := range value.values {
			if value.values[index] < 1 {
				_ = index
			}
		}
		bump(0)
		value.values[1]++
	}
}

func round14FalseBranchReplacement(value *round14ReceiverOuter, replacement []float64) {
	bump := value.round14ReceiverBump
	value.round14FalseReplacement(replacement)
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\)`
		for index := range value.values {
			if value.values[index] > 0 {
				_ = index
			}
		}
		for index := range value.values {
			if value.values[index] < 1 {
				_ = index
			}
		}
		bump(0)
		value.values[1]++
	}
}

func round14ForwardedReplacement(value *round14ReceiverOuter, replacement []float64) {
	bump := value.round14ReceiverBump
	value.round14ReplaceViaHelper(replacement)
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\)`
		for index := range value.values {
			if value.values[index] > 0 {
				_ = index
			}
		}
		for index := range value.values {
			if value.values[index] < 1 {
				_ = index
			}
		}
		bump(0)
		value.values[1]++
	}
}

func round14Cycle() bool { return round14Cycle() }

func round14IIFERecursive(values []float64) {
	_ = func() bool { return round14Cycle() }()
	for step := 0; step < 4; step++ {
		for index := range values {
			if values[index] > 0 {
				_ = index
			}
		}
		for index := range values {
			if values[index] < 1 {
				_ = index
			}
		}
		values[0]++
	}
}

func round14IIFEShortCircuitedRecursive(values []float64) {
	_ = true || func() bool { return round14Cycle() }()
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\)`
		for index := range values {
			if values[index] > 0 {
				_ = index
			}
		}
		for index := range values {
			if values[index] < 1 {
				_ = index
			}
		}
		values[0]++
	}
}
