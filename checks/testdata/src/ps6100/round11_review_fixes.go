package ps6100

type leaf struct{ values []float64 }
type middle struct{ leaf *leaf }
type root struct{ middle *middle }

func MultiplePrefixReplacements(value *root, replacement []float64) {
	stored := &value.middle.leaf.values
	value.middle.leaf = &leaf{values: replacement}
	value.middle = &middle{leaf: &leaf{values: replacement}}
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\)`
		for index := range value.middle.leaf.values {
			if value.middle.leaf.values[index] > 0 {
				_ = index
			}
		}
		for index := range value.middle.leaf.values {
			if value.middle.leaf.values[index] < 1 {
				_ = index
			}
		}
		(*stored)[0]++
		value.middle.leaf.values[1]++
	}
}

func AddressAfterMultipleReplacements(value *root, replacement []float64) {
	value.middle.leaf = &leaf{values: replacement}
	value.middle = &middle{leaf: &leaf{values: replacement}}
	stored := &value.middle.leaf.values
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\)`
		for index := range value.middle.leaf.values {
			if value.middle.leaf.values[index] > 0 {
				_ = index
			}
		}
		for index := range value.middle.leaf.values {
			if value.middle.leaf.values[index] < 1 {
				_ = index
			}
		}
		(*stored)[0]++
		value.middle.leaf.values[1]++
	}
}

type cyclic struct {
	next   *cyclic
	values []float64
}

func PointerCycleReplacement(value *cyclic, replacement []float64) {
	value.next = value
	stored := &value.next.values
	value.next = &cyclic{values: replacement}
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\)`
		for index := range value.next.values {
			if value.next.values[index] > 0 {
				_ = index
			}
		}
		for index := range value.next.values {
			if value.next.values[index] < 1 {
				_ = index
			}
		}
		(*stored)[0]++
		value.next.values[1]++
	}
}

type embeddedLeaf struct{ values []float64 }
type embeddedRoot struct{ *embeddedLeaf }

func PromotedFieldReplacement(value *embeddedRoot, replacement []float64) {
	stored := &value.values
	value.embeddedLeaf = &embeddedLeaf{values: replacement}
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
		(*stored)[0]++
		value.values[1]++
	}
}

func AddressCopyReplacement(value *root, replacement []float64) {
	stored := &value.middle.leaf.values
	copyOfStored := stored
	value.middle = &middle{leaf: &leaf{values: replacement}}
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\)`
		for index := range value.middle.leaf.values {
			if value.middle.leaf.values[index] > 0 {
				_ = index
			}
		}
		for index := range value.middle.leaf.values {
			if value.middle.leaf.values[index] < 1 {
				_ = index
			}
		}
		(*copyOfStored)[0]++
		value.middle.leaf.values[1]++
	}
}

func escape(any) {}

func AddressEscapeBeforeReplacement(value *root, replacement []float64) {
	stored := &value.middle.leaf.values
	escape(stored)
	value.middle = &middle{leaf: &leaf{values: replacement}}
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\)`
		for index := range value.middle.leaf.values {
			if value.middle.leaf.values[index] > 0 {
				_ = index
			}
		}
		for index := range value.middle.leaf.values {
			if value.middle.leaf.values[index] < 1 {
				_ = index
			}
		}
		value.middle.leaf.values[1]++
	}
}

type series[T ~float64] []T

func (values series[T]) above(index int, limits ...T) bool { return values[index] > limits[0] }
func (values series[T]) below(index int, limits ...T) bool { return values[index] < limits[0] }
func (values series[T]) bump(indexes ...int)               { values[indexes[0]]++ }
func (values series[T]) aboveOne(index int, limit T) bool  { return values[index] > limit }
func (values series[T]) belowOne(index int, limit T) bool  { return values[index] < limit }
func (values series[T]) bumpOne(index int)                 { values[index]++ }

func GenericVariadicReceiver(values series[float64]) {
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\)`
		for index := range values {
			if values.above(index, 0) {
				_ = index
			}
		}
		for index := range values {
			if values.below(index, 1) {
				_ = index
			}
		}
		values.bump(0)
		values[1]++
	}
}

func GenericReceiverPredicatesOnly(values series[float64]) {
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\)`
		for index := range values {
			if values.above(index, 0) {
				_ = index
			}
		}
		for index := range values {
			if values.below(index, 1) {
				_ = index
			}
		}
		values[0]++
		values[1]++
	}
}

func GenericReceiverMutationOnly(values series[float64]) {
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\)`
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
		values.bump(0)
		values[1]++
	}
}

func GenericNonvariadicReceiver(values series[float64]) {
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\)`
		for index := range values {
			if values.aboveOne(index, 0) {
				_ = index
			}
		}
		for index := range values {
			if values.belowOne(index, 1) {
				_ = index
			}
		}
		values.bumpOne(0)
		values[1]++
	}
}

func GenericVariadicMethodExpression(values series[float64]) {
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\)`
		for index := range values {
			if series[float64].above(values, index, 0) {
				_ = index
			}
		}
		for index := range values {
			if series[float64].below(values, index, 1) {
				_ = index
			}
		}
		series[float64].bump(values, 0)
		values[1]++
	}
}

func GenericVariadicEllipsis(values series[float64]) {
	indexes := []int{0}
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\)`
		for index := range values {
			if values.above(index, []float64{0}...) {
				_ = index
			}
		}
		for index := range values {
			if values.below(index, []float64{1}...) {
				_ = index
			}
		}
		values.bump(indexes...)
		values[1]++
	}
}

func VariadicUnknownEllipsisControl(values series[float64], limits []float64) {
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

// Both local predicate helpers panic before returning because their variadic
// slices are empty. Reducing those calls to one cached classification would
// change observable behavior, so this remains silent.
func VariadicPredicatePanicControl(values series[float64]) {
	for step := 0; step < 4; step++ {
		for index := range values {
			if values.above(index) {
				_ = index
			}
		}
		for index := range values {
			if values.below(index) {
				_ = index
			}
		}
		values[0]++
	}
}

type plainSeries []float64

func (values plainSeries) aboveVariadic(index int, limits ...float64) bool {
	return values[index] > limits[0]
}
func (values plainSeries) belowVariadic(index int, limits ...float64) bool {
	return values[index] < limits[0]
}
func (values plainSeries) bumpVariadic(indexes ...int) { values[indexes[0]]++ }
func (values plainSeries) aboveOne(index int, limit float64) bool {
	return values[index] > limit
}
func (values plainSeries) belowOne(index int, limit float64) bool {
	return values[index] < limit
}
func (values plainSeries) bumpOne(index int) { values[index]++ }

func VariadicReceiverControl(values plainSeries) {
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\)`
		for index := range values {
			if values.aboveVariadic(index, 0) {
				_ = index
			}
		}
		for index := range values {
			if values.belowVariadic(index, 1) {
				_ = index
			}
		}
		values.bumpVariadic(0)
		values[1]++
	}
}

func NonvariadicReceiverControl(values plainSeries) {
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\)`
		for index := range values {
			if values.aboveOne(index, 0) {
				_ = index
			}
		}
		for index := range values {
			if values.belowOne(index, 1) {
				_ = index
			}
		}
		values.bumpOne(0)
		values[1]++
	}
}

func PlainReceiverMutationOnly(values plainSeries) {
	for step := 0; step < 4; step++ { // want `2 bounded indexed mutation site\(s\)`
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
		values.bumpOne(0)
		values[1]++
	}
}

func returned(values []float64) []float64 { return values }
func returnedView(values []float64) []float64 {
	view := values[2:]
	return view
}
func returnedDirectView(values []float64) []float64 { return values[2:] }

func ResliceReturnedHelper(other []float64) {
	alpha := returned(other)[2:]
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
		other[2]++
		alpha[1]++
	}
}

func ResliceReturnedLocal(other []float64) {
	alpha := returnedView(other)
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
		other[2]++
		alpha[1]++
	}
}

func ResliceReturnedDirectView(other []float64) {
	alpha := returnedDirectView(other)
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
		other[2]++
		alpha[1]++
	}
}

func TupleOverlappingViews(other []float64) {
	alpha, overlap := other[2:], other[3:]
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
		overlap[0]++
		alpha[0]++
	}
}

func TupleNonoverlappingViews(other []float64) {
	alpha, disjoint := other[2:4], other[4:]
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\)`
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
		disjoint[0]++
		alpha[0]++
	}
}

func DynamicThreeIndex(other []float64, low, high, maximum int) {
	alpha := other[low:high:maximum]
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
		other[0]++
		alpha[1]++
	}
}

func ZeroTripReturnedView(other []float64) {
	alpha := returned(other)[2:2]
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

func ChannelHelper[T any](values <-chan T) T { return <-values }

func ChannelThroughGenericHelper(alpha []float64, decisions <-chan bool) {
	for step := 0; step < 4; step++ {
		for index := range alpha {
			if ChannelHelper(decisions) && alpha[index] > 0 {
				_ = index
			}
		}
		for index := range alpha {
			if ChannelHelper(decisions) && alpha[index] < 1 {
				_ = index
			}
		}
		alpha[0]++
	}
}

type opaqueRound11Receiver interface {
	above(int, ...float64) bool
	below(int, ...float64) bool
	bump(...int)
}

func OpaqueReceiverControl(alpha []float64, helper opaqueRound11Receiver) {
	for step := 0; step < 4; step++ {
		for index := range alpha {
			if helper.above(index, 0) && alpha[index] > 0 {
				_ = index
			}
		}
		for index := range alpha {
			if helper.below(index, 1) && alpha[index] < 1 {
				_ = index
			}
		}
		helper.bump(0)
		alpha[1]++
	}
}

func MapAliasPredicate(alpha []float64, flags map[int]bool) {
	alias := flags
	for step := 0; step < 4; step++ { // want `iterative loop repeats`
		for index := range alpha {
			if alias[index] && alpha[index] > 0 {
				_ = index
			}
		}
		for index := range alpha {
			if alias[index] && alpha[index] < 1 {
				_ = index
			}
		}
		flags[0] = !flags[0]
		alpha[1]++
	}
}

func AppendBeforeLoop(other []float64) {
	alpha := other[2:4:4]
	alpha = append(alpha, 1)
	for step := 0; step < 4; step++ { // want `1 bounded indexed mutation site\(s\).*opaque pre-loop rebind may alias tracked input`
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
		alpha[0]++
	}
}
