package ps4008

func round26Invoke(callback func()) { callback() }

func round26GenericInvoke[Callback ~func()](callback Callback) { callback() }

func maybeRangePeerDefinitionsRound26(a, b [][]float64, output []float64, rows, columns int, items []int, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for range items {
		if flag {
			saved = func() { base = 0 }
		} else {
			saved = func() { base = 0 }
		}
	}
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop`
				base = inner
				round26Invoke(saved)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func maybeForPeerDefinitionsRound26(a, b [][]float64, output []float64, rows, columns int, enter, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for attempt := 0; enter && attempt < 1; attempt++ {
		if flag {
			saved = func() { base = 0 }
		} else {
			saved = func() { base = 0 }
		}
	}
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop`
				base = inner
				round26GenericInvoke(saved)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func guaranteedRangeBreakBeforePeersRound26(a, b [][]float64, output []float64, rows, columns int, stop, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for range [1]int{} {
		if stop {
			break
		}
		if flag {
			saved = func() { base = 0 }
		} else {
			saved = func() { base = 0 }
		}
	}
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop`
				base = inner
				round26Invoke(saved)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func guaranteedRangeContinueBeforePeersRound26(a, b [][]float64, output []float64, rows, columns int, stop, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for range [1]int{} {
		if stop {
			continue
		}
		if flag {
			saved = func() { base = 0 }
		} else {
			saved = func() { base = 0 }
		}
	}
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop`
				base = inner
				round26GenericInvoke(saved)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func guaranteedRangeGotoBeforePeersRound26(a, b [][]float64, output []float64, rows, columns int, stop, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for range [1]int{} {
		if stop {
			goto after
		}
		if flag {
			saved = func() { base = 0 }
		} else {
			saved = func() { base = 0 }
		}
	}
after:
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop`
				base = inner
				round26Invoke(saved)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func gotoBeforePeersRound26(a, b [][]float64, output []float64, rows, columns int, stop, flag bool) {
	base := 0
	saved := func() { base += 0 }
	if stop {
		goto after
	}
	if flag {
		saved = func() { base = 0 }
	} else {
		saved = func() { base = 0 }
	}
after:
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop`
				base = inner
				round26GenericInvoke(saved)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func nestedBreakBeforePeersRound26(a, b [][]float64, output []float64, rows, columns int, stop, nested, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for range [1]int{} {
		if stop {
			if nested {
				break
			}
		}
		if flag {
			saved = func() { base = 0 }
		} else {
			saved = func() { base = 0 }
		}
	}
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ { // want `innermost loop`
				base = inner
				round26GenericInvoke(saved)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func guaranteedRangeExhaustivePeersRound26(a, b [][]float64, output []float64, rows, columns int, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for range [1]int{} {
		if flag {
			saved = func() { base = 0 }
		} else {
			saved = func() { base = 0 }
		}
	}
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base = inner
				round26Invoke(saved)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func guaranteedForExhaustivePeersRound26(a, b [][]float64, output []float64, rows, columns int, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for attempt := 0; attempt < 1; attempt++ {
		if flag {
			saved = func() { base = 0 }
		} else {
			saved = func() { base = 0 }
		}
	}
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base = inner
				round26GenericInvoke(saved)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func guaranteedRangeReturnGuardRound26(a, b [][]float64, output []float64, rows, columns int, stop, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for range [1]int{} {
		if stop {
			return
		}
		if flag {
			saved = func() { base = 0 }
		} else {
			saved = func() { base = 0 }
		}
	}
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base = inner
				round26Invoke(saved)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func guaranteedRangePanicGuardRound26(a, b [][]float64, output []float64, rows, columns int, stop, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for range [1]int{} {
		if stop {
			panic("stop")
		}
		if flag {
			saved = func() { base = 0 }
		} else {
			saved = func() { base = 0 }
		}
	}
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base = inner
				round26GenericInvoke(saved)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func guaranteedRangeNestedBlockPeersRound26(a, b [][]float64, output []float64, rows, columns int, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for range [1]int{} {
		{
			if flag {
				saved = func() { base = 0 }
			} else {
				saved = func() { base = 0 }
			}
		}
	}
	for row := 0; row < rows; row++ {
		for column := 0; column < columns; column++ {
			var sum float64
			for inner := 0; inner < rows; inner++ {
				base = inner
				round26Invoke(saved)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}
