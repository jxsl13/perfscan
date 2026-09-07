package ps6100

// Recursive helper-return provenance must terminate analysis without treating
// code after the source-proved non-returning call as reachable.
func round12RecursiveView(values []float64) []float64 { return round12RecursiveView(values) }

func round12RecursiveControl(values []float64) {
	alias := round12RecursiveView(values)
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
		alias[0]++
	}
}

func round12MutualFirst(values []float64) []float64  { return round12MutualSecond(values) }
func round12MutualSecond(values []float64) []float64 { return round12MutualFirst(values) }

type round12RecursiveValues []float64

func (values round12RecursiveValues) view() round12RecursiveValues { return values.view() }

func round12RecursiveControls(values []float64, named round12RecursiveValues) {
	first := round12MutualFirst(values)
	second := named.view()
	_, _ = first, second
}

func round12ConditionalRecursionControl(values []float64, recurse bool) {
	if recurse {
		_ = round12RecursiveView(values)
	}
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

func round12PairCycle(values []float64) ([]float64, []float64) { return round12PairCycle(values) }
func round12PairView(values []float64) []float64 {
	first, _ := round12PairCycle(values)
	return first
}

func round12PairControl(values []float64) { _ = round12PairView(values) }

// Range assignment targets are writes, even though they have no AssignStmt.
func round12RangeIndexAssignment(values []float64) {
	for step := 0; step < 4; step++ {
		for index := 0; index < len(values); index++ {
			for index = range [2]int{} {
			}
			if values[index] > 0 {
				_ = index
			}
		}
		for index := 0; index < len(values); index++ {
			for index = range [2]int{} {
			}
			if values[index] < 1 {
				_ = index
			}
		}
		values[0]++
	}
}

func round12RangeValueAssignment(values []float64) {
	for step := 0; step < 4; step++ {
		for index := range values {
			for _, index = range [1]int{0} {
			}
			if values[index] > 0 {
				_ = index
			}
		}
		for index := range values {
			for _, index = range [1]int{0} {
			}
			if values[index] < 1 {
				_ = index
			}
		}
		values[0]++
	}
}

func round12IntegerRangeAssignment(values []float64) {
	for step := 0; step < 4; step++ {
		for index := range values {
			for index = range 2 {
			}
			if values[index] > 0 {
				_ = index
			}
		}
		for index := range values {
			for index = range 2 {
			}
			if values[index] < 1 {
				_ = index
			}
		}
		values[0]++
	}
}

func round12RangeAddressControl(values []float64) {
	for step := 0; step < 4; step++ {
		for index := range values {
			pointer := &index
			for _, *pointer = range [1]int{0} {
			}
			if values[index] > 0 {
				_ = index
			}
		}
		for index := range values {
			pointer := &index
			for _, *pointer = range [1]int{0} {
			}
			if values[index] < 1 {
				_ = index
			}
		}
		values[0]++
	}
}

func round12RangeBulkWrite(values, updates []float64) {
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\).*bulk-mutate values`
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
		for _, values[0] = range updates {
		}
		values[1]++
	}
}

func round12RangeAliasRetarget(values, other []float64) {
	alias := values
	for _, alias = range [][]float64{other} {
	}
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\) \(values\[1\]@[0-9]+\)`
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
		alias[0]++
		values[1]++
	}
}

type round12Array [4]float64

func round12ArrayBump(values [4]float64)              { values[0]++ }
func round12GenericArrayBump[T ~[4]float64](values T) { values[0]++ }
func (values round12Array) bump()                     { values[0]++ }

func round12ArrayCopyControls(values [4]float64, named round12Array) {
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
		round12ArrayBump(values)
		round12GenericArrayBump(values)
	}
	for step := 0; step < 4; step++ {
		for index := range named {
			if named[index] > 0 {
				_ = index
			}
		}
		for index := range named {
			if named[index] < 1 {
				_ = index
			}
		}
		named.bump()
	}
}

type round12PromotedLeaf struct{ values []float64 }
type round12PromotedInner struct{ *round12PromotedLeaf }
type round12PromotedOuter struct{ *round12PromotedInner }

func (value *round12PromotedLeaf) bump(index int)           { value.values[index]++ }
func (value *round12PromotedLeaf) above(index int) bool     { return value.values[index] > 0 }
func (value *round12PromotedLeaf) below(index int) bool     { return value.values[index] < 1 }
func round12BumpLeaf(value *round12PromotedLeaf, index int) { value.values[index]++ }

func round12PromotedDirect(value *round12PromotedOuter) {
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
		value.bump(0)
		value.values[1]++
	}
}

func round12PromotedStored(value *round12PromotedOuter) {
	bump := value.bump
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

func round12PromotedExpression(value *round12PromotedOuter) {
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
		(*round12PromotedOuter).bump(value, 0)
		value.values[1]++
	}
}

func round12PromotedDetached(value *round12PromotedOuter, replacement []float64) {
	bump := value.bump
	value.round12PromotedInner = &round12PromotedInner{round12PromotedLeaf: &round12PromotedLeaf{values: replacement}}
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

func round12PromotedAttached(value *round12PromotedOuter, replacement []float64) {
	value.round12PromotedInner = &round12PromotedInner{round12PromotedLeaf: &round12PromotedLeaf{values: replacement}}
	bump := value.bump
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

func round12PromotedPredicates(value *round12PromotedOuter) {
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\)`
		for index := range value.values {
			if value.above(index) {
				_ = index
			}
		}
		for index := range value.values {
			if value.below(index) {
				_ = index
			}
		}
		value.values[0]++
	}
}

func round12PromotedNestedHelper(value *round12PromotedOuter) {
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
		round12BumpLeaf(value.round12PromotedInner.round12PromotedLeaf, 0)
		value.values[1]++
	}
}

type round12NestedLeaf struct{ values []float64 }
type round12NestedBox struct{ item *round12NestedLeaf }

func (value *round12NestedLeaf) bump(index int)             { value.values[index]++ }
func round12NestedBump(value *round12NestedLeaf, index int) { value.values[index]++ }

func round12NestedPointerMethod(value *round12NestedBox) {
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\)`
		for index := range value.item.values {
			if value.item.values[index] > 0 {
				_ = index
			}
		}
		for index := range value.item.values {
			if value.item.values[index] < 1 {
				_ = index
			}
		}
		value.item.bump(0)
		value.item.values[1]++
	}
}

func round12NestedPointerHelper(value *round12NestedBox) {
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\)`
		for index := range value.item.values {
			if value.item.values[index] > 0 {
				_ = index
			}
		}
		for index := range value.item.values {
			if value.item.values[index] < 1 {
				_ = index
			}
		}
		round12NestedBump(value.item, 0)
		value.item.values[1]++
	}
}

func round12Tail(values []float64, low int) []float64 { return values[low:] }
func round12LocalTail(values []float64) []float64 {
	low := 2
	return values[low:]
}
func round12FullTail(values []float64, low int) []float64 {
	return values[low:len(values):cap(values)]
}
func round12Empty(values []float64) []float64 { return values[2:2] }
func round12Bounds(values []float64, low, high int) []float64 {
	return values[low:high]
}

func round12HelperOffset(other []float64) {
	alpha := round12Tail(other, 2)
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\).*alpha\[0\]`
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

func round12HelperLocalOffset(other []float64) {
	alpha := round12LocalTail(other)
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\).*alpha\[0\]`
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

func round12HelperFullOffset(other []float64) {
	alpha := round12FullTail(other, 2)
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\).*alpha\[0\]`
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

func round12ComposedOffset(other []float64) {
	alpha := round12Tail(round12Tail(other, 1), 1)
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\).*alpha\[0\]`
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

func round12MutableOffset(other []float64) {
	low := 2
	alpha := other[low:]
	low = 4
	_ = low
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\).*alpha\[0\]`
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

func round12HelperSwap(other []float64) {
	alpha, retained := round12Tail(other, 2), other
	other, retained = retained, other
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\).*alpha\[0\]`
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
		retained[2]++
		alpha[1]++
	}
}

func round12EmptyViews(other []float64) {
	first := round12Empty(other)
	for step := 0; step < 4; step++ {
		for index := range first {
			if first[index] > 0 {
				_ = index
			}
		}
		for index := range first {
			if first[index] < 1 {
				_ = index
			}
		}
		first[0]++
	}
	second := round12Bounds(other, 2, 2)
	for step := 0; step < 4; step++ {
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
		second[0]++
	}
}

type round12VariadicValues []float64

func (values round12VariadicValues) above(index int, limits ...float64) bool {
	return values[index] > limits[0]
}
func (values round12VariadicValues) below(index int, limits ...float64) bool {
	return values[index] < limits[0]
}
func (values round12VariadicValues) bump(indexes ...int) { values[indexes[0]]++ }
func (values round12VariadicValues) tail(indexes ...int) round12VariadicValues {
	return values[indexes[0]:]
}

func round12VariadicControl(values round12VariadicValues) {
	bump := values.bump
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\)`
		for index := range values {
			if values.above(index, []float64{0}...) {
				_ = index
			}
		}
		for index := range values {
			if round12VariadicValues.below(values, index, []float64{1}...) {
				_ = index
			}
		}
		bump([]int{0}...)
		round12VariadicValues.bump(values, 1)
	}
}

func round12VariadicView(values round12VariadicValues) {
	alias := values.tail(2)
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\).*alias\[0\]`
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
		values[2]++
		alias[1]++
	}
}

func round12OpaqueVariadicControl(values round12VariadicValues, limits []float64) {
	for step := 0; step < 4; step++ {
		for index := range values {
			if values.above(index, limits...) {
				_ = index
			}
		}
		for index := range values {
			if values.below(index, limits...) {
				_ = index
			}
		}
		values[0]++
	}
}
