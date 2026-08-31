package ps6092

type Number interface {
	~float32 | ~float64
}

type Binary[F Number] interface {
	Apply(F, F) F
}

type Closure[F Number] func() F

type BinaryAlias[F Number] = Binary[F]

type EmbeddedBinary[F Number] interface {
	Binary[F]
}

type BinarySet[F Number] interface {
	Add[F] | Sub[F]
	Apply(F, F) F
}

type hiddenBinary[F Number] interface {
	apply(F, F) F
}

type Add[F Number] struct{}

func (Add[F]) Apply(left, right F) F { return left + right }

type Sub[F Number] struct{}

func (Sub[F]) Apply(left, right F) F { return left - right }

type PointerAdd[F Number] struct{}

func (*PointerAdd[F]) Apply(left, right F) F { return left + right }

type hiddenAdd[F Number] struct{}

func (hiddenAdd[F]) apply(left, right F) F { return left + right }

type GenericRunner[F Number, Op Binary[F]] struct {
	operation Op
}

type GenericWrapper[Op any] struct{}

func (GenericWrapper[Op]) Apply(left, right int) int { return left + right }

type ConcreteAdd struct{}

func (ConcreteAdd) Apply(left, right float64) float64 { return left + right }

type Iterator interface {
	More() bool
	Step()
}

type Items interface {
	Values() []int
}

func elemBinary[F Number, Op Binary[F]](dst, left, right []F, operation Op) {
	for index := range dst {
		dst[index] = operation.Apply(left[index], right[index]) // want `type-parameter receiver Op calls interface-constraint method Apply on a runtime-bound range loop.*advisory, no automatic fix.*local instantiations include 2 distinct zero-size struct operation types`
	}
}

func instantiateSharedShapes(dst, left, right []float64) {
	elemBinary(dst, left, right, Add[float64]{})
	elemBinary(dst, left, right, Sub[float64]{})
	elemBinary(dst, left, right, &PointerAdd[float64]{})
}

func differentOtherShapes[F Number, Op Binary[F]](dst, left, right []F, operation Op) {
	for index := range dst {
		dst[index] = operation.Apply(left[index], right[index]) // want `advisory, no automatic fix\)$`
	}
}

func instantiateDifferentOtherShapes(dst32, left32, right32 []float32, dst64, left64, right64 []float64) {
	differentOtherShapes(dst32, left32, right32, Add[float32]{})
	differentOtherShapes(dst64, left64, right64, Sub[float64]{})
}

func countedRuntime[F Number, Op Binary[F]](dst, left, right []F, operation Op, count int) {
	for index := 0; index < count; index++ {
		dst[index] = operation.Apply(left[index], right[index]) // want `runtime or complex-bound for loop`
	}
}

func fixedFive[F Number, Op Binary[F]](dst, left, right [5]F, operation Op) {
	for index := 0; index < 5; index++ {
		dst[index] = operation.Apply(left[index], right[index]) // want `more than 4 possible executions`
	}
}

func fixedFiveRange[F Number, Op Binary[F]](dst, left, right [5]F, operation Op) {
	for index := range dst {
		dst[index] = operation.Apply(left[index], right[index]) // want `more than 4 possible executions`
	}
}

func fixedFiveIntegerRange[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for range 5 {
		result = operation.Apply(left, right) // want `more than 4 possible executions`
	}
	return result
}

func fixedFiveStringRange[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for range "ééééé" {
		result = operation.Apply(left, right) // want `more than 4 possible executions`
	}
	return result
}

func fixedFiveSliceLiteral[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for range []int{1, 2, 3, 4, 5} {
		result = operation.Apply(left, right) // want `more than 4 possible executions`
	}
	return result
}

func fixedFiveMapLiteral[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for range map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true} {
		result = operation.Apply(left, right) // want `more than 4 possible executions`
	}
	return result
}

func duplicateDynamicMapKeys[F Number, Op Binary[F]](left, right F, operation Op, key int) F {
	var result F
	for range map[int]bool{key: true, key: false, key: true, key: false, key: true} {
		result = operation.Apply(left, right)
	}
	return result
}

func duplicatePureMapKeyExpressions[F Number, Op Binary[F]](left, right F, operation Op, key int) F {
	var result F
	for range map[int]bool{key + 0: true, key + 0: false, key + 0: true, key + 0: false, key + 0: true} {
		result = operation.Apply(left, right)
	}
	return result
}

func duplicatePureMapValues[F Number, Op Binary[F]](left, right F, operation Op, key, value int) F {
	var result F
	for range map[int]int{key: value, key: value + 1, key: value + 2, key: value + 3, key: value + 4} {
		result = operation.Apply(left, right)
	}
	return result
}

func distinctPureMapKeyExpressions[F Number, Op Binary[F]](left, right F, operation Op, key int) F {
	var result F
	for range map[int]bool{key + 0: true, key + 1: true, key + 2: true, key + 3: true, key + 4: true} {
		result = operation.Apply(left, right) // want `more than 4 possible executions`
	}
	return result
}

func changingMapKey(key *int) int {
	(*key)++
	return *key
}

func duplicateKeysWithImpureMapValues[F Number, Op Binary[F]](left, right F, operation Op, key int) F {
	var result F
	for range map[int]int{
		key: changingMapKey(&key),
		key: changingMapKey(&key),
		key: changingMapKey(&key),
		key: changingMapKey(&key),
		key: changingMapKey(&key),
	} {
		result = operation.Apply(left, right) // want `more than 4 possible executions`
	}
	return result
}

func duplicateKeysWithInterveningImpureConstantEntry[F Number, Op Binary[F]](left, right F, operation Op, key int) F {
	var result F
	for range map[int]int{
		key:  0,
		1000: changingMapKey(&key),
		key:  changingMapKey(&key),
		key:  changingMapKey(&key),
		key:  changingMapKey(&key),
		key:  changingMapKey(&key),
	} {
		result = operation.Apply(left, right) // want `more than 4 possible executions`
	}
	return result
}

func duplicateImpureMapKeyExpressions[F Number, Op Binary[F]](left, right F, operation Op, key int) F {
	var result F
	for range map[int]bool{
		changingMapKey(&key): true,
		changingMapKey(&key): false,
		changingMapKey(&key): true,
		changingMapKey(&key): false,
		changingMapKey(&key): true,
	} {
		result = operation.Apply(left, right) // want `more than 4 possible executions`
	}
	return result
}

func duplicatePossiblyNaNMapKeys[F Number, Op Binary[F]](left, right F, operation Op, key float64) F {
	var result F
	for range map[float64]bool{key: true, key: false, key: true, key: false, key: true} {
		result = operation.Apply(left, right) // want `more than 4 possible executions`
	}
	return result
}

func nestedProductSix[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for outer := 0; outer < 2; outer++ {
		for inner := 0; inner < 3; inner++ {
			result = operation.Apply(left, right) // want `more than 4 possible executions`
		}
	}
	return result
}

func smallHeaderButMutated[F Number, Op Binary[F]](left, right F, operation Op, reset bool) F {
	var result F
	for index := 0; index < 4; index++ {
		result = operation.Apply(left, right) // want `runtime or complex-bound for loop`
		if reset {
			index--
		}
	}
	return result
}

func prealiasedSmallHeader[F Number, Op Binary[F]](left, right F, operation Op, reset bool) F {
	var result F
	index := 0
	pointer := &index
	for index = 0; index < 4; index++ {
		result = operation.Apply(left, right) // want `runtime or complex-bound for loop`
		if reset {
			*pointer = 0
		}
	}
	return result
}

func wrappingCounter[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for index := int8(126); index <= 127; index++ {
		result = operation.Apply(left, right) // want `more than 4 possible executions`
	}
	return result
}

func wrappingFourTimes[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for index := int8(124); index >= 124; index++ {
		result = operation.Apply(left, right)
	}
	return result
}

func wrappingDownFourTimes[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for index := int8(-125); index <= -125; index-- {
		result = operation.Apply(left, right)
	}
	return result
}

func wrappingUnsignedFourTimes[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for index := uint8(252); index >= 252; index++ {
		result = operation.Apply(left, right)
	}
	return result
}

func subtractMinInt64OneTrip[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for index := int64(0); index == 0; index -= -9223372036854775808 {
		result = operation.Apply(left, right)
	}
	return result
}

func conditionalBreak[F Number, Op Binary[F]](left, right F, operation Op, stop bool) F {
	var result F
	for {
		result = operation.Apply(left, right) // want `runtime or complex-bound for loop`
		if stop {
			break
		}
	}
	return result
}

func labeledContinue[F Number, Op Binary[F]](left, right F, operation Op, count int) F {
	var result F
outer:
	for range count {
		for {
			result = operation.Apply(left, right) // want `interface-constraint method Apply on a runtime or complex-bound for loop`
			continue outer
		}
	}
	return result
}

func gotoRepeatedInsideLoop[F Number, Op Binary[F]](left, right F, operation Op, repeat bool) F {
	var result F
	for {
	again:
		result = operation.Apply(left, right) // want `interface-constraint method Apply on a runtime or complex-bound for loop`
		if repeat {
			goto again
		}
		break
	}
	return result
}

func fixedHeaderWithInternalGoto[F Number, Op Binary[F]](left, right F, operation Op, repeat bool) F {
	var result F
	for iteration := 0; iteration < 4; iteration++ {
	again:
		result = operation.Apply(left, right) // want `interface-constraint method Apply on a runtime or complex-bound for loop`
		if repeat {
			goto again
		}
	}
	return result
}

func yielded(yield func(int) bool) {
	for index := range 8 {
		if !yield(index) {
			return
		}
	}
}

func rangeOverFunc[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for range yielded {
		result = operation.Apply(left, right) // want `interface-constraint method Apply on a runtime-bound range loop`
	}
	return result
}

func callInCondition[It Iterator](iterator It) {
	for iterator.More() { // want `type-parameter receiver It calls interface-constraint method More on a runtime or complex-bound for loop`
	}
}

func callBeforeFourTripLimit[It Iterator](iterator It) {
	for index := 0; iterator.More() && index < 4; index++ { // want `more than 4 possible executions`
	}
}

func callAfterFourTripLimit[It Iterator](iterator It) {
	for index := 0; index < 4 && iterator.More(); index++ {
	}
}

func callSkippedByTerminalDisjunction[It Iterator](iterator It) {
	for index := 0; (index >= 4 || iterator.More()) && index < 4; index++ {
	}
}

func callInPost[It Iterator](iterator It, count int) {
	for index := 0; index < count; iterator.Step() { // want `type-parameter receiver It calls interface-constraint method Step on a runtime or complex-bound for loop`
		index++
	}
}

func methodExpression[F Number, Op Binary[F]](left, right F, operation Op, count int) F {
	var result F
	for range count {
		result = Op.Apply(operation, left, right) // want `type-parameter receiver Op calls interface-constraint method Apply on a runtime-bound range loop`
	}
	return result
}

func dereferencedPointerToTypeParameter[F Number, Op Binary[F]](left, right F, operation *Op, count int) F {
	var result F
	for range count {
		result = (*operation).Apply(left, right) // want `type-parameter receiver Op calls interface-constraint method Apply on a runtime-bound range loop`
	}
	return result
}

func embeddedConstraint[F Number, Op EmbeddedBinary[F]](left, right F, operation Op, count int) F {
	var result F
	for range count {
		result = operation.Apply(left, right) // want `interface-constraint method Apply on a runtime-bound range loop`
	}
	return result
}

func aliasConstraint[F Number, Op BinaryAlias[F]](left, right F, operation Op, count int) F {
	var result F
	for range count {
		result = operation.Apply(left, right) // want `interface-constraint method Apply on a runtime-bound range loop`
	}
	return result
}

func unionConstraint[F Number, Op BinarySet[F]](left, right F, operation Op, count int) F {
	var result F
	for range count {
		result = operation.Apply(left, right) // want `interface-constraint method Apply on a runtime-bound range loop`
	}
	return result
}

func unexportedConstraintMethod[F Number, Op hiddenBinary[F]](left, right F, operation Op, count int) F {
	var result F
	for range count {
		result = operation.apply(left, right) // want `interface-constraint method apply on a runtime-bound range loop`
	}
	return result
}

func (runner GenericRunner[F, Op]) Run(dst, left, right []F) {
	for index := range dst {
		dst[index] = runner.operation.Apply(left[index], right[index]) // want `type-parameter receiver Op calls interface-constraint method Apply on a runtime-bound range loop`
	}
}

func outerRuntimeInnerSmall[F Number, Op Binary[F]](left, right F, operation Op, count int) F {
	var result F
	for range count {
		for inner := 0; inner < 4; inner++ {
			result = operation.Apply(left, right) // want `runtime-bound range loop`
		}
	}
	return result
}

func incompleteEvidenceDoesNotSuppress[F Number, Op Binary[F]](left, right F, operation Op, count int) F {
	var result F
	for range count {
		//perfscan:generic-dispatch-verified benchmark looked fine
		result = operation.Apply(left, right) // want `interface-constraint method Apply on a runtime-bound range loop`
	}
	return result
}

func compilerEvidenceSuppresses[F Number, Op Binary[F]](left, right F, operation Op, count int) F {
	var result F
	for range count {
		//perfscan:generic-dispatch-verified go1.27 darwin/arm64: go tool objdump shows a direct call and no indirect branch
		result = operation.Apply(left, right)
	}
	return result
}

func ordinaryIgnoreWithEvidenceSuppresses[F Number, Op Binary[F]](left, right F, operation Op, count int) F {
	var result F
	for range count {
		//perfscan:ignore PS6092 go1.27 linux/arm64: go tool compile -S shows a direct call and no BLR
		result = operation.Apply(left, right)
	}
	return result
}

func concreteReceiver(dst, left, right []float64, operation ConcreteAdd) {
	for index := range dst {
		dst[index] = operation.Apply(left[index], right[index])
	}
}

func interfaceReceiver[F Number](left, right F, operation Binary[F], count int) F {
	var result F
	for range count {
		result = operation.Apply(left, right)
	}
	return result
}

func outsideLoop[F Number, Op Binary[F]](left, right F, operation Op) F {
	return operation.Apply(left, right)
}

func fixedFour[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for index := 0; index < 4; index++ {
		result = operation.Apply(left, right)
	}
	return result
}

func fixedFourNotEqual[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for index := 0; index != 4; index++ {
		result = operation.Apply(left, right)
	}
	return result
}

func fixedFourDescending[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for index := 4; index > 0; index-- {
		result = operation.Apply(left, right)
	}
	return result
}

func fixedFourConjunction[F Number, Op Binary[F]](left, right F, operation Op, keepGoing func() bool) F {
	var result F
	for index := 0; index < 4 && keepGoing(); index++ {
		result = operation.Apply(left, right)
	}
	return result
}

func conditionMayMutateCounter(index *int) bool {
	*index = 0
	return true
}

func conjunctionMayMutate[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for index := 0; index < 4 && conditionMayMutateCounter(&index); index++ {
		result = operation.Apply(left, right) // want `runtime or complex-bound for loop`
	}
	return result
}

func fixedFourWithDormantMutator[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for index := 0; index < 4; index++ {
		_ = func() { index = 0 }
		result = operation.Apply(left, right)
	}
	return result
}

func fixedFourWithInvokedMutator[F Number, Op Binary[F]](left, right F, operation Op, reset bool) F {
	var result F
	for index := 0; index < 4; index++ {
		func() {
			if reset {
				index = 0
			}
		}()
		result = operation.Apply(left, right) // want `runtime or complex-bound for loop`
	}
	return result
}

func fixedFourWithNamedMutator[F Number, Op Binary[F]](left, right F, operation Op, reset bool) F {
	var result F
	for index := 0; index < 4; index++ {
		mutate := func() {
			if reset {
				index = 0
			}
		}
		mutate()
		result = operation.Apply(left, right) // want `runtime or complex-bound for loop`
	}
	return result
}

func fixedFourWithDeferredMutator[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for index := 0; index < 4; index++ {
		defer (func() { index = 0 })()
		result = operation.Apply(left, right)
	}
	return result
}

func fixedFourRange[F Number, Op Binary[F]](left, right F, operation Op, values [4]int) F {
	var result F
	for range values {
		result = operation.Apply(left, right)
	}
	return result
}

func fixedFourPointerRange[F Number, Op Binary[F]](left, right F, operation Op, values *[4]int) F {
	var result F
	for range values {
		result = operation.Apply(left, right)
	}
	return result
}

func fixedFourIntegerRange[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for range 4 {
		result = operation.Apply(left, right)
	}
	return result
}

func fixedFourStringRange[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for range "éééé" {
		result = operation.Apply(left, right)
	}
	return result
}

func fixedFourSliceLiteral[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for range []int{1, 2, 3, 4} {
		result = operation.Apply(left, right)
	}
	return result
}

func fixedFourKeyedSliceLiteral[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for range []int{3: 1} {
		result = operation.Apply(left, right)
	}
	return result
}

func fixedFiveKeyedSliceLiteral[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for range []int{4: 1} {
		result = operation.Apply(left, right) // want `more than 4 possible executions`
	}
	return result
}

func mixedKeyedSliceLiteral[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for range []int{3: 1, 1: 1, 2: 1, 4: 1} {
		result = operation.Apply(left, right) // want `more than 4 possible executions`
	}
	return result
}

func fixedFourMapLiteral[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for range map[int]bool{1: true, 2: true, 3: true, 4: true} {
		result = operation.Apply(left, right)
	}
	return result
}

func nestedProductFour[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for outer := 0; outer < 2; outer++ {
		for inner := 0; inner < 2; inner++ {
			result = operation.Apply(left, right)
		}
	}
	return result
}

func deadOuterLoop[F Number, Op Binary[F]](left, right F, operation Op, count int) F {
	var result F
	for range 0 {
		for range count {
			result = operation.Apply(left, right)
		}
	}
	return result
}

func oneShotUnbounded[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for {
		result = operation.Apply(left, right)
		break
	}
	return result
}

func oneShotBooleanHeader[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for again := true; again; again = false {
		result = operation.Apply(left, right)
	}
	return result
}

func oneShotBooleanConjunction[F Number, Op Binary[F]](left, right F, operation Op, keepGoing func() bool) F {
	var result F
	for again := true; again && keepGoing(); again = false {
		result = operation.Apply(left, right)
	}
	return result
}

func oneShotBooleanReversedConjunction[F Number, Op Binary[F]](left, right F, operation Op, keepGoing func() bool) F {
	var result F
	for again := true; keepGoing() && again; again = false {
		result = operation.Apply(left, right)
	}
	return result
}

func oneShotBooleanDisjunctionWithFalse[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for again := true; again || false; again = false {
		result = operation.Apply(left, right)
	}
	return result
}

func revivableBooleanHeader[F Number, Op Binary[F]](left, right F, operation Op, keepGoing func() bool) F {
	var result F
	for again := true; again || keepGoing(); again = false {
		result = operation.Apply(left, right) // want `runtime or complex-bound for loop`
	}
	return result
}

func shortCircuitedBooleanConditionAcrossOuter[It Iterator](iterator It) {
	for range [3]int{} {
		for again := true; again && iterator.More(); again = false {
		}
	}
}

func repeatedBooleanConditionAcrossOuter[It Iterator](iterator It) {
	for range [3]int{} {
		for again := true; iterator.More() && again; again = false { // want `more than 4 possible executions`
		}
	}
}

func panicStopsLoop[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for {
		result = operation.Apply(left, right)
		_ = result
		panic("stop")
	}
}

func oneShotFixedLarge[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for range [50]int{} {
		result = operation.Apply(left, right)
		break
	}
	return result
}

func callInLoopInitializer[F Number, Op Binary[F]](left, right F, operation Op, count int) F {
	var result F
	for result = operation.Apply(left, right); count > 0; count-- {
	}
	return result
}

func callInRangeOperand[It Items](iterator It) {
	for range iterator.Values() {
	}
}

func constantDeadBranch[F Number, Op Binary[F]](left, right F, operation Op, count int) F {
	var result F
	for range count {
		if false {
			result = operation.Apply(left, right)
		}
	}
	return result
}

func constantDeadSwitchCase[F Number, Op Binary[F]](left, right F, operation Op, count int) F {
	const disabled = false
	var result F
	for range count {
		switch {
		case disabled:
			result = operation.Apply(left, right)
		}
	}
	return result
}

func constantDeadTaggedSwitchCase[F Number, Op Binary[F]](left, right F, operation Op, count int) F {
	var result F
	for range count {
		switch 1 {
		case 2:
			result = operation.Apply(left, right)
		}
	}
	return result
}

func constantLiveSwitchCase[F Number, Op Binary[F]](left, right F, operation Op, count int) F {
	const enabled = true
	var result F
	for range count {
		switch {
		case false, enabled:
			result = operation.Apply(left, right) // want `runtime-bound range loop`
		}
	}
	return result
}

func fallthroughIntoConstantDeadSwitchCase[F Number, Op Binary[F]](left, right F, operation Op, count int) F {
	const disabled = false
	var result F
	for range count {
		switch {
		case true:
			fallthrough
		case disabled:
			result = operation.Apply(left, right) // want `runtime-bound range loop`
		}
	}
	return result
}

func labeledFallthroughIntoConstantDeadSwitchCase[F Number, Op Binary[F]](left, right F, operation Op, count int) F {
	var result F
	for range count {
		switch 1 {
		case 1:
			goto live
		live:
			fallthrough
		case 2:
			result = operation.Apply(left, right) // want `runtime-bound range loop`
		}
	}
	return result
}

func nestedLabeledFallthroughIntoConstantDeadSwitchCase[F Number, Op Binary[F]](left, right F, operation Op, count int) F {
	var result F
	for range count {
		switch 1 {
		case 1:
			if count < 0 {
				goto inner
			}
			goto outer
		outer:
		inner:
			fallthrough
		case 2:
			result = operation.Apply(left, right) // want `runtime-bound range loop`
		}
	}
	return result
}

func labeledBreakDoesNotReachConstantDeadSwitchCase[F Number, Op Binary[F]](left, right F, operation Op, count int) F {
	var result F
	for range count {
		switch 1 {
		case 1:
			goto stop
		stop:
			break
		case 2:
			result = operation.Apply(left, right)
		}
	}
	return result
}

func constantDeadShortCircuit[It Iterator](iterator It, count int) {
	for range count {
		_ = false && iterator.More()
	}
}

func capturedMethodValue[F Number, Op Binary[F]](left, right F, operation Op, count int) F {
	apply := operation.Apply
	var result F
	for range count {
		result = apply(left, right)
	}
	return result
}

func sameNamedConcreteMethod[Op any](wrapper GenericWrapper[Op], count int) int {
	var result int
	for range count {
		result = wrapper.Apply(result, 1)
	}
	return result
}

func methodValueCreatedInsideLoop[F Number, Op Binary[F]](left, right F, operation Op, count int) F {
	var result F
	for range count {
		apply := operation.Apply
		result = apply(left, right)
	}
	return result
}

func labeledOneShot[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
loop:
	for {
		result = operation.Apply(left, right)
		break loop
	}
	return result
}

func gotoSkipsCall[F Number, Op Binary[F]](left, right F, operation Op, count int) F {
	var result F
	for range count {
		goto after
		result = operation.Apply(left, right)
	after:
	}
	return result
}

func nonInvokedClosure[F Number, Op Binary[F]](left, right F, operation Op, count int) {
	for range count {
		_ = func() F { return operation.Apply(left, right) }
	}
}

func discardedClosureWithOwnLoop[F Number, Op Binary[F]](left, right F, operation Op, count int) {
	_ = func() F {
		var result F
		for range count {
			result = operation.Apply(left, right)
		}
		return result
	}
}

func blankAssignedClosureWithOwnLoop[F Number, Op Binary[F]](left, right F, operation Op, count int) {
	var keep int
	_, keep = func() F {
		var result F
		for range count {
			result = operation.Apply(left, right)
		}
		return result
	}, 1
	_ = keep
}

func keptAssignedClosureWithOwnLoop[F Number, Op Binary[F]](left, right F, operation Op, count int) {
	var keep func() F
	keep, _ = func() F {
		var result F
		for range count {
			result = operation.Apply(left, right) // want `runtime-bound range loop`
		}
		return result
	}, 1
	_ = keep
}

func blankConvertedAssignedClosureWithOwnLoop[F Number, Op Binary[F]](left, right F, operation Op, count int) {
	var keep int
	_, keep = Closure[F](func() F {
		var result F
		for range count {
			result = operation.Apply(left, right)
		}
		return result
	}), 1
	_ = keep
}

func keptConvertedAssignedClosureWithOwnLoop[F Number, Op Binary[F]](left, right F, operation Op, count int) {
	var keep Closure[F]
	keep, _ = Closure[F](func() F {
		var result F
		for range count {
			result = operation.Apply(left, right) // want `runtime-bound range loop`
		}
		return result
	}), 1
	_ = keep
}

func blankConvertedDeclaredClosureWithOwnLoop[F Number, Op Binary[F]](left, right F, operation Op, count int) {
	var _ = Closure[F](func() F {
		var result F
		for range count {
			result = operation.Apply(left, right)
		}
		return result
	})
}

func keptConvertedDeclaredClosureWithOwnLoop[F Number, Op Binary[F]](left, right F, operation Op, count int) {
	var keep = Closure[F](func() F {
		var result F
		for range count {
			result = operation.Apply(left, right) // want `runtime-bound range loop`
		}
		return result
	})
	_ = keep
}

func blankInterfaceConvertedClosureWithOwnLoop[F Number, Op Binary[F]](left, right F, operation Op, count int) {
	var _ = any(func() F {
		var result F
		for range count {
			result = operation.Apply(left, right)
		}
		return result
	})
}

func keptInterfaceConvertedClosureWithOwnLoop[F Number, Op Binary[F]](left, right F, operation Op, count int) {
	var keep = any(func() F {
		var result F
		for range count {
			result = operation.Apply(left, right) // want `runtime-bound range loop`
		}
		return result
	})
	_ = keep
}

func blankDeclaredClosureWithOwnLoop[F Number, Op Binary[F]](left, right F, operation Op, count int) {
	var _ = func() F {
		var result F
		for range count {
			result = operation.Apply(left, right)
		}
		return result
	}
}

func closureInsideDeadBranch[F Number, Op Binary[F]](left, right F, operation Op, count int) func() F {
	if false {
		return func() F {
			var result F
			for range count {
				result = operation.Apply(left, right)
			}
			return result
		}
	}
	return nil
}

func closureInsideZeroTripLoop[F Number, Op Binary[F]](left, right F, operation Op, count int) func() F {
	var closure func() F
	for range 0 {
		closure = func() F {
			var result F
			for range count {
				result = operation.Apply(left, right)
			}
			return result
		}
	}
	return closure
}

func immediatelyInvokedClosure[F Number, Op Binary[F]](left, right F, operation Op, count int) F {
	var result F
	for range count {
		result = func() F {
			return operation.Apply(left, right) // want `interface-constraint method Apply on a runtime-bound range loop`
		}()
	}
	return result
}

func deadCallInsideImmediatelyInvokedClosure[F Number, Op Binary[F]](left, right F, operation Op, count int) F {
	var result F
	for range count {
		func() {
			return
			result = operation.Apply(left, right)
		}()
	}
	return result
}

func invokedClosureNestedProductSix[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for outer := 0; outer < 2; outer++ {
		func() {
			for inner := 0; inner < 3; inner++ {
				result = operation.Apply(left, right) // want `more than 4 possible executions`
			}
		}()
	}
	return result
}

func invokedClosureNestedProductFour[F Number, Op Binary[F]](left, right F, operation Op) F {
	var result F
	for outer := 0; outer < 2; outer++ {
		func() {
			for inner := 0; inner < 2; inner++ {
				result = operation.Apply(left, right)
			}
		}()
	}
	return result
}

func invokedClosureInsideDeadOuter[F Number, Op Binary[F]](left, right F, operation Op, count int) F {
	var result F
	for range 0 {
		func() {
			for range count {
				result = operation.Apply(left, right)
			}
		}()
	}
	return result
}

func closureWithOwnLoop[F Number, Op Binary[F]](left, right F, operation Op, count int) func() F {
	return func() F {
		var result F
		for range count {
			result = operation.Apply(left, right) // want `interface-constraint method Apply on a runtime-bound range loop`
		}
		return result
	}
}

func unreachableClosureConstruction[F Number, Op Binary[F]](left, right F, operation Op, count int) func() F {
	return nil
	return func() F {
		var result F
		for range count {
			result = operation.Apply(left, right)
		}
		return result
	}
}
