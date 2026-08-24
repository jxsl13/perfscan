//go:build go1.23

package ps6088reach

import "sync"

func nilFunctionAliasCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func nilFunctionAliasOne(n int) {
	var stop func()
	stop()
	nilFunctionAliasCandidate(n)
}

func nilFunctionAliasTwo(n int) {
	stop := (func())(nil)
	stop()
	nilFunctionAliasCandidate(n)
}

func valueSpecPrefixCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func valueSpecPrefixOne(n int) {
	var _, _ = func() int { select {} }(), func() int {
		valueSpecPrefixCandidate(n)
		return 0
	}()
}

func valueSpecPrefixTwo(n int) {
	var (
		_ = func() int { select {} }()
		_ = func() int {
			valueSpecPrefixCandidate(n)
			return 0
		}()
	)
}

func deadCandidateBeforeLoop(n int) {
	var dead map[int]int
	dead[0] = 1
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func deadCandidateCallers() {
	deadCandidateBeforeLoop(2)
	deadCandidateBeforeLoop(3)
}

func aliasedMapCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `aliasedMapCandidate creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func aliasedMapOne(n int) {
	values := map[int]int{}
	alias := values
	alias[0] = 1
	alias[1] = 2
	for range values {
		aliasedMapCandidate(n)
	}
}

func aliasedMapTwo(n int) {
	values := map[int]int{}
	alias := values
	alias[0] = 1
	alias[1] = 2
	for range values {
		aliasedMapCandidate(n)
	}
}

var packageValues map[int]int

func packageMapCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `packageMapCandidate creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func packageMapOne(n int) {
	for range packageValues {
		packageMapCandidate(n)
	}
}

func packageMapTwo(n int) {
	for range packageValues {
		packageMapCandidate(n)
	}
}

func setPackageValues() {
	packageValues = map[int]int{0: 0, 1: 1}
}

func nilArrayPointerCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `nilArrayPointerCandidate creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func nilArrayPointerOne(n int) {
	var values *[2]int
	for range values {
		nilArrayPointerCandidate(n)
	}
}

func nilArrayPointerTwo(n int) {
	var values *[2]int
	for range values {
		nilArrayPointerCandidate(n)
	}
}

func zeroArrayCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func zeroArrayOne(n int) {
	var values [0]int
	for range values {
		zeroArrayCandidate(n)
	}
}

func zeroArrayTwo(n int) {
	var values [0]int
	for range values {
		zeroArrayCandidate(n)
	}
}

func candidateInDeadSelect(ready <-chan int, n int) {
	var dead map[int]int
	select {
	case dead[0] = <-ready:
		var wg sync.WaitGroup
		for index := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				consume(index)
			}()
		}
		wg.Wait()
	}
}

func candidateInDeadSelectCallers(ready <-chan int) {
	candidateInDeadSelect(ready, 2)
	candidateInDeadSelect(ready, 3)
}

func candidateInDeadRange(n int) {
	var dead map[int]int
	for dead[0] = range []int{1} {
		var wg sync.WaitGroup
		for index := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				consume(index)
			}()
		}
		wg.Wait()
	}
}

func candidateInDeadRangeCallers() {
	candidateInDeadRange(2)
	candidateInDeadRange(3)
}

func zeroArraySlicePanicCandidate(n int) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
	return n
}

func zeroArraySlicePanicCaller(n int) {
	for range n {
		_ = []any{zeroArraySlicePanicCandidate(n), (*[0]int)(nil)[:]}
	}
}

func terminatingSwitchCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `terminatingSwitchCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func terminatingSwitchCaller(n int) {
	for range n {
		terminatingSwitchCandidate(n)
		switch {
		default:
			_ = (*[2]int)(make([]int, 1))
		}
	}
}

func fallthroughContinueCaller(n, tag int, enabled bool) {
	for range n {
		terminatingSwitchCandidate(n)
		switch tag {
		case 0:
			if enabled {
				continue
			}
			fallthrough
		default:
			panic("stop")
		}
	}
}

func terminatingSelectCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func terminatingSelectCaller(n int) {
	for range n {
		terminatingSelectCandidate(n)
		select {
		default:
			_ = (*[2]int)(make([]int, 1))
		}
	}
}

func selectContinueCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `selectContinueCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func selectContinueCaller(n int, enabled bool) {
	for range n {
		selectContinueCandidate(n)
		select {
		default:
			if enabled {
				continue
			}
			panic("stop")
		}
	}
}

func selectSiblingCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `selectSiblingCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func selectSiblingCaller(ready <-chan struct{}, n int) {
	for range n {
		select {
		case <-ready:
			selectSiblingCandidate(n)
		default:
			panic("stop")
		}
	}
}

func selectNestedTerminatingCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func selectNestedTerminatingCaller(ready <-chan struct{}, n int, enabled bool) {
	for range n {
		select {
		case <-ready:
			if enabled {
				selectNestedTerminatingCandidate(n)
				var dead map[int]int
				dead[0] = 1
			} else {
				break
			}
		default:
			panic("stop")
		}
	}
}

func selectBreakCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `selectBreakCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func selectBreakCaller(ready <-chan struct{}, n int) {
	for range n {
		select {
		case <-ready:
			selectBreakCandidate(n)
			break
			panic("unreachable")
		default:
			panic("stop")
		}
	}
}

func selectLabeledContinueCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `selectLabeledContinueCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func selectLabeledContinueCaller(ready <-chan struct{}, n int) {
Outer:
	for range n {
		for range n {
			select {
			case <-ready:
				selectLabeledContinueCandidate(n)
				for range 1 {
					continue Outer
				}
			default:
				panic("stop")
			}
		}
	}
}

func selectSwitchBreakCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `selectSwitchBreakCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func selectSwitchBreakCaller(ready <-chan struct{}, n int, enabled bool) {
	for range n {
		select {
		case <-ready:
			switch {
			case true:
				selectSwitchBreakCandidate(n)
				if enabled {
					break
				}
				fallthrough
			default:
				panic("stop")
			}
		default:
			panic("stop")
		}
	}
}

func eagerGoOperandCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func eagerGoOperandCaller(groups, n int) {
	for range groups {
		eagerGoOperandCandidate(n)
		go consume(func() int { panic("stop") }())
	}
}

func eagerDeferOperandCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func eagerDeferOperandCaller(groups, n int) {
	for range groups {
		eagerDeferOperandCandidate(n)
		defer consume(func() int { panic("stop") }())
	}
}

func suffixPostBypassCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `suffixPostBypassCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func suffixPostBypassCaller(ready <-chan struct{}, n int) {
Outer:
	for range n {
		select {
		case <-ready:
			suffixPostBypassCandidate(n)
			for index := 0; index < 1; func() { select {} }() {
				continue Outer
			}
		default:
			panic("stop")
		}
	}
}

func suffixLabeledBreakCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `suffixLabeledBreakCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func suffixLabeledBreakCaller(ready <-chan struct{}, n int) {
	for range n {
	Selected:
		select {
		case <-ready:
			suffixLabeledBreakCandidate(n)
			for {
				break Selected
			}
		default:
			panic("stop")
		}
	}
}

func suffixLocalContinuePostCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func suffixLocalContinuePostCaller(ready <-chan struct{}, n int) {
	for range n {
		select {
		case <-ready:
			suffixLocalContinuePostCandidate(n)
			for index := 0; index < 1; func() { select {} }() {
				switch {
				default:
					continue
				}
			}
		default:
			panic("stop")
		}
	}
}

func enclosingSelectBlockerCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func enclosingSelectBlockerCaller(ready <-chan struct{}, n int) {
	for range n {
		select {
		case <-ready:
			enclosingSelectBlockerCandidate(n)
		default:
			panic("stop")
		}
		func() { select {} }()
	}
}

func labeledSwitchBlockerCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func labeledSwitchBlockerCaller(ready <-chan struct{}, n int) {
	for range n {
		select {
		case <-ready:
		Done:
			switch {
			default:
				labeledSwitchBlockerCandidate(n)
				for {
					break Done
				}
			}
			func() { select {} }()
		default:
			panic("stop")
		}
	}
}

func zeroSuffixLoopCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func zeroSuffixLoopCaller(n int) {
Outer:
	for range n {
		select {
		default:
			zeroSuffixLoopCandidate(n)
			for range 0 {
				continue Outer
			}
			panic("stop")
		}
	}
}

func nonemptySuffixLoopCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `nonemptySuffixLoopCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func nonemptySuffixLoopCaller(n int) {
Outer:
	for range n {
		select {
		default:
			nonemptySuffixLoopCandidate(n)
			for range 1 {
				continue Outer
			}
			panic("unreachable")
		}
	}
}

func rangeAssignmentSuffixCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func rangeAssignmentSuffixCaller(n int) {
Outer:
	for range n {
		select {
		default:
			rangeAssignmentSuffixCandidate(n)
			for ((map[int]int)(nil))[0] = range []int{1} {
				continue Outer
			}
		}
	}
}

func safeRangeAssignmentSuffixCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `safeRangeAssignmentSuffixCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func safeRangeAssignmentSuffixCaller(n int) {
Outer:
	for range n {
		select {
		default:
			safeRangeAssignmentSuffixCandidate(n)
			var output [1]int
			for output[0] = range []int{1} {
				continue Outer
			}
		}
	}
}

func targetSingleTripLoopCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func targetSingleTripLoopCaller(n int) {
	for range n {
		for range 1 {
			targetSingleTripLoopCandidate(n)
			continue
		}
		panic("stop")
	}
}

func targetRepeatedLoopCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `targetRepeatedLoopCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func targetRepeatedLoopCaller(n int) {
	for range n {
		for range 2 {
			targetRepeatedLoopCandidate(n)
			continue
		}
		panic("stop")
	}
}

func selectReceiveLHSTerminatingCandidate(n int) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
	return 0
}

func selectReceiveLHSTerminatingCaller(output []int, ready <-chan int, n int) {
	for range n {
		select {
		case output[selectReceiveLHSTerminatingCandidate(n)] = <-ready:
			panic("stop")
			break
		default:
		}
	}
}

func selectLaterCommBlockerCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func consumeInt(value int) int { return value }

func selectLaterCommBlockerCaller(output chan<- int, n int) {
	select {
	case func() chan<- int {
		selectLaterCommBlockerCandidate(n)
		return output
	}() <- 0:
	case output <- consumeInt(func() int { select {} }()):
	}
}

func selectLaterCommBlockerCallerTwo(output chan<- int, n int) {
	select {
	case func() chan<- int {
		selectLaterCommBlockerCandidate(n)
		return output
	}() <- 0:
	case output <- consumeInt(func() int { select {} }()):
	}
}

func selectLaterCommReturningCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `selectLaterCommReturningCandidate creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func selectLaterCommReturningCaller(output chan<- int, n int) {
	select {
	case func() chan<- int {
		selectLaterCommReturningCandidate(n)
		return output
	}() <- 0:
	case output <- consumeInt(1):
	}
}

func selectLaterCommReturningCallerTwo(output chan<- int, n int) {
	select {
	case func() chan<- int {
		selectLaterCommReturningCandidate(n)
		return output
	}() <- 0:
	case output <- consumeInt(1):
	}
}

func sliceBoundsCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func sliceBoundsCaller(n int) {
	for range n {
		sliceBoundsCandidate(n)
		_ = []int{}[1:]
	}
}

func validSliceBoundsCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `validSliceBoundsCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func validSliceBoundsCaller(n int) {
	for range n {
		validSliceBoundsCandidate(n)
		_ = []int{0}[1:]
	}
}

func dynamicSliceBoundsCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func dynamicSliceBoundsCaller(n int) {
	for range n {
		dynamicSliceBoundsCandidate(n)
		_ = make([]int, n, 1)[2:n]
	}
}

func fullSliceBoundsCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func fullSliceBoundsCaller(n int) {
	for range n {
		fullSliceBoundsCandidate(n)
		_ = make([]int, n, 1)[2:n:n]
	}
}

func validDynamicSliceBoundsCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `validDynamicSliceBoundsCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func validDynamicSliceBoundsCaller(n int) {
	for range n {
		validDynamicSliceBoundsCandidate(n)
		_ = make([]int, n, n)[0:n]
	}
}

func aliasedSliceBoundsCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func aliasedSliceBoundsCaller(n int) {
	values := []int{}
	for range n {
		aliasedSliceBoundsCandidate(n)
		_ = values[1:]
	}
}

func nestedSliceBoundsCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func nestedSliceBoundsCaller(n int) {
	for range n {
		nestedSliceBoundsCandidate(n)
		_ = ([]int{})[:][1:]
	}
}

func beyondLengthSliceCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `beyondLengthSliceCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func beyondLengthSliceCaller(n int) {
	for range n {
		beyondLengthSliceCandidate(n)
		_ = make([]int, 0, 1)[:1]
	}
}

func nestedStringBoundsCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func nestedStringBoundsCaller(n int) {
	for range n {
		nestedStringBoundsCandidate(n)
		_ = ("a"[:0])[:1]
	}
}

func validNestedStringBoundsCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `validNestedStringBoundsCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func validNestedStringBoundsCaller(n int) {
	for range n {
		validNestedStringBoundsCandidate(n)
		_ = ("a"[:1])[:1]
	}
}

func aliasedBoundCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func aliasedBoundCaller(n int) {
	bound := 1
	for range n {
		aliasedBoundCandidate(n)
		_ = []int{}[bound:]
	}
}

func validAliasedBoundCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `validAliasedBoundCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func validAliasedBoundCaller(n int) {
	bound := 0
	for range n {
		validAliasedBoundCandidate(n)
		_ = []int{}[bound:]
	}
}

func aliasedMakeSizeCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func aliasedMakeSizeCaller(n int) {
	size := 0
	for range n {
		aliasedMakeSizeCandidate(n)
		_ = make([]int, size)[1:]
	}
}

func validAliasedMakeSizeCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `validAliasedMakeSizeCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func validAliasedMakeSizeCaller(n int) {
	size := 1
	for range n {
		validAliasedMakeSizeCandidate(n)
		_ = make([]int, size)[1:]
	}
}

func sliceAliasChainCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func sliceAliasChainCaller(n int) {
	base := []int{}
	values := base
	for range n {
		sliceAliasChainCandidate(n)
		_ = values[1:]
	}
}

func mutatedSliceAliasCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `mutatedSliceAliasCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func mutatedSliceAliasCaller(n int) {
	base := []int{}
	values := base
	values = append(values, 0)
	for range n {
		mutatedSliceAliasCandidate(n)
		_ = values[1:]
	}
}

func transitiveMapAliasCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `transitiveMapAliasCandidate creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func transitiveMapAliasCaller(n int) {
	base := map[int]int{}
	middle := base
	leaf := middle
	leaf[0] = 0
	leaf[1] = 0
	for range base {
		transitiveMapAliasCandidate(n)
	}
	middle[2] = 0
}

func transitiveMapAliasSecondCaller(n int) {
	transitiveMapAliasCandidate(n)
}

func invalidMakeCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func negativeMakeCaller(n int) {
	size := -1
	for range n {
		invalidMakeCandidate(n)
		_ = make([]int, size)
	}
}

func invertedMakeBoundsCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func invertedMakeBoundsCaller(n int) {
	length, capacity := 2, 1
	for range n {
		invertedMakeBoundsCandidate(n)
		_ = make([]int, length, capacity)
	}
}

func validMakeCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `validMakeCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func zeroMakeCaller(n int) {
	size := 0
	for range n {
		validMakeCandidate(n)
		_ = make([]int, size)
	}
}

func zeroDivisorCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func divideByZeroCaller(n int) {
	zero := 0
	for range n {
		zeroDivisorCandidate(n)
		_ = n / zero
	}
}

func zeroRemainderCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func remainderByZeroCaller(n int) {
	zero := 0
	for range n {
		zeroRemainderCandidate(n)
		_ = n % zero
	}
}

func validDivisorCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `validDivisorCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func validDivideCaller(n int) {
	one := 1
	for range n {
		validDivisorCandidate(n)
		_ = n / one
	}
}

func genericSliceMakeCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func genericSliceMakeCaller[S ~[]int](n int) {
	size := -1
	for range n {
		genericSliceMakeCandidate(n)
		_ = make(S, size)
	}
}

func genericChannelMakeCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func genericChannelMakeCaller[C ~chan int](n int) {
	size := -1
	for range n {
		genericChannelMakeCandidate(n)
		_ = make(C, size)
	}
}

func validGenericMakeCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `validGenericMakeCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func validGenericMakeCaller[S ~[]int](n int) {
	size := 0
	for range n {
		validGenericMakeCandidate(n)
		_ = make(S, size)
	}
}

func genericZeroDivisorCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func genericZeroDivisorCaller[T ~int](n int) {
	var zero T
	for range n {
		genericZeroDivisorCandidate(n)
		_ = T(n) / zero
	}
}

func genericZeroRemainderCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func genericZeroRemainderCaller[T ~int](n int) {
	var zero T
	for range n {
		genericZeroRemainderCandidate(n)
		_ = T(n) % zero
	}
}

func mixedNumericCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `mixedNumericCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func mixedNumericCaller[T interface{ ~int | ~float64 }](n int) {
	var zero T
	for range n {
		mixedNumericCandidate(n)
		_ = T(n) / zero
	}
}

func negativeLeftShiftCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func negativeLeftShiftCaller(n int) {
	shift := -1
	for range n {
		negativeLeftShiftCandidate(n)
		_ = n << shift
	}
}

func negativeRightShiftCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func negativeRightShiftCaller(n int) {
	shift := -1
	for range n {
		negativeRightShiftCandidate(n)
		_ = n >> shift
	}
}

func zeroShiftCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `zeroShiftCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func zeroShiftCaller(n int) {
	shift := 0
	for range n {
		zeroShiftCandidate(n)
		_ = n << shift
	}
}

func oversizedSliceMakeCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func oversizedSliceMakeCaller(n int) {
	size := uint64(1) << 63
	for range n {
		oversizedSliceMakeCandidate(n)
		_ = make([]int, size)
	}
}

func oversizedChannelMakeCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func oversizedChannelMakeCaller(n int) {
	size := uint64(1) << 63
	for range n {
		oversizedChannelMakeCandidate(n)
		_ = make(chan int, size)
	}
}

func representableUnsignedMakeCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `representableUnsignedMakeCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func representableUnsignedMakeCaller(n int) {
	size := uint64(1)
	for range n {
		representableUnsignedMakeCandidate(n)
		_ = make([]int, size)
	}
}

type intersectedSliceConstraint interface {
	~[]int | ~chan int
	~[]int | ~map[int]int
}

func intersectedSliceMakeCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func intersectedSliceMakeCaller[S intersectedSliceConstraint](n int) {
	size := -1
	for range n {
		intersectedSliceMakeCandidate(n)
		_ = make(S, size)
	}
}

func validIntersectedSliceMakeCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `validIntersectedSliceMakeCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func validIntersectedSliceMakeCaller[S intersectedSliceConstraint](n int) {
	size := 0
	for range n {
		validIntersectedSliceMakeCandidate(n)
		_ = make(S, size)
	}
}

type intersectedIntegerConstraint interface {
	~int | ~float64
	~int | ~string
}

func intersectedIntegerZeroCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func intersectedIntegerZeroCaller[T intersectedIntegerConstraint](n int) {
	var zero T
	for range n {
		intersectedIntegerZeroCandidate(n)
		_ = T(n) / zero
	}
}

type intersectedMixedNumericConstraint interface {
	~int | ~float64
	~int | ~float64 | ~string
}

func intersectedMixedNumericCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `intersectedMixedNumericCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func intersectedMixedNumericCaller[T intersectedMixedNumericConstraint](n int) {
	var zero T
	for range n {
		intersectedMixedNumericCandidate(n)
		_ = T(n) / zero
	}
}

type comparableIntegerConstraint interface {
	~int | ~[]byte
	comparable
}

func comparableIntegerZeroCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func comparableIntegerZeroCaller[T comparableIntegerConstraint](n int) {
	var zero T
	for range n {
		comparableIntegerZeroCandidate(n)
		_ = T(n) / zero
	}
}

type comparableChannelConstraint interface {
	~[]int | ~chan int
	comparable
}

func comparableChannelMakeCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func comparableChannelMakeCaller[C comparableChannelConstraint](n int) {
	size := -1
	for range n {
		comparableChannelMakeCandidate(n)
		_ = make(C, size)
	}
}

type comparableMixedNumericConstraint interface {
	~int | ~float64
	comparable
}

func comparableMixedNumericCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `comparableMixedNumericCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func comparableMixedNumericCaller[T comparableMixedNumericConstraint](n int) {
	var zero T
	for range n {
		comparableMixedNumericCandidate(n)
		_ = T(n) / zero
	}
}

type exactIntegerWithMethod int

func (exactIntegerWithMethod) mark() {}

type exactFloatWithoutMethod float64

type exactMethodIntegerConstraint interface {
	exactIntegerWithMethod | exactFloatWithoutMethod
	mark()
}

func exactMethodIntegerZeroCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func exactMethodIntegerZeroCaller[T exactMethodIntegerConstraint](n int) {
	var zero T
	for range n {
		exactMethodIntegerZeroCandidate(n)
		_ = T(n) / zero
	}
}

type exactSliceWithMethod []int

func (exactSliceWithMethod) mark() {}

type exactSliceWithoutMethod []int

type exactMethodSliceConstraint interface {
	exactSliceWithMethod | exactSliceWithoutMethod
	mark()
}

func exactMethodSliceMakeCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func exactMethodSliceMakeCaller[S exactMethodSliceConstraint](n int) {
	size := -1
	for range n {
		exactMethodSliceMakeCandidate(n)
		_ = make(S, size)
	}
}

type exactFloatWithMethod float64

func (exactFloatWithMethod) mark() {}

type exactMixedMethodNumericConstraint interface {
	exactIntegerWithMethod | exactFloatWithMethod
	mark()
}

func exactMixedMethodNumericCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `exactMixedMethodNumericCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func exactMixedMethodNumericCaller[T exactMixedMethodNumericConstraint](n int) {
	var zero T
	for range n {
		exactMixedMethodNumericCandidate(n)
		_ = T(n) / zero
	}
}

func stableAssertionCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func stableAssertionCaller(n int) {
	value := any(1)
	alias := value
	for range n {
		stableAssertionCandidate(n)
		_ = alias.(string)
	}
}

func zeroInterfaceAssertionCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func zeroInterfaceAssertionCaller(n int) {
	var value any
	for range n {
		zeroInterfaceAssertionCandidate(n)
		_ = value.(string)
	}
}

func validStableAssertionCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `validStableAssertionCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func validStableAssertionCaller(n int) {
	value := any("text")
	alias := value
	for range n {
		validStableAssertionCandidate(n)
		_ = alias.(string)
	}
}

func explicitNilInterfaceAssertionCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func explicitNilInterfaceAssertionCaller(n int) {
	value := any(nil)
	for range n {
		explicitNilInterfaceAssertionCandidate(n)
		_ = value.(*int)
	}
}

func initializedNilInterfaceAssertionCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func initializedNilInterfaceAssertionCaller(n int) {
	var value any = nil
	for range n {
		initializedNilInterfaceAssertionCandidate(n)
		_ = value.(*int)
	}
}

func nestedNilInterfaceAssertionCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func nestedNilInterfaceAssertionCaller(n int) {
	value := any(any(nil))
	for range n {
		nestedNilInterfaceAssertionCandidate(n)
		_ = value.(*int)
	}
}

func boxedTypedNilAssertionCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `boxedTypedNilAssertionCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func boxedTypedNilAssertionCaller(n int) {
	value := any((*int)(nil))
	for range n {
		boxedTypedNilAssertionCandidate(n)
		_ = value.(*int)
	}
}

type namedAssertionSlice []int

func unnamedToNamedAssertionCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func unnamedToNamedAssertionCaller(n int) {
	value := any([]int{})
	for range n {
		unnamedToNamedAssertionCandidate(n)
		_ = value.(namedAssertionSlice)
	}
}

func namedToUnnamedAssertionCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func namedToUnnamedAssertionCaller(n int) {
	value := any(namedAssertionSlice{})
	for range n {
		namedToUnnamedAssertionCandidate(n)
		_ = value.([]int)
	}
}

func identicalNamedAssertionCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `identicalNamedAssertionCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func identicalNamedAssertionCaller(n int) {
	value := any(namedAssertionSlice{})
	for range n {
		identicalNamedAssertionCandidate(n)
		_ = value.(namedAssertionSlice)
	}
}

func genericAssertionCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `genericAssertionCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func genericAssertionCaller[T int | string](n int) {
	value := any(1)
	for range n {
		genericAssertionCandidate(n)
		_ = value.(T)
	}
}

func genericCompositeAssertionCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `genericCompositeAssertionCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func genericCompositeAssertionCaller[T int | string](n int) {
	value := any([]int{})
	for range n {
		genericCompositeAssertionCandidate(n)
		_ = value.([]T)
	}
}

func genericSignatureTargetAssertionCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `genericSignatureTargetAssertionCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func genericSignatureTargetAssertionCaller[T int | string](n int) {
	value := any((func(int))(nil))
	for range n {
		genericSignatureTargetAssertionCandidate(n)
		_ = value.(func(T))
	}
}

func genericSignatureDynamicAssertionCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `genericSignatureDynamicAssertionCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func genericSignatureDynamicAssertionCaller[T int | string](n int) {
	value := any(func(T) {})
	for range n {
		genericSignatureDynamicAssertionCandidate(n)
		_ = value.(func(int))
	}
}

type genericAssertionMethodValue struct{}

func (genericAssertionMethodValue) apply(int) {}

func genericInterfaceMethodAssertionCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `genericInterfaceMethodAssertionCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func genericInterfaceMethodAssertionCaller[T int | string](n int) {
	value := any(genericAssertionMethodValue{})
	for range n {
		genericInterfaceMethodAssertionCandidate(n)
		_ = value.(interface{ apply(T) })
	}
}

func genericSliceDynamicAssertionCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `genericSliceDynamicAssertionCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func genericSliceDynamicAssertionCaller[T int | string](n int) {
	value := any([]T{})
	for range n {
		genericSliceDynamicAssertionCandidate(n)
		_ = value.([]int)
	}
}

func genericTypedNilBoxCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `genericTypedNilBoxCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func genericTypedNilBoxCaller[T ~*int](n int) {
	var zero T
	value := any(zero)
	for range n {
		genericTypedNilBoxCandidate(n)
		_ = value.(T)
	}
}

func genericExplicitNilBoxCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `genericExplicitNilBoxCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func genericExplicitNilBoxCaller[T ~*int](n int) {
	value := any(T(nil))
	for range n {
		genericExplicitNilBoxCandidate(n)
		_ = value.(T)
	}
}

func genericConvertedBoxCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `genericConvertedBoxCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func genericConvertedBoxCaller[T int | string](n int) {
	value := any(T(0))
	for range n {
		genericConvertedBoxCandidate(n)
		_ = value.(T)
	}
}

func uncomparableInterfaceComparisonCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func uncomparableInterfaceComparisonCaller(n int) {
	left := any([]int{})
	for range n {
		uncomparableInterfaceComparisonCandidate(n)
		_ = left == left
	}
}

func uncomparableInterfaceInequalityCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func uncomparableInterfaceInequalityCaller(n int) {
	for range n {
		uncomparableInterfaceInequalityCandidate(n)
		_ = any(map[int]int{}) != any(map[int]int{})
	}
}

func differentDynamicComparisonCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `differentDynamicComparisonCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func differentDynamicComparisonCaller(n int) {
	for range n {
		differentDynamicComparisonCandidate(n)
		_ = any([]int{}) == any([]string{})
	}
}

func comparableInterfaceComparisonCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `comparableInterfaceComparisonCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func comparableInterfaceComparisonCaller(n int) {
	for range n {
		comparableInterfaceComparisonCandidate(n)
		_ = any(1) == any(1)
	}
}

func nilInterfaceComparisonCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `nilInterfaceComparisonCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func nilInterfaceComparisonCaller(n int) {
	var value any
	for range n {
		nilInterfaceComparisonCandidate(n)
		_ = value == nil
	}
}

func setComparableInterface(value *any) int {
	*value = any(1)
	return 0
}

func argumentMutatedComparisonCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `argumentMutatedComparisonCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func argumentMutatedComparisonCaller(n int) {
	value := any([]int{})
	alias := value
	for range n {
		argumentMutatedComparisonCandidate(n)
		func(_ int) {
			_ = alias == alias
		}(setComparableInterface(&alias))
	}
}

func backedgeMutatedComparisonCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `backedgeMutatedComparisonCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func backedgeMutatedComparisonCaller(n int) {
	value := any([]int{})
	for index := 0; index < 3; index++ {
		if index > 0 {
			backedgeMutatedComparisonCandidate(n)
			_ = value == value
		}
		value = any(1)
	}
}

func operandMutatedComparisonCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `operandMutatedComparisonCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func operandMutatedComparisonCaller(n int) {
	value := any([]int{})
	for range n {
		operandMutatedComparisonCandidate(n)
		_ = any(func() []int {
			value = any(1)
			return nil
		}()) == value
	}
}

func stableOperandComparisonCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func stableOperandComparisonCaller(n int) {
	value := any([]int{})
	for range n {
		stableOperandComparisonCandidate(n)
		_ = any(func() []int { return nil }()) == value
	}
}

func nestedConversionComparisonCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func nestedConversionComparisonCaller(n int) {
	value := any([]int{})
	for range n {
		nestedConversionComparisonCandidate(n)
		_ = any(value) == any(value)
	}
}

func nestedComparableComparisonCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `nestedComparableComparisonCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func nestedComparableComparisonCaller(n int) {
	value := any(1)
	for range n {
		nestedComparableComparisonCandidate(n)
		_ = any(value) == any(value)
	}
}

func nestedDifferentComparisonCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `nestedDifferentComparisonCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func nestedDifferentComparisonCaller(n int) {
	left := any([]int{})
	right := any([]string{})
	for range n {
		nestedDifferentComparisonCandidate(n)
		_ = any(left) == any(right)
	}
}

func convertedAliasComparisonCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func convertedAliasComparisonCaller(n int) {
	value := any([]int{})
	alias := any(value)
	for range n {
		convertedAliasComparisonCandidate(n)
		_ = alias == alias
	}
}

func concreteSnapshotComparisonCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func concreteSnapshotComparisonCaller(n int) {
	slice := []int{}
	value := any(slice)
	for range n {
		concreteSnapshotComparisonCandidate(n)
		_ = value == value
	}
}

func mutatedSourceSnapshotComparisonCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func mutatedSourceSnapshotComparisonCaller(n int) {
	value := any([]int{})
	alias := any(value)
	value = any(1)
	for range n {
		mutatedSourceSnapshotComparisonCandidate(n)
		_ = alias == alias
	}
}

func comparableSnapshotComparisonCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `comparableSnapshotComparisonCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func comparableSnapshotComparisonCaller(n int) {
	value := any(1)
	alias := any(value)
	for range n {
		comparableSnapshotComparisonCandidate(n)
		_ = alias == alias
	}
}

func differentSnapshotComparisonCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `differentSnapshotComparisonCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func differentSnapshotComparisonCaller(n int) {
	left := any(any([]int{}))
	right := any(any([]string{}))
	for range n {
		differentSnapshotComparisonCandidate(n)
		_ = left == right
	}
}

func concretePriorUseComparisonCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func consumeComparisonSlice([]int) {}

func concretePriorUseComparisonCaller(n int) {
	slice := []int{}
	consumeComparisonSlice(slice)
	slice = append(slice, 1)
	for range n {
		concretePriorUseComparisonCandidate(n)
		_ = any(slice) == any(slice)
	}
}

func byValueInterfaceComparisonCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func consumeComparisonInterface(any) {}

func byValueInterfaceComparisonCaller(n int) {
	value := any([]int{})
	consumeComparisonInterface(value)
	for range n {
		byValueInterfaceComparisonCandidate(n)
		_ = value == value
	}
}

func addressedInterfaceComparisonCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `addressedInterfaceComparisonCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func replaceComparisonInterface(value *any) { *value = 1 }

func addressedInterfaceComparisonCaller(n int) {
	value := any([]int{})
	replaceComparisonInterface(&value)
	for range n {
		addressedInterfaceComparisonCandidate(n)
		_ = value == value
	}
}

func repeatedSnapshotRebindCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedSnapshotRebindCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func repeatedSnapshotRebindCaller(n int) {
	value := any([]int{})
	for iteration := 0; iteration < 3; iteration++ {
		alias := value
		if iteration > 0 {
			repeatedSnapshotRebindCandidate(n)
			_ = alias == alias
		}
		value = any(1)
	}
}

func repeatedStableSnapshotCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func repeatedStableSnapshotCaller(n int) {
	value := any([]int{})
	for iteration := 0; iteration < 3; iteration++ {
		alias := value
		if iteration > 0 {
			repeatedStableSnapshotCandidate(n)
			_ = alias == alias
		}
	}
}

func convertedRepeatedSnapshotCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `convertedRepeatedSnapshotCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func convertedRepeatedSnapshotCaller(n int) {
	value := any([]int{})
	for iteration := 0; iteration < 3; iteration++ {
		alias := any(value)
		if iteration > 0 {
			convertedRepeatedSnapshotCandidate(n)
			_ = alias == alias
		}
		value = any(1)
	}
}

func convertedStableSnapshotCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func convertedStableSnapshotCaller(n int) {
	value := any([]int{})
	for iteration := 0; iteration < 3; iteration++ {
		alias := any(value)
		if iteration > 0 {
			convertedStableSnapshotCandidate(n)
			_ = alias == alias
		}
	}
}

func simultaneousSnapshotCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func simultaneousSnapshotCaller(n int) {
	value := any([]int{})
	value, alias := any(1), value
	_ = value
	for range 3 {
		simultaneousSnapshotCandidate(n)
		_ = alias == alias
	}
}

func priorRebindSnapshotCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `priorRebindSnapshotCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func priorRebindSnapshotCaller(n int) {
	value := any([]int{})
	value = any(1)
	alias := value
	for range 3 {
		priorRebindSnapshotCandidate(n)
		_ = alias == alias
	}
}

func deadInterfaceWriteCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func deadInterfaceWriteCaller(n int) {
	value := any([]int{})
	if false {
		value = any(1)
	}
	for range 3 {
		deadInterfaceWriteCandidate(n)
		_ = value == value
	}
}

func postLoopInterfaceWriteCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func postLoopInterfaceWriteCaller(n int) {
	value := any([]int{})
	for range 3 {
		postLoopInterfaceWriteCandidate(n)
		_ = value == value
	}
	value = any(1)
}

func reachableInterfaceWriteCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `reachableInterfaceWriteCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func reachableInterfaceWriteCaller(n int) {
	value := any([]int{})
	for iteration := range 3 {
		reachableInterfaceWriteCandidate(n)
		if iteration == 0 {
			value = any(1)
		}
		_ = value == value
	}
}

func dormantInterfaceWriteCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func dormantInterfaceWriteCaller(n int) {
	value := any([]int{})
	_ = func() { value = any(1) }
	for range 3 {
		dormantInterfaceWriteCandidate(n)
		_ = value == value
	}
}

func iifeInterfaceWriteCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func iifeDeadInterfaceWriteCaller(n int) {
	func() {
		value := any([]int{})
		if false {
			value = any(1)
		}
		for range 3 {
			iifeInterfaceWriteCandidate(n)
			_ = any(value) == any(value)
		}
	}()
}

func iifeLiveInterfaceWriteCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `iifeLiveInterfaceWriteCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func iifeLiveInterfaceWriteCaller(n int) {
	func() {
		value := any([]int{})
		for iteration := 0; iteration < 3; iteration++ {
			if iteration > 0 {
				iifeLiveInterfaceWriteCandidate(n)
				_ = any(value) == any(value)
			}
			value = any(1)
		}
	}()
}

func outerBeforeIIFEWriteCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `outerBeforeIIFEWriteCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func outerBeforeIIFEWriteCaller(n int) {
	value := any([]int{})
	value = any(1)
	func() {
		for range 3 {
			outerBeforeIIFEWriteCandidate(n)
			_ = any(value) == any(value)
		}
	}()
}

func outerAfterIIFEWriteCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func outerAfterIIFEWriteCaller(n int) {
	value := any([]int{})
	func() {
		for range 3 {
			outerAfterIIFEWriteCandidate(n)
			_ = any(value) == any(value)
		}
	}()
	value = any(1)
}

func nestedOuterAfterIIFEWriteCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func nestedOuterAfterIIFEWriteCallerOne(n int) {
	value := any([]int{})
	func() {
		func() {
			for range 3 {
				nestedOuterAfterIIFEWriteCandidate(n)
				_ = value == value
			}
		}()
	}()
	value = any(1)
}

func nestedOuterBeforeIIFEWriteCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `nestedOuterBeforeIIFEWriteCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func nestedOuterBeforeIIFEWriteCaller(n int) {
	value := any([]int{})
	value = any(1)
	func() {
		func() {
			for range 3 {
				nestedOuterBeforeIIFEWriteCandidate(n)
				_ = value == value
			}
		}()
	}()
}

func deferredInterfaceWriteCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func deferredInterfaceWriteCaller(n int) {
	value := any([]int{})
	defer func() {
		value = any(1)
	}()
	for range 3 {
		deferredInterfaceWriteCandidate(n)
		_ = value == value
	}
}

func synchronousBeforeDeferredWriteCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `synchronousBeforeDeferredWriteCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func synchronousBeforeDeferredWriteCaller(n int) {
	value := any([]int{})
	defer func() {
		value = any([]int{})
	}()
	value = any(1)
	for range 3 {
		synchronousBeforeDeferredWriteCandidate(n)
		_ = value == value
	}
}

func deferredLIFOAfterCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func deferredLIFOAfterCaller(n int) {
	value := any([]int{})
	defer func() {
		value = any(1)
	}()
	defer func() {
		for range 3 {
			deferredLIFOAfterCandidate(n)
			_ = value == value
		}
	}()
}

func deferredLIFOBeforeCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `deferredLIFOBeforeCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func deferredLIFOBeforeCaller(n int) {
	value := any([]int{})
	defer func() {
		for range 3 {
			deferredLIFOBeforeCandidate(n)
			_ = value == value
		}
	}()
	defer func() {
		value = any(1)
	}()
}

func gotoDeferredRegistrationCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `gotoDeferredRegistrationCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func gotoDeferredRegistrationCaller(n int) {
	value := any([]int{})
	goto target
writer:
	defer func() {
		value = any(1)
	}()
	return
target:
	defer func() {
		for range 3 {
			gotoDeferredRegistrationCandidate(n)
			_ = value == value
		}
	}()
	goto writer
}

func singleTripDeferredRegistrationCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func singleTripDeferredRegistrationCaller(n int) {
	value := any([]int{})
	for range 1 {
		defer func() {
			value = any(1)
		}()
		defer func() {
			for range 3 {
				singleTripDeferredRegistrationCandidate(n)
				_ = value == value
			}
		}()
	}
}

func repeatedDeferredRegistrationCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedDeferredRegistrationCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func repeatedDeferredRegistrationCaller(n int) {
	value := any([]int{})
	for range 2 {
		defer func() {
			value = any(1)
		}()
		defer func() {
			for range 3 {
				repeatedDeferredRegistrationCandidate(n)
				_ = value == value
			}
		}()
	}
}

func singleTripWriterFirstCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `singleTripWriterFirstCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func singleTripWriterFirstCaller(n int) {
	value := any([]int{})
	for range 1 {
		defer func() {
			for range 3 {
				singleTripWriterFirstCandidate(n)
				_ = value == value
			}
		}()
		defer func() {
			value = any(1)
		}()
	}
}

func nestedScopeDeferredWriteCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func nestedScopeDeferredWriteCaller(n int) {
	value := any([]int{})
	defer func() {
		value = any(1)
	}()
	func() {
		defer func() {
			for range 3 {
				nestedScopeDeferredWriteCandidate(n)
				_ = value == value
			}
		}()
	}()
}

func nestedDeferredOwnerWriteCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func nestedDeferredOwnerWriteCaller(n int) {
	value := any([]int{})
	defer func() {
		value = any(1)
	}()
	defer func() {
		defer func() {
			for range 3 {
				nestedDeferredOwnerWriteCandidate(n)
				_ = value == value
			}
		}()
	}()
}

func nestedDeferredOwnerWriterFirstCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `nestedDeferredOwnerWriterFirstCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func nestedDeferredOwnerWriterFirstCaller(n int) {
	value := any([]int{})
	defer func() {
		defer func() {
			for range 3 {
				nestedDeferredOwnerWriterFirstCandidate(n)
				_ = value == value
			}
		}()
	}()
	defer func() {
		value = any(1)
	}()
}

func asyncNestedDeferredWriteCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `asyncNestedDeferredWriteCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func asyncNestedDeferredWriteCaller(n int) {
	value := any([]int{})
	defer func() {
		value = any(1)
	}()
	defer func() {
		go func() {
			defer func() {
				for range 3 {
					asyncNestedDeferredWriteCandidate(n)
					_ = value == value
				}
			}()
		}()
	}()
}

func asyncDirectDeferredWriteCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `asyncDirectDeferredWriteCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func asyncDirectDeferredWriteCaller(n int) {
	value := any([]int{})
	defer func() {
		value = any(1)
	}()
	defer func() {
		go func() {
			for range 3 {
				asyncDirectDeferredWriteCandidate(n)
				_ = value == value
			}
		}()
	}()
}

func alreadyClosedChannelCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func alreadyClosedChannelCaller(n int) {
	ready := make(chan struct{})
	close(ready)
	for range 3 {
		alreadyClosedChannelCandidate(n)
		close(ready)
	}
}

func reboundClosedChannelCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `reboundClosedChannelCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func reboundClosedChannelCaller(n int) {
	ready := make(chan struct{})
	close(ready)
	ready = make(chan struct{})
	for range 3 {
		reboundClosedChannelCandidate(n)
		close(ready)
	}
}

func gotoBypassedCloseCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `gotoBypassedCloseCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func gotoBypassedCloseCaller(n int) {
	ready := make(chan struct{})
	goto loop
	close(ready)
loop:
	for range 3 {
		gotoBypassedCloseCandidate(n)
		close(ready)
	}
}

func exposedClosedChannelCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `exposedClosedChannelCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func exposedClosedChannelCaller(n int) {
	ready := make(chan struct{})
	pointer := &ready
	close(ready)
	*pointer = make(chan struct{})
	for range 3 {
		exposedClosedChannelCandidate(n)
		close(ready)
	}
}

func backedgeReboundClosedChannelCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `backedgeReboundClosedChannelCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func backedgeReboundClosedChannelCaller(n int) {
	ready := make(chan struct{})
	close(ready)
	for iteration := range 3 {
		if iteration > 0 {
			backedgeReboundClosedChannelCandidate(n)
			close(ready)
		}
		ready = make(chan struct{})
	}
}

func closedChannelSendCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func closedChannelSendCaller(n int) {
	ready := make(chan int)
	close(ready)
	for range 3 {
		closedChannelSendCandidate(n)
		ready <- 1
	}
}

func closedChannelSendSnapshotCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func closedChannelSendSnapshotCaller(n int) {
	ready := make(chan int, 1)
	close(ready)
	for range 3 {
		closedChannelSendSnapshotCandidate(n)
		ready <- func() int {
			ready = make(chan int, 1)
			return 1
		}()
	}
}

func closedChannelSelectCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func closedChannelSelectCaller(n int) {
	ready := make(chan int)
	close(ready)
	for range 3 {
		closedChannelSelectCandidate(n)
		select {
		case ready <- 1:
		}
	}
}

func closedChannelSelectDefaultCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func closedChannelSelectDefaultCaller(n int) {
	ready := make(chan int)
	close(ready)
	for range 3 {
		closedChannelSelectDefaultCandidate(n)
		select {
		case ready <- 1:
		default:
		}
	}
}

func closedChannelSelectSiblingCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `closedChannelSelectSiblingCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func closedChannelSelectSiblingCaller(n int) {
	closed := make(chan int)
	close(closed)
	ready := make(chan int, 1)
	ready <- 1
	for range 3 {
		closedChannelSelectSiblingCandidate(n)
		select {
		case closed <- 1:
		case <-ready:
			ready <- 1
		}
	}
}

func closedChannelSelectSnapshotCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func closedChannelSelectSnapshotCaller(n int) {
	closed := make(chan int)
	close(closed)
	var disabled chan int
	for range 3 {
		closedChannelSelectSnapshotCandidate(n)
		select {
		case closed <- 1:
		case disabled <- func() int {
			closed = make(chan int, 1)
			return 1
		}():
		}
	}
}

func reboundChannelSendCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `reboundChannelSendCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func reboundChannelSendCaller(n int) {
	ready := make(chan int, 1)
	close(ready)
	ready = make(chan int, 1)
	for range 3 {
		reboundChannelSendCandidate(n)
		ready <- 1
		<-ready
	}
}

func unhashableMapKeyCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func unhashableMapKeyCaller(n int) {
	values := make(map[any]int)
	key := any([]int{})
	for range 3 {
		unhashableMapKeyCandidate(n)
		values[key] = 1
	}
}

func unhashableMapKeySnapshotCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func unhashableMapKeySnapshotCaller(n int) {
	values := make(map[any]int)
	key := any([]int{})
	for range 3 {
		unhashableMapKeySnapshotCandidate(n)
		values[key] = func() int {
			key = any("now comparable")
			return 1
		}()
	}
}

func unhashableEarlierStoreCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func unhashableEarlierStoreCaller(n int) {
	values := make(map[any]int)
	key := any([]int{})
	for range 3 {
		unhashableEarlierStoreCandidate(n)
		key, values[key] = any("now comparable"), 1
	}
}

func reboundMapKeyCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `reboundMapKeyCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func reboundMapKeyCaller(n int) {
	values := make(map[any]int)
	key := any([]int{})
	for iteration := range 3 {
		if iteration > 0 {
			reboundMapKeyCandidate(n)
			values[key] = 1
		}
		key = any("comparable on later iterations")
	}
}

func unhashableMapLookupCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func unhashableMapLookupCaller(n int) {
	values := make(map[any]int)
	key := any([]int{})
	for range 3 {
		unhashableMapLookupCandidate(n)
		_ = values[key]
	}
}

func unhashableMapLookupSnapshotCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func unhashableMapLookupSnapshotCaller(n int) {
	values := make(map[any]int)
	key := any([]int{})
	for range 3 {
		unhashableMapLookupSnapshotCandidate(n)
		_ = values[key] + func() int {
			key = any("now comparable")
			return 0
		}()
	}
}

func reboundMapLookupCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `reboundMapLookupCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func reboundMapLookupCaller(n int) {
	values := make(map[any]int)
	key := any([]int{})
	for iteration := range 3 {
		if iteration > 0 {
			reboundMapLookupCandidate(n)
			_ = values[key]
		}
		key = any("comparable on later iterations")
	}
}

func hashableMapLookupCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `hashableMapLookupCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func hashableMapLookupCaller(n int) {
	values := make(map[any]int)
	key := any("key")
	for range 3 {
		hashableMapLookupCandidate(n)
		_ = values[key]
	}
}

func unhashableMapDeleteCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func unhashableMapDeleteCaller(n int) {
	values := make(map[any]int)
	key := any([]int{})
	for range 3 {
		unhashableMapDeleteCandidate(n)
		delete(values, key)
	}
}

func hashableMapDeleteCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `hashableMapDeleteCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func hashableMapDeleteCaller(n int) {
	values := make(map[any]int)
	key := any("key")
	for range 3 {
		hashableMapDeleteCandidate(n)
		delete(values, key)
	}
}

func hashableMapKeyCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `hashableMapKeyCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func hashableMapKeyCaller(n int) {
	values := make(map[any]int)
	key := any("key")
	for range 3 {
		hashableMapKeyCandidate(n)
		values[key] = 1
	}
}

func recoveredClosedChannelCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `recoveredClosedChannelCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func recoveredClosedChannelCaller(n int) {
	ready := make(chan struct{})
	close(ready)
	for range 3 {
		recoveredClosedChannelCandidate(n)
		func() {
			defer func() {
				_ = recover()
			}()
			close(ready)
		}()
	}
}

func recoveredClosedChannelUnreachableTailCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `recoveredClosedChannelUnreachableTailCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func recoveredClosedChannelUnreachableTailCaller(n int) {
	ready := make(chan struct{})
	close(ready)
	for range 3 {
		recoveredClosedChannelUnreachableTailCandidate(n)
		func() {
			defer func() {
				_ = recover()
			}()
			close(ready)
			if n > 0 {
				var sink *int
				*sink = 1
			}
			switch n {
			case 1:
				consume(1)
			}
		}()
	}
}

func unrecoveredClosedChannelCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func unrecoveredClosedChannelCaller(n int) {
	ready := make(chan struct{})
	close(ready)
	for range 3 {
		unrecoveredClosedChannelCandidate(n)
		func() {
			close(ready)
		}()
	}
}

func outerRecoveredClosedChannelCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func outerRecoveredClosedChannelCaller(n int) {
	ready := make(chan struct{})
	close(ready)
	defer func() {
		_ = recover()
	}()
	for range 3 {
		outerRecoveredClosedChannelCandidate(n)
		close(ready)
	}
}

func panickingPointerRecoverStoreCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func panickingPointerRecoverStoreCaller(n int) {
	ready := make(chan struct{})
	close(ready)
	var sink *any
	for range 3 {
		panickingPointerRecoverStoreCandidate(n)
		func() {
			defer func() {
				*sink = recover()
			}()
			close(ready)
		}()
	}
}

func panickingMapRecoverStoreCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func panickingMapRecoverStoreCaller(n int) {
	ready := make(chan struct{})
	close(ready)
	var sink map[int]any
	for range 3 {
		panickingMapRecoverStoreCandidate(n)
		func() {
			defer func() {
				sink[0] = recover()
			}()
			close(ready)
		}()
	}
}

func blockingRecoveredInvocationCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func blockingRecoveredInvocationCaller(n int) {
	var stop func(int)
	for range 3 {
		blockingRecoveredInvocationCandidate(n)
		func() {
			defer func() {
				_ = recover()
			}()
			stop(func() int {
				select {}
			}())
		}()
	}
}

func returningRecoveredInvocationArgument() int {
	return 0
}

func returningRecoveredInvocationCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `returningRecoveredInvocationCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func returningRecoveredInvocationCaller(n int) {
	var stop func(int)
	for range 3 {
		returningRecoveredInvocationCandidate(n)
		func() {
			defer func() {
				_ = recover()
			}()
			stop(returningRecoveredInvocationArgument())
		}()
	}
}

func deferredNilReceiveCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func deferredNilReceiveCaller(n int) {
	for range 3 {
		deferredNilReceiveCandidate(n)
		func() {
			defer func() {
				<-((chan struct{})(nil))
			}()
		}()
	}
}

func deferredNilStoreCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func deferredNilStoreCaller(n int) {
	var sink *int
	for range 3 {
		deferredNilStoreCandidate(n)
		func() {
			defer func() {
				*sink = 1
			}()
		}()
	}
}

func nonreturnInterfaceWriteCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func nonreturnInterfaceWriteCaller(n int, stop bool) {
	value := any([]int{})
	if stop {
		panic("stop")
		value = any(1)
	}
	for range 3 {
		nonreturnInterfaceWriteCandidate(n)
		_ = value == value
	}
}

func emptySelectInterfaceWriteCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func emptySelectInterfaceWriteCaller(n int, stop bool) {
	value := any([]int{})
	if stop {
		select {}
		value = any(1)
	}
	for range 3 {
		emptySelectInterfaceWriteCandidate(n)
		_ = value == value
	}
}

func returningInterfaceWriteCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `returningInterfaceWriteCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func returningInterfaceWriteCaller(n int, update bool) {
	value := any([]int{})
	if update {
		consume(0)
		value = any(1)
	}
	for range 3 {
		returningInterfaceWriteCandidate(n)
		_ = value == value
	}
}

func oppositeGuardInterfaceWriteCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func oppositeGuardInterfaceWriteCaller(n int, flag bool) {
	value := any([]int{})
	if flag {
		value = any(1)
	}
	if !flag {
		for range 3 {
			oppositeGuardInterfaceWriteCandidate(n)
			_ = value == value
		}
	}
}

func sameGuardInterfaceWriteCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `sameGuardInterfaceWriteCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func sameGuardInterfaceWriteCaller(n int, flag bool) {
	value := any([]int{})
	if flag {
		value = any(1)
		for range 3 {
			sameGuardInterfaceWriteCandidate(n)
			_ = value == value
		}
	}
}

type interfaceGuardToggle bool

func (flag *interfaceGuardToggle) setTrue() {
	*flag = true
}

func pointerReceiverGuardWriteCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `pointerReceiverGuardWriteCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func pointerReceiverGuardWriteCaller(n int) {
	value := any([]int{})
	flag := interfaceGuardToggle(false)
	if !flag {
		value = any(1)
	}
	flag.setTrue()
	if flag {
		for range 3 {
			pointerReceiverGuardWriteCandidate(n)
			_ = value == value
		}
	}
}

func stablePointerReceiverGuardCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func stablePointerReceiverGuardCaller(n int) {
	value := any([]int{})
	flag := interfaceGuardToggle(false)
	if !flag {
		value = any(1)
	}
	if flag {
		for range 3 {
			stablePointerReceiverGuardCandidate(n)
			_ = value == value
		}
	}
}

func switchGuardInterfaceWriteCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func switchGuardInterfaceWriteCaller(n int, tag int) {
	value := any([]int{})
	switch tag {
	case 0:
		value = any(1)
	case 1:
		for range 3 {
			switchGuardInterfaceWriteCandidate(n)
			_ = value == value
		}
	}
}

func fallthroughInterfaceWriteCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `fallthroughInterfaceWriteCandidate creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func fallthroughInterfaceWriteCaller(n int, tag int) {
	value := any([]int{})
	switch tag {
	case 1:
		value = any(1)
		fallthrough
	case 2:
		for range 3 {
			fallthroughInterfaceWriteCandidate(n)
			_ = value == value
		}
	}
}

func genericMapCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func genericMapSingleTrip[M ~map[int]int](values M, n int) {
	for range values {
		genericMapCandidate(n)
		clear(values)
	}
}

func selectLHSCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func selectLHSOne(ready <-chan int, n int) {
	select {
	case ((map[int]int)(nil))[0] = <-ready:
		selectLHSCandidate(n)
	}
}

func selectLHSTwo(ready <-chan int, n int) {
	select {
	case ((map[int]int)(nil))[0] = <-ready:
		selectLHSCandidate(n)
	}
}

func rangeLHSCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func rangeLHSOne(n int) {
	for ((map[int]int)(nil))[0] = range []int{1} {
		rangeLHSCandidate(n)
	}
}

func rangeLHSTwo(n int) {
	for ((map[int]int)(nil))[0] = range []int{1} {
		rangeLHSCandidate(n)
	}
}

func gotoCandidate(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `gotoCandidate creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func gotoOne(n int) {
	goto live
	neverReturns()
live:
	consume(0)
	gotoCandidate(n)
}

func gotoTwo(n int) {
	goto live
	neverReturns()
live:
	consume(0)
	gotoCandidate(n)
}

func neverReturns() { select {} }

func consume(int) {}
