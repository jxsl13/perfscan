package ps6010

import "sync"

func parallelForRows(n int, fn func(int)) {
	var wg sync.WaitGroup
	for row := 0; row < n; row++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn(row)
		}()
	}
	wg.Wait()
}

func matvec(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o]
		}
		dst[o] = acc
	}
}

// Bias-seeded accumulator: still reported, but the seed is not the canonical
// 0.0 literal, so no automatic rewrite is offered.
func matvecBias(dst [64]float64, a, w, bias []float64, out, n int) {
	for o := 0; o < out; o++ {
		acc := bias[o]
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
}

// Reordered weight index: still reported, but the index is not the canonical
// i*out+o shape, so no automatic rewrite is offered.
func matvecSwapped(dst [64]float64, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[o+i*out] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
}

// Both factors vary with the output index: nothing is reloaded invariant.
func hadamard(dst, a, b []float64, n int) {
	for o := 0; o < n; o++ {
		acc := 0.0
		for i := 0; i < 1; i++ {
			acc += a[o] * b[o]
		}
		dst[o] = acc
	}
}

// No enclosing output loop: a single dot product has nothing to amortize
// across.
func dot(a, b []float64) float64 {
	acc := 0.0
	for i := range a {
		acc += a[i] * b[i]
	}
	return acc
}

// Production-shaped baseline from issue #911. Each callback exclusively owns
// one tmp row, but the scalar j loop re-streams the corresponding inner row.
func rowOwnedMatrixProductBefore(inner, v []float64, n int) {
	tmp := make([]float64, n*n)
	parallelForRows(n, func(a int) {
		row := a * n
		for j := 0; j < n; j++ {
			sum := 0.0
			for b := 0; b < n; b++ {
				sum += inner[row+b] * v[j*n+b] // want `this operand does not vary with the output index j but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
			}
			tmp[row+j] = sum
		}
	})
}

// The successful issue #911 shape: four adjacent output columns share one
// inner-row load, every accumulator still reduces b in ascending order, and
// the original scalar body handles the n%4 tail. PS6010 must stay silent.
func rowOwnedMatrixProductTiled(tmp, inner, v []float64, n int) {
	parallelForRows(n, func(a int) {
		row := a * n
		j := 0
		for ; j+3 < n; j += 4 {
			var s0, s1, s2, s3 float64
			for b := 0; b < n; b++ {
				innerAB := inner[row+b]
				s0 += innerAB * v[j*n+b]
				s1 += innerAB * v[(j+1)*n+b]
				s2 += innerAB * v[(j+2)*n+b]
				s3 += innerAB * v[(j+3)*n+b]
			}
			tmp[row+j] = s0
			tmp[row+j+1] = s1
			tmp[row+j+2] = s2
			tmp[row+j+3] = s3
		}
		for ; j < n; j++ {
			sum := 0.0
			for b := 0; b < n; b++ {
				sum += inner[row+b] * v[j*n+b]
			}
			tmp[row+j] = sum
		}
	})
}

// Four accumulators alone are not the remedy when each one reloads the
// supposedly shared inner-row operand. Keep reporting this near miss.
func rowOwnedMatrixProductReloadedTile(inner, v []float64, n int) {
	tmp := make([]float64, n*n)
	parallelForRows(n, func(a int) {
		row := a * n
		for j := 0; j+3 < n; j += 4 {
			var s0, s1, s2, s3 float64
			for b := 0; b < n; b++ {
				s0 += inner[row+b] * v[j*n+b] // want `this operand does not vary with the output index j but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
				s1 += inner[row+b] * v[(j+1)*n+b]
				s2 += inner[row+b] * v[(j+2)*n+b]
				s3 += inner[row+b] * v[(j+3)*n+b]
			}
			tmp[row+j] = s0
			tmp[row+j+1] = s1
			tmp[row+j+2] = s2
			tmp[row+j+3] = s3
		}
	})
}

// Restarting the scalar loop at zero is a full replay, not an n%4 tail. It
// must still expose the original re-streaming finding.
func rowOwnedMatrixProductFullScalarReplay(inner, v []float64, n int) {
	tmp := make([]float64, n*n)
	parallelForRows(n, func(a int) {
		row := a * n
		j := 0
		for ; j+3 < n; j += 4 {
			var s0, s1, s2, s3 float64
			for b := 0; b < n; b++ {
				innerAB := inner[row+b]
				s0 += innerAB * v[j*n+b]
				s1 += innerAB * v[(j+1)*n+b]
				s2 += innerAB * v[(j+2)*n+b]
				s3 += innerAB * v[(j+3)*n+b]
			}
			tmp[row+j] = s0
			tmp[row+j+1] = s1
			tmp[row+j+2] = s2
			tmp[row+j+3] = s3
		}
		for j := 0; j < n; j++ {
			sum := 0.0
			for b := 0; b < n; b++ {
				sum += inner[row+b] * v[j*n+b] // want `this operand does not vary with the output index j but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
			}
			tmp[row+j] = sum
		}
	})
}

// A body-local o shadows the output-loop object. The access is not indexed by
// the outer output variable and must not be reported merely because names match.
func shadowedOutputIndex(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			o := i
			sum += a[i] * w[i*n+o]
		}
		dst[o] = sum
	}
}

// A once-defined derived index dominates the accumulation and preserves the
// real inner-loop dependency.
func derivedIndex(dst [64]float64, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			idx := i
			sum += a[idx] * w[idx*out+o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = sum
	}
}

// Reassignment invalidates the derived-index proof even when the first value
// came from the inner loop variable.
func derivedIndexReassignedBeforeUse(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			idx := i
			idx = 0
			sum += a[idx] * w[idx*out+o]
		}
		dst[o] = sum
	}
}

// A later reassignment also makes the object non-single-assignment; do not use
// its final dependency state to classify an earlier constant-index access.
func derivedIndexReassignedAfterUse(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			idx := 0
			sum += a[idx] * w[idx*out+o]
			idx = i
		}
		dst[o] = sum
	}
}

// Branch-only definitions are deliberately outside the conservative
// direct-statement dominance proof.
func branchOnlyDerivedIndex(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			if i >= 0 {
				idx := i
				sum += a[idx] * w[idx*out+o]
			}
		}
		dst[o] = sum
	}
}

// A closure can reassign the captured derived object before the access. Its
// write must invalidate the single-assignment proof too.
func closureReassignedDerivedIndex(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			idx := i
			func() { idx = 0 }()
			sum += a[idx] * w[idx*out+o]
		}
		dst[o] = sum
	}
}

// A derived index that also carries the output dimension is not invariant
// across output elements, even though it still depends on the inner loop.
func outputDependentDerivedIndex(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			innerIndex := i
			outputIndex := o
			idx := innerIndex + outputIndex
			sum += a[idx] * w[o]
		}
		dst[o] = sum
	}
}

// Taking the address makes the local indirectly mutable. The analyzer cannot
// use the direct initializer to prove the later access invariant.
func addressTakenDerivedIndex(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			idx := i
			pointer := &idx
			*pointer = o
			sum += a[idx] * w[o]
		}
		dst[o] = sum
	}
}

type mutableDerivedIndex int

func (index *mutableDerivedIndex) set(value int) {
	*index = mutableDerivedIndex(value)
}

// A pointer-receiver method implicitly takes the address of an addressable
// receiver and must invalidate the same proof as an explicit &idx escape.
func implicitlyAddressTakenDerivedIndex(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			idx := mutableDerivedIndex(i)
			idx.set(o)
			sum += a[idx] * w[o]
		}
		dst[o] = sum
	}
}

// All RHS expressions of a simultaneous assignment observe the old state.
// In particular, idx receives the old output-dependent value of previous.
func simultaneousAssignmentUsesSnapshot(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		previous := o
		idx := 0
		for i := 0; i < n; i++ {
			previous, idx = i, previous
			sum += a[idx] * w[o]
		}
		dst[o] = sum
	}
}

// Simultaneous assignments whose RHS expressions both directly depend on the
// inner loop remain valid derived-index proofs.
func simultaneousAssignmentInnerDependencies(dst [64]float64, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		left, idx := 0, 0
		for i := 0; i < n; i++ {
			left, idx = i, i
			_ = left
			sum += a[idx] * w[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = sum
	}
}

func advanceIndex(index *int, output int) {
	*index = output
}

// Address-taking uncertainty also propagates from referenced dependencies. If
// the loop object itself escapes, a later alias cannot be considered pure-inner.
func addressTakenLoopIndex(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			advanceIndex(&i, o)
			idx := i
			sum += a[idx] * w[o]
		}
		dst[o] = sum
	}
}

// An enclosing local is outside the inner body's tracked definition state. It
// may carry the output dimension and therefore cannot prove invariance.
func enclosingOutputAlias(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		offset := o
		for i := 0; i < n; i++ {
			idx := i + offset
			sum += a[idx] * w[o]
		}
		dst[o] = sum
	}
}

// Address escapes outside the inner body also invalidate otherwise untracked
// parameters and enclosing locals used by the indexed operand.
func addressTakenEnclosingParameter(dst, a, w []float64, out, n, base int) {
	for o := 0; o < out; o++ {
		advanceIndex(&base, o)
		sum := 0.0
		for i := 0; i < n; i++ {
			idx := i + base
			sum += a[idx] * w[o]
		}
		dst[o] = sum
	}
}

// A dereferenced parameter may be mutated through any alias and cannot prove an
// output-invariant index without points-to immutability evidence.
func dereferencedIndexInput(dst, a, w []float64, out, n int, base *int) {
	for o := 0; o < out; o++ {
		*base = o
		sum := 0.0
		for i := 0; i < n; i++ {
			idx := i + *base
			sum += a[idx] * w[o]
		}
		dst[o] = sum
	}
}

// Member writes invalidate dependency facts stored in their aggregate root.
func mutatedDerivedStructMember(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			position := struct{ index int }{index: i}
			position.index = o
			sum += a[position.index] * w[o]
		}
		dst[o] = sum
	}
}

func mutatedDerivedArrayElement(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			position := [1]int{i}
			position[0] = o
			sum += a[position[0]] * w[o]
		}
		dst[o] = sum
	}
}

// Taking a member's address exposes the complete aggregate to indirect
// mutation, so neither a field nor an element remains a proven derived index.
func addressTakenDerivedStructMember(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			position := struct{ index int }{index: i}
			pointer := &position.index
			*pointer = o
			sum += a[position.index] * w[o]
		}
		dst[o] = sum
	}
}

func addressTakenDerivedArrayElement(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			position := [1]int{i}
			pointer := &position[0]
			*pointer = o
			sum += a[position[0]] * w[o]
		}
		dst[o] = sum
	}
}

// Slicing an array exposes its backing storage without an explicit address
// expression, so writes through the slice invalidate the derived aggregate.
func slicedDerivedArrayStorage(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			position := [1]int{i}
			alias := position[:]
			alias[0] = o
			sum += a[position[0]] * w[o]
		}
		dst[o] = sum
	}
}

func slicedDerivedArraySelectorStorage(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			position := struct{ indexes [1]int }{indexes: [1]int{i}}
			alias := position.indexes[:]
			alias[0] = o
			sum += a[position.indexes[0]] * w[o]
		}
		dst[o] = sum
	}
}

var hiddenOutputOffset int

func lookupIndex(inner int) int { return inner + hiddenOutputOffset }

// Ordinary calls may depend on package, receiver, or closure state that is not
// present in their arguments, so their results are not derived-index proofs.
func derivedCallResult(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		hiddenOutputOffset = o
		sum := 0.0
		for i := 0; i < n; i++ {
			idx := lookupIndex(i)
			sum += a[idx] * w[o]
		}
		dst[o] = sum
	}
}

// Reference-backed aggregates can share storage after a value copy. A write
// through the alias therefore invalidates the original root as an index proof.
func copiedDerivedSlice(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			position := []int{i}
			alias := position
			alias[0] = o
			sum += a[position[0]] * w[o]
		}
		dst[o] = sum
	}
}

func copiedDerivedMap(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			position := map[int]int{0: i}
			alias := position
			alias[0] = o
			sum += a[position[0]] * w[o]
		}
		dst[o] = sum
	}
}

func copiedDerivedNestedSlice(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			position := struct{ indexes []int }{indexes: []int{i}}
			alias := position
			alias.indexes[0] = o
			sum += a[position.indexes[0]] * w[o]
		}
		dst[o] = sum
	}
}

func crossFilePackageVariable(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		setPackageOffset(o)
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += a[i+packageOffset] * w[o]
		}
		dst[o] = sum
	}
}

func indexFor(output int) int { return output }

// Calls are conservative for hidden state, but their explicit arguments still
// prove that the other operand varies with the output dimension.
func callArgumentOutputDependency(dst [64]float64, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += a[i] * w[indexFor(o)] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = sum
	}
}

func dereferencedOutputDependency(dst [64]float64, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		output := &o
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += a[i] * w[*output] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = sum
	}
}

// Distinct reference-backed parameters may alias at the call site. A write
// through alias can make the indexes vary with the output dimension.
func aliasedInputIndexes(dst, a, w []float64, indexes, alias []int, out, n int) {
	for o := 0; o < out; o++ {
		alias[0] = o
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += a[indexes[i]] * w[o]
		}
		dst[o] = sum
	}
}

type pointerConfig struct{ offset int }

func setPointerOffset(config *pointerConfig, output int) { config.offset = output }

func implicitPointerBackedRead(dst, a, w []float64, config *pointerConfig, out, n int) {
	for o := 0; o < out; o++ {
		setPointerOffset(config, o)
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += a[i+config.offset] * w[o]
		}
		dst[o] = sum
	}
}

func genericAliasedIndexes[S ~[]int](dst, a, w []float64, indexes, alias S, out, n int) {
	for o := 0; o < out; o++ {
		alias[0] = o
		sum := 0.0
		for i := 0; i < n; i++ {
			idx := i + indexes[0]
			sum += a[idx] * w[o]
		}
		dst[o] = sum
	}
}

func futureEnclosingAssignment(dst, a, w []float64, out, n, offset int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += a[i+offset] * w[o]
		}
		dst[o] = sum
		offset = 0
	}
}

func conditionalEnclosingAssignment(dst, a, w []float64, out, n, offset int, zero bool) {
	if zero {
		offset = 0
	}
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += a[i+offset] * w[o]
		}
		dst[o] = sum
	}
}

// Even distinct slice parameters may share a backing array. At n=1/out=4,
// calling this with dst == a makes each scalar store visible to later reads;
// the root is unknown; delaying all four stores is invalid, so this stays silent.
func aliasedDestinationInput(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o]
		}
		dst[o] = acc
	}
}

// Arrays contain value-only storage, so distinct parameters and the local
// destination cannot alias. Keep the canonical automatic rewrite available
// when the type information proves that delayed stores are safe.
func matvecArrays(a [4]float64, w [16]float64, out, n int) [4]float64 {
	var dst [4]float64
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}

// The final index alone is insufficient: selecting the row by o makes the
// accessed operand differ for every output, for both slices and arrays.
func outputSelectedSliceRows(dst, w []float64, rows [][]float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += rows[o][i] * w[o]
		}
		dst[o] = sum
	}
}

func outputSelectedArrayRows(dst *[4]float64, w [4]float64, rows [4][8]float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += rows[o][i] * w[o]
		}
		dst[o] = sum
	}
}

// The same output dependency must follow a derived row header into row[i].
func derivedOutputSelectedSliceRow(dst, w []float64, rows [][]float64, out, n int) {
	for o := 0; o < out; o++ {
		row := rows[o]
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += row[i] * w[o]
		}
		dst[o] = sum
	}
}

func derivedOutputSelectedArrayRow(dst *[4]float64, w [4]float64, rows [4][8]float64, out, n int) {
	for o := 0; o < out; o++ {
		row := rows[o]
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += row[i] * w[o]
		}
		dst[o] = sum
	}
}

// A goto can enter the use without executing the syntactically earlier write.
// The local sequential state is therefore not a dominance proof.
func skippedDerivedWriteByGoto(dst, a, w []float64, out, n, idx int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			if i == 0 {
				goto use
			}
			idx = i
		use:
			sum += a[idx] * w[o]
		}
		dst[o] = sum
	}
}

// Continue is also control transfer; conservatively decline derived-value
// facts in such bodies until the check carries a full CFG dominator tree.
func derivedWriteWithContinue(dst, a, w []float64, out, n, idx int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			if i == 0 {
				continue
			}
			idx = i
			sum += a[idx] * w[o]
		}
		dst[o] = sum
	}
}

// A value constrained to integer underlying types cannot hide reference-backed
// storage, so a single-write conversion from i remains an eligible index fact.
func integerTypeParameterIndex[I ~int](dst [64]float64, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			idx := I(i)
			sum += a[idx] * w[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = sum
	}
}

// psO21227 is deliberately in the same block as the loop. The generated
// output-index name must be made unique so its short declaration remains valid.
func matvecArraysNameCollision(a [4]float64, w [16]float64, out, n int) [4]float64 {
	var dst [4]float64
	psO21227 := -1
	_ = psO21227
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}

type ps6010Integer interface {
	~int | ~int32 | ~uint64
}

func integerUnionTypeParameterIndex[I ps6010Integer](dst [64]float64, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			idx := I(i)
			sum += a[idx] * w[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = sum
	}
}

// The explicit alias shares a's backing array. Mutating it with o makes a[i]
// output-dependent even though the access syntax itself mentions only i.
func directlyMutatedAliasedContainer(dst, a, w []float64, out, n int) {
	alias := a
	for o := 0; o < out; o++ {
		alias[0] = float64(o)
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += a[i] * w[o]
		}
		dst[o] = sum
	}
}

func mutatePS6010Container(values []float64, output int) {
	if len(values) != 0 {
		values[0] = float64(output)
	}
}

// An opaque call receiving a can mutate its backing storage. Without a purity
// summary the container root is unknown, so no invariance claim is safe.
func opaquelyMutatedContainer(dst, a, w []float64, out, n int) {
	for o := 0; o < out; o++ {
		mutatePS6010Container(a, o)
		sum := 0.0
		for i := 0; i < n; i++ {
			sum += a[i] * w[o]
		}
		dst[o] = sum
	}
}

// psO23260 lives in the switch clause's implicit scope. The fix must
// select another output-index name and the complete fixed file must compile.
func matvecArraysSwitchCollision(a [4]float64, w [16]float64, out, n int, enabled bool) [4]float64 {
	var dst [4]float64
	switch {
	case enabled:
		psO23260 := -1
		_ = psO23260
		for o := 0; o < out; o++ {
			acc := 0.0
			for i := 0; i < n; i++ {
				acc += a[i] * w[i*out+o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
			}
			dst[o] = acc
		}
	}
	return dst
}

// psO23873 pins the corresponding implicit scope in a select clause.
func matvecArraysSelectCollision(a [4]float64, w [16]float64, out, n int, ready <-chan struct{}) [4]float64 {
	var dst [4]float64
	select {
	case <-ready:
		psO23873 := -1
		_ = psO23873
		for o := 0; o < out; o++ {
			acc := 0.0
			for i := 0; i < n; i++ {
				acc += a[i] * w[i*out+o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
			}
			dst[o] = acc
		}
	default:
	}
	return dst
}

// A labeled loop cannot be replaced by the multi-statement tile template: the
// label would attach only to the first generated statement and change targets.
func matvecArraysLabeled(a [4]float64, w [16]float64, out, n int) [4]float64 {
	var dst [4]float64
	if out < 0 {
		goto outputLoop
	}
outputLoop:
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * w[i*out+o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}
