package ps4008

type round24Offset int

func (value *round24Offset) reset() { *value = 0 }
func (value *round24Offset) keep()  { *value += 0 }

func round24Retarget(target **round24Offset, replacement *round24Offset) { *target = replacement }

type round24CallableBox struct{ callback func() }

func round24CallablePair(first, second func()) (func(), func()) { return first, second }

func methodValueSnapshotResetRound24(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base, other := round24Offset(inner), round24Offset(0)
				receiver := &base
				saved := receiver.reset
				receiver = &other
				saved()
				sum += a[row][int(base)] * b[int(base)][column]
			}
			output[column] = sum
		}
	}
}

func methodValueSnapshotKeepRound24(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop`
				base, other := round24Offset(inner), round24Offset(0)
				receiver := &base
				saved := receiver.keep
				receiver = &other
				saved()
				sum += a[row][int(base)] * b[int(base)][column]
			}
			output[column] = sum
		}
	}
}

func directMethodValueResetRound24(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base := round24Offset(inner)
				receiver := &base
				saved := receiver.reset
				saved()
				sum += a[row][int(base)] * b[int(base)][column]
			}
			output[column] = sum
		}
	}
}

func ambiguousMethodValueSnapshotRound24(a, b [][]float64, output []float64, rows, columns int, flag bool) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop`
				base, other := round24Offset(inner), round24Offset(0)
				receiver := &base
				if flag {
					receiver = &base
				} else {
					receiver = &other
				}
				saved := receiver.reset
				saved()
				sum += a[row][int(base)] * b[int(base)][column]
			}
			output[column] = sum
		}
	}
}

func escapedMethodValueSnapshotRound24(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop`
				base, other := round24Offset(inner), round24Offset(0)
				receiver := &base
				round24Retarget(&receiver, &other)
				saved := receiver.reset
				saved()
				sum += a[row][int(base)] * b[int(base)][column]
			}
			output[column] = sum
		}
	}
}

func callableFieldSnapshotResetRound24(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base := inner
				box := round24CallableBox{callback: func() { base = 0 }}
				saved := box.callback
				box.callback = func() { base += 0 }
				saved()
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func callableFieldSnapshotKeepRound24(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop`
				base := inner
				box := round24CallableBox{callback: func() { base += 0 }}
				saved := box.callback
				box.callback = func() { base = 0 }
				saved()
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func directCallableFieldResetRound24(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base := inner
				box := round24CallableBox{callback: func() { base = 0 }}
				saved := box.callback
				saved()
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func callableArraySnapshotResetRound24(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base := inner
				callbacks := [1]func(){func() { base = 0 }}
				saved := callbacks[0]
				callbacks[0] = func() { base += 0 }
				saved()
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func callableArraySnapshotKeepRound24(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop`
				base := inner
				callbacks := [1]func(){func() { base += 0 }}
				saved := callbacks[0]
				callbacks[0] = func() { base = 0 }
				saved()
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func directCallableArrayResetRound24(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base := inner
				callbacks := [1]func(){func() { base = 0 }}
				saved := callbacks[0]
				saved()
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func returnedCallableSnapshotResetRound24(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base := inner
				callback := func() { base = 0 }
				saved, _ := round24CallablePair(callback, func() { base += 0 })
				callback = func() { base += 0 }
				saved()
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func returnedCallableSnapshotKeepRound24(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop`
				base := inner
				callback := func() { base += 0 }
				saved, _ := round24CallablePair(callback, func() { base = 0 })
				callback = func() { base = 0 }
				saved()
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func directReturnedCallableResetRound24(a, b [][]float64, output []float64, rows, columns int) {
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base := inner
				callback := func() { base = 0 }
				saved, _ := round24CallablePair(callback, func() { base += 0 })
				saved()
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}
