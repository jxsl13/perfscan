package ps1006

func round18Reset(target *int) int {
	*target = 0
	return 0
}

func round18Set(target *int, value int) int {
	*target = value
	return 0
}

func round18Write(target *int, value, ignored int) { *target = value }
func round18Compound(target *int)                  { *target += 0 }
func round18Increment(target *int)                 { (*target)++ }
func round18ForwardReset(target *int)              { round18Reset(target) }
func round18Invoke(callback func())                { callback() }

func round18GenericWrite[T ~int](target *T, value T, ignored int) { *target = value }

type round18Number int

func (target *round18Number) reset() { *target = 0 }

func round18TupleKeep(target, other *int) { *target, *other = *target, *other }
func round18TupleReset(target, other *int) {
	*target, *other = 0, *target
}

func argumentSnapshotReportsRound18(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			round18Write(&base, base, round18Reset(&base))
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func reverseArgumentSnapshotSilentRound18(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := 0
			round18Write(&base, base, round18Set(&base, row*columns))
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func conditionalPointerAliasReportsRound18(a, output []float64, rows, columns int, flag bool) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base, other := row*columns, 0
			pointer := &base
			if flag {
				pointer = &other
			}
			round18Reset(pointer)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func nestedPointerAliasReportsRound18(a, output []float64, rows, columns int, flag bool) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base, other := row*columns, 0
			pointer := &base
			if flag {
				pointer = &other
			}
			round18ForwardReset(pointer)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func callbackPointerAliasReportsRound18(a, output []float64, rows, columns int, flag bool) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base, other := row*columns, 0
			pointer := &base
			if flag {
				pointer = &other
			}
			round18Invoke(func() { *pointer = 0 })
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func methodPointerAliasReportsRound18(a, output []float64, rows, columns int, flag bool) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base, other := round18Number(row*columns), round18Number(0)
			pointer := &base
			if flag {
				pointer = &other
			}
			pointer.reset()
			sum += a[int(base)+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func genericArgumentSnapshotReportsRound18(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			round18GenericWrite(&base, base, round18Reset(&base))
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func compoundReportsRound18(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			round18Compound(&base)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func incDecReportsRound18(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			round18Increment(&base)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func compoundIndependentSilentRound18(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := 0
			round18Compound(&base)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func incDecIndependentSilentRound18(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := 0
			round18Increment(&base)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func tupleKeepReportsRound18(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base, other := row*columns, 0
			round18TupleKeep(&base, &other)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func tupleResetSilentRound18(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base, other := row*columns, 0
			round18TupleReset(&base, &other)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func uniqueResetSilentRound18(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			round18Reset(&base)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func round20Retarget(pointer **int, target *int) int {
	*pointer = target
	return 0
}

func round20ForwardRetarget(pointer **int, target *int) int {
	return round20Retarget(pointer, target)
}

func round20GenericRetarget[T ~int](pointer **T, target *T) int {
	*pointer = target
	return 0
}

func round20ResetAfter(_ int, target *int)   { *target = 0 }
func round20ResetBefore(target *int, _ int)  { *target = 0 }
func round20ResetSecond(_ *int, target *int) { *target = 0 }

func round20RetargetPair(pointer **int, target *int) (int, *int) {
	*pointer = target
	return 0, *pointer
}

func round20SnapshotPair(target *int, ignored int) (int, *int) {
	return ignored, target
}

func round20ResetExpanded(_ int, target *int) { *target = 0 }

func round20KeepAfterPrint(target *int) {
	if target == nil {
		println()
	}
	*target += 0
}

func round20KeepSubtractAfterPrint(target *int) {
	if target == nil {
		println()
	}
	*target -= 0
}

func round20IncrementAfterPrint(target *int) {
	if target == nil {
		println()
	}
	(*target)++
}

func round20ResetAfterPrint(target *int) {
	if target == nil {
		println()
	}
	*target = 0
}

func round20TupleKeepBlank(target *int) {
	*target, _ = *target, round18Reset(target)
}

func round20TupleResetBlank(target *int) {
	*target, _ = 0, *target
}

func round20NestedCallbackKeep(target *int) {
	round18Invoke(func() {
		if target == nil {
			println()
		}
		*target += 0
	})
}

type round20Number int

func (target *round20Number) resetAfter(_ int) { *target = 0 }

func (target *round20Number) resetAndReturn() int {
	*target = 0
	return 0
}

func round20IgnoreTwo(_, _ int) {}

func round20RetargetNumber(pointer **round20Number, target *round20Number) int {
	*pointer = target
	return 0
}

func laterPointerArgumentUsesRetargetedValueRound20(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base, other := row*columns, 0
			pointer := &base
			round20ResetAfter(round20Retarget(&pointer, &other), pointer)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func laterPointerArgumentThroughWrapperRound20(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base, other := row*columns, 0
			pointer := &base
			round20ResetAfter(round20ForwardRetarget(&pointer, &other), pointer)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func laterGenericPointerArgumentRound20(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base, other := row*columns, 0
			pointer := &base
			round20ResetAfter(round20GenericRetarget(&pointer, &other), pointer)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func retargetCollapsesMayAliasRound20(a, output []float64, rows, columns int, flag bool) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base, other, final := row*columns, 0, 0
			pointer := &base
			if flag {
				pointer = &other
			}
			round20ResetAfter(round20Retarget(&pointer, &final), pointer)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func earlierPointerArgumentSnapshotControlRound20(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base, other := row*columns, 0
			pointer := &base
			round20ResetBefore(pointer, round20Retarget(&pointer, &other))
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func repeatedPointerArgumentControlRound20(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			pointer := &base
			round20ResetSecond(pointer, pointer)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func methodReceiverSnapshotControlRound20(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base, other := round20Number(row*columns), round20Number(0)
			pointer := &base
			pointer.resetAfter(round20RetargetNumber(&pointer, &other))
			sum += a[int(base)+column]
		}
		output[column] = sum
	}
}

func laterMethodReceiverUsesRetargetedValueRound20(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base, other := round20Number(row*columns), round20Number(0)
			pointer := &base
			round20IgnoreTwo(round20RetargetNumber(&pointer, &other), pointer.resetAndReturn())
			sum += a[int(base)+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func tupleExpansionUsesRetargetedValueRound20(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base, other := row*columns, 0
			pointer := &base
			round20ResetExpanded(round20RetargetPair(&pointer, &other))
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func tupleExpansionSnapshotControlRound20(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base, other := row*columns, 0
			pointer := &base
			round20ResetExpanded(round20SnapshotPair(pointer, round20Retarget(&pointer, &other)))
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func unsupportedReadModifyFormsRound20(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			round20KeepAfterPrint(&base)
			round20KeepSubtractAfterPrint(&base)
			round20IncrementAfterPrint(&base)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func unsupportedOverwriteControlRound20(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			round20ResetAfterPrint(&base)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func tupleBlankCapturesPriorRound20(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			round20TupleKeepBlank(&base)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func tupleBlankOverwriteControlRound20(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			round20TupleResetBlank(&base)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func nestedCallbackReadModifyRound20(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			round20NestedCallbackKeep(&base)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}
