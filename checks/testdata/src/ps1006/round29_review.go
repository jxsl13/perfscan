package ps1006

func round29Invoke(callback func()) { callback() }

func round29GenericInvoke[Callback ~func()](callback Callback) { callback() }

// directLabeledLoopsReachPeersRound29 pins the original Round29 regression:
// a direct labeled for that breaks to itself must not hide the exhaustive
// definitions that follow it. The range twin covers internal continue.
func directLabeledLoopsReachPeersRound29(a, output []float64, rows, columns int, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for range [1]int{} {
	directFor:
		for {
			break directFor
		}
	directRange:
		for range [1]int{} {
			continue directRange
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
			round29Invoke(saved)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func unlabeledAndMaybeLoopsReachPeersRound29(a, output []float64, rows, columns int, enter, flag bool, items []int) {
	base := 0
	saved := func() { base += 0 }
	for range [1]int{} {
		for {
			break
		}
	maybeFor:
		for enter {
			break maybeFor
		}
	maybeRange:
		for range items {
			continue maybeRange
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
			round29GenericInvoke(saved)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func nestedLabelsAndBranchStatementsReachPeersRound29(a, output []float64, rows, columns int, choose, stop, explode, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for range [1]int{} {
	localRange:
		for range [1]int{} {
		inner:
			for {
				break inner
			}
			continue localRange
		}
	localSwitch:
		switch {
		case choose:
			break localSwitch
		default:
		}
	localTypeSwitch:
		switch any(choose).(type) {
		case bool:
			break localTypeSwitch
		}
	localSelect:
		select {
		default:
			break localSelect
		}
	shadowed:
		for {
			func() {
			shadowed:
				for {
					break shadowed
				}
			}()
			if stop {
				return
			}
			if explode {
				panic("stop")
			}
			break shadowed
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
			round29Invoke(saved)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func zeroTripOuterTransfersDoNotBypassPeersRound29(a, output []float64, rows, columns int, flag bool) {
	base := 0
	saved := func() { base += 0 }
outer:
	for range [1]int{} {
		for false {
			break outer
		}
		for range [0]int{} {
			continue outer
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
			round29GenericInvoke(saved)
			sum += a[base+column]
		}
		output[column] = sum
	}
}

func maybeTripOuterBreakBypassesPeersRound29(a, output []float64, rows, columns int, enter, escape, flag bool) {
	base := 0
	saved := func() { base += 0 }
outer:
	for range [1]int{} {
	direct:
		for attempt := 0; enter && attempt < 1; attempt++ {
			if escape {
				break outer
			}
			break direct
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
			round29Invoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func guaranteedTripOuterContinueBypassesPeersRound29(a, output []float64, rows, columns int, escape, flag bool) {
	base := 0
	saved := func() { base += 0 }
outer:
	for range [1]int{} {
	direct:
		for range [1]int{} {
			if escape {
				continue outer
			}
			continue direct
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
			round29GenericInvoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}

func directLoopGotoBypassesPeersRound29(a, output []float64, rows, columns int, escape, flag bool) {
	base := 0
	saved := func() { base += 0 }
	for range [1]int{} {
	direct:
		for {
			if escape {
				goto afterSetup
			}
			break direct
		}
		if flag {
			saved = func() { base = 0 }
		} else {
			saved = func() { base = 0 }
		}
	}
afterSetup:
	for column := 0; column < columns; column++ {
		var sum float64
		for row := 0; row < rows; row++ {
			base = row * columns
			round29Invoke(saved)
			sum += a[base+column] // want `the inner loop variable`
		}
		output[column] = sum
	}
}
