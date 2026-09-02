package ps1006

func round28Invoke(callback func()) { callback() }

func round28GenericInvoke[Callback ~func()](callback Callback) { callback() }

func internalLabeledBranchesReachPeersRound28(a, output []float64, rows, columns int, choose, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for range [1]int{} {
	localSwitch:
		switch {
		case choose:
			break localSwitch
		default:
		}
		switch {
		case choose:
		localLoop:
			for range [1]int{} {
				continue localLoop
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
			round28Invoke(saved)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func outerLabeledBreakBypassesPeersRound28(a, output []float64, rows, columns int, stop, flag bool) {
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
			round28GenericInvoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func outerLabeledContinueBypassesPeersRound28(a, output []float64, rows, columns int, stop, flag bool) {
	base := 0
	saved := func() { base += 0 }
outer:
	for range [1]int{} {
		switch {
		case stop:
			continue outer
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
			round28Invoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}
