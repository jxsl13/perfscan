package ps6095

import . "ps6095state"

var packageWeight = 2.0

func exactQuotient(numerator, denominator float64) float64 {
	return numerator / denominator
}

func exactReciprocal(denominator float64) float64 {
	return 1 / denominator
}

var helperCalls int

func impureQuotient(numerator, denominator float64) float64 {
	helperCalls++
	return numerator / denominator
}

func observe() {}

func mutate(values []float64) {
	if len(values) != 0 {
		values[0]++
	}
}

func scalarOutput(output, gradient []float64, weight, denominator float64) {
	for d := range output {
		output[d] += gradient[d] * (weight / denominator) // want `this floating-point quotient is invariant in output index d; compute the original division once and reuse its rounded result \(never replace it with reciprocal multiplication, which is not bit-equivalent\)`
	}
}

func countedDot(input []float64, weight, denominator float64) float64 {
	var sum float64
	for d := 0; d < len(input); d++ {
		sum += input[d] * (weight / denominator) // want `this floating-point quotient is invariant in output index d; compute the original division once and reuse its rounded result \(never replace it with reciprocal multiplication, which is not bit-equivalent\)`
	}
	return sum
}

func helperOutput(output, gradient []float64, weight, denominator float64) {
	for d := range output {
		output[d] = gradient[d] * exactQuotient(weight, denominator) // want `this floating-point quotient is invariant in output index d; compute the original division once and reuse its rounded result \(never replace it with reciprocal multiplication, which is not bit-equivalent\)`
	}
}

// An unrelated call cannot mutate unaddressed scalar inputs.
func scalarWithCall(output []float64, weight, denominator float64) {
	for d := range output {
		observe()
		output[d] = weight / denominator // want `this floating-point quotient is invariant in output index d; compute the original division once and reuse its rounded result \(never replace it with reciprocal multiplication, which is not bit-equivalent\)`
	}
}

// The output is fresh and therefore cannot alias either input slice. This is
// a sound storage-backed finding, but receives no fix because hoisting an index
// can move a bounds panic onto a zero-trip path.
func freshOutput(weights, denominators, gradient []float64, token int) []float64 {
	output := make([]float64, len(gradient))
	for d := range output {
		output[d] = gradient[d] * (weights[token] / denominators[token]) // want `this floating-point quotient is invariant in output index d; compute the original division once and reuse its rounded result \(never replace it with reciprocal multiplication, which is not bit-equivalent\)`
	}
	return output
}

// Negative: numerator depends on the output index.
func outputDependent(output, weights []float64, denominator float64) {
	for d := range output {
		output[d] = weights[d] / denominator
	}
}

// Negative: the scalar numerator changes between iterations.
func scalarWrite(output []float64, weight, denominator float64) {
	for d := range output {
		weight++
		output[d] = weight / denominator
	}
}

// Negative: taking the address makes mutation through a call possible.
func addressedScalar(output []float64, weight, denominator float64) {
	pointer := &weight
	for d := range output {
		*pointer += float64(d)
		output[d] = weight / denominator
	}
}

// Negative: a closure captures the otherwise scalar input.
func capturedScalar(output []float64, weight, denominator float64) {
	read := func() float64 { return weight }
	for d := range output {
		output[d] = read() + weight/denominator
	}
}

// Negative: package state can change outside the analyzed function.
func packageInput(output []float64, denominator float64) {
	for d := range output {
		output[d] = packageWeight / denominator
	}
}

// Negative: output and weights are slice parameters and can alias.
func possibleAlias(output, weights []float64, token int, denominator float64) {
	for d := range output {
		output[d] = weights[token] / denominator
	}
}

// Negative: an explicit write reaches numerator storage.
func storageWrite(weights []float64, token int, denominator float64) []float64 {
	output := make([]float64, 16)
	for d := range output {
		weights[token]++
		output[d] = weights[token] / denominator
	}
	return output
}

// Negative: an unknown call can mutate storage or an alias of it.
func storageCall(weights []float64, token int, denominator float64) []float64 {
	output := make([]float64, 16)
	for d := range output {
		mutate(weights)
		output[d] = weights[token] / denominator
	}
	return output
}

// Negative: a local alias store can reach numerator storage.
func aliasStore(weights []float64, token int, denominator float64) []float64 {
	output := make([]float64, 16)
	alias := weights
	for d := range output {
		alias[token]++
		output[d] = weights[token] / denominator
	}
	return output
}

// Negative: the helper has a side effect, so no cross-function purity summary
// is inferred.
func impureHelper(output []float64, weight, denominator float64) {
	for d := range output {
		output[d] = impureQuotient(weight, denominator)
	}
}

// Negative: this is already reciprocal multiplication. PS6095 must never
// present it as the exact replacement for weight/denominator.
func reciprocalMultiply(output []float64, weight, denominator float64) {
	for d := range output {
		output[d] = float64(d+1) * weight * (1 / denominator)
	}
}

// Negative: the same exclusion applies through a summarized reciprocal
// helper.
func reciprocalHelper(output []float64, weight, denominator float64) {
	for d := range output {
		output[d] = float64(d+1) * weight * exactReciprocal(denominator)
	}
}

// Negative: transparent conversions and unary operators do not disguise an
// existing reciprocal multiplication as an exact quotient-hoisting target.
func wrappedReciprocalMultiply(output []float64, denominator float64) {
	for d := range output {
		output[d] = float64(d+1) * float64(-(1 / denominator))
	}
}

// Negative: a fixed trip count below the material threshold.
func tinyLoop(output *[3]float64, weight, denominator float64) {
	for d := 0; d < 3; d++ {
		output[d] = weight / denominator
	}
}

// Negative: the quotient does not feed an output or dot-product update.
func unrelatedLoop(weight, denominator float64) {
	for d := range 16 {
		_ = d
		_ = weight / denominator
	}
}

// Negative: the quotient is unreachable after a terminating panic call.
func deadAfterPanic(output []float64, weight, denominator float64) {
	for d := range output {
		panic("stop")
		output[d] = weight / denominator
	}
}

// Negative: a quotient guarded by the output index does not run for every
// output element.
func conditionalUse(output []float64, weight, denominator float64) {
	for d := range output {
		if d&1 == 0 {
			output[d] = weight / denominator
		}
	}
}

// Negative: a loop exit means the apparent dynamic domain is not evidence that
// the quotient repeats materially.
func earlyBreak(output []float64, weight, denominator float64) {
	for d := range output {
		output[d] = weight / denominator
		break
	}
}

// Negative: hoisting would move a possible negative-shift panic onto a
// zero-trip path.
func shiftingOperand(output []float64, numerator int, shift int, denominator float64) {
	for d := range output {
		output[d] = float64(numerator<<shift) / denominator
	}
}

// Negative: an unrelated indexed LHS in a multi-assignment is not evidence
// that the quotient itself feeds an output.
func unrelatedMulti(output []float64, weight, denominator float64) {
	var quotient float64
	for d := range output {
		output[d], quotient = 0, weight/denominator
	}
	_ = quotient
}

type namedFloat32 float32

// Negative: a constant quotient is folded by the compiler. Hoisting it would
// also lose its assignment-context conversion and can make valid code fail to
// compile for float32 or a named floating type.
func constantQuotient(output []namedFloat32) {
	for d := range output {
		output[d] = 1.0 / 3.0
	}
}

func bumpAndLen(values []float64) int {
	values[0]++
	return 16
}

// Negative: the counted-loop condition runs on every iteration and can mutate
// operand storage.
func conditionCall(weights []float64, denominator float64) []float64 {
	output := make([]float64, 16)
	for d := 0; d < bumpAndLen(weights); d++ {
		output[d] = weights[0] / denominator
	}
	return output
}

// Negative: a non-returning call after the candidate prevents a backedge, so
// the quotient executes at most once.
func deadBackedge(output []float64, weight, denominator float64) {
	for d := range output {
		output[d] = weight / denominator
		panic("stop")
	}
}

func boolToFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

// Negative: the right side of a short-circuit expression is conditional.
func shortCircuit(output []float64, enabled bool, weight, denominator float64) {
	for d := range output {
		output[d] = boolToFloat(enabled && weight/denominator > 0)
	}
}

// Negative: writing the induction object defeats the material loop domain.
func indexWrite(output []float64, weight, denominator float64) {
	for d := 0; d < len(output); d++ {
		output[d] = weight / denominator
		d = len(output)
	}
}

type mutableFloat float64

func (value *mutableFloat) increment() {
	(*value)++
}

// Negative: a pointer-receiver call implicitly takes and mutates the address
// of the scalar quotient input.
func pointerReceiver(output []float64, weight mutableFloat, denominator float64) {
	for d := range output {
		weight.increment()
		output[d] = float64(weight) / denominator
	}
}

// Negative: the implicit address can escape through a method value before the
// loop and be mutated by an otherwise opaque call.
func pointerMethodValue(output []float64, weight mutableFloat, denominator float64) {
	increment := weight.increment
	for d := range output {
		increment()
		output[d] = float64(weight) / denominator
	}
}

func shiftHelper(numerator, shift int, denominator float64) float64 {
	return float64(numerator<<shift) / denominator
}

// Negative: a helper containing a potentially panicking shift receives no
// purity summary and cannot be hoisted onto a zero-trip path.
func helperShift(output []float64, shift int, denominator float64) {
	for d := range output {
		output[d] = shiftHelper(1, shift, denominator)
	}
}

// Report remains useful, but an automatic declaration is unsafe in a function
// containing goto because it may turn a legal jump into one over a declaration.
func gotoScope(output []float64, weight, denominator float64) {
	if len(output) == 0 {
		goto done
	}
	for d := range output {
		output[d] = weight / denominator // want `this floating-point quotient is invariant in output index d; compute the original division once and reuse its rounded result \(never replace it with reciprocal multiplication, which is not bit-equivalent\)`
	}
done:
}

// A source comment inside the quotient must survive; keep the report advisory.
func commentedQuotient(output []float64, weight, denominator float64) {
	for d := range output {
		output[d] = (weight /* preserve exactness rationale */ / denominator) // want `this floating-point quotient is invariant in output index d; compute the original division once and reuse its rounded result \(never replace it with reciprocal multiplication, which is not bit-equivalent\)`
	}
}

// Negative: a dot-imported package variable is still mutable global state.
func dotImportedGlobal(output []float64, denominator float64) {
	for d := range output {
		Bump()
		output[d] = Weight / denominator
	}
}

// The scalar candidate is preferred and fixed even when a storage-backed
// advisory candidate appears first in the same loop.
func mixedCandidates(weights []float64, token int, weight, denominator1, denominator2 float64) []float64 {
	output := make([]float64, 16)
	for d := range output {
		output[d] = weights[token]/denominator1 + weight/denominator2 // want `this floating-point quotient is invariant in output index d; compute the original division once and reuse its rounded result \(never replace it with reciprocal multiplication, which is not bit-equivalent\)`
	}
	return output
}

// Multiple scalar candidates are bundled into one fix so their insertion
// edits cannot overlap.
func bundledScalarCandidates(output, gradient []float64, numerator1, denominator1, numerator2, denominator2 float64) {
	for d := range output {
		output[d] = gradient[d]*(numerator1/denominator1) + numerator2/denominator2 // want `this floating-point quotient is invariant in output index d; compute the original division once and reuse its rounded result \(never replace it with reciprocal multiplication, which is not bit-equivalent\)`
	}
}

// Negative: a nested loop that advances the outer induction object makes the
// quotient execute only once despite the material-looking outer bound.
func nestedIndexWrite(output []float64, weight, denominator float64) {
	for index := 0; index < len(output); index++ {
		output[index] = weight / denominator
		for index+1 < len(output) {
			index++
		}
	}
}

func mutateIndex(index *int, bound int) {
	*index = bound
}

// Negative: address escape lets an opaque call advance the induction object,
// so neither a diagnostic nor an automatic hoist is sound.
func escapedIndexMutation(output []float64, weight, denominator float64) {
	for index := 0; index < len(output); index++ {
		output[index] = weight / denominator
		mutateIndex(&index, len(output))
	}
}

// Function literals have their own CFG and lexical fix scope. Captured scalar
// values remain eligible when they are stable within the literal.
func literalKernel(output []float64, numerator, denominator float64) func() {
	return func() {
		for index := range output {
			output[index] = numerator / denominator // want `this floating-point quotient is invariant in output index index; compute the original division once and reuse its rounded result \(never replace it with reciprocal multiplication, which is not bit-equivalent\)`
		}
	}
}

// Negative: the returned literal shares numerator with a sibling mutator.
// Mutations in enclosing/sibling function scopes invalidate its cache.
func siblingClosureMutation(output []float64, numerator, denominator float64) func() {
	mutate := func() { numerator++ }
	mutate()
	return func() {
		for index := range output {
			output[index] = numerator / denominator
		}
	}
}

// Negative: changing a counted-loop bound after the quotient makes the loop
// execute once despite its initially material domain.
func countedBoundWrite(output []float64, numerator, denominator float64) {
	bound := len(output)
	for index := 0; index < bound; index++ {
		output[index] = numerator / denominator
		bound = 0
	}
}

// Negative: an escaped counted-loop bound can be changed by an opaque call.
func countedBoundEscape(output []float64, numerator, denominator float64) {
	bound := len(output)
	for index := 0; index < bound; index++ {
		output[index] = numerator / denominator
		mutateIndex(&bound, 0)
	}
}
