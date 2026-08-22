package ps6080_bindings

import "errors"

type LoopBindingQuant uint8

const (
	LoopBindingA LoopBindingQuant = iota
	LoopBindingB                  // want `quant variant LoopBindingB \(LoopBindingQuant\).*absent from 1 of 3 reachable CPU matmul dispatch layers`
	LoopBindingC
)

func loopBindingByteSize(q LoopBindingQuant) int {
	switch q {
	case LoopBindingA, LoopBindingB, LoopBindingC:
		return 8
	default:
		return 0
	}
}
func loopBindingDecode(q LoopBindingQuant) []float32 {
	switch q {
	case LoopBindingA, LoopBindingB, LoopBindingC:
		return []float32{}
	default:
		return nil
	}
}
func loopBindingFull(q LoopBindingQuant) bool {
	return q == LoopBindingA || q == LoopBindingB || q == LoopBindingC
}
func LoopBindingQMatMul(q LoopBindingQuant) bool {
	dispatch := func() bool { return q == LoopBindingA || q == LoopBindingB || q == LoopBindingC }
	for index := 0; index < 2; index++ {
		_ = dispatch()
		dispatch = func() bool { return q == LoopBindingA || q == LoopBindingC }
	}
	return loopBindingFull(q)
}

type IfElseBindingQuant uint8

const (
	IfElseBindingA IfElseBindingQuant = iota
	IfElseBindingB                    // want `quant variant IfElseBindingB \(IfElseBindingQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers`
	IfElseBindingC
)

func ifElseBindingByteSize(q IfElseBindingQuant) int {
	switch q {
	case IfElseBindingA, IfElseBindingB, IfElseBindingC:
		return 8
	default:
		return 0
	}
}
func ifElseBindingDecode(q IfElseBindingQuant) []float32 {
	switch q {
	case IfElseBindingA, IfElseBindingB, IfElseBindingC:
		return []float32{}
	default:
		return nil
	}
}
func ifElseBindingFull(q IfElseBindingQuant) bool {
	return q == IfElseBindingA || q == IfElseBindingB || q == IfElseBindingC
}
func IfElseBindingQMatMul(q IfElseBindingQuant, route bool) bool {
	dispatch := func() bool { return q == IfElseBindingA || q == IfElseBindingC }
	if route {
		dispatch = func() bool { return q == IfElseBindingA || q == IfElseBindingB || q == IfElseBindingC }
	} else {
		_ = dispatch()
	}
	return ifElseBindingFull(q)
}

type AliasBindingQuant uint8
type AliasBindingHandler func() bool

const (
	AliasBindingA AliasBindingQuant = iota
	AliasBindingB                   // want `quant variant AliasBindingB \(AliasBindingQuant\).*absent from 2 of 3 reachable CPU matmul dispatch layers`
	AliasBindingC
)

func aliasBindingByteSize(q AliasBindingQuant) int {
	switch q {
	case AliasBindingA, AliasBindingB, AliasBindingC:
		return 8
	default:
		return 0
	}
}
func aliasBindingDecode(q AliasBindingQuant) []float32 {
	switch q {
	case AliasBindingA, AliasBindingB, AliasBindingC:
		return []float32{}
	default:
		return nil
	}
}
func aliasBindingFull(q AliasBindingQuant) bool {
	return q == AliasBindingA || q == AliasBindingB || q == AliasBindingC
}
func AliasBindingQMatMul(q AliasBindingQuant) bool {
	partial := AliasBindingHandler(func() bool { return q == AliasBindingA || q == AliasBindingC })
	alias := partial
	_ = alias()
	return aliasBindingFull(q)
}
func ConversionBindingQMatMul(q AliasBindingQuant) bool {
	_ = AliasBindingHandler(func() bool { return q == AliasBindingA || q == AliasBindingC })()
	return aliasBindingFull(q)
}

type ParameterBindingQuant uint8

const (
	ParameterBindingA ParameterBindingQuant = iota
	ParameterBindingB                       // want `quant variant ParameterBindingB \(ParameterBindingQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers`
	ParameterBindingC
)

func parameterBindingByteSize(q ParameterBindingQuant) int {
	switch q {
	case ParameterBindingA, ParameterBindingB, ParameterBindingC:
		return 8
	default:
		return 0
	}
}
func parameterBindingDecode(q ParameterBindingQuant) []float32 {
	switch q {
	case ParameterBindingA, ParameterBindingB, ParameterBindingC:
		return []float32{}
	default:
		return nil
	}
}
func parameterBindingFull(q ParameterBindingQuant) bool {
	return q == ParameterBindingA || q == ParameterBindingB || q == ParameterBindingC
}
func ParameterBindingQMatMul(q ParameterBindingQuant) bool {
	run := func(callback func() bool) bool { return callback() }
	_ = run(func() bool { return q == ParameterBindingA || q == ParameterBindingC })
	return parameterBindingFull(q)
}

type RepeatBindingQuant uint8

const (
	RepeatBindingA RepeatBindingQuant = iota
	RepeatBindingB                    // want `quant variant RepeatBindingB \(RepeatBindingQuant\).*absent from 2 of 4 reachable CPU matmul dispatch layers`
	RepeatBindingC
)

func repeatBindingByteSize(q RepeatBindingQuant) int {
	switch q {
	case RepeatBindingA, RepeatBindingB, RepeatBindingC:
		return 8
	default:
		return 0
	}
}
func repeatBindingDecode(q RepeatBindingQuant) []float32 {
	switch q {
	case RepeatBindingA, RepeatBindingB, RepeatBindingC:
		return []float32{}
	default:
		return nil
	}
}
func repeatBindingFull(q RepeatBindingQuant) bool {
	return q == RepeatBindingA || q == RepeatBindingB || q == RepeatBindingC
}
func RepeatBindingQMatMul(q RepeatBindingQuant) bool {
	dispatch := func() bool { return q == RepeatBindingA || q == RepeatBindingB || q == RepeatBindingC }
	run := func() bool {
		result := dispatch()
		dispatch = func() bool { return q == RepeatBindingA || q == RepeatBindingC }
		return result
	}
	_ = run()
	_ = run()
	return repeatBindingFull(q)
}
func NestedSetterBindingQMatMul(q RepeatBindingQuant, route bool) bool {
	dispatch := func() bool {
		return q == RepeatBindingA || q == RepeatBindingB || q == RepeatBindingC
	}
	set := func() {
		dispatch = func() bool { return q == RepeatBindingA || q == RepeatBindingC }
	}
	run := func() bool {
		set()
		if route {
			_ = route
		}
		return dispatch()
	}
	_ = run()
	return repeatBindingFull(q)
}

// Captured callable effects flow transitively through higher-order helpers.
type EffectBindingQuant uint8

const (
	EffectBindingA EffectBindingQuant = iota
	EffectBindingB                    // want `quant variant EffectBindingB \(EffectBindingQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers`
	EffectBindingC
)

func effectBindingByteSize(q EffectBindingQuant) int {
	switch q {
	case EffectBindingA, EffectBindingB, EffectBindingC:
		return 8
	default:
		return 0
	}
}
func effectBindingDecode(q EffectBindingQuant) []float32 {
	switch q {
	case EffectBindingA, EffectBindingB, EffectBindingC:
		return []float32{}
	default:
		return nil
	}
}
func effectBindingFull(q EffectBindingQuant) bool {
	return q == EffectBindingA || q == EffectBindingB || q == EffectBindingC
}
func HigherOrderEffectQMatMul(q EffectBindingQuant) bool {
	dispatch := func() bool { return effectBindingFull(q) }
	set := func() {
		dispatch = func() bool { return q == EffectBindingA || q == EffectBindingC }
	}
	apply := func(callback func()) { callback() }
	run := func() bool {
		apply(set)
		return dispatch()
	}
	_ = run()
	return effectBindingFull(q)
}

// A callable effect on a path that cannot return does not reach later calls.
type PanicBindingQuant uint8

const (
	PanicBindingA PanicBindingQuant = iota
	PanicBindingB
	PanicBindingC
)

func panicBindingByteSize(q PanicBindingQuant) int {
	switch q {
	case PanicBindingA, PanicBindingB, PanicBindingC:
		return 8
	default:
		return 0
	}
}
func panicBindingDecode(q PanicBindingQuant) []float32 {
	switch q {
	case PanicBindingA, PanicBindingB, PanicBindingC:
		return []float32{}
	default:
		return nil
	}
}
func panicBindingFull(q PanicBindingQuant) bool {
	return q == PanicBindingA || q == PanicBindingB || q == PanicBindingC
}
func PanicEffectQMatMul(q PanicBindingQuant) bool {
	dispatch := func() bool { return panicBindingFull(q) }
	set := func() {
		dispatch = func() bool { return q == PanicBindingA || q == PanicBindingC }
		panic("stop")
	}
	run := func() bool {
		set()
		return dispatch()
	}
	_ = dispatch()
	_ = run()
	return panicBindingFull(q)
}

// Address-taking invalidates stale callable targets, while a guaranteed
// one-element range assignment can resolve its exact replacement.
type IndirectBindingQuant uint8

const (
	IndirectBindingA IndirectBindingQuant = iota
	IndirectBindingB
	IndirectBindingC
)

func indirectBindingByteSize(q IndirectBindingQuant) int {
	switch q {
	case IndirectBindingA, IndirectBindingB, IndirectBindingC:
		return 8
	default:
		return 0
	}
}
func indirectBindingDecode(q IndirectBindingQuant) []float32 {
	switch q {
	case IndirectBindingA, IndirectBindingB, IndirectBindingC:
		return []float32{}
	default:
		return nil
	}
}
func indirectBindingFull(q IndirectBindingQuant) bool {
	return q == IndirectBindingA || q == IndirectBindingB || q == IndirectBindingC
}
func PointerBindingQMatMul(q IndirectBindingQuant) bool {
	dispatch := func() bool { return q == IndirectBindingA || q == IndirectBindingC }
	pointer := &dispatch
	*pointer = func() bool { return indirectBindingFull(q) }
	_ = dispatch()
	return indirectBindingFull(q)
}
func PointerSharedBindingQMatMul(q IndirectBindingQuant) bool {
	dispatch := func() bool { return q == IndirectBindingA || q == IndirectBindingC }
	var pointer *func() bool
	dispatch, pointer = func() bool { return q == IndirectBindingA || q == IndirectBindingC }, &dispatch
	*pointer = func() bool { return indirectBindingFull(q) }
	_ = dispatch()
	return indirectBindingFull(q)
}
func PointerPersistentBindingQMatMul(q IndirectBindingQuant) bool {
	dispatch := func() bool { return q == IndirectBindingA || q == IndirectBindingC }
	pointer := &dispatch
	dispatch = func() bool { return q == IndirectBindingA || q == IndirectBindingC }
	*pointer = func() bool { return indirectBindingFull(q) }
	_ = dispatch()
	return indirectBindingFull(q)
}
func DuplicateBindingQMatMul(q IndirectBindingQuant) bool {
	dispatch := func() bool { return q == IndirectBindingA || q == IndirectBindingC }
	dispatch, dispatch = func() bool { return q == IndirectBindingA || q == IndirectBindingC }, func() bool {
		return indirectBindingFull(q)
	}
	_ = dispatch()
	return indirectBindingFull(q)
}
func RangeAssignBindingQMatMul(q IndirectBindingQuant) bool {
	dispatch := func() bool { return q == IndirectBindingA || q == IndirectBindingC }
	for _, dispatch = range [1]func() bool{func() bool { return indirectBindingFull(q) }} {
	}
	_ = dispatch()
	return indirectBindingFull(q)
}
func RangeMutationBindingQMatMul(q IndirectBindingQuant) bool {
	dispatch := func() bool { return q == IndirectBindingA || q == IndirectBindingC }
	handlers := [1]func() bool{func() bool { return q == IndirectBindingA || q == IndirectBindingC }}
	handlers[0] = func() bool { return indirectBindingFull(q) }
	for _, dispatch = range handlers {
	}
	_ = dispatch()
	return indirectBindingFull(q)
}
func RangeSnapshotBindingQMatMul(q IndirectBindingQuant) bool {
	handler := func() bool { return indirectBindingFull(q) }
	handlers := [1]func() bool{handler}
	handler = func() bool { return q == IndirectBindingA || q == IndirectBindingC }
	var dispatch func() bool
	for _, dispatch = range handlers {
	}
	_ = dispatch()
	return indirectBindingFull(q)
}
func invokeIndirectSetter(set func()) {
	set()
}
func invokeFirstIndirectSetter(setters []func()) {
	setters[0]()
}
func NamedSetterBindingQMatMul(q IndirectBindingQuant) bool {
	dispatch := func() bool { return q == IndirectBindingA || q == IndirectBindingC }
	setter := func() {
		dispatch = func() bool { return indirectBindingFull(q) }
	}
	invokeIndirectSetter(setter)
	_ = dispatch()
	return indirectBindingFull(q)
}
func NamedSetterAliasBindingQMatMul(q IndirectBindingQuant) bool {
	dispatch := func() bool { return q == IndirectBindingA || q == IndirectBindingC }
	setter := func() {
		dispatch = func() bool { return indirectBindingFull(q) }
	}
	invoke := invokeIndirectSetter
	invoke(setter)
	_ = dispatch()
	return indirectBindingFull(q)
}
func TransitiveNamedSetterBindingQMatMul(q IndirectBindingQuant) bool {
	dispatch := func() bool { return q == IndirectBindingA || q == IndirectBindingC }
	mutate := func() {
		dispatch = func() bool { return indirectBindingFull(q) }
	}
	setter := func() { mutate() }
	invokeIndirectSetter(setter)
	_ = dispatch()
	return indirectBindingFull(q)
}
func AggregateNamedSetterBindingQMatMul(q IndirectBindingQuant) bool {
	dispatch := func() bool { return q == IndirectBindingA || q == IndirectBindingC }
	setter := func() {
		dispatch = func() bool { return indirectBindingFull(q) }
	}
	invokeFirstIndirectSetter([]func(){setter})
	_ = dispatch()
	return indirectBindingFull(q)
}
func FactoryNamedSetterBindingQMatMul(q IndirectBindingQuant) bool {
	dispatch := func() bool { return q == IndirectBindingA || q == IndirectBindingC }
	invokeIndirectSetter(func() func() {
		return func() {
			dispatch = func() bool { return indirectBindingFull(q) }
		}
	}())
	_ = dispatch()
	return indirectBindingFull(q)
}
func StoredSetterBindingQMatMul(q IndirectBindingQuant) bool {
	dispatch := func() bool { return q == IndirectBindingA || q == IndirectBindingC }
	setter := func() {
		dispatch = func() bool { return indirectBindingFull(q) }
	}
	holder := struct{ setter func() }{setter: setter}
	invokeIndirectSetter(holder.setter)
	_ = dispatch()
	return indirectBindingFull(q)
}
func StoredSetterInvokeBindingQMatMul(q IndirectBindingQuant) bool {
	dispatch := func() bool { return q == IndirectBindingA || q == IndirectBindingC }
	setter := func() {
		dispatch = func() bool { return indirectBindingFull(q) }
	}
	holder := struct{ setter func() }{setter: setter}
	holder.setter()
	_ = dispatch()
	return indirectBindingFull(q)
}

// Mutually exclusive call sites execute the wrapper only once, so its
// post-call captured rebind cannot affect another invocation.
type ExclusiveBindingQuant uint8

const (
	ExclusiveBindingA ExclusiveBindingQuant = iota
	ExclusiveBindingB
	ExclusiveBindingC
)

func exclusiveBindingByteSize(q ExclusiveBindingQuant) int {
	switch q {
	case ExclusiveBindingA, ExclusiveBindingB, ExclusiveBindingC:
		return 8
	default:
		return 0
	}
}
func exclusiveBindingDecode(q ExclusiveBindingQuant) []float32 {
	switch q {
	case ExclusiveBindingA, ExclusiveBindingB, ExclusiveBindingC:
		return []float32{}
	default:
		return nil
	}
}
func exclusiveBindingFull(q ExclusiveBindingQuant) bool {
	return q == ExclusiveBindingA || q == ExclusiveBindingB || q == ExclusiveBindingC
}
func ExclusiveBindingQMatMul(q ExclusiveBindingQuant, route bool) bool {
	dispatch := func() bool {
		return q == ExclusiveBindingA || q == ExclusiveBindingB || q == ExclusiveBindingC
	}
	run := func() bool {
		result := dispatch()
		dispatch = func() bool { return q == ExclusiveBindingA || q == ExclusiveBindingC }
		return result
	}
	if route {
		_ = run()
	} else {
		_ = run()
	}
	return exclusiveBindingFull(q)
}
func FreshBindingQMatMul(q ExclusiveBindingQuant) bool {
	run := func() bool {
		dispatch := func() bool {
			return q == ExclusiveBindingA || q == ExclusiveBindingB || q == ExclusiveBindingC
		}
		result := dispatch()
		dispatch = func() bool { return q == ExclusiveBindingA || q == ExclusiveBindingC }
		return result
	}
	for index := 0; index < 2; index++ {
		_ = run()
	}
	return exclusiveBindingFull(q)
}

type ConstantBindingQuant uint8

const (
	ConstantBindingA ConstantBindingQuant = iota
	ConstantBindingB                      // want `quant variant ConstantBindingB \(ConstantBindingQuant\).*absent from 2 of 3 reachable CPU matmul dispatch layers`
	ConstantBindingC
)

func constantBindingByteSize(q ConstantBindingQuant) int {
	switch q {
	case ConstantBindingA, ConstantBindingB, ConstantBindingC:
		return 8
	default:
		return 0
	}
}
func constantBindingDecode(q ConstantBindingQuant) []float32 {
	switch q {
	case ConstantBindingA, ConstantBindingB, ConstantBindingC:
		return []float32{}
	default:
		return nil
	}
}
func constantBindingFull(q ConstantBindingQuant) bool {
	return q == ConstantBindingA || q == ConstantBindingB || q == ConstantBindingC
}
func DeadBindingQMatMul(q ConstantBindingQuant) bool {
	dispatch := func() bool { return q == ConstantBindingA || q == ConstantBindingC }
	if false {
		dispatch = func() bool { return q == ConstantBindingA || q == ConstantBindingB || q == ConstantBindingC }
	}
	_ = dispatch()
	return constantBindingFull(q)
}
func GuaranteedBindingQMatMul(q ConstantBindingQuant) bool {
	dispatch := func() bool { return q == ConstantBindingA || q == ConstantBindingB || q == ConstantBindingC }
	if true {
		dispatch = func() bool { return q == ConstantBindingA || q == ConstantBindingC }
	}
	_ = dispatch()
	return constantBindingFull(q)
}

// A compile-time-dead tagged switch arm cannot change the invoked callable.
type SwitchBindingQuant uint8

const (
	SwitchBindingA SwitchBindingQuant = iota
	SwitchBindingB
	SwitchBindingC
)

func switchBindingByteSize(q SwitchBindingQuant) int {
	switch q {
	case SwitchBindingA, SwitchBindingB, SwitchBindingC:
		return 8
	default:
		return 0
	}
}
func switchBindingDecode(q SwitchBindingQuant) []float32 {
	switch q {
	case SwitchBindingA, SwitchBindingB, SwitchBindingC:
		return []float32{}
	default:
		return nil
	}
}
func switchBindingFull(q SwitchBindingQuant) bool {
	return q == SwitchBindingA || q == SwitchBindingB || q == SwitchBindingC
}
func SwitchBindingQMatMul(q SwitchBindingQuant) bool {
	dispatch := func() bool { return switchBindingFull(q) }
	switch 1 {
	case 2:
		dispatch = func() bool { return q == SwitchBindingA || q == SwitchBindingC }
	}
	_ = dispatch()
	return switchBindingFull(q)
}

// Statically empty ranges cannot change the invoked callable.
type RangeBindingQuant uint8

const (
	RangeBindingA RangeBindingQuant = iota
	RangeBindingB
	RangeBindingC
)

func rangeBindingByteSize(q RangeBindingQuant) int {
	switch q {
	case RangeBindingA, RangeBindingB, RangeBindingC:
		return 8
	default:
		return 0
	}
}
func rangeBindingDecode(q RangeBindingQuant) []float32 {
	switch q {
	case RangeBindingA, RangeBindingB, RangeBindingC:
		return []float32{}
	default:
		return nil
	}
}
func rangeBindingFull(q RangeBindingQuant) bool {
	return q == RangeBindingA || q == RangeBindingB || q == RangeBindingC
}
func RangeBindingQMatMul(q RangeBindingQuant) bool {
	dispatch := func() bool { return rangeBindingFull(q) }
	partial := func() bool { return q == RangeBindingA || q == RangeBindingC }
	for range [0]int{} {
		dispatch = partial
	}
	for range "" {
		dispatch = partial
	}
	for range 0 {
		dispatch = partial
	}
	for range []int{} {
		dispatch = partial
	}
	for range map[int]int{} {
		dispatch = partial
	}
	for range []int(nil) {
		dispatch = partial
	}
	for range map[int]int(nil) {
		dispatch = partial
	}
	for range make([]int, 0, 8) {
		dispatch = partial
	}
	for range make(map[int]int, 8) {
		dispatch = partial
	}
	_ = dispatch()
	return rangeBindingFull(q)
}

type FanoutBindingQuant uint8

const (
	FanoutBindingA FanoutBindingQuant = iota
	FanoutBindingB                    // want `quant variant FanoutBindingB \(FanoutBindingQuant\).*absent from 1 of 2 reachable CPU matmul dispatch layers`
	FanoutBindingC
)

func fanoutBindingByteSize(q FanoutBindingQuant) int {
	switch q {
	case FanoutBindingA, FanoutBindingB, FanoutBindingC:
		return 8
	default:
		return 0
	}
}
func fanoutBindingDecode(q FanoutBindingQuant) []float32 {
	switch q {
	case FanoutBindingA, FanoutBindingB, FanoutBindingC:
		return []float32{}
	default:
		return nil
	}
}
func fanoutBindingFull(q FanoutBindingQuant) bool {
	return q == FanoutBindingA || q == FanoutBindingB || q == FanoutBindingC
}
func FanoutBindingQMatMul(q FanoutBindingQuant) bool {
	level0 := func() bool { return q == FanoutBindingA || q == FanoutBindingC }
	level1 := func() bool { return level0() || level0() }
	level2 := func() bool { return level1() || level1() }
	level3 := func() bool { return level2() || level2() }
	level4 := func() bool { return level3() || level3() }
	level5 := func() bool { return level4() || level4() }
	level6 := func() bool { return level5() || level5() }
	level7 := func() bool { return level6() || level6() }
	level8 := func() bool { return level7() || level7() }
	level9 := func() bool { return level8() || level8() }
	level10 := func() bool { return level9() || level9() }
	level11 := func() bool { return level10() || level10() }
	_ = level11()
	return fanoutBindingFull(q)
}

type NamedReachQuant uint8

const (
	NamedReachA NamedReachQuant = iota // want `quant variant NamedReachA \(NamedReachQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	NamedReachB                        // want `quant variant NamedReachB \(NamedReachQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	NamedReachC                        // want `quant variant NamedReachC \(NamedReachQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
)

func namedReachByteSize(q NamedReachQuant) int {
	switch q {
	case NamedReachA, NamedReachB, NamedReachC:
		return 8
	default:
		return 0
	}
}
func namedReachDecode(q NamedReachQuant) []float32 {
	switch q {
	case NamedReachA, NamedReachB, NamedReachC:
		return []float32{}
	default:
		return nil
	}
}
func NamedReachQMatMulOne(q NamedReachQuant) (ok bool, err error) {
	err = errors.New("unsupported")
	switch q {
	case NamedReachA, NamedReachC:
		ok = true
		return
	default:
		return
	}
}
func NamedReachQMatMulTwo(q NamedReachQuant) (ok bool, err error) {
	err = errors.New("unsupported")
	switch q {
	case NamedReachA, NamedReachC:
		ok = true
		return
	default:
		return
	}
}

type SimpleRecursiveBindingQuant uint8

const (
	SimpleRecursiveBindingA SimpleRecursiveBindingQuant = iota
	SimpleRecursiveBindingB                             // want `quant variant SimpleRecursiveBindingB \(SimpleRecursiveBindingQuant\).*absent from 2 of 3 reachable CPU matmul dispatch layers`
	SimpleRecursiveBindingC
)

func simpleRecursiveBindingByteSize(q SimpleRecursiveBindingQuant) int {
	switch q {
	case SimpleRecursiveBindingA, SimpleRecursiveBindingB, SimpleRecursiveBindingC:
		return 8
	default:
		return 0
	}
}
func simpleRecursiveBindingDecode(q SimpleRecursiveBindingQuant) []float32 {
	switch q {
	case SimpleRecursiveBindingA, SimpleRecursiveBindingB, SimpleRecursiveBindingC:
		return []float32{}
	default:
		return nil
	}
}
func simpleRecursiveBindingFull(q SimpleRecursiveBindingQuant) bool {
	return q == SimpleRecursiveBindingA || q == SimpleRecursiveBindingB || q == SimpleRecursiveBindingC
}
func simpleRecursiveBindingPartialOne(q SimpleRecursiveBindingQuant) bool {
	return q == SimpleRecursiveBindingA || q == SimpleRecursiveBindingC
}
func simpleRecursiveBindingPartialTwo(q SimpleRecursiveBindingQuant) bool {
	return q == SimpleRecursiveBindingA || q == SimpleRecursiveBindingC
}
func simpleRecursiveBindingNamedCallback(func()) {}
func SimpleRecursiveBindingQMatMul(q SimpleRecursiveBindingQuant) bool {
	var visit func(int) bool
	visit = func(depth int) bool {
		simpleRecursiveBindingNamedCallback(func() {})
		if depth == 0 {
			return simpleRecursiveBindingFull(q)
		}
		return visit(depth - 1)
	}
	visit(1)
	return simpleRecursiveBindingPartialOne(q) && simpleRecursiveBindingPartialTwo(q)
}

type DeadGotoBindingQuant uint8

const (
	DeadGotoBindingA DeadGotoBindingQuant = iota
	DeadGotoBindingB                      // want `quant variant DeadGotoBindingB \(DeadGotoBindingQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	DeadGotoBindingC
)

func deadGotoBindingByteSize(q DeadGotoBindingQuant) int {
	switch q {
	case DeadGotoBindingA, DeadGotoBindingB, DeadGotoBindingC:
		return 8
	default:
		return 0
	}
}
func deadGotoBindingDecode(q DeadGotoBindingQuant) []float32 {
	switch q {
	case DeadGotoBindingA, DeadGotoBindingB, DeadGotoBindingC:
		return []float32{}
	default:
		return nil
	}
}
func deadGotoBindingFull(q DeadGotoBindingQuant) bool {
	return q == DeadGotoBindingA || q == DeadGotoBindingB || q == DeadGotoBindingC
}
func deadGotoBindingPartialOne(q DeadGotoBindingQuant) bool {
	return q == DeadGotoBindingA || q == DeadGotoBindingC
}
func deadGotoBindingPartialTwo(q DeadGotoBindingQuant) bool {
	return q == DeadGotoBindingA || q == DeadGotoBindingC
}
func DeadGotoBindingQMatMul(q DeadGotoBindingQuant) bool {
	var visit func()
	visit = func() { _ = deadGotoBindingFull(q) }
	goto done
	visit()
done:
	return deadGotoBindingPartialOne(q) && deadGotoBindingPartialTwo(q)
}

type DeadDefinitionBindingQuant uint8

const (
	DeadDefinitionBindingA DeadDefinitionBindingQuant = iota
	DeadDefinitionBindingB                            // want `quant variant DeadDefinitionBindingB \(DeadDefinitionBindingQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	DeadDefinitionBindingC
)

func deadDefinitionBindingByteSize(q DeadDefinitionBindingQuant) int {
	switch q {
	case DeadDefinitionBindingA, DeadDefinitionBindingB, DeadDefinitionBindingC:
		return 8
	default:
		return 0
	}
}
func deadDefinitionBindingDecode(q DeadDefinitionBindingQuant) []float32 {
	switch q {
	case DeadDefinitionBindingA, DeadDefinitionBindingB, DeadDefinitionBindingC:
		return []float32{}
	default:
		return nil
	}
}
func deadDefinitionBindingFull(q DeadDefinitionBindingQuant) bool {
	return q == DeadDefinitionBindingA || q == DeadDefinitionBindingB || q == DeadDefinitionBindingC
}
func deadDefinitionBindingPartialOne(q DeadDefinitionBindingQuant) bool {
	return q == DeadDefinitionBindingA || q == DeadDefinitionBindingC
}
func deadDefinitionBindingPartialTwo(q DeadDefinitionBindingQuant) bool {
	return q == DeadDefinitionBindingA || q == DeadDefinitionBindingC
}
func DeadDefinitionBindingQMatMul(q DeadDefinitionBindingQuant) bool {
	var visit func()
	goto call
	visit = func() { _ = deadDefinitionBindingFull(q) }
call:
	;
	visit()
	return deadDefinitionBindingPartialOne(q) && deadDefinitionBindingPartialTwo(q)
}

type DormantLocalMapQuant uint8

const (
	DormantLocalMapA DormantLocalMapQuant = iota
	DormantLocalMapB                      // want `quant variant DormantLocalMapB \(DormantLocalMapQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	DormantLocalMapC
)

func dormantLocalMapByteSize(q DormantLocalMapQuant) int {
	switch q {
	case DormantLocalMapA, DormantLocalMapB, DormantLocalMapC:
		return 8
	default:
		return 0
	}
}
func dormantLocalMapDecode(q DormantLocalMapQuant) []float32 {
	switch q {
	case DormantLocalMapA, DormantLocalMapB, DormantLocalMapC:
		return []float32{}
	default:
		return nil
	}
}
func dormantLocalMapKernel() bool { return true }
func dormantLocalMapPartialOne(q DormantLocalMapQuant) bool {
	return q == DormantLocalMapA || q == DormantLocalMapC
}
func dormantLocalMapPartialTwo(q DormantLocalMapQuant) bool {
	return q == DormantLocalMapA || q == DormantLocalMapC
}
func DormantLocalMapQMatMul(q DormantLocalMapQuant) bool {
	table := map[DormantLocalMapQuant]func() bool{
		DormantLocalMapA: dormantLocalMapKernel,
		DormantLocalMapB: dormantLocalMapKernel,
		DormantLocalMapC: dormantLocalMapKernel,
	}
	var alias map[DormantLocalMapQuant]func() bool
	_ = func() { alias = table }
	_ = func() { _ = table[q]() }
	_ = alias[q]
	return dormantLocalMapPartialOne(q) && dormantLocalMapPartialTwo(q)
}

type TerminalSwitchRejectQuant uint8

const (
	TerminalSwitchRejectA TerminalSwitchRejectQuant = iota
	TerminalSwitchRejectB                           // want `quant variant TerminalSwitchRejectB \(TerminalSwitchRejectQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	TerminalSwitchRejectC
)

func terminalSwitchRejectByteSize(q TerminalSwitchRejectQuant) int {
	switch q {
	case TerminalSwitchRejectA, TerminalSwitchRejectB, TerminalSwitchRejectC:
		return 8
	default:
		return 0
	}
}
func terminalSwitchRejectDecode(q TerminalSwitchRejectQuant) []float32 {
	switch q {
	case TerminalSwitchRejectA, TerminalSwitchRejectB, TerminalSwitchRejectC:
		return []float32{}
	default:
		return nil
	}
}
func terminalSwitchRejectOne(q TerminalSwitchRejectQuant) bool {
	if q == TerminalSwitchRejectB {
		route := q == TerminalSwitchRejectA
		switch route {
		case false:
			fallthrough
		default:
			return false
		}
	}
	return true
}
func terminalSwitchRejectTwo(q TerminalSwitchRejectQuant) bool {
	if q == TerminalSwitchRejectB {
		route := q == TerminalSwitchRejectC
		switch route {
		case false:
			fallthrough
		default:
			return false
		}
	}
	return true
}
func TerminalSwitchRejectQMatMul(q TerminalSwitchRejectQuant) bool {
	return terminalSwitchRejectOne(q) && terminalSwitchRejectTwo(q)
}

type CapturedMapAliasQuant uint8

const (
	CapturedMapAliasA CapturedMapAliasQuant = iota
	CapturedMapAliasB                       // want `quant variant CapturedMapAliasB \(CapturedMapAliasQuant\).*absent from 2 of 3 reachable CPU matmul dispatch layers`
	CapturedMapAliasC
)

func capturedMapAliasByteSize(q CapturedMapAliasQuant) int {
	switch q {
	case CapturedMapAliasA, CapturedMapAliasB, CapturedMapAliasC:
		return 8
	default:
		return 0
	}
}
func capturedMapAliasDecode(q CapturedMapAliasQuant) []float32 {
	switch q {
	case CapturedMapAliasA, CapturedMapAliasB, CapturedMapAliasC:
		return []float32{}
	default:
		return nil
	}
}
func capturedMapAliasKernel() bool { return true }
func capturedMapAliasPartialOne(q CapturedMapAliasQuant) bool {
	return q == CapturedMapAliasA || q == CapturedMapAliasC
}
func capturedMapAliasPartialTwo(q CapturedMapAliasQuant) bool {
	return q == CapturedMapAliasA || q == CapturedMapAliasC
}
func CapturedMapAliasQMatMul(q CapturedMapAliasQuant) bool {
	table := map[CapturedMapAliasQuant]func() bool{
		CapturedMapAliasA: capturedMapAliasKernel,
		CapturedMapAliasB: capturedMapAliasKernel,
		CapturedMapAliasC: capturedMapAliasKernel,
	}
	var alias map[CapturedMapAliasQuant]func() bool
	run := func() { _ = alias[q]() }
	alias = table
	run()
	return capturedMapAliasPartialOne(q) && capturedMapAliasPartialTwo(q)
}

type CapturedMapMutationQuant uint8

const (
	CapturedMapMutationA CapturedMapMutationQuant = iota
	CapturedMapMutationB                          // want `quant variant CapturedMapMutationB \(CapturedMapMutationQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	CapturedMapMutationC
)

func capturedMapMutationByteSize(q CapturedMapMutationQuant) int {
	switch q {
	case CapturedMapMutationA, CapturedMapMutationB, CapturedMapMutationC:
		return 8
	default:
		return 0
	}
}
func capturedMapMutationDecode(q CapturedMapMutationQuant) []float32 {
	switch q {
	case CapturedMapMutationA, CapturedMapMutationB, CapturedMapMutationC:
		return []float32{}
	default:
		return nil
	}
}
func capturedMapMutationKernel() bool { return true }
func capturedMapMutationPartialOne(q CapturedMapMutationQuant) bool {
	return q == CapturedMapMutationA || q == CapturedMapMutationC
}
func capturedMapMutationPartialTwo(q CapturedMapMutationQuant) bool {
	return q == CapturedMapMutationA || q == CapturedMapMutationC
}
func CapturedMapMutationQMatMul(q CapturedMapMutationQuant) bool {
	table := map[CapturedMapMutationQuant]func() bool{
		CapturedMapMutationA: capturedMapMutationKernel,
		CapturedMapMutationB: capturedMapMutationKernel,
		CapturedMapMutationC: capturedMapMutationKernel,
	}
	var alias map[CapturedMapMutationQuant]func() bool
	mutate := func() { delete(alias, CapturedMapMutationB) }
	alias = table
	mutate()
	_ = table[q]
	return capturedMapMutationPartialOne(q) && capturedMapMutationPartialTwo(q)
}

type LateLocalMapQuant uint8

const (
	LateLocalMapA LateLocalMapQuant = iota
	LateLocalMapB                   // want `quant variant LateLocalMapB \(LateLocalMapQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	LateLocalMapC
)

func lateLocalMapByteSize(q LateLocalMapQuant) int {
	switch q {
	case LateLocalMapA, LateLocalMapB, LateLocalMapC:
		return 8
	default:
		return 0
	}
}
func lateLocalMapDecode(q LateLocalMapQuant) []float32 {
	switch q {
	case LateLocalMapA, LateLocalMapB, LateLocalMapC:
		return []float32{}
	default:
		return nil
	}
}
func lateLocalMapKernel() bool { return true }
func lateLocalMapPartialOne(q LateLocalMapQuant) bool {
	return q == LateLocalMapA || q == LateLocalMapC
}
func lateLocalMapPartialTwo(q LateLocalMapQuant) bool {
	return q == LateLocalMapA || q == LateLocalMapC
}
func LateLocalMapQMatMul(q LateLocalMapQuant) bool {
	var table map[LateLocalMapQuant]func() bool
	run := func() { _ = table[q] }
	run()
	table = map[LateLocalMapQuant]func() bool{
		LateLocalMapA: lateLocalMapKernel,
		LateLocalMapB: lateLocalMapKernel,
		LateLocalMapC: lateLocalMapKernel,
	}
	return lateLocalMapPartialOne(q) && lateLocalMapPartialTwo(q)
}

type LocalCapturedMapAliasQuant uint8

const (
	LocalCapturedMapAliasA LocalCapturedMapAliasQuant = iota
	LocalCapturedMapAliasB                            // want `quant variant LocalCapturedMapAliasB \(LocalCapturedMapAliasQuant\).*absent from 2 of 3 reachable CPU matmul dispatch layers`
	LocalCapturedMapAliasC
)

func localCapturedMapAliasByteSize(q LocalCapturedMapAliasQuant) int {
	switch q {
	case LocalCapturedMapAliasA, LocalCapturedMapAliasB, LocalCapturedMapAliasC:
		return 8
	default:
		return 0
	}
}
func localCapturedMapAliasDecode(q LocalCapturedMapAliasQuant) []float32 {
	switch q {
	case LocalCapturedMapAliasA, LocalCapturedMapAliasB, LocalCapturedMapAliasC:
		return []float32{}
	default:
		return nil
	}
}
func localCapturedMapAliasKernel() bool { return true }
func localCapturedMapAliasPartialOne(q LocalCapturedMapAliasQuant) bool {
	return q == LocalCapturedMapAliasA || q == LocalCapturedMapAliasC
}
func localCapturedMapAliasPartialTwo(q LocalCapturedMapAliasQuant) bool {
	return q == LocalCapturedMapAliasA || q == LocalCapturedMapAliasC
}
func LocalCapturedMapAliasQMatMul(q LocalCapturedMapAliasQuant) bool {
	var table map[LocalCapturedMapAliasQuant]func() bool
	run := func() {
		alias := table
		_ = alias[q]()
	}
	table = map[LocalCapturedMapAliasQuant]func() bool{
		LocalCapturedMapAliasA: localCapturedMapAliasKernel,
		LocalCapturedMapAliasB: localCapturedMapAliasKernel,
		LocalCapturedMapAliasC: localCapturedMapAliasKernel,
	}
	run()
	return localCapturedMapAliasPartialOne(q) && localCapturedMapAliasPartialTwo(q)
}

type ReverseClosureMapAliasQuant uint8

const (
	ReverseClosureMapAliasA ReverseClosureMapAliasQuant = iota
	ReverseClosureMapAliasB                             // want `quant variant ReverseClosureMapAliasB \(ReverseClosureMapAliasQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	ReverseClosureMapAliasC
)

func reverseClosureMapAliasByteSize(q ReverseClosureMapAliasQuant) int {
	switch q {
	case ReverseClosureMapAliasA, ReverseClosureMapAliasB, ReverseClosureMapAliasC:
		return 8
	default:
		return 0
	}
}
func reverseClosureMapAliasDecode(q ReverseClosureMapAliasQuant) []float32 {
	switch q {
	case ReverseClosureMapAliasA, ReverseClosureMapAliasB, ReverseClosureMapAliasC:
		return []float32{}
	default:
		return nil
	}
}
func reverseClosureMapAliasKernel() bool { return true }
func reverseClosureMapAliasPartialOne(q ReverseClosureMapAliasQuant) bool {
	return q == ReverseClosureMapAliasA || q == ReverseClosureMapAliasC
}
func reverseClosureMapAliasPartialTwo(q ReverseClosureMapAliasQuant) bool {
	return q == ReverseClosureMapAliasA || q == ReverseClosureMapAliasC
}
func ReverseClosureMapAliasQMatMul(q ReverseClosureMapAliasQuant) bool {
	table := map[ReverseClosureMapAliasQuant]func() bool{
		ReverseClosureMapAliasA: reverseClosureMapAliasKernel,
		ReverseClosureMapAliasB: reverseClosureMapAliasKernel,
		ReverseClosureMapAliasC: reverseClosureMapAliasKernel,
	}
	var alias map[ReverseClosureMapAliasQuant]func() bool
	set := func() { alias = table }
	use := func() { _ = alias[q]() }
	use()
	set()
	return reverseClosureMapAliasPartialOne(q) && reverseClosureMapAliasPartialTwo(q)
}

type OrderedClosureMapAliasQuant uint8

const (
	OrderedClosureMapAliasA OrderedClosureMapAliasQuant = iota
	OrderedClosureMapAliasB                             // want `quant variant OrderedClosureMapAliasB \(OrderedClosureMapAliasQuant\).*absent from 2 of 3 reachable CPU matmul dispatch layers`
	OrderedClosureMapAliasC
)

func orderedClosureMapAliasByteSize(q OrderedClosureMapAliasQuant) int {
	switch q {
	case OrderedClosureMapAliasA, OrderedClosureMapAliasB, OrderedClosureMapAliasC:
		return 8
	default:
		return 0
	}
}
func orderedClosureMapAliasDecode(q OrderedClosureMapAliasQuant) []float32 {
	switch q {
	case OrderedClosureMapAliasA, OrderedClosureMapAliasB, OrderedClosureMapAliasC:
		return []float32{}
	default:
		return nil
	}
}
func orderedClosureMapAliasKernel() bool { return true }
func orderedClosureMapAliasPartialOne(q OrderedClosureMapAliasQuant) bool {
	return q == OrderedClosureMapAliasA || q == OrderedClosureMapAliasC
}
func orderedClosureMapAliasPartialTwo(q OrderedClosureMapAliasQuant) bool {
	return q == OrderedClosureMapAliasA || q == OrderedClosureMapAliasC
}
func OrderedClosureMapAliasQMatMul(q OrderedClosureMapAliasQuant) bool {
	table := map[OrderedClosureMapAliasQuant]func() bool{
		OrderedClosureMapAliasA: orderedClosureMapAliasKernel,
		OrderedClosureMapAliasB: orderedClosureMapAliasKernel,
		OrderedClosureMapAliasC: orderedClosureMapAliasKernel,
	}
	var alias map[OrderedClosureMapAliasQuant]func() bool
	use := func() { _ = alias[q]() }
	set := func() { alias = table }
	set()
	use()
	return orderedClosureMapAliasPartialOne(q) && orderedClosureMapAliasPartialTwo(q)
}

type ConditionalClosureMapAliasQuant uint8

const (
	ConditionalClosureMapAliasA ConditionalClosureMapAliasQuant = iota
	ConditionalClosureMapAliasB                                 // want `quant variant ConditionalClosureMapAliasB \(ConditionalClosureMapAliasQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	ConditionalClosureMapAliasC
)

func conditionalClosureMapAliasByteSize(q ConditionalClosureMapAliasQuant) int {
	switch q {
	case ConditionalClosureMapAliasA, ConditionalClosureMapAliasB, ConditionalClosureMapAliasC:
		return 8
	default:
		return 0
	}
}
func conditionalClosureMapAliasDecode(q ConditionalClosureMapAliasQuant) []float32 {
	switch q {
	case ConditionalClosureMapAliasA, ConditionalClosureMapAliasB, ConditionalClosureMapAliasC:
		return []float32{}
	default:
		return nil
	}
}
func conditionalClosureMapAliasKernel() bool { return true }
func conditionalClosureMapAliasPartialOne(q ConditionalClosureMapAliasQuant) bool {
	return q == ConditionalClosureMapAliasA || q == ConditionalClosureMapAliasC
}
func conditionalClosureMapAliasPartialTwo(q ConditionalClosureMapAliasQuant) bool {
	return q == ConditionalClosureMapAliasA || q == ConditionalClosureMapAliasC
}
func ConditionalClosureMapAliasQMatMul(q ConditionalClosureMapAliasQuant) bool {
	table := map[ConditionalClosureMapAliasQuant]func() bool{
		ConditionalClosureMapAliasA: conditionalClosureMapAliasKernel,
		ConditionalClosureMapAliasB: conditionalClosureMapAliasKernel,
		ConditionalClosureMapAliasC: conditionalClosureMapAliasKernel,
	}
	var alias map[ConditionalClosureMapAliasQuant]func() bool
	const choose = false
	set := func() {
		if choose {
			alias = table
		}
	}
	use := func() { _ = alias[q]() }
	set()
	use()
	return conditionalClosureMapAliasPartialOne(q) && conditionalClosureMapAliasPartialTwo(q)
}

type ReassignedClosureMapAliasQuant uint8

const (
	ReassignedClosureMapAliasA ReassignedClosureMapAliasQuant = iota
	ReassignedClosureMapAliasB                                // want `quant variant ReassignedClosureMapAliasB \(ReassignedClosureMapAliasQuant\).*absent from 2 of 3 reachable CPU matmul dispatch layers`
	ReassignedClosureMapAliasC
)

func reassignedClosureMapAliasByteSize(q ReassignedClosureMapAliasQuant) int {
	switch q {
	case ReassignedClosureMapAliasA, ReassignedClosureMapAliasB, ReassignedClosureMapAliasC:
		return 8
	default:
		return 0
	}
}
func reassignedClosureMapAliasDecode(q ReassignedClosureMapAliasQuant) []float32 {
	switch q {
	case ReassignedClosureMapAliasA, ReassignedClosureMapAliasB, ReassignedClosureMapAliasC:
		return []float32{}
	default:
		return nil
	}
}
func reassignedClosureMapAliasKernel() bool { return true }
func reassignedClosureMapAliasPartialOne(q ReassignedClosureMapAliasQuant) bool {
	return q == ReassignedClosureMapAliasA || q == ReassignedClosureMapAliasC
}
func reassignedClosureMapAliasPartialTwo(q ReassignedClosureMapAliasQuant) bool {
	return q == ReassignedClosureMapAliasA || q == ReassignedClosureMapAliasC
}
func ReassignedClosureMapAliasQMatMul(q ReassignedClosureMapAliasQuant) bool {
	table := map[ReassignedClosureMapAliasQuant]func() bool{
		ReassignedClosureMapAliasA: reassignedClosureMapAliasKernel,
		ReassignedClosureMapAliasB: reassignedClosureMapAliasKernel,
		ReassignedClosureMapAliasC: reassignedClosureMapAliasKernel,
	}
	var alias map[ReassignedClosureMapAliasQuant]func() bool
	set := func() {
		alias = nil
		alias = table
	}
	use := func() { _ = alias[q]() }
	set()
	use()
	return reassignedClosureMapAliasPartialOne(q) && reassignedClosureMapAliasPartialTwo(q)
}

type TransitiveClosureMapAliasQuant uint8

const (
	TransitiveClosureMapAliasA TransitiveClosureMapAliasQuant = iota
	TransitiveClosureMapAliasB                                // want `quant variant TransitiveClosureMapAliasB \(TransitiveClosureMapAliasQuant\).*absent from 2 of 3 reachable CPU matmul dispatch layers`
	TransitiveClosureMapAliasC
)

func transitiveClosureMapAliasByteSize(q TransitiveClosureMapAliasQuant) int {
	switch q {
	case TransitiveClosureMapAliasA, TransitiveClosureMapAliasB, TransitiveClosureMapAliasC:
		return 8
	default:
		return 0
	}
}
func transitiveClosureMapAliasDecode(q TransitiveClosureMapAliasQuant) []float32 {
	switch q {
	case TransitiveClosureMapAliasA, TransitiveClosureMapAliasB, TransitiveClosureMapAliasC:
		return []float32{}
	default:
		return nil
	}
}
func transitiveClosureMapAliasKernel() bool { return true }
func transitiveClosureMapAliasPartialOne(q TransitiveClosureMapAliasQuant) bool {
	return q == TransitiveClosureMapAliasA || q == TransitiveClosureMapAliasC
}
func transitiveClosureMapAliasPartialTwo(q TransitiveClosureMapAliasQuant) bool {
	return q == TransitiveClosureMapAliasA || q == TransitiveClosureMapAliasC
}
func TransitiveClosureMapAliasQMatMul(q TransitiveClosureMapAliasQuant) bool {
	table := map[TransitiveClosureMapAliasQuant]func() bool{
		TransitiveClosureMapAliasA: transitiveClosureMapAliasKernel,
		TransitiveClosureMapAliasB: transitiveClosureMapAliasKernel,
		TransitiveClosureMapAliasC: transitiveClosureMapAliasKernel,
	}
	var alias map[TransitiveClosureMapAliasQuant]func() bool
	set := func() {
		temporary := table
		alias = temporary
	}
	use := func() { _ = alias[q]() }
	set()
	use()
	return transitiveClosureMapAliasPartialOne(q) && transitiveClosureMapAliasPartialTwo(q)
}

type NestedClosureMapAliasQuant uint8

const (
	NestedClosureMapAliasA NestedClosureMapAliasQuant = iota
	NestedClosureMapAliasB                            // want `quant variant NestedClosureMapAliasB \(NestedClosureMapAliasQuant\).*absent from 2 of 3 reachable CPU matmul dispatch layers`
	NestedClosureMapAliasC
)

func nestedClosureMapAliasByteSize(q NestedClosureMapAliasQuant) int {
	switch q {
	case NestedClosureMapAliasA, NestedClosureMapAliasB, NestedClosureMapAliasC:
		return 8
	default:
		return 0
	}
}
func nestedClosureMapAliasDecode(q NestedClosureMapAliasQuant) []float32 {
	switch q {
	case NestedClosureMapAliasA, NestedClosureMapAliasB, NestedClosureMapAliasC:
		return []float32{}
	default:
		return nil
	}
}
func nestedClosureMapAliasKernel() bool { return true }
func nestedClosureMapAliasPartialOne(q NestedClosureMapAliasQuant) bool {
	return q == NestedClosureMapAliasA || q == NestedClosureMapAliasC
}
func nestedClosureMapAliasPartialTwo(q NestedClosureMapAliasQuant) bool {
	return q == NestedClosureMapAliasA || q == NestedClosureMapAliasC
}
func NestedClosureMapAliasQMatMul(q NestedClosureMapAliasQuant) bool {
	table := map[NestedClosureMapAliasQuant]func() bool{
		NestedClosureMapAliasA: nestedClosureMapAliasKernel,
		NestedClosureMapAliasB: nestedClosureMapAliasKernel,
		NestedClosureMapAliasC: nestedClosureMapAliasKernel,
	}
	var alias map[NestedClosureMapAliasQuant]func() bool
	inner := func() { alias = table }
	set := func() { inner() }
	use := func() { _ = alias[q]() }
	set()
	use()
	return nestedClosureMapAliasPartialOne(q) && nestedClosureMapAliasPartialTwo(q)
}

type EarlyClosureMutationQuant uint8

const (
	EarlyClosureMutationA EarlyClosureMutationQuant = iota
	EarlyClosureMutationB                           // want `quant variant EarlyClosureMutationB \(EarlyClosureMutationQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	EarlyClosureMutationC
)

func earlyClosureMutationByteSize(q EarlyClosureMutationQuant) int {
	switch q {
	case EarlyClosureMutationA, EarlyClosureMutationB, EarlyClosureMutationC:
		return 8
	default:
		return 0
	}
}
func earlyClosureMutationDecode(q EarlyClosureMutationQuant) []float32 {
	switch q {
	case EarlyClosureMutationA, EarlyClosureMutationB, EarlyClosureMutationC:
		return []float32{}
	default:
		return nil
	}
}
func earlyClosureMutationKernel() bool { return true }
func earlyClosureMutationPartialOne(q EarlyClosureMutationQuant) bool {
	return q == EarlyClosureMutationA || q == EarlyClosureMutationC
}
func earlyClosureMutationPartialTwo(q EarlyClosureMutationQuant) bool {
	return q == EarlyClosureMutationA || q == EarlyClosureMutationC
}
func EarlyClosureMutationQMatMul(q EarlyClosureMutationQuant) bool {
	var table map[EarlyClosureMutationQuant]func() bool
	mutate := func() { table[EarlyClosureMutationB] = nil }
	table = map[EarlyClosureMutationQuant]func() bool{
		EarlyClosureMutationA: earlyClosureMutationKernel,
		EarlyClosureMutationB: earlyClosureMutationKernel,
		EarlyClosureMutationC: earlyClosureMutationKernel,
	}
	mutate()
	_ = table[q]
	return earlyClosureMutationPartialOne(q) && earlyClosureMutationPartialTwo(q)
}

type NamedInvokeMapQuant uint8

const (
	NamedInvokeMapA NamedInvokeMapQuant = iota
	NamedInvokeMapB                     // want `quant variant NamedInvokeMapB \(NamedInvokeMapQuant\).*absent from 2 of 3 reachable CPU matmul dispatch layers`
	NamedInvokeMapC
)

func namedInvokeMapByteSize(q NamedInvokeMapQuant) int {
	switch q {
	case NamedInvokeMapA, NamedInvokeMapB, NamedInvokeMapC:
		return 8
	default:
		return 0
	}
}
func namedInvokeMapDecode(q NamedInvokeMapQuant) []float32 {
	switch q {
	case NamedInvokeMapA, NamedInvokeMapB, NamedInvokeMapC:
		return []float32{}
	default:
		return nil
	}
}
func namedInvokeMapKernel() bool { return true }
func namedInvokeMapPartialOne(q NamedInvokeMapQuant) bool {
	return q == NamedInvokeMapA || q == NamedInvokeMapC
}
func namedInvokeMapPartialTwo(q NamedInvokeMapQuant) bool {
	return q == NamedInvokeMapA || q == NamedInvokeMapC
}
func namedInvokeMapRun(function func())   { function() }
func namedInvokeMapOuter(function func()) { namedInvokeMapRun(function) }
func NamedInvokeMapQMatMul(q NamedInvokeMapQuant) bool {
	var table map[NamedInvokeMapQuant]func() bool
	run := func() { _ = table[q]() }
	table = map[NamedInvokeMapQuant]func() bool{
		NamedInvokeMapA: namedInvokeMapKernel,
		NamedInvokeMapB: namedInvokeMapKernel,
		NamedInvokeMapC: namedInvokeMapKernel,
	}
	namedInvokeMapOuter(run)
	return namedInvokeMapPartialOne(q) && namedInvokeMapPartialTwo(q)
}

type ParameterMapQuant uint8

const (
	ParameterMapA ParameterMapQuant = iota
	ParameterMapB                   // want `quant variant ParameterMapB \(ParameterMapQuant\).*absent from 2 of 5 reachable CPU matmul dispatch layers`
	ParameterMapC
)

func parameterMapByteSize(q ParameterMapQuant) int {
	switch q {
	case ParameterMapA, ParameterMapB, ParameterMapC:
		return 8
	default:
		return 0
	}
}
func parameterMapDecode(q ParameterMapQuant) []float32 {
	switch q {
	case ParameterMapA, ParameterMapB, ParameterMapC:
		return []float32{}
	default:
		return nil
	}
}
func parameterMapKernel() bool { return true }
func parameterMapPartialOne(q ParameterMapQuant) bool {
	return q == ParameterMapA || q == ParameterMapC
}
func parameterMapPartialTwo(q ParameterMapQuant) bool {
	return q == ParameterMapA || q == ParameterMapC
}
func parameterMapApply(
	function func(map[ParameterMapQuant]func() bool),
	first map[ParameterMapQuant]func() bool,
	second map[ParameterMapQuant]func() bool,
) {
	function(first)
	function(second)
}
func parameterMapMutatingApply(
	function func(map[ParameterMapQuant]func() bool),
	source map[ParameterMapQuant]func() bool,
) {
	mutate := func() { delete(source, ParameterMapB) }
	mutate()
	function(source)
}
func parameterMapNamedMutator(source map[ParameterMapQuant]func() bool) {
	delete(source, ParameterMapB)
}
func parameterMapChooseMutator() bool { return false }
func ParameterMapQMatMul(q ParameterMapQuant) bool {
	table := map[ParameterMapQuant]func() bool{
		ParameterMapA: parameterMapKernel,
		ParameterMapB: parameterMapKernel,
		ParameterMapC: parameterMapKernel,
	}
	use := func(source map[ParameterMapQuant]func() bool) { _ = source[q]() }
	use(table)
	applied := map[ParameterMapQuant]func() bool{
		ParameterMapA: parameterMapKernel,
		ParameterMapB: parameterMapKernel,
		ParameterMapC: parameterMapKernel,
	}
	appliedSecond := map[ParameterMapQuant]func() bool{
		ParameterMapA: parameterMapKernel,
		ParameterMapB: parameterMapKernel,
		ParameterMapC: parameterMapKernel,
	}
	applyUse := func(source map[ParameterMapQuant]func() bool) { _ = source[q]() }
	parameterMapApply(applyUse, applied, appliedSecond)
	rebound := map[ParameterMapQuant]func() bool{
		ParameterMapA: parameterMapKernel,
		ParameterMapB: parameterMapKernel,
		ParameterMapC: parameterMapKernel,
	}
	rebindAndUse := func(source map[ParameterMapQuant]func() bool) {
		source = nil
		_ = source[q]
	}
	rebindAndUse(rebound)
	fallthroughMap := map[ParameterMapQuant]func() bool{
		ParameterMapA: parameterMapKernel,
		ParameterMapB: parameterMapKernel,
		ParameterMapC: parameterMapKernel,
	}
	fallthroughUse := func(source map[ParameterMapQuant]func() bool) {
		switch true {
		case true:
			source = nil
			fallthrough
		default:
			_ = source[q]
		}
	}
	fallthroughUse(fallthroughMap)
	mutated := map[ParameterMapQuant]func() bool{
		ParameterMapA: parameterMapKernel,
		ParameterMapB: parameterMapKernel,
		ParameterMapC: parameterMapKernel,
	}
	mutatingUse := func(source map[ParameterMapQuant]func() bool) { _ = source[q]() }
	parameterMapMutatingApply(mutatingUse, mutated)
	mixed := map[ParameterMapQuant]func() bool{
		ParameterMapA: parameterMapKernel,
		ParameterMapB: parameterMapKernel,
		ParameterMapC: parameterMapKernel,
	}
	mixedUse := func(source map[ParameterMapQuant]func() bool) { _ = source[q]() }
	if parameterMapChooseMutator() {
		mixedUse = parameterMapNamedMutator
	}
	mixedUse(mixed)
	return parameterMapPartialOne(q) && parameterMapPartialTwo(q)
}

type SameAssignmentMapQuant uint8

const (
	SameAssignmentMapA SameAssignmentMapQuant = iota
	SameAssignmentMapB                        // want `quant variant SameAssignmentMapB \(SameAssignmentMapQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	SameAssignmentMapC
)

func sameAssignmentMapByteSize(q SameAssignmentMapQuant) int {
	switch q {
	case SameAssignmentMapA, SameAssignmentMapB, SameAssignmentMapC:
		return 8
	default:
		return 0
	}
}
func sameAssignmentMapDecode(q SameAssignmentMapQuant) []float32 {
	switch q {
	case SameAssignmentMapA, SameAssignmentMapB, SameAssignmentMapC:
		return []float32{}
	default:
		return nil
	}
}
func sameAssignmentMapKernel() bool { return true }
func sameAssignmentMapPartialOne(q SameAssignmentMapQuant) bool {
	return q == SameAssignmentMapA || q == SameAssignmentMapC
}
func sameAssignmentMapPartialTwo(q SameAssignmentMapQuant) bool {
	return q == SameAssignmentMapA || q == SameAssignmentMapC
}
func SameAssignmentMapQMatMul(q SameAssignmentMapQuant) bool {
	var table map[SameAssignmentMapQuant]func() bool
	var selected func() bool
	table, selected = map[SameAssignmentMapQuant]func() bool{
		SameAssignmentMapA: sameAssignmentMapKernel,
		SameAssignmentMapB: sameAssignmentMapKernel,
		SameAssignmentMapC: sameAssignmentMapKernel,
	}, table[q]
	_ = selected
	return sameAssignmentMapPartialOne(q) && sameAssignmentMapPartialTwo(q)
}

type ConvertedMapQuant uint8
type convertedMapDispatch map[ConvertedMapQuant]func() bool

const (
	ConvertedMapA ConvertedMapQuant = iota
	ConvertedMapB                   // want `quant variant ConvertedMapB \(ConvertedMapQuant\).*absent from 2 of 3 reachable CPU matmul dispatch layers`
	ConvertedMapC
)

func convertedMapByteSize(q ConvertedMapQuant) int {
	switch q {
	case ConvertedMapA, ConvertedMapB, ConvertedMapC:
		return 8
	default:
		return 0
	}
}
func convertedMapDecode(q ConvertedMapQuant) []float32 {
	switch q {
	case ConvertedMapA, ConvertedMapB, ConvertedMapC:
		return []float32{}
	default:
		return nil
	}
}
func convertedMapKernel() bool { return true }
func convertedMapPartialOne(q ConvertedMapQuant) bool {
	return q == ConvertedMapA || q == ConvertedMapC
}
func convertedMapPartialTwo(q ConvertedMapQuant) bool {
	return q == ConvertedMapA || q == ConvertedMapC
}
func ConvertedMapQMatMul(q ConvertedMapQuant) bool {
	table := convertedMapDispatch((map[ConvertedMapQuant]func() bool{
		ConvertedMapA: convertedMapKernel,
		ConvertedMapB: convertedMapKernel,
		ConvertedMapC: convertedMapKernel,
	}))
	_ = table[q]()
	return convertedMapPartialOne(q) && convertedMapPartialTwo(q)
}

type EscapingSwitchQuant uint8

const (
	EscapingSwitchA EscapingSwitchQuant = iota
	EscapingSwitchB
	EscapingSwitchC
)

func escapingSwitchByteSize(q EscapingSwitchQuant) int {
	switch q {
	case EscapingSwitchA, EscapingSwitchB, EscapingSwitchC:
		return 8
	default:
		return 0
	}
}
func escapingSwitchDecode(q EscapingSwitchQuant) []float32 {
	switch q {
	case EscapingSwitchA, EscapingSwitchB, EscapingSwitchC:
		return []float32{}
	default:
		return nil
	}
}
func escapingSwitchOne(q EscapingSwitchQuant, escape bool) bool {
	if q == EscapingSwitchB {
		switch escape {
		case true:
			if escape {
				break
			}
			return false
		default:
			return false
		}
	}
	return true
}
func escapingSwitchTwo(q EscapingSwitchQuant, escape bool) bool {
	if q == EscapingSwitchB {
		switch escape {
		case true:
			if escape {
				break
			}
			return false
		default:
			return false
		}
	}
	return true
}
func escapingSwitchSupportOne(q EscapingSwitchQuant, shape bool) bool {
	if q == EscapingSwitchB {
		if shape {
			return true
		}
	}
	return true
}
func escapingSwitchSupportTwo(q EscapingSwitchQuant, shape bool) bool {
	if q == EscapingSwitchB {
		if shape {
			return true
		}
	}
	return true
}
func EscapingSwitchQMatMul(q EscapingSwitchQuant) bool {
	return escapingSwitchOne(q, true) && escapingSwitchTwo(q, true) &&
		escapingSwitchSupportOne(q, true) && escapingSwitchSupportTwo(q, true)
}

type NestedSwitchRejectQuant uint8

const (
	NestedSwitchRejectA NestedSwitchRejectQuant = iota
	NestedSwitchRejectB                         // want `quant variant NestedSwitchRejectB \(NestedSwitchRejectQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	NestedSwitchRejectC
)

func nestedSwitchRejectByteSize(q NestedSwitchRejectQuant) int {
	switch q {
	case NestedSwitchRejectA, NestedSwitchRejectB, NestedSwitchRejectC:
		return 8
	default:
		return 0
	}
}
func nestedSwitchRejectDecode(q NestedSwitchRejectQuant) []float32 {
	switch q {
	case NestedSwitchRejectA, NestedSwitchRejectB, NestedSwitchRejectC:
		return []float32{}
	default:
		return nil
	}
}
func nestedSwitchRejectOne(q NestedSwitchRejectQuant, route bool) bool {
	if q == NestedSwitchRejectB {
		switch route {
		case true:
			for {
				break
			}
			fallthrough
		default:
			return false
		}
	} else if q == NestedSwitchRejectA || q == NestedSwitchRejectC {
		return true
	}
	return true
}
func nestedSwitchRejectTwo(q NestedSwitchRejectQuant, route bool) bool {
	if q == NestedSwitchRejectB {
		switch route {
		case true:
			for {
				break
			}
			return false
		default:
			return false
		}
	} else if q == NestedSwitchRejectA || q == NestedSwitchRejectC {
		return true
	}
	return true
}
func NestedSwitchRejectQMatMul(q NestedSwitchRejectQuant) bool {
	return nestedSwitchRejectOne(q, true) && nestedSwitchRejectTwo(q, true)
}

type CapturedKillMapQuant uint8

const (
	CapturedKillMapA CapturedKillMapQuant = iota
	CapturedKillMapB                      // want `quant variant CapturedKillMapB \(CapturedKillMapQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	CapturedKillMapC
)

func capturedKillMapByteSize(q CapturedKillMapQuant) int {
	switch q {
	case CapturedKillMapA, CapturedKillMapB, CapturedKillMapC:
		return 8
	}
	return 0
}
func capturedKillMapDecode(q CapturedKillMapQuant) []float32 {
	switch q {
	case CapturedKillMapA, CapturedKillMapB, CapturedKillMapC:
		return []float32{}
	}
	return nil
}
func capturedKillMapKernel() bool { return true }
func capturedKillMapPartialOne(q CapturedKillMapQuant) bool {
	return q == CapturedKillMapA || q == CapturedKillMapC
}
func capturedKillMapPartialTwo(q CapturedKillMapQuant) bool {
	return q == CapturedKillMapA || q == CapturedKillMapC
}
func CapturedKillMapQMatMul(q CapturedKillMapQuant) bool {
	table := map[CapturedKillMapQuant]func() bool{
		CapturedKillMapA: capturedKillMapKernel,
		CapturedKillMapB: capturedKillMapKernel,
		CapturedKillMapC: capturedKillMapKernel,
	}
	alias := table
	run := func() {
		alias = nil
		_ = alias[q]
	}
	run()
	return capturedKillMapPartialOne(q) && capturedKillMapPartialTwo(q)
}

type MutuallyExclusiveMapQuant uint8

const (
	MutuallyExclusiveMapA MutuallyExclusiveMapQuant = iota
	MutuallyExclusiveMapB                           // want `quant variant MutuallyExclusiveMapB \(MutuallyExclusiveMapQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	MutuallyExclusiveMapC
)

func mutuallyExclusiveMapByteSize(q MutuallyExclusiveMapQuant) int {
	switch q {
	case MutuallyExclusiveMapA, MutuallyExclusiveMapB, MutuallyExclusiveMapC:
		return 8
	}
	return 0
}
func mutuallyExclusiveMapDecode(q MutuallyExclusiveMapQuant) []float32 {
	switch q {
	case MutuallyExclusiveMapA, MutuallyExclusiveMapB, MutuallyExclusiveMapC:
		return []float32{}
	}
	return nil
}
func mutuallyExclusiveMapKernel() bool { return true }
func mutuallyExclusiveMapPartialOne(q MutuallyExclusiveMapQuant) bool {
	return q == MutuallyExclusiveMapA || q == MutuallyExclusiveMapC
}
func mutuallyExclusiveMapPartialTwo(q MutuallyExclusiveMapQuant) bool {
	return q == MutuallyExclusiveMapA || q == MutuallyExclusiveMapC
}
func MutuallyExclusiveMapQMatMul(q MutuallyExclusiveMapQuant, choose bool) bool {
	table := map[MutuallyExclusiveMapQuant]func() bool{
		MutuallyExclusiveMapA: mutuallyExclusiveMapKernel,
		MutuallyExclusiveMapB: mutuallyExclusiveMapKernel,
		MutuallyExclusiveMapC: mutuallyExclusiveMapKernel,
	}
	var alias map[MutuallyExclusiveMapQuant]func() bool
	set := func() { alias = table }
	use := func() { _ = alias[q] }
	if choose {
		set()
	} else {
		use()
	}
	return mutuallyExclusiveMapPartialOne(q) && mutuallyExclusiveMapPartialTwo(q)
}

type ParameterAfterUseQuant uint8

const (
	ParameterAfterUseA ParameterAfterUseQuant = iota
	ParameterAfterUseB                        // want `quant variant ParameterAfterUseB \(ParameterAfterUseQuant\).*absent from 2 of 4 reachable CPU matmul dispatch layers`
	ParameterAfterUseC
)

func parameterAfterUseByteSize(q ParameterAfterUseQuant) int {
	switch q {
	case ParameterAfterUseA, ParameterAfterUseB, ParameterAfterUseC:
		return 8
	}
	return 0
}
func parameterAfterUseDecode(q ParameterAfterUseQuant) []float32 {
	switch q {
	case ParameterAfterUseA, ParameterAfterUseB, ParameterAfterUseC:
		return []float32{}
	}
	return nil
}
func parameterAfterUseKernel() bool { return true }
func parameterAfterUseChoose() bool { return false }
func parameterAfterUsePartialOne(q ParameterAfterUseQuant) bool {
	return q == ParameterAfterUseA || q == ParameterAfterUseC
}
func parameterAfterUsePartialTwo(q ParameterAfterUseQuant) bool {
	return q == ParameterAfterUseA || q == ParameterAfterUseC
}
func ParameterAfterUseQMatMul(q ParameterAfterUseQuant) bool {
	table := map[ParameterAfterUseQuant]func() bool{
		ParameterAfterUseA: parameterAfterUseKernel,
		ParameterAfterUseB: parameterAfterUseKernel,
		ParameterAfterUseC: parameterAfterUseKernel,
	}
	use := func(source map[ParameterAfterUseQuant]func() bool) {
		_ = source[q]()
		source = nil
	}
	use(table)
	conditionalTable := map[ParameterAfterUseQuant]func() bool{
		ParameterAfterUseA: parameterAfterUseKernel,
		ParameterAfterUseB: parameterAfterUseKernel,
		ParameterAfterUseC: parameterAfterUseKernel,
	}
	conditionalUse := func(source map[ParameterAfterUseQuant]func() bool) {
		if parameterAfterUseChoose() {
			source = nil
		}
		_ = source[q]()
	}
	conditionalUse(conditionalTable)
	return parameterAfterUsePartialOne(q) && parameterAfterUsePartialTwo(q)
}

type MixedForwardMapQuant uint8

const (
	MixedForwardMapA MixedForwardMapQuant = iota
	MixedForwardMapB                      // want `quant variant MixedForwardMapB \(MixedForwardMapQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	MixedForwardMapC
)

func mixedForwardMapByteSize(q MixedForwardMapQuant) int {
	switch q {
	case MixedForwardMapA, MixedForwardMapB, MixedForwardMapC:
		return 8
	}
	return 0
}
func mixedForwardMapDecode(q MixedForwardMapQuant) []float32 {
	switch q {
	case MixedForwardMapA, MixedForwardMapB, MixedForwardMapC:
		return []float32{}
	}
	return nil
}
func mixedForwardMapKernel() bool { return true }
func mixedForwardMapPartialOne(q MixedForwardMapQuant) bool {
	return q == MixedForwardMapA || q == MixedForwardMapC
}
func mixedForwardMapPartialTwo(q MixedForwardMapQuant) bool {
	return q == MixedForwardMapA || q == MixedForwardMapC
}
func mixedForwardMapApply(
	function func(map[MixedForwardMapQuant]func() bool),
	table map[MixedForwardMapQuant]func() bool,
) {
	function(table)
}
func mixedForwardMapMutator(table map[MixedForwardMapQuant]func() bool) {
	delete(table, MixedForwardMapB)
}
func MixedForwardMapQMatMul(q MixedForwardMapQuant, mutate bool) bool {
	table := map[MixedForwardMapQuant]func() bool{
		MixedForwardMapA: mixedForwardMapKernel,
		MixedForwardMapB: mixedForwardMapKernel,
		MixedForwardMapC: mixedForwardMapKernel,
	}
	function := func(source map[MixedForwardMapQuant]func() bool) { _ = source[q]() }
	if mutate {
		function = mixedForwardMapMutator
	}
	mixedForwardMapApply(function, table)
	return mixedForwardMapPartialOne(q) && mixedForwardMapPartialTwo(q)
}

type LiteralArgumentMapQuant uint8

const (
	LiteralArgumentMapA LiteralArgumentMapQuant = iota
	LiteralArgumentMapB                         // want `quant variant LiteralArgumentMapB \(LiteralArgumentMapQuant\).*absent from 2 of 3 reachable CPU matmul dispatch layers`
	LiteralArgumentMapC
)

func literalArgumentMapByteSize(q LiteralArgumentMapQuant) int {
	switch q {
	case LiteralArgumentMapA, LiteralArgumentMapB, LiteralArgumentMapC:
		return 8
	}
	return 0
}
func literalArgumentMapDecode(q LiteralArgumentMapQuant) []float32 {
	switch q {
	case LiteralArgumentMapA, LiteralArgumentMapB, LiteralArgumentMapC:
		return []float32{}
	}
	return nil
}
func literalArgumentMapKernel() bool { return true }
func literalArgumentMapPartialOne(q LiteralArgumentMapQuant) bool {
	return q == LiteralArgumentMapA || q == LiteralArgumentMapC
}
func literalArgumentMapPartialTwo(q LiteralArgumentMapQuant) bool {
	return q == LiteralArgumentMapA || q == LiteralArgumentMapC
}
func LiteralArgumentMapQMatMul(q LiteralArgumentMapQuant) bool {
	use := func(source map[LiteralArgumentMapQuant]func() bool) { _ = source[q]() }
	use(map[LiteralArgumentMapQuant]func() bool{
		LiteralArgumentMapA: literalArgumentMapKernel,
		LiteralArgumentMapB: literalArgumentMapKernel,
		LiteralArgumentMapC: literalArgumentMapKernel,
	})
	return literalArgumentMapPartialOne(q) && literalArgumentMapPartialTwo(q)
}

type GotoClosureAliasQuant uint8

const (
	GotoClosureAliasA GotoClosureAliasQuant = iota
	GotoClosureAliasB                       // want `quant variant GotoClosureAliasB \(GotoClosureAliasQuant\).*absent from 2 of 6 reachable CPU matmul dispatch layers`
	GotoClosureAliasC
)

func gotoClosureAliasByteSize(q GotoClosureAliasQuant) int {
	switch q {
	case GotoClosureAliasA, GotoClosureAliasB, GotoClosureAliasC:
		return 8
	}
	return 0
}
func gotoClosureAliasDecode(q GotoClosureAliasQuant) []float32 {
	switch q {
	case GotoClosureAliasA, GotoClosureAliasB, GotoClosureAliasC:
		return []float32{}
	}
	return nil
}
func gotoClosureAliasKernel() bool { return true }
func gotoClosureAliasPartialOne(q GotoClosureAliasQuant) bool {
	return q == GotoClosureAliasA || q == GotoClosureAliasC
}
func gotoClosureAliasPartialTwo(q GotoClosureAliasQuant) bool {
	return q == GotoClosureAliasA || q == GotoClosureAliasC
}
func gotoClosureAliasRun(
	set func(map[GotoClosureAliasQuant]func() bool),
	use func(),
	source map[GotoClosureAliasQuant]func() bool,
) {
	set(source)
	use()
}
func gotoClosureAliasRunRepeated(
	set func(map[GotoClosureAliasQuant]func() bool),
	kill, use func(),
	source map[GotoClosureAliasQuant]func() bool,
) {
	set(source)
	kill()
	set(source)
	use()
}
func gotoClosureAliasRunArguments(
	set func(map[GotoClosureAliasQuant]func() bool),
	use func(),
	first, second map[GotoClosureAliasQuant]func() bool,
) {
	set(first)
	set(second)
	use()
}
func gotoClosureAliasRunMiddle(
	set func(map[GotoClosureAliasQuant]func() bool),
	kill, use func(),
	source map[GotoClosureAliasQuant]func() bool,
) {
	set(source)
	kill()
	set(source)
	use()
	kill()
	set(source)
}
func GotoClosureAliasQMatMul(q GotoClosureAliasQuant) bool {
	table := map[GotoClosureAliasQuant]func() bool{
		GotoClosureAliasA: gotoClosureAliasKernel,
		GotoClosureAliasB: gotoClosureAliasKernel,
		GotoClosureAliasC: gotoClosureAliasKernel,
	}
	var alias map[GotoClosureAliasQuant]func() bool
	set := func() { alias = table }
	use := func() { _ = alias[q]() }
	namedTable := map[GotoClosureAliasQuant]func() bool{
		GotoClosureAliasA: gotoClosureAliasKernel,
		GotoClosureAliasB: gotoClosureAliasKernel,
		GotoClosureAliasC: gotoClosureAliasKernel,
	}
	var namedAlias map[GotoClosureAliasQuant]func() bool
	namedSet := func(source map[GotoClosureAliasQuant]func() bool) { namedAlias = source }
	namedUse := func() { _ = namedAlias[q]() }
	gotoClosureAliasRun(namedSet, namedUse, namedTable)
	repeatedTable := map[GotoClosureAliasQuant]func() bool{
		GotoClosureAliasA: gotoClosureAliasKernel,
		GotoClosureAliasB: gotoClosureAliasKernel,
		GotoClosureAliasC: gotoClosureAliasKernel,
	}
	var repeatedAlias map[GotoClosureAliasQuant]func() bool
	repeatedSet := func(source map[GotoClosureAliasQuant]func() bool) { repeatedAlias = source }
	repeatedKill := func() { repeatedAlias = nil }
	repeatedUse := func() { _ = repeatedAlias[q]() }
	gotoClosureAliasRunRepeated(repeatedSet, repeatedKill, repeatedUse, repeatedTable)
	argumentTable := map[GotoClosureAliasQuant]func() bool{
		GotoClosureAliasA: gotoClosureAliasKernel,
		GotoClosureAliasB: gotoClosureAliasKernel,
		GotoClosureAliasC: gotoClosureAliasKernel,
	}
	var argumentAlias map[GotoClosureAliasQuant]func() bool
	argumentSet := func(source map[GotoClosureAliasQuant]func() bool) { argumentAlias = source }
	argumentUse := func() { _ = argumentAlias[q] }
	gotoClosureAliasRunArguments(argumentSet, argumentUse, argumentTable, nil)
	middleTable := map[GotoClosureAliasQuant]func() bool{
		GotoClosureAliasA: gotoClosureAliasKernel,
		GotoClosureAliasB: gotoClosureAliasKernel,
		GotoClosureAliasC: gotoClosureAliasKernel,
	}
	var middleAlias map[GotoClosureAliasQuant]func() bool
	middleSet := func(source map[GotoClosureAliasQuant]func() bool) { middleAlias = source }
	middleKill := func() { middleAlias = nil }
	middleUse := func() { _ = middleAlias[q]() }
	gotoClosureAliasRunMiddle(middleSet, middleKill, middleUse, middleTable)
	goto setCall
useCall:
	use()
	return gotoClosureAliasPartialOne(q) && gotoClosureAliasPartialTwo(q)
setCall:
	set()
	goto useCall
}

type RangeRebindMapQuant uint8

const (
	RangeRebindMapA RangeRebindMapQuant = iota
	RangeRebindMapB                     // want `quant variant RangeRebindMapB \(RangeRebindMapQuant\).*absent from 2 of 5 reachable CPU matmul dispatch layers`
	RangeRebindMapC
)

func rangeRebindMapByteSize(q RangeRebindMapQuant) int {
	switch q {
	case RangeRebindMapA, RangeRebindMapB, RangeRebindMapC:
		return 8
	}
	return 0
}
func rangeRebindMapDecode(q RangeRebindMapQuant) []float32 {
	switch q {
	case RangeRebindMapA, RangeRebindMapB, RangeRebindMapC:
		return []float32{}
	}
	return nil
}
func rangeRebindMapKernel() bool { return true }
func rangeRebindMapPartialOne(q RangeRebindMapQuant) bool {
	return q == RangeRebindMapA || q == RangeRebindMapC
}
func rangeRebindMapPartialTwo(q RangeRebindMapQuant) bool {
	return q == RangeRebindMapA || q == RangeRebindMapC
}
func RangeRebindMapQMatMul(q RangeRebindMapQuant) bool {
	table := map[RangeRebindMapQuant]func() bool{
		RangeRebindMapA: rangeRebindMapKernel,
		RangeRebindMapB: rangeRebindMapKernel,
		RangeRebindMapC: rangeRebindMapKernel,
	}
	use := func(source map[RangeRebindMapQuant]func() bool) {
		for _, source = range []map[RangeRebindMapQuant]func() bool{nil} {
		}
		_ = source[q]
	}
	use(table)
	emptyTable := map[RangeRebindMapQuant]func() bool{
		RangeRebindMapA: rangeRebindMapKernel,
		RangeRebindMapB: rangeRebindMapKernel,
		RangeRebindMapC: rangeRebindMapKernel,
	}
	emptyUse := func(source map[RangeRebindMapQuant]func() bool) {
		for _, source = range []map[RangeRebindMapQuant]func() bool{} {
		}
		_ = source[q]()
	}
	emptyUse(emptyTable)
	reboundTable := map[RangeRebindMapQuant]func() bool{
		RangeRebindMapA: rangeRebindMapKernel,
		RangeRebindMapB: rangeRebindMapKernel,
		RangeRebindMapC: rangeRebindMapKernel,
	}
	for _, reboundTable = range []map[RangeRebindMapQuant]func() bool{nil} {
	}
	_ = reboundTable[q]
	aliasTable := map[RangeRebindMapQuant]func() bool{
		RangeRebindMapA: rangeRebindMapKernel,
		RangeRebindMapB: rangeRebindMapKernel,
		RangeRebindMapC: rangeRebindMapKernel,
	}
	for _, alias := range []map[RangeRebindMapQuant]func() bool{aliasTable} {
		_ = alias[q]()
	}
	identityTable := map[RangeRebindMapQuant]func() bool{
		RangeRebindMapA: rangeRebindMapKernel,
		RangeRebindMapB: rangeRebindMapKernel,
		RangeRebindMapC: rangeRebindMapKernel,
	}
	for _, identityTable = range []map[RangeRebindMapQuant]func() bool{identityTable} {
	}
	_ = identityTable[q]()
	return rangeRebindMapPartialOne(q) && rangeRebindMapPartialTwo(q)
}

type MultiResultMapQuant uint8

const (
	MultiResultMapA MultiResultMapQuant = iota
	MultiResultMapB                     // want `quant variant MultiResultMapB \(MultiResultMapQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	MultiResultMapC
)

func multiResultMapByteSize(q MultiResultMapQuant) int {
	switch q {
	case MultiResultMapA, MultiResultMapB, MultiResultMapC:
		return 8
	}
	return 0
}
func multiResultMapDecode(q MultiResultMapQuant) []float32 {
	switch q {
	case MultiResultMapA, MultiResultMapB, MultiResultMapC:
		return []float32{}
	}
	return nil
}
func multiResultMapKernel() bool { return true }
func multiResultMapPartialOne(q MultiResultMapQuant) bool {
	return q == MultiResultMapA || q == MultiResultMapC
}
func multiResultMapPartialTwo(q MultiResultMapQuant) bool {
	return q == MultiResultMapA || q == MultiResultMapC
}
func multiResultMapPair() (map[MultiResultMapQuant]func() bool, bool) { return nil, false }
func MultiResultMapQMatMul(q MultiResultMapQuant) bool {
	table := map[MultiResultMapQuant]func() bool{
		MultiResultMapA: multiResultMapKernel,
		MultiResultMapB: multiResultMapKernel,
		MultiResultMapC: multiResultMapKernel,
	}
	alias := table
	alias, _ = multiResultMapPair()
	_ = alias[q]
	return multiResultMapPartialOne(q) && multiResultMapPartialTwo(q)
}

type CyclicMapAliasQuant uint8
type cyclicMapAliasNamed map[CyclicMapAliasQuant]func() bool

const (
	CyclicMapAliasA CyclicMapAliasQuant = iota
	CyclicMapAliasB                     // want `quant variant CyclicMapAliasB \(CyclicMapAliasQuant\).*absent from 2 of 6 reachable CPU matmul dispatch layers`
	CyclicMapAliasC
)

func cyclicMapAliasByteSize(q CyclicMapAliasQuant) int {
	switch q {
	case CyclicMapAliasA, CyclicMapAliasB, CyclicMapAliasC:
		return 8
	}
	return 0
}
func cyclicMapAliasDecode(q CyclicMapAliasQuant) []float32 {
	switch q {
	case CyclicMapAliasA, CyclicMapAliasB, CyclicMapAliasC:
		return []float32{}
	}
	return nil
}
func cyclicMapAliasKernel() bool { return true }
func cyclicMapAliasPartialOne(q CyclicMapAliasQuant) bool {
	return q == CyclicMapAliasA || q == CyclicMapAliasC
}
func cyclicMapAliasPartialTwo(q CyclicMapAliasQuant) bool {
	return q == CyclicMapAliasA || q == CyclicMapAliasC
}
func CyclicMapAliasQMatMul(q CyclicMapAliasQuant) bool {
	table := map[CyclicMapAliasQuant]func() bool{
		CyclicMapAliasA: cyclicMapAliasKernel,
		CyclicMapAliasB: cyclicMapAliasKernel,
		CyclicMapAliasC: cyclicMapAliasKernel,
	}
	alias := table
	alias = nil
	alias = alias
	_ = alias[q]
	selfTable := map[CyclicMapAliasQuant]func() bool{
		CyclicMapAliasA: cyclicMapAliasKernel,
		CyclicMapAliasB: cyclicMapAliasKernel,
		CyclicMapAliasC: cyclicMapAliasKernel,
	}
	selfAlias := selfTable
	selfAlias = selfAlias
	_ = selfAlias[q]()
	directTable := map[CyclicMapAliasQuant]func() bool{
		CyclicMapAliasA: cyclicMapAliasKernel,
		CyclicMapAliasB: cyclicMapAliasKernel,
		CyclicMapAliasC: cyclicMapAliasKernel,
	}
	directTable = directTable
	_ = directTable[q]()
	convertedTable := map[CyclicMapAliasQuant]func() bool{
		CyclicMapAliasA: cyclicMapAliasKernel,
		CyclicMapAliasB: cyclicMapAliasKernel,
		CyclicMapAliasC: cyclicMapAliasKernel,
	}
	convertedAlias := cyclicMapAliasNamed(convertedTable)
	convertedAlias = cyclicMapAliasNamed(convertedAlias)
	_ = convertedAlias[q]()
	roundTripTable := map[CyclicMapAliasQuant]func() bool{
		CyclicMapAliasA: cyclicMapAliasKernel,
		CyclicMapAliasB: cyclicMapAliasKernel,
		CyclicMapAliasC: cyclicMapAliasKernel,
	}
	roundTripAlias := roundTripTable
	roundTripTable = roundTripAlias
	_ = roundTripTable[q]()
	return cyclicMapAliasPartialOne(q) && cyclicMapAliasPartialTwo(q)
}

type ConditionalSetterMapQuant uint8

const (
	ConditionalSetterMapA ConditionalSetterMapQuant = iota
	ConditionalSetterMapB                           // want `quant variant ConditionalSetterMapB \(ConditionalSetterMapQuant\).*absent from 2 of 4 reachable CPU matmul dispatch layers`
	ConditionalSetterMapC
)

func conditionalSetterMapByteSize(q ConditionalSetterMapQuant) int {
	switch q {
	case ConditionalSetterMapA, ConditionalSetterMapB, ConditionalSetterMapC:
		return 8
	}
	return 0
}
func conditionalSetterMapDecode(q ConditionalSetterMapQuant) []float32 {
	switch q {
	case ConditionalSetterMapA, ConditionalSetterMapB, ConditionalSetterMapC:
		return []float32{}
	}
	return nil
}
func conditionalSetterMapKernel() bool { return true }
func conditionalSetterMapPartialOne(q ConditionalSetterMapQuant) bool {
	return q == ConditionalSetterMapA || q == ConditionalSetterMapC
}
func conditionalSetterMapPartialTwo(q ConditionalSetterMapQuant) bool {
	return q == ConditionalSetterMapA || q == ConditionalSetterMapC
}
func ConditionalSetterMapQMatMul(q ConditionalSetterMapQuant, choose, other bool) bool {
	table := map[ConditionalSetterMapQuant]func() bool{
		ConditionalSetterMapA: conditionalSetterMapKernel,
		ConditionalSetterMapB: conditionalSetterMapKernel,
		ConditionalSetterMapC: conditionalSetterMapKernel,
	}
	var alias map[ConditionalSetterMapQuant]func() bool
	set := func() {
		if choose {
			alias = table
		}
	}
	use := func() { _ = alias[q]() }
	set()
	use()
	killTable := map[ConditionalSetterMapQuant]func() bool{
		ConditionalSetterMapA: conditionalSetterMapKernel,
		ConditionalSetterMapB: conditionalSetterMapKernel,
		ConditionalSetterMapC: conditionalSetterMapKernel,
	}
	killAlias := killTable
	kill := func() {
		if choose {
			killAlias = nil
		}
	}
	kill()
	_ = killAlias[q]()
	directKilledTable := map[ConditionalSetterMapQuant]func() bool{
		ConditionalSetterMapA: conditionalSetterMapKernel,
		ConditionalSetterMapB: conditionalSetterMapKernel,
		ConditionalSetterMapC: conditionalSetterMapKernel,
	}
	directKilledAlias := directKilledTable
	if choose {
		directKilledAlias = nil
	} else {
		directKilledAlias = nil
	}
	_ = directKilledAlias[q]
	closureKilledTable := map[ConditionalSetterMapQuant]func() bool{
		ConditionalSetterMapA: conditionalSetterMapKernel,
		ConditionalSetterMapB: conditionalSetterMapKernel,
		ConditionalSetterMapC: conditionalSetterMapKernel,
	}
	closureKilledAlias := closureKilledTable
	completeKill := func() {
		if choose {
			closureKilledAlias = nil
		} else {
			closureKilledAlias = nil
		}
	}
	completeKill()
	_ = closureKilledAlias[q]
	elseIfKilledTable := map[ConditionalSetterMapQuant]func() bool{
		ConditionalSetterMapA: conditionalSetterMapKernel,
		ConditionalSetterMapB: conditionalSetterMapKernel,
		ConditionalSetterMapC: conditionalSetterMapKernel,
	}
	elseIfKilledAlias := elseIfKilledTable
	if choose {
		elseIfKilledAlias = nil
	} else if other {
		elseIfKilledAlias = nil
	} else {
		elseIfKilledAlias = nil
	}
	_ = elseIfKilledAlias[q]
	switchKilledTable := map[ConditionalSetterMapQuant]func() bool{
		ConditionalSetterMapA: conditionalSetterMapKernel,
		ConditionalSetterMapB: conditionalSetterMapKernel,
		ConditionalSetterMapC: conditionalSetterMapKernel,
	}
	switchKilledAlias := switchKilledTable
	switch {
	case choose:
		switchKilledAlias = nil
	case other:
		switchKilledAlias = nil
	default:
		switchKilledAlias = nil
	}
	_ = switchKilledAlias[q]
	otherKilledTable := map[ConditionalSetterMapQuant]func() bool{
		ConditionalSetterMapA: conditionalSetterMapKernel,
		ConditionalSetterMapB: conditionalSetterMapKernel,
		ConditionalSetterMapC: conditionalSetterMapKernel,
	}
	otherMap := make(map[ConditionalSetterMapQuant]func() bool)
	otherKilledAlias := otherKilledTable
	if choose {
		otherKilledAlias = otherMap
	} else {
		otherKilledAlias = otherMap
	}
	_ = otherKilledAlias[q]
	return conditionalSetterMapPartialOne(q) && conditionalSetterMapPartialTwo(q)
}

type StructFieldMapQuant uint8

const (
	StructFieldMapA StructFieldMapQuant = iota
	StructFieldMapB                     // want `quant variant StructFieldMapB \(StructFieldMapQuant\).*absent from 2 of 3 reachable CPU matmul dispatch layers`
	StructFieldMapC
)

func structFieldMapByteSize(q StructFieldMapQuant) int {
	switch q {
	case StructFieldMapA, StructFieldMapB, StructFieldMapC:
		return 8
	}
	return 0
}
func structFieldMapDecode(q StructFieldMapQuant) []float32 {
	switch q {
	case StructFieldMapA, StructFieldMapB, StructFieldMapC:
		return []float32{}
	}
	return nil
}
func structFieldMapKernel() bool { return true }
func structFieldMapPartialOne(q StructFieldMapQuant) bool {
	return q == StructFieldMapA || q == StructFieldMapC
}
func structFieldMapPartialTwo(q StructFieldMapQuant) bool {
	return q == StructFieldMapA || q == StructFieldMapC
}
func StructFieldMapQMatMul(q StructFieldMapQuant) bool {
	holder := struct {
		dispatch map[StructFieldMapQuant]func() bool
	}{
		dispatch: map[StructFieldMapQuant]func() bool{
			StructFieldMapA: structFieldMapKernel,
			StructFieldMapB: structFieldMapKernel,
			StructFieldMapC: structFieldMapKernel,
		},
	}
	_ = holder.dispatch[q]()
	discarded := struct {
		dispatch map[StructFieldMapQuant]func() bool
	}{
		dispatch: map[StructFieldMapQuant]func() bool{
			StructFieldMapA: structFieldMapKernel,
			StructFieldMapB: structFieldMapKernel,
			StructFieldMapC: structFieldMapKernel,
		},
	}
	discarded = struct {
		dispatch map[StructFieldMapQuant]func() bool
	}{}
	_ = discarded.dispatch[q]
	return structFieldMapPartialOne(q) && structFieldMapPartialTwo(q)
}

type DiamondClosureMapQuant uint8

const (
	DiamondClosureMapA DiamondClosureMapQuant = iota
	DiamondClosureMapB                        // want `quant variant DiamondClosureMapB \(DiamondClosureMapQuant\).*absent from 2 of 3 reachable CPU matmul dispatch layers`
	DiamondClosureMapC
)

func diamondClosureMapByteSize(q DiamondClosureMapQuant) int {
	switch q {
	case DiamondClosureMapA, DiamondClosureMapB, DiamondClosureMapC:
		return 8
	}
	return 0
}
func diamondClosureMapDecode(q DiamondClosureMapQuant) []float32 {
	switch q {
	case DiamondClosureMapA, DiamondClosureMapB, DiamondClosureMapC:
		return []float32{}
	}
	return nil
}
func diamondClosureMapKernel() bool { return true }
func diamondClosureMapPartialOne(q DiamondClosureMapQuant) bool {
	return q == DiamondClosureMapA || q == DiamondClosureMapC
}
func diamondClosureMapPartialTwo(q DiamondClosureMapQuant) bool {
	return q == DiamondClosureMapA || q == DiamondClosureMapC
}
func DiamondClosureMapQMatMul(q DiamondClosureMapQuant) bool {
	table := map[DiamondClosureMapQuant]func() bool{
		DiamondClosureMapA: diamondClosureMapKernel,
		DiamondClosureMapB: diamondClosureMapKernel,
		DiamondClosureMapC: diamondClosureMapKernel,
	}
	var alias map[DiamondClosureMapQuant]func() bool
	f0 := func() { alias = table }
	f1 := func() { f0(); f0() }
	f2 := func() { f1(); f1() }
	f3 := func() { f2(); f2() }
	f4 := func() { f3(); f3() }
	f5 := func() { f4(); f4() }
	f6 := func() { f5(); f5() }
	f7 := func() { f6(); f6() }
	f8 := func() { f7(); f7() }
	f9 := func() { f8(); f8() }
	f10 := func() { f9(); f9() }
	f11 := func() { f10(); f10() }
	f12 := func() { f11(); f11() }
	f12()
	_ = alias[q]()
	return diamondClosureMapPartialOne(q) && diamondClosureMapPartialTwo(q)
}

type SparseCallbackMapQuant uint8

const (
	SparseCallbackMapA SparseCallbackMapQuant = iota
	SparseCallbackMapB                        // want `quant variant SparseCallbackMapB \(SparseCallbackMapQuant\).*absent from 2 of 2 reachable CPU matmul dispatch layers`
	SparseCallbackMapC
)

func sparseCallbackMapByteSize(q SparseCallbackMapQuant) int {
	switch q {
	case SparseCallbackMapA, SparseCallbackMapB, SparseCallbackMapC:
		return 8
	}
	return 0
}

func sparseCallbackMapDecode(q SparseCallbackMapQuant) []float32 {
	switch q {
	case SparseCallbackMapA, SparseCallbackMapB, SparseCallbackMapC:
		return []float32{}
	}
	return nil
}

func sparseCallbackMapKernel() bool { return true }

func sparseCallbackMapPartialOne(q SparseCallbackMapQuant) bool {
	return q == SparseCallbackMapA || q == SparseCallbackMapC
}

func sparseCallbackMapPartialTwo(q SparseCallbackMapQuant) bool {
	return q == SparseCallbackMapA || q == SparseCallbackMapC
}

func sparseCallbackInvokeSecond(_ func(), second func()) { second() }

func sparseCallbackForwardFirst(first func()) {
	sparseCallbackInvokeSecond(first, func() {})
}

func SparseCallbackMapQMatMul(q SparseCallbackMapQuant) bool {
	table := map[SparseCallbackMapQuant]func() bool{
		SparseCallbackMapA: sparseCallbackMapKernel,
		SparseCallbackMapB: sparseCallbackMapKernel,
		SparseCallbackMapC: sparseCallbackMapKernel,
	}
	var alias map[SparseCallbackMapQuant]func() bool
	dormant := func() { alias = table }
	sparseCallbackForwardFirst(dormant)
	_ = alias[q]
	return sparseCallbackMapPartialOne(q) && sparseCallbackMapPartialTwo(q)
}
