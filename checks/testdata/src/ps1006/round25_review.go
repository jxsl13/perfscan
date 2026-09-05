package ps1006

type round25Offset int

func (value *round25Offset) reset() { *value = 0 }
func (value *round25Offset) keep()  { *value += 0 }

type round25CallableBox struct{ callback func() }

func round25Invoke(callback func()) { callback() }

func round25GenericInvoke[Callback ~func()](callback Callback) { callback() }

func round25CallablePair(first, second func()) (func(), func()) {
	return first, second
}

func maybeForSavedCallableRound25(a, output []float64, rows, columns int, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for attempt := 0; flag && attempt < 1; attempt++ {
		saved = func() { base = 0 }
	}
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round25Invoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func maybeRangeSavedGenericCallableRound25(a, output []float64, rows, columns int, items []int) {
	base := 0
	saved := func() { base += 0 }
	for range items {
		saved = func() { base = 0 }
	}
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round25GenericInvoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func zeroRangeSavedGenericCallableRound25(a, output []float64, rows, columns int) {
	base := 0
	saved := func() { base += 0 }
	for range [0]struct{}{} {
		saved = func() { base = 0 }
	}
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round25GenericInvoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func zeroRangeRetainsOnlyIncomingResetRound25(a, output []float64, rows, columns int) {
	base := 0
	saved := func() { base = 0 }
	for range [0]struct{}{} {
		saved = func() { base += 0 }
	}
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round25GenericInvoke(saved)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func zeroForRetainsOnlyIncomingResetRound25(a, output []float64, rows, columns int) {
	base := 0
	saved := func() { base = 0 }
	for false {
		saved = func() { base += 0 }
	}
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round25Invoke(saved)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func maybeForPostSavedCallableRound25(a, output []float64, rows, columns int, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for attempt := 0; flag && attempt < 1; attempt, saved = attempt+1, func() { base = 0 } {
	}
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round25Invoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func maybeMethodValueRound25(a, output []float64, rows, columns int, flag bool) {
	base := round25Offset(0)
	saved := (&base).keep
	for attempt := 0; flag && attempt < 1; attempt++ {
		saved = (&base).reset
	}
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = round25Offset(row * columns)
			saved()
			sum += a[int(base)+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func maybeMethodReceiverRound25(a, output []float64, rows, columns int, items []int) {
	base, other := round25Offset(0), round25Offset(0)
	receiver := &base
	for range items {
		receiver = &other
	}
	saved := receiver.reset
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = round25Offset(row * columns)
			saved()
			sum += a[int(base)+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func maybeFieldMutationSnapshotRound25(a, output []float64, rows, columns int, flag bool) {
	base := 0
	container := round25CallableBox{callback: func() { base += 0 }}
	for attempt := 0; flag && attempt < 1; attempt++ {
		container.callback = func() { base = 0 }
	}
	saved := container.callback
	container.callback = func() { base = 0 }
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round25GenericInvoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func maybeArrayMutationSnapshotRound25(a, output []float64, rows, columns int, items []int) {
	base := 0
	callbacks := [1]func(){func() { base += 0 }}
	for range items {
		callbacks[0] = func() { base = 0 }
	}
	saved := callbacks[0]
	callbacks[0] = func() { base = 0 }
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round25Invoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func maybeTupleSourceSnapshotRound25(a, output []float64, rows, columns int, flag bool) {
	base := 0
	callback := func() { base += 0 }
	for attempt := 0; flag && attempt < 1; attempt++ {
		callback = func() { base = 0 }
	}
	saved, _ := round25CallablePair(callback, func() { base = 0 })
	callback = func() { base = 0 }
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round25GenericInvoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func escapedCallableTupleSnapshotRound25(a, output []float64, rows, columns int, escape func(*func())) {
	base := 0
	callback := func() { base = 0 }
	escape(&callback)
	saved, _ := round25CallablePair(callback, func() { base = 0 })
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			saved()
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func escapedFieldSnapshotRound25(a, output []float64, rows, columns int, escape func(*round25CallableBox)) {
	base := 0
	container := round25CallableBox{callback: func() { base = 0 }}
	escape(&container)
	saved := container.callback
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			saved()
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func escapedMethodReceiverSnapshotRound25(a, output []float64, rows, columns int, escape func(**round25Offset)) {
	base := round25Offset(0)
	receiver := &base
	escape(&receiver)
	saved := receiver.reset
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = round25Offset(row * columns)
			saved()
			sum += a[int(base)+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func exactOneSavedResetRound25(a, output []float64, rows, columns int) {
	base := 0
	saved := func() { base += 0 }
	for attempt := 0; attempt < 1; attempt++ {
		saved = func() { base = 0 }
	}
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			saved()
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func exactOnePostSavedResetRound25(a, output []float64, rows, columns int) {
	base := 0
	saved := func() { base += 0 }
	for attempt := 0; attempt < 1; attempt, saved = attempt+1, func() { base = 0 } {
	}
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			saved()
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func guaranteedRangeSavedResetRound25(a, output []float64, rows, columns int) {
	base := 0
	saved := func() { base += 0 }
	for range [1]struct{}{} {
		saved = func() { base = 0 }
	}
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round25GenericInvoke(saved)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func snapshotBeforeLaterMutationRound25(a, output []float64, rows, columns int) {
	base := 0
	callback := func() { base = 0 }
	saved, _ := round25CallablePair(callback, func() { base += 0 })
	callback = func() { base += 0 }
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			saved()
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func exactOneSavedKeepRound25(a, output []float64, rows, columns int) {
	base := 0
	saved := func() { base = 0 }
	for attempt := 0; attempt < 1; attempt++ {
		saved = func() { base += 0 }
	}
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			saved()
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}
