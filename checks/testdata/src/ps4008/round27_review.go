package ps4008

func round27Invoke(callback func()) { callback() }

func round27GenericInvoke[Callback ~func()](callback Callback) { callback() }

func switchBreakThenExhaustivePeersRound27(a, b [][]float64, output []float64, rows, columns int, choose, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for range [1]int{} {
		switch {
		case choose:
			break
		default:
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
				round27Invoke(saved)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func switchWithoutDefaultThenExhaustivePeersRound27(a, b [][]float64, output []float64, rows, columns int, choose, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for range [1]int{} {
		switch {
		case choose:
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
			for inner := 0; inner < rows; inner++ {
				base = inner
				round27GenericInvoke(saved)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func selectDefaultBreakThenExhaustivePeersRound27(a, b [][]float64, output []float64, rows, columns int, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for range [1]int{} {
		select {
		default:
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
			for inner := 0; inner < rows; inner++ {
				base = inner
				round27Invoke(saved)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func forwardGotoMarkerThenExhaustivePeersRound27(a, b [][]float64, output []float64, rows, columns int, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for range [1]int{} {
		goto marker
	marker:
		;
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
				round27GenericInvoke(saved)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func switchFallthroughThenExhaustivePeersRound27(a, b [][]float64, output []float64, rows, columns int, choose, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for range [1]int{} {
		switch {
		case choose:
			fallthrough
		default:
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
				round27Invoke(saved)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func switchNestedLoopBreakThenExhaustivePeersRound27(a, b [][]float64, output []float64, rows, columns int, choose, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for range [1]int{} {
		switch {
		case choose:
			for {
				break
			}
		default:
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
				round27GenericInvoke(saved)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func switchTerminatingClausesThenExhaustivePeersRound27(a, b [][]float64, output []float64, rows, columns int, stop, explode, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for range [1]int{} {
		switch {
		case stop:
			return
		case explode:
			panic("stop")
		default:
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
				round27Invoke(saved)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func switchContinueBypassesPeersRound27(a, b [][]float64, output []float64, rows, columns int, stop, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for range [1]int{} {
		switch {
		case stop:
			continue
		default:
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
				round27GenericInvoke(saved)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func switchGotoBypassesPeersRound27(a, b [][]float64, output []float64, rows, columns int, stop, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for range [1]int{} {
		switch {
		case stop:
			goto after
		default:
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
				round27Invoke(saved)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func labeledOuterBreakBypassesPeersRound27(a, b [][]float64, output []float64, rows, columns int, stop, flag bool) {
	base := 0
	saved := func() { base += 0 }
outer:
	for range [1]int{} {
		switch {
		case stop:
			break outer
		default:
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
				round27GenericInvoke(saved)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func blockingSelectBeforePeersRound27(a, b [][]float64, output []float64, rows, columns int, ready <-chan struct{}, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for range [1]int{} {
		select {
		case <-ready:
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
				round27Invoke(saved)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}

func selectContinueBypassesPeersRound27(a, b [][]float64, output []float64, rows, columns int, ready <-chan struct{}, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for range [1]int{} {
		select {
		case <-ready:
			continue
		default:
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
				round27GenericInvoke(saved)
				sum += a[row][base] * b[base][column]
			}
			output[column] = sum
		}
	}
}
