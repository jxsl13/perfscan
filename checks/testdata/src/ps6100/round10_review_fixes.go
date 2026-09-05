package ps6100

type round10DeepRoot struct{ next *Box }

func round10DeepSelectorRebind(root *round10DeepRoot, other []float64) {
	root.next.state.alpha = other
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\) \(root.next.state.alpha\[0\]@[0-9]+\)`
		for index := range root.next.state.alpha {
			if root.next.state.alpha[index] > 0 {
				_ = index
			}
		}
		for index := range root.next.state.alpha {
			if root.next.state.alpha[index] < 1 {
				_ = index
			}
		}
		other[0]++
	}
}

type round10NamedBox struct{ state *NamedState }

func (values *Slice) round10WritePtrFive() {
	(*values)[0]++
	(*values)[1]++
	(*values)[2]++
	(*values)[3]++
	(*values)[4]++
}

// A pointer-method value captures the old field address before its selector
// prefix is replaced. Only the explicit replacement-field write is relevant.
func round10StoredReceiverThenPrefixReplacement(box *round10NamedBox, replacement Slice) {
	stored := box.state.values.writePtrZero
	box.state = &NamedState{values: replacement}
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\) \(box.state.values\[1\]@[0-9]+\)`
		for index := range box.state.values {
			if box.state.values[index] > 0 {
				_ = index
			}
		}
		for index := range box.state.values {
			if box.state.values[index] < 1 {
				_ = index
			}
		}
		stored()
		box.state.values[1]++
	}
}

// Forming the method value after replacement captures the scanned field.
func round10PrefixReplacementThenStoredReceiver(box *round10NamedBox, replacement Slice) {
	box.state = &NamedState{values: replacement}
	stored := box.state.values.writePtrZero
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\) \(box.state.values\[0\]@[0-9]+, box.state.values\[1\]@[0-9]+\)`
		for index := range box.state.values {
			if box.state.values[index] > 0 {
				_ = index
			}
		}
		for index := range box.state.values {
			if box.state.values[index] < 1 {
				_ = index
			}
		}
		stored()
		box.state.values[1]++
	}
}

// Five old-object writes must not consume the replacement's four-site budget.
func round10StoredReceiverOldFiveThenPrefixReplacement(box *round10NamedBox, replacement Slice) {
	stored := box.state.values.round10WritePtrFive
	box.state = &NamedState{values: replacement}
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\) \(box.state.values\[0\]@[0-9]+\)`
		for index := range box.state.values {
			if box.state.values[index] > 0 {
				_ = index
			}
		}
		for index := range box.state.values {
			if box.state.values[index] < 1 {
				_ = index
			}
		}
		stored()
		box.state.values[0]++
	}
}

// Replacing a dereferenced root likewise cannot retarget the saved field address.
func round10StoredReceiverThenDereferenceReplacement(slot **round10NamedBox, replacement Slice) {
	stored := (*slot).state.values.round10WritePtrFive
	*slot = &round10NamedBox{state: &NamedState{values: replacement}}
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\) \(\(\*slot\).state.values\[0\]@[0-9]+\)`
		for index := range (*slot).state.values {
			if (*slot).state.values[index] > 0 {
				_ = index
			}
		}
		for index := range (*slot).state.values {
			if (*slot).state.values[index] < 1 {
				_ = index
			}
		}
		stored()
		(*slot).state.values[0]++
	}
}

func round10AddressAliasThenPrefixReplacement(box *round10NamedBox, replacement Slice) {
	saved := &box.state.values
	box.state = &NamedState{values: replacement}
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\) \(box.state.values\[1\]@[0-9]+\).*address exposing box.state.values escapes local proof`
		for index := range box.state.values {
			if box.state.values[index] > 0 {
				_ = index
			}
		}
		for index := range box.state.values {
			if box.state.values[index] < 1 {
				_ = index
			}
		}
		(*saved)[0]++
		box.state.values[1]++
	}
}

func round10PrefixReplacementThenAddressAlias(box *round10NamedBox, replacement Slice) {
	box.state = &NamedState{values: replacement}
	saved := &box.state.values
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\) \(box.state.values\[0\]@[0-9]+, box.state.values\[1\]@[0-9]+\).*address exposing box.state.values escapes local proof`
		for index := range box.state.values {
			if box.state.values[index] > 0 {
				_ = index
			}
		}
		for index := range box.state.values {
			if box.state.values[index] < 1 {
				_ = index
			}
		}
		(*saved)[0]++
		box.state.values[1]++
	}
}

// Array assignment copies elements rather than preserving source aliases.
func round10ArrayPointerRebind(slot *[8]float64, other [8]float64) {
	*slot = other
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\) \(slot\[1\]@[0-9]+\)`
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
		(*slot)[1]++
	}
}

// Both nonconstant bounds are the same runtime value, so the scans are zero-trip.
func round10EqualDynamicBounds(other []float64, bound int) {
	alpha := other[bound:bound]
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
	}
}

func round10EqualLenBounds(other []float64) {
	alpha := other[len(other):len(other)]
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
	}
}

func round10DynamicOffsetExact(other []float64, low int) {
	alpha := other[low:]
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\) \(alpha\[-low \+ low\]@[0-9]+, alpha\[1\]@[0-9]+\)`
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
		other[low]++
		alpha[1]++
	}
}

func round10DynamicOffsetDirectOnly(other []float64, low int) {
	alpha := other[low:]
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\) \(alpha\[1\]@[0-9]+\)`
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
		alpha[1]++
	}
}

func round10ConstantOffsetDirectOnly(other []float64) {
	alpha := other[2:]
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\) \(alpha\[1\]@[0-9]+\)`
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
		alpha[1]++
	}
}

func round10TupleResliceInit(other, third []float64) {
	alpha, retained := other[2:], third
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\) \(alpha\[0\]@[0-9]+, alpha\[1\]@[0-9]+\)`
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
		retained[0]++
		alpha[1]++
	}
}

func round10FullSliceInit(other []float64) {
	alpha := other[2:len(other):len(other)]
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\) \(alpha\[0\]@[0-9]+, alpha\[1\]@[0-9]+\)`
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

func round10CopyThroughReboundSource(other, source []float64) {
	alpha := other[2:]
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\) \(alpha\[0\]@[0-9]+\).*opaque call may bulk-mutate or retain alpha`
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
		copy(other[2:], source)
		alpha[0]++
	}
}

func round10ClearThroughReboundSource(other []float64) {
	alpha := other[2:]
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\) \(alpha\[0\]@[0-9]+\).*opaque call may bulk-mutate or retain alpha`
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
		clear(other[2:])
		alpha[0]++
	}
}

// The source write precedes the scanned view and must not count against it.
func round10OutsideOverlappingView(other []float64) {
	alpha := other[2:]
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\) \(alpha\[0\]@[0-9]+\)`
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
		alpha[0]++
	}
}

type round10GenericSlice[T ~float64] []T

func (values round10GenericSlice[T]) round10WriteGeneric(index int) { values[index]++ }

func round10GenericReceiverHelper(alpha round10GenericSlice[float64]) {
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\) \(alpha\[0\]@[0-9]+, alpha\[1\]@[0-9]+\)`
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
		alpha.round10WriteGeneric(0)
		alpha[1]++
	}
}

func round10StoredGenericReceiver(alpha round10GenericSlice[float64]) {
	stored := alpha.round10WriteGeneric
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\) \(alpha\[0\]@[0-9]+, alpha\[1\]@[0-9]+\)`
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
		stored(0)
		alpha[1]++
	}
}
