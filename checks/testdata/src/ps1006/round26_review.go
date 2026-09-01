package ps1006

func round26Invoke(callback func()) { callback() }

func round26GenericInvoke[Callback ~func()](callback Callback) { callback() }

func maybeRangePeerDefinitionsRound26(a, output []float64, rows, columns int, items []int, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for range items {
		if flag {
			saved = func() { base = 0 }
		} else {
			saved = func() { base = 0 }
		}
	}
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round26Invoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func maybeForPeerDefinitionsRound26(a, output []float64, rows, columns int, enter, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for attempt := 0; enter && attempt < 1; attempt++ {
		if flag {
			saved = func() { base = 0 }
		} else {
			saved = func() { base = 0 }
		}
	}
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round26GenericInvoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func guaranteedRangeBreakBeforePeersRound26(a, output []float64, rows, columns int, stop, flag bool) {
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
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round26Invoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func guaranteedRangeContinueBeforePeersRound26(a, output []float64, rows, columns int, stop, flag bool) {
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
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round26GenericInvoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func guaranteedRangeGotoBeforePeersRound26(a, output []float64, rows, columns int, stop, flag bool) {
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
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round26Invoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func gotoBeforePeersRound26(a, output []float64, rows, columns int, stop, flag bool) {
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
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round26GenericInvoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func nestedBreakBeforePeersRound26(a, output []float64, rows, columns int, stop, nested, flag bool) {
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
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round26GenericInvoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func guaranteedRangeExhaustivePeersRound26(a, output []float64, rows, columns int, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for range [1]int{} {
		if flag {
			saved = func() { base = 0 }
		} else {
			saved = func() { base = 0 }
		}
	}
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round26Invoke(saved)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func guaranteedForExhaustivePeersRound26(a, output []float64, rows, columns int, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for attempt := 0; attempt < 1; attempt++ {
		if flag {
			saved = func() { base = 0 }
		} else {
			saved = func() { base = 0 }
		}
	}
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round26GenericInvoke(saved)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func guaranteedRangeReturnGuardRound26(a, output []float64, rows, columns int, stop, flag bool) {
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
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round26Invoke(saved)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func guaranteedRangePanicGuardRound26(a, output []float64, rows, columns int, stop, flag bool) {
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
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round26GenericInvoke(saved)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func guaranteedRangeNestedBlockPeersRound26(a, output []float64, rows, columns int, flag bool) {
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
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round26Invoke(saved)
			sum += a[base+column]
		}
		output[column] = sum
	}
}
