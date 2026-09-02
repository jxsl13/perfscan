package ps6099

import (
	m "math"
)

func ExpSIMDF64(dst []float64) // known external assembly leaf

func LogSIMDF32(dst []float32) // wrong precision and operation for F64 cases

func PowSIMDF64(dst []float64, exponent float64) // known batched variable-power leaf

func wrappedExp[T ~float64](value T) float64 {
	return m.Exp(float64(value))
}

func forwardedExp[T ~float64](value T) float64 {
	return wrappedExp(value)
}

var globalInput float64

func ignoresInput(_ float64) float64 {
	return m.Exp(globalInput)
}

func rbf(output []float64, features [][20]float64, query [20]float64, gamma float64) {
	for row := range features { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64.*measure a two-pass candidate.*iterative solver, tolerance is not an acceptance gate`
		var squaredDistance float64
		for column := range features[row] {
			delta := features[row][column] - query[column]
			squaredDistance += delta * delta
		}
		output[row] = m.Exp(-gamma * squaredDistance)
	}
}

func maybeZeroRangeDependency(output []float64, rows [][]float64, scalar float64) {
	for index := range output {
		value := scalar
		for _, candidate := range rows[index] {
			value = candidate
		}
		output[index] = m.Exp(value)
	}
}

func zeroLengthArrayRangeDependency(output []float64, rows [][0]float64, scalar float64) {
	for index := range output {
		value := scalar
		for _, candidate := range rows[index] {
			value = candidate
		}
		output[index] = m.Exp(value)
	}
}

func guaranteedArrayRangeDependency(output []float64, rows [][1]float64, scalar float64) {
	for index := range output { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		value := scalar
		for _, candidate := range rows[index] {
			value = candidate
		}
		output[index] = m.Exp(value)
	}
}

func maybeZeroForDependency(output []float64, rows [][]float64, scalar float64) {
	for index := range output {
		value := scalar
		for column := 0; column < len(rows[index]); column++ {
			value = rows[index][column]
		}
		output[index] = m.Exp(value)
	}
}

func guaranteedForDependency(output []float64, rows [][1]float64, scalar float64) {
	for index := range output { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		value := scalar
		for column := 0; column < 1; column++ {
			value = rows[index][column]
		}
		output[index] = m.Exp(value)
	}
}

func exactTwoRangeDoesNotUseOneIterationSummary(output, input []float64, scalar float64) {
	for index := range output {
		value := scalar
		dependent := input[index]
		for range [2]int{} {
			value = dependent
			dependent = scalar
		}
		output[index] = m.Exp(value)
	}
}

func exactTwoForDoesNotUseOneIterationSummary(output, input []float64, scalar float64) {
	for index := range output {
		value := scalar
		dependent := input[index]
		for iteration := 0; iteration < 2; iteration++ {
			value = dependent
			dependent = scalar
		}
		output[index] = m.Exp(value)
	}
}

func exactOneRangeRetainsDependency(output, input []float64, scalar float64) {
	for index := range output { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		value := scalar
		dependent := input[index]
		for range [1]int{} {
			value = dependent
			dependent = scalar
		}
		output[index] = m.Exp(value)
	}
}

func exactOneForRetainsDependency(output, input []float64, scalar float64) {
	for index := range output { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		value := scalar
		dependent := input[index]
		for iteration := 0; iteration < 1; iteration++ {
			value = dependent
			dependent = scalar
		}
		output[index] = m.Exp(value)
	}
}

func millionIterationFixedPoint(output, input []float64, scalar float64) {
	for index := range output { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		value := scalar
		dependent := input[index]
		for range 1_000_000 {
			value = dependent
		}
		output[index] = m.Exp(value)
	}
}

func maybeZeroBranchDependency(output []float64, rows [][]float64, scalar float64, clamp bool) {
	for index := range output {
		value := scalar
		for _, candidate := range rows[index] {
			if clamp {
				value = candidate
			} else {
				value = rows[index][0]
			}
		}
		output[index] = m.Exp(value)
	}
}

func maybeZeroPreservesDependentIncoming(output []float64, rows [][]float64) {
	for index := range output { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		value := rows[index][0]
		for _, candidate := range rows[index] {
			value = candidate
		}
		output[index] = m.Exp(value)
	}
}

func maybeZeroKillsDependentIncoming(output []float64, rows [][]float64, scalar float64) {
	for index := range output {
		value := rows[index][0]
		for range rows[index] {
			value = scalar
		}
		output[index] = m.Exp(value)
	}
}

func guaranteedBranchDependency(output []float64, left, right [][1]float64, choose bool) {
	for index := range output { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		value := 0.0
		for range left[index] {
			if choose {
				value = left[index][0]
			} else {
				value = right[index][0]
			}
		}
		output[index] = m.Exp(value)
	}
}

func guaranteedPointerArrayRange(output []float64, rows []*[1]float64, scalar float64) {
	for index := range output { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		value := scalar
		for _, candidate := range rows[index] {
			value = candidate
		}
		output[index] = m.Exp(value)
	}
}

func wrapped(output, input []float64) {
	for index := 0; index < len(input); index++ { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		output[index] = forwardedExp(-input[index])
	}
}

func rangeValue(output, input []float64) {
	for index, value := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		output[index] = m.Exp(value)
	}
}

func variablePower(output, input []float64, exponent float64) {
	for index := range input { // want `loop calls scalar math.Pow exactly once per independent output element.*PowSIMDF64`
		output[index] = m.Pow(input[index], exponent)
	}
}

func varyingPower(output, base, exponents []float64) {
	for index := range base {
		output[index] = m.Pow(base[index], exponents[index])
	}
}

func exponentOnlyVaries(output, exponents []float64, base float64) {
	for index := range exponents {
		output[index] = m.Pow(base, exponents[index])
	}
}

func sideEffectExponent(output, input []float64, nextExponent func() float64) {
	for index := range input {
		output[index] = m.Pow(input[index], nextExponent())
	}
}

func updatedExponent(output, input []float64, exponent float64) {
	for index := range input {
		exponent++
		output[index] = m.Pow(input[index], exponent)
	}
}

func wrappedPower(value, exponent float64) float64 {
	return m.Pow(value, exponent)
}

func noWrappedPower(output, input []float64, exponent float64) {
	for index := range input {
		output[index] = wrappedPower(input[index], exponent)
	}
}

func cheapPowerHandledElsewhere(output, input []float64) {
	for index := range input {
		output[index] = m.Pow(input[index], 2)
	}
}

func freePowerZero(output, input []float64) {
	for index := range input {
		output[index] = m.Pow(input[index], 0)
	}
}

func freePowerOne(output, input []float64) {
	for index := range input {
		output[index] = m.Pow(input[index], 1)
	}
}

func expensiveFractionalPower(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Pow exactly once per independent output element.*PowSIMDF64`
		output[index] = m.Pow(input[index], 1.5)
	}
}

func expensiveLargeIntegralPower(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Pow exactly once per independent output element.*PowSIMDF64`
		output[index] = m.Pow(input[index], 9)
	}
}

func conditional(output, input []float64) {
	for index := range input {
		if input[index] > 0 {
			output[index] = m.Exp(input[index])
		}
	}
}

func twoCalls(output, input []float64) {
	for index := range input {
		_ = m.Exp(-input[index])
		output[index] = m.Exp(input[index])
	}
}

func readsOutput(output, input []float64) {
	for index := range input {
		output[index] = m.Exp(input[index] + output[index])
	}
}

func loopLocalDestination(input []float64) {
	for index := range input {
		output := make([]float64, len(input))
		output[index] = m.Exp(input[index])
	}
}

func invariantCall(output, input []float64, scalar float64) {
	for index := range input {
		output[index] = m.Exp(scalar)
	}
}

func wrapperIgnoresIteration(output, input []float64) {
	for index := range input {
		output[index] = ignoresInput(input[index])
	}
}

func skippedByContinue(output, input []float64) {
	for index := range input {
		if input[index] < 0 {
			continue
		}
		output[index] = m.Exp(input[index])
	}
}

func noncanonical(output, input []float64) {
	for index := 1; index < len(input); index++ {
		output[index] = m.Exp(input[index])
	}
}

func logWrongPrecision(output, input []float64) {
	for index := range input {
		output[index] = m.Log(input[index])
	}
}

func overwrittenDependency(output, input []float64, scalar float64) {
	for index := range input {
		value := input[index]
		value = scalar
		output[index] = m.Exp(value)
	}
}

func conditionalDependencyKill(output, input []float64, scalar float64, clamp bool) {
	for index := range input {
		value := input[index]
		if clamp {
			value = scalar
		}
		output[index] = m.Exp(value)
	}
}

func straightLineDependentOverwrite(output, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		value := 1.0
		value = input[index]
		output[index] = m.Exp(value)
	}
}

func bothBranchesDependent(output, left, right []float64, choose bool) {
	for index := range output { // want `loop calls scalar math.Exp exactly once per independent output element.*ExpSIMDF64`
		var value float64
		if choose {
			value = left[index]
		} else {
			value = right[index]
		}
		output[index] = m.Exp(value)
	}
}

func indirectDependencyKill(output, input []float64, scalar float64) {
	for index := range input {
		value := input[index]
		valuePointer := &value
		*valuePointer = scalar
		output[index] = m.Exp(value)
	}
}

func changedCountingIndex(output, input []float64) {
	for index := 0; index < len(input); index++ {
		index++
		output[index] = m.Exp(input[index])
	}
}

func changedRangeIndex(output, input []float64) {
	for index := range input {
		index++
		output[index] = m.Exp(input[index])
	}
}

func aliasedCountingIndex(output, input []float64) {
	for index := 0; index < len(input); index++ {
		indexPointer := &index
		(*indexPointer)++
		output[index] = m.Exp(input[index])
	}
}

func preAliasedCountingIndex(output, input []float64) {
	var index int
	indexPointer := &index
	for index = 0; index < len(input); index++ {
		(*indexPointer)++
		output[index] = m.Exp(input[index])
	}
}

func escapedCountingIndex(output, input []float64, mutate func(*int)) {
	for index := 0; index < len(input); index++ {
		mutate(&index)
		output[index] = m.Exp(input[index])
	}
}

func aliasedRangeIndex(output, input []float64) {
	for index := range input {
		indexPointer := &index
		*indexPointer += 1
		output[index] = m.Exp(input[index])
	}
}

func aliasedRangeValue(output, input []float64) {
	for index, value := range input {
		valuePointer := &value
		*valuePointer = input[index+1]
		output[index] = m.Exp(value)
	}
}

type outputState struct {
	values []float64
}

func stableDestinationRoot(current *outputState, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent current.values element.*ExpSIMDF64`
		current.values[index] = m.Exp(input[index])
	}
}

func unusedPreLoopAlias(current *outputState, input []float64) {
	alias := current
	_ = alias
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent current.values element.*ExpSIMDF64`
		current.values[index] = m.Exp(input[index])
	}
}

func preLoopAliasRead(current *outputState, input []float64) {
	alias := current
	for index := range input {
		_ = alias.values[index]
		current.values[index] = m.Exp(input[index])
	}
}

func preLoopAliasChainRebind(current *outputState, input []float64) {
	alias := current
	alias2 := alias
	for index := range input {
		alias2.values = input
		current.values[index] = m.Exp(input[index])
	}
}

type outputAliasHolder struct {
	state *outputState
}

func preLoopStructAliasMutation(current *outputState, input []float64) {
	holder := outputAliasHolder{state: current}
	for index := range input {
		holder.state.values = input
		current.values[index] = m.Exp(input[index])
	}
}

var escapedOutputAlias *outputState

func preLoopGlobalAliasEscape(current *outputState, input []float64) {
	escapedOutputAlias = current
	for index := range input {
		current.values[index] = m.Exp(input[index])
	}
}

func identityAlias[T any](value T) T { return value }

func preLoopGenericWrapperEscape(current *outputState, input []float64) {
	alias := identityAlias(current)
	_ = alias
	for index := range input {
		current.values[index] = m.Exp(input[index])
	}
}

func preLoopAliasCapture(current *outputState, input []float64) {
	alias := current
	for index := range input {
		capture := func() { alias.values = nil }
		_ = capture
		current.values[index] = m.Exp(input[index])
	}
}

func preLoopAliasOpaqueUse(current *outputState, input []float64, mutate func(*outputState)) {
	alias := current
	for index := range input {
		mutate(alias)
		current.values[index] = m.Exp(input[index])
	}
}

func preLoopAliasAddress(current *outputState, input []float64) {
	alias := current
	for index := range input {
		aliasAddress := &alias
		_ = aliasAddress
		current.values[index] = m.Exp(input[index])
	}
}

func preLoopDestinationEscape(current *outputState, input []float64, register func(*outputState)) {
	register(current)
	for index := range input {
		current.values[index] = m.Exp(input[index])
	}
}

func unrelatedPreLoopAlias(current, unrelated *outputState, input []float64) {
	alias := unrelated
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent current.values element.*ExpSIMDF64`
		_ = alias.values[index]
		current.values[index] = m.Exp(input[index])
	}
}

func canonicalWriteThroughAlias(output, input []float64) {
	alias := output
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent alias element.*ExpSIMDF64`
		alias[index] = m.Exp(input[index])
	}
}

func sourceAliasReadWhileWritingAlias(output, input []float64) {
	alias := output
	for index := range input {
		_ = output[index]
		alias[index] = m.Exp(input[index])
	}
}

func changedDestinationRoot(current *outputState, states []*outputState, input []float64) {
	for index := range input {
		current = states[index]
		current.values[index] = m.Exp(input[index])
	}
}

type nestedOutputState struct {
	inner outputState
}

func stableDestinationPrefix(current *nestedOutputState, input []float64) {
	for index := range input { // want `loop calls scalar math.Exp exactly once per independent current.inner.values element.*ExpSIMDF64`
		current.inner.values[index] = m.Exp(input[index])
	}
}

func changedDestinationPrefix(current *nestedOutputState, states []outputState, input []float64) {
	for index := range input {
		current.inner = states[index]
		current.inner.values[index] = m.Exp(input[index])
	}
}

func escapedDestinationRoot(current *outputState, input []float64, mutate func(*outputState)) {
	for index := range input {
		mutate(current)
		current.values[index] = m.Exp(input[index])
	}
}

func opaqueCallWithDestinationAlias(current *outputState, input []float64, mutate func()) {
	for index := range input {
		mutate()
		current.values[index] = m.Exp(input[index])
	}
}

func addressedDestinationRoot(current *outputState, input []float64, mutate func(**outputState)) {
	for index := range input {
		mutate(&current)
		current.values[index] = m.Exp(input[index])
	}
}

func capturedDestinationRoot(current *outputState, input []float64) {
	for index := range input {
		capture := func() { current.values = nil }
		_ = capture
		current.values[index] = m.Exp(input[index])
	}
}

func escapedDestinationMethod(current *outputState, input []float64) {
	for index := range input {
		current.mutate()
		current.values[index] = m.Exp(input[index])
	}
}

func (current *outputState) mutate() { current.values = nil }

func rebindingIntermediate(current *nestedOutputState, input []float64) {
	for index := range input {
		current.inner = outputState{values: input}
		current.inner.values[index] = m.Exp(input[index])
	}
}
