package ps1006

func round23Keep(target *int)  { *target += 0 }
func round23Reset(target *int) { *target = 0 }

func round23Invoke(callback func())           { callback() }
func round23ForwardInvoke(callback func())    { round23Invoke(callback) }
func round23GenericInvoke[F ~func()](value F) { value() }

type round23Invoker struct{}

func (round23Invoker) invoke(callback func()) { callback() }

func round23Retarget(target **int, replacement *int) { *target = replacement }
func round23ForwardRetarget(target **int, replacement *int) {
	round23Retarget(target, replacement)
}
func round23GenericRetarget[T any](target **T, replacement *T) { *target = replacement }
func round23OptionalRetarget(flag bool, target **int, replacement *int) {
	for attempt := 0; flag && attempt < 1; attempt++ {
		round23Retarget(target, replacement)
	}
}

type round23Retargeter struct{}

func (round23Retargeter) retarget(target **int, replacement *int) {
	*target = replacement
}

type round23CallbackBox struct {
	callback func(*int)
}

func savedCallableReportsRound23(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			callback := func() { round23Keep(&base) }
			saved := callback
			callback = func() { round23Reset(&base) }
			saved()
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func wrappedSavedCallableReportsRound23(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			callback := func() { round23Keep(&base) }
			saved := callback
			callback = func() { round23Reset(&base) }
			round23ForwardInvoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func genericSavedCallableReportsRound23(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			callback := func() { round23Keep(&base) }
			saved := callback
			callback = func() { round23Reset(&base) }
			round23GenericInvoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func methodSavedCallableReportsRound23(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			callback := func() { round23Keep(&base) }
			saved := callback
			callback = func() { round23Reset(&base) }
			round23Invoker{}.invoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func tupleSavedCallableReportsRound23(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			callback := func() { round23Keep(&base) }
			saved, other := callback, func() { round23Reset(&base) }
			callback = other
			saved()
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func branchSavedCallableReportsRound23(a, output []float64, rows, columns int, flag bool) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			callback := func() { round23Keep(&base) }
			var saved func()
			if flag {
				saved = callback
			} else {
				saved = callback
			}
			callback = func() { round23Reset(&base) }
			saved()
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func reversedSavedCallableControlRound23(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			callback := func() { round23Reset(&base) }
			saved := callback
			callback = func() { round23Keep(&base) }
			saved()
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func ifRetargetReportsRound23(a, output []float64, rows, columns int, flag bool) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base, left, right := row*columns, 0, 0
			pointer := &base
			if flag {
				round23Retarget(&pointer, &left)
			} else {
				round23Retarget(&pointer, &right)
			}
			round23Reset(pointer)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func wrappedIfRetargetReportsRound23(a, output []float64, rows, columns int, flag bool) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base, left, right := row*columns, 0, 0
			pointer := &base
			if flag {
				round23ForwardRetarget(&pointer, &left)
			} else {
				round23ForwardRetarget(&pointer, &right)
			}
			round23Reset(pointer)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func genericIfRetargetReportsRound23(a, output []float64, rows, columns int, flag bool) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base, left, right := row*columns, 0, 0
			pointer := &base
			if flag {
				round23GenericRetarget(&pointer, &left)
			} else {
				round23GenericRetarget(&pointer, &right)
			}
			round23Reset(pointer)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func methodIfRetargetReportsRound23(a, output []float64, rows, columns int, flag bool) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base, left, right := row*columns, 0, 0
			pointer := &base
			if flag {
				round23Retargeter{}.retarget(&pointer, &left)
			} else {
				round23Retargeter{}.retarget(&pointer, &right)
			}
			round23Reset(pointer)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func switchRetargetReportsRound23(a, output []float64, rows, columns int, choice int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base, left, right := row*columns, 0, 0
			pointer := &base
			switch choice {
			case 0:
				round23Retarget(&pointer, &left)
			default:
				round23Retarget(&pointer, &right)
			}
			round23Reset(pointer)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func selectRetargetReportsRound23(a, output []float64, rows, columns int, ready <-chan struct{}) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base, left, right := row*columns, 0, 0
			pointer := &base
			select {
			case <-ready:
				round23Retarget(&pointer, &left)
			default:
				round23Retarget(&pointer, &right)
			}
			round23Reset(pointer)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func directAssignmentRetargetReportsRound23(a, output []float64, rows, columns int, flag bool) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base, left, right := row*columns, 0, 0
			pointer := &base
			if flag {
				pointer = &left
			} else {
				pointer = &right
			}
			round23Reset(pointer)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func retargetBackControlRound23(a, output []float64, rows, columns int, flag bool) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base, other := row*columns, 0
			pointer := &other
			if flag {
				round23Retarget(&pointer, &base)
			} else {
				round23GenericRetarget(&pointer, &base)
			}
			round23Reset(pointer)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func callbackFieldResetControlRound23(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			box := round23CallbackBox{callback: func(target *int) { *target = 0 }}
			box.callback(&base)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func distinctCallbackFieldResetControlRound23(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			resetBox := round23CallbackBox{callback: func(target *int) { *target = 0 }}
			keepBox := round23CallbackBox{callback: func(target *int) { *target += 0 }}
			_ = keepBox
			resetBox.callback(&base)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func callbackFieldRMWReportsRound23(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			box := round23CallbackBox{callback: func(target *int) { *target += 0 }}
			box.callback(&base)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func reassignedCallbackFieldReportsRound23(a, output []float64, rows, columns int) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			box := round23CallbackBox{callback: func(target *int) { *target = 0 }}
			box.callback = func(target *int) { *target += 0 }
			box.callback(&base)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func escapedCallbackFieldReportsRound23(a, output []float64, rows, columns int, escape func(*round23CallbackBox)) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base := row * columns
			box := round23CallbackBox{callback: func(target *int) { *target = 0 }}
			escape(&box)
			box.callback(&base)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func optionalLoopRetargetReportsRound23(a, output []float64, rows, columns int, flag bool) {
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base, other := row*columns, 0
			pointer := &base
			round23OptionalRetarget(flag, &pointer, &other)
			round23Reset(pointer)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}
