package ps1006

func round27Invoke(callback func()) { callback() }

func round27GenericInvoke[Callback ~func()](callback Callback) { callback() }

func switchBreakThenExhaustivePeersRound27(a, output []float64, rows, columns int, choose, flag bool) {
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
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round27Invoke(saved)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func switchWithoutDefaultThenExhaustivePeersRound27(a, output []float64, rows, columns int, choose, flag bool) {
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
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round27GenericInvoke(saved)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func selectDefaultBreakThenExhaustivePeersRound27(a, output []float64, rows, columns int, flag bool) {
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
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round27Invoke(saved)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func forwardGotoMarkerThenExhaustivePeersRound27(a, output []float64, rows, columns int, flag bool) {
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
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round27GenericInvoke(saved)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func switchFallthroughThenExhaustivePeersRound27(a, output []float64, rows, columns int, choose, flag bool) {
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
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round27Invoke(saved)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func switchNestedLoopBreakThenExhaustivePeersRound27(a, output []float64, rows, columns int, choose, flag bool) {
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
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round27GenericInvoke(saved)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func switchTerminatingClausesThenExhaustivePeersRound27(a, output []float64, rows, columns int, stop, explode, flag bool) {
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
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round27Invoke(saved)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func switchContinueBypassesPeersRound27(a, output []float64, rows, columns int, stop, flag bool) {
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
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round27GenericInvoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func switchGotoBypassesPeersRound27(a, output []float64, rows, columns int, stop, flag bool) {
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
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round27Invoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func labeledOuterBreakBypassesPeersRound27(a, output []float64, rows, columns int, stop, flag bool) {
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
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round27GenericInvoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func blockingSelectBeforePeersRound27(a, output []float64, rows, columns int, ready <-chan struct{}, flag bool) {
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
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round27Invoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func selectContinueBypassesPeersRound27(a, output []float64, rows, columns int, ready <-chan struct{}, flag bool) {
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
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round27GenericInvoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}
