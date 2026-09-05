package ps6100

type Slice []float64

type State struct {
	alpha []float64
}

func (values Slice) writeZero() { values[0]++ }

type Box struct {
	state *State
}

type NamedState struct {
	values Slice
}

func (values *Slice) writePtrZero() { (*values)[0]++ }

// The source aliases the scanned header through a pointer-to-selector chain.
func PointerSelectorChain(box **Box, other []float64) {
	(*box).state.alpha = other
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\) \(\(\*box\).state.alpha\[0\]@[0-9]+\)`
		for index := range (*box).state.alpha {
			if (*box).state.alpha[index] > 0 {
				_ = index
			}
		}
		for index := range (*box).state.alpha {
			if (*box).state.alpha[index] < 1 {
				_ = index
			}
		}
		other[0]++
	}
}

// The same pointer-to-selector chain without an explicit dereference.
func NestedPointerSelector(box *Box, other []float64) {
	box.state.alpha = other
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\) \(box.state.alpha\[0\]@[0-9]+\)`
		for index := range box.state.alpha {
			if box.state.alpha[index] > 0 {
				_ = index
			}
		}
		for index := range box.state.alpha {
			if box.state.alpha[index] < 1 {
				_ = index
			}
		}
		other[0]++
	}
}

// A stored pointer-receiver method captures the address of the field, so it
// observes the replacement header assigned afterward.
func StoredPointerFieldReceiver(state *NamedState, replacement Slice) {
	stored := state.values.writePtrZero
	state.values = replacement
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\) \(state.values\[0\]@[0-9]+, state.values\[1\]@[0-9]+\)`
		for index := range state.values {
			if state.values[index] > 0 {
				_ = index
			}
		}
		for index := range state.values {
			if state.values[index] < 1 {
				_ = index
			}
		}
		stored()
		state.values[1]++
	}
}

// A direct pointer-receiver call on the field after rebinding observes the
// current field header and must count as a write to the scanned replacement.
func DirectPointerFieldReceiver(state *NamedState, replacement Slice) {
	state.values = replacement
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\) \(state.values\[0\]@[0-9]+, state.values\[1\]@[0-9]+\)`
		for index := range state.values {
			if state.values[index] > 0 {
				_ = index
			}
		}
		for index := range state.values {
			if state.values[index] < 1 {
				_ = index
			}
		}
		state.values.writePtrZero()
		state.values[1]++
	}
}

// A stored pointer receiver on a local captures &alpha and therefore follows
// the later header rebind.
func StoredPointerLocalReceiver(alpha, replacement Slice) {
	stored := (&alpha).writePtrZero
	alpha = replacement
	for step := 0; step < 4; step++ { // want "iterative loop repeats"
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
		stored()
		alpha[1]++
	}
}

// The direct pointer call is on old, whose header was copied before alpha was
// rebound, so old's mutation does not affect the scanned replacement.
func DirectPointerLocalSnapshot(alpha, replacement Slice) {
	old := alpha
	alpha = replacement
	for step := 0; step < 4; step++ { // want "iterative loop repeats"
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
		(&old).writePtrZero()
		alpha[1]++
	}
}

// Full-slice expressions preserve the backing-store offset: other[1] is
// exactly alpha[0]. Both explicit writes are refresh sites.
func FullSliceCapacityOffset(alpha, other []float64) {
	alpha = other[1:len(other):len(other)]
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
		other[1]++
		alpha[1]++
	}
}

// A constant-empty ordinary reslice makes both scans zero-trip.
func ConstantEmptyLowHigh(alpha, other []float64) {
	alpha = other[2:2]
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

// A constant-zero high bound also proves an empty view.
func ConstantEmptyHigh(alpha, other []float64) {
	alpha = other[:0]
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

// A full-slice expression with zero high and max is likewise empty.
func ConstantEmptyFullSlice(alpha, other []float64) {
	alpha = other[:0:0]
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

// Unknown bounds must remain conservatively executable.
func UnknownBounds(alpha, other []float64, low, high int) {
	alpha = other[low:high]
	for step := 0; step < 4; step++ { // want "iterative loop repeats"
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

// The retained header aliases the scanned slice even though its own header is
// resliced afterward; retained[1] is alpha[0].
func OverlappingForwardReslices(alpha []float64) {
	retained := alpha
	alpha = alpha[2:]
	retained = retained[1:]
	for step := 0; step < 4; step++ { // want "iterative loop repeats"
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
		retained[1]++
		alpha[1]++
	}
}

// Apply overlapping reslices in the reverse assignment order. The saved
// header still reaches alpha[0] through retained[2].
func OverlappingReverseReslices(alpha []float64) {
	retained := alpha
	retained = retained[1:]
	alpha = alpha[2:]
	for step := 0; step < 4; step++ { // want "iterative loop repeats"
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
		retained[1]++
		alpha[1]++
	}
}

// append can either preserve or replace the backing store, so a retained
// pre-append header is an alias ambiguity that must be surfaced.
func AppendMayReallocate(alpha []float64) {
	old := alpha
	alpha = append(alpha, 1)
	for step := 0; step < 4; step++ { // want "iterative loop repeats"
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
		old[0]++
		alpha[1]++
	}
}

// A cap-limited full slice forces append to allocate, so old cannot alias the
// scanned result.
func AppendForcedReallocate(alpha []float64) {
	old := alpha
	alpha = append(alpha[:len(alpha):len(alpha)], 1)
	for step := 0; step < 4; step++ { // want "iterative loop repeats"
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
		old[0]++
		alpha[1]++
	}
}

// copy is a bulk write and must appear as an unsafe-refresh hazard.
func CopyBulkMutation(alpha, source []float64) {
	for step := 0; step < 4; step++ { // want "iterative loop repeats"
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
		copy(alpha, source)
		alpha[0]++
	}
}

func genericWrite[T ~[]float64](values T, index int) { values[index]++ }

func GenericHelperMutation(alpha []float64) {
	for step := 0; step < 4; step++ { // want "iterative loop repeats"
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
		genericWrite(alpha, 0)
		alpha[1]++
	}
}

func MethodExpressionMutation(alpha Slice) {
	for step := 0; step < 4; step++ { // want "iterative loop repeats"
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
		Slice.writeZero(alpha)
		alpha[1]++
	}
}

// Array assignment copies elements rather than aliasing the source.
func ArrayCopyRebind(alpha, other [8]float64) {
	alpha = other
	for step := 0; step < 4; step++ { // want "iterative loop repeats"
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
		alpha[1]++
	}
}

// A map lookup may be a predicate dependency while alpha remains the changed
// per-element input; the map mutation must not be mistaken for alpha mutation.
func MapPredicateDependency(alpha []float64, flags map[int]bool) {
	for step := 0; step < 4; step++ { // want "iterative loop repeats"
		for index := range alpha {
			if flags[index] && alpha[index] > 0 {
				_ = index
			}
		}
		for index := range alpha {
			if flags[index] && alpha[index] < 1 {
				_ = index
			}
		}
		flags[0] = !flags[0]
		alpha[1]++
	}
}

// A zero-length byte slice converted from an empty string is zero-trip.
func EmptyStringSlice(alpha []byte) {
	alpha = []byte("")
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

// snapshot retains other's header before both roots are rebound; only the
// snapshot write aliases the scanned alpha value.
func AliasBeforeMultipleRebinds(alpha, other, third []float64) {
	snapshot := other
	alpha = other
	other = third
	for step := 0; step < 4; step++ { // want "iterative loop repeats"
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
		snapshot[0]++
		other[0]++
		alpha[1]++
	}
}

// A retained alias of an intermediate alpha value must not be conflated with
// the final scanned value after a second rebind.
func AliasBetweenMultipleRebinds(alpha, other, third []float64) {
	alpha = other
	intermediate := alpha
	alpha = third
	for step := 0; step < 4; step++ { // want "iterative loop repeats"
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
		intermediate[0]++
		alpha[1]++
	}
}

// Tuple assignment snapshots both headers before rebinding them.
func TupleSnapshotMultiple(alpha, other, third []float64) {
	kept := other
	alpha, other = other, third
	for step := 0; step < 4; step++ { // want "iterative loop repeats"
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
		kept[0]++
		other[0]++
		alpha[1]++
	}
}

// Five distinct writes through the retained source exceed the limit after a
// full-slice offset rebind, so this must remain silent.
func FullSliceOffsetOverLimit(alpha, other []float64) {
	alpha = other[1:len(other):cap(other)]
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
		other[1]++
		other[2]++
		other[3]++
		other[4]++
		other[5]++
	}
}

// The direct alpha write makes a candidate unless all five retained-source
// writes are correctly rebased. The true six-site count is over the limit.
func FullSliceOffsetOverLimitWithDirect(alpha, other []float64) {
	alpha = other[1:len(other):cap(other)]
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
		other[1]++
		other[2]++
		other[3]++
		other[4]++
		other[5]++
	}
}
