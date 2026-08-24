//go:build go1.22

package ps6088

import (
	"os"
	"ps6088dep"
	"runtime"
	"sync"
	"unsafe"
)

func repeatedByCallSites(n int, work func(int)) {
	var wg sync.WaitGroup
	workers := min(runtime.GOMAXPROCS(0), n)
	for index := range workers {
		wg.Add(1)
		go func() { // want `repeatedByCallSites creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites; its launched body directly invokes callback parameter work.*not proof that goroutine construction is the bottleneck.*controlled order-alternating end-to-end A/B.*advisory, no automatic fix`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func callSiteOne(n int) { repeatedByCallSites(n, consume) }

func callSiteTwo(n int) { repeatedByCallSites(n, consume) }

func repeatedByInitializers(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedByInitializers creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites; its launched body directly invokes callback parameter work`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

var initializerCallOne = repeatedByInitializers(2, consume)
var initializerCallTwo = repeatedByInitializers(3, consume)

func repeatedByNestedInitializers(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedByNestedInitializers creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites; its launched body directly invokes callback parameter work`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func wrapInitializer(value int) int { return value }

var nestedInitializerCallOne = wrapInitializer(repeatedByNestedInitializers(2, consume))
var nestedInitializerCallTwo = wrapInitializer(repeatedByNestedInitializers(3, consume))

// Constant-dead nested initializer operands and deferred function-literal
// bodies do not add production call-site evidence.
var deadNestedInitializerCall = false && repeatedByNestedInitializers(4, consume) > 0
var deferredNestedInitializerCall = func() int {
	return repeatedByNestedInitializers(5, consume)
}

func repeatedByIIFEInitializers(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedByIIFEInitializers creates a fresh function-local sync.WaitGroup generation.*3 direct production call sites; its launched body directly invokes callback parameter work`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

var iifeInitializerCallOne = func() int {
	return repeatedByIIFEInitializers(2, consume)
}()

var iifeInitializerCallTwo = (func() int {
	if false {
		return repeatedByIIFEInitializers(4, consume)
	}
	return repeatedByIIFEInitializers(3, consume)
})()

type initializerFunc func() int

var convertedIIFEInitializerCall = initializerFunc(func() int {
	return repeatedByIIFEInitializers(5, consume)
})()

func repeatedByBodyIIFEs(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedByBodyIIFEs creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites; its launched body directly invokes callback parameter work`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func bodyIIFECalls(n int) {
	func() {
		if false {
			repeatedByBodyIIFEs(n, consume)
		}
		repeatedByBodyIIFEs(n, consume)
	}()
	(func() { repeatedByBodyIIFEs(n, consume) })()
}

func repeatedByOuterIIFELoop(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedByOuterIIFELoop creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body; its launched body directly invokes callback parameter work`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func outerIIFELoop(groups, n int) {
	for range groups {
		func() { repeatedByOuterIIFELoop(n, consume) }()
	}
}

func repeatedByInnerIIFELoop(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedByInnerIIFELoop creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body; its launched body directly invokes callback parameter work`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func innerIIFELoop(groups, n int) {
	func() {
		for range groups {
			repeatedByInnerIIFELoop(n, consume)
		}
	}()
}

func repeatedOnlyInMutatingOuterIIFEAfter(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func mutatingOuterIIFEAfter(groups, n int) {
	for index := 0; index < groups; index++ {
		func() {
			repeatedOnlyInMutatingOuterIIFEAfter(n, consume)
			groups = 0
		}()
	}
}

func repeatedOnlyInMutatingOuterIIFEBefore(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func mutatingOuterIIFEBefore(groups, n int) {
	for index := 0; index < groups; index++ {
		func() {
			groups = 0
			repeatedOnlyInMutatingOuterIIFEBefore(n, consume)
		}()
	}
}

func repeatedOnlyBeforeDeferredIIFEWrite(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func deferredIIFEWrite(groups, n int) {
	for index := 0; index < groups; index++ {
		func() {
			defer func() { groups = 0 }()
			repeatedOnlyBeforeDeferredIIFEWrite(n, consume)
		}()
	}
}

func zeroIIFEBound(bound *int) { *bound = 0 }

func repeatedOnlyBeforeDeferredNamedWrite(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func deferredNamedIIFEWrite(groups, n int) {
	for index := 0; index < groups; index++ {
		func() {
			defer zeroIIFEBound(&groups)
			repeatedOnlyBeforeDeferredNamedWrite(n, consume)
		}()
	}
}

func repeatedByOuterIIFEHarmlessDefer(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedByOuterIIFEHarmlessDefer creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body; its launched body directly invokes callback parameter work`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func outerIIFEHarmlessDefer(groups, n int) {
	for range groups {
		func() {
			defer func() {}()
			repeatedByOuterIIFEHarmlessDefer(n, consume)
		}()
	}
}

func repeatedOnlyBeforeIIFEGoexit(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func iifeGoexit(groups, n int) {
	for range groups {
		func() {
			repeatedOnlyBeforeIIFEGoexit(n, consume)
			runtime.Goexit()
		}()
	}
}

func repeatedOnlyBeforeIIFEExit(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func iifeExit(groups, n int) {
	for range groups {
		func() {
			repeatedOnlyBeforeIIFEExit(n, consume)
			os.Exit(0)
		}()
	}
}

func repeatedOnlyBeforeIIFEEmptySelect(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func iifeEmptySelect(groups, n int) {
	for range groups {
		func() {
			repeatedOnlyBeforeIIFEEmptySelect(n, consume)
			select {}
		}()
	}
}

func repeatedOnlyBeforeIIFEInfiniteLoop(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func iifeInfiniteLoop(groups, n int) {
	for range groups {
		func() {
			repeatedOnlyBeforeIIFEInfiniteLoop(n, consume)
			for {
			}
		}()
	}
}

func repeatedOnlyBeforeDeferredExit(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func iifeDeferredExit(groups, n int) {
	for range groups {
		func() {
			defer os.Exit(0)
			repeatedOnlyBeforeDeferredExit(n, consume)
		}()
	}
}

func repeatedByCompletingDeferredRegistration(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedByCompletingDeferredRegistration creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body; its launched body directly invokes callback parameter work`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func completingDeferredRegistrations(groups, n int) {
	for range groups {
		defer func() { repeatedByCompletingDeferredRegistration(n, consume) }()
	}
}

func repeatedOnlyInBlockedDeferredRegistration(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func blockedDeferredRegistrations(groups, n int) {
	for range groups {
		defer func() {
			repeatedOnlyInBlockedDeferredRegistration(n, consume)
			select {}
		}()
	}
}

func repeatedOnlyInExitingDeferredRegistration(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func exitingDeferredRegistrations(groups, n int) {
	for range groups {
		defer func() {
			repeatedOnlyInExitingDeferredRegistration(n, consume)
			os.Exit(0)
		}()
	}
}

func repeatedOnlyInInfiniteDeferredRegistration(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func infiniteDeferredRegistrations(groups, n int) {
	for range groups {
		defer func() {
			repeatedOnlyInInfiniteDeferredRegistration(n, consume)
			for {
			}
		}()
	}
}

func repeatedOnlyInNeverUnwoundDeferredRegistration(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func neverUnwoundDeferredRegistrations(groups, n int) {
	for range groups {
		defer func() { repeatedOnlyInNeverUnwoundDeferredRegistration(n, consume) }()
	}
	select {}
}

func repeatedByCompletingDirectDefer(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedByCompletingDirectDefer creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body; its launched body directly invokes callback parameter work`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func completingDirectDefers(groups, n int) {
	for range groups {
		defer repeatedByCompletingDirectDefer(n, consume)
	}
}

func repeatedOnlyInNeverUnwoundDirectDefer(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func neverUnwoundDirectDefers(groups, n int) {
	for range groups {
		defer repeatedOnlyInNeverUnwoundDirectDefer(n, consume)
	}
	select {}
}

func repeatedOnlyBeforeLaterBlockingDirectDefer(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func laterBlockingDirectDefer(groups, n int) {
	for range groups {
		defer repeatedOnlyBeforeLaterBlockingDirectDefer(n, consume)
	}
	defer func() { select {} }()
}

func repeatedOnlyBeforeLaterBlockingNestedDefer(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func laterBlockingNestedDefer(groups, n int) {
	for range groups {
		defer func() { repeatedOnlyBeforeLaterBlockingNestedDefer(n, consume) }()
	}
	defer func() { select {} }()
}

func repeatedOnlyBeforeSynchronousBlockingIIFE(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func synchronousBlockingIIFEAfterDefers(groups, n int) {
	for range groups {
		defer repeatedOnlyBeforeSynchronousBlockingIIFE(n, consume)
	}
	func() { select {} }()
}

func repeatedOnlyInTwoDeadDeferredSites(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func firstDeadDeferredSite(n int) {
	defer repeatedOnlyInTwoDeadDeferredSites(n, consume)
	select {}
}

func secondDeadDeferredSite(n int) {
	defer repeatedOnlyInTwoDeadDeferredSites(n, consume)
	select {}
}

func repeatedOnlyInDeadInitializerDefers(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func repeatedOnlyBeforeNonreturningSibling(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func nonreturningSiblingLoop(groups, n int) {
	for range groups {
		_ = []int{
			repeatedOnlyBeforeNonreturningSibling(n, consume),
			func() int { select {} }(),
		}
	}
}

func repeatedOnlyBeforeDirectPanicSibling(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func directPanicSiblingLoop(groups, n int) {
	for range groups {
		_ = []any{
			repeatedOnlyBeforeDirectPanicSibling(n, consume),
			(*[2]int)(make([]int, 1)),
		}
	}
}

func repeatedOnlyBeforePanicSliceSibling(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func panicSliceSiblingLoop(groups, n int) {
	for range groups {
		_ = []any{
			repeatedOnlyBeforePanicSliceSibling(n, consume),
			([]int(nil))[1:],
		}
	}
}

func repeatedOnlyBeforePanicAssertionSibling(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func panicAssertionSiblingLoop(groups, n int) {
	for range groups {
		_ = []any{
			repeatedOnlyBeforePanicAssertionSibling(n, consume),
			any(0).(string),
		}
	}
}

func repeatedOnlyInsidePanicAssertion(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func nestedPanicAssertionLoop(groups, n int) {
	for range groups {
		_ = any(repeatedOnlyInsidePanicAssertion(n, consume)).(string)
	}
}

func repeatedOnlyInsidePanicArrayConversion(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func nestedPanicArrayConversionLoop(groups, n int) {
	for range groups {
		_ = (*[2]int)([]int{repeatedOnlyInsidePanicArrayConversion(n, consume)})
	}
}

func repeatedOnlyBeforeEnclosingNonreturnIIFE(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func enclosingNonreturnIIFELoop(groups, n int) {
	for range groups {
		func(int) { select {} }(repeatedOnlyBeforeEnclosingNonreturnIIFE(n, consume))
	}
}

func repeatedOnlyAfterEarlierNonreturnIIFE(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func earlierNonreturnIIFELoop(groups, n int) {
	for range groups {
		_ = []int{
			func() int { select {} }(),
			repeatedOnlyAfterEarlierNonreturnIIFE(n, consume),
		}
	}
}

func repeatedOnlyBeforeNilChannelSend(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func nilChannelSendLoop(groups, n int) {
	for range groups {
		(chan int)(nil) <- repeatedOnlyBeforeNilChannelSend(n, consume)
	}
}

func repeatedOnlyBeforeNilMapAssignment(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func nilMapAssignmentLoop(groups, n int) {
	for range groups {
		map[int]int(nil)[0] = repeatedOnlyBeforeNilMapAssignment(n, consume)
	}
}

func repeatedOnlyInNilMapIncDecIndex(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func nilMapIncDecLoop(groups, n int) {
	for range groups {
		map[int]int(nil)[repeatedOnlyInNilMapIncDecIndex(n, consume)]++
	}
}

func repeatedOnlyBeforeNilChannelReceive(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func laterNilChannelReceiveLoop(groups, n int) {
	for range groups {
		_ = []any{
			repeatedOnlyBeforeNilChannelReceive(n, consume),
			<-(chan int)(nil),
		}
	}
}

func repeatedOnlyAfterNilChannelReceive(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func earlierNilChannelReceiveLoop(groups, n int) {
	for range groups {
		_ = []any{
			<-(chan int)(nil),
			repeatedOnlyAfterNilChannelReceive(n, consume),
		}
	}
}

func repeatedOnlyBeforeNilFunctionInvocation(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func nilFunctionInvocationLoop(groups, n int) {
	for range groups {
		(func(int))(nil)(repeatedOnlyBeforeNilFunctionInvocation(n, consume))
	}
}

func repeatedInAsyncNilFunctionGo(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedInAsyncNilFunctionGo creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func asyncNilFunctionGoLoop(groups, n int) {
	for range groups {
		go (func(int))(nil)(repeatedInAsyncNilFunctionGo(n, consume))
	}
}

func repeatedInDeferredNilFunctionCall(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedInDeferredNilFunctionCall creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func deferredNilFunctionLoop(groups, n int) {
	for range groups {
		defer (func(int))(nil)(repeatedInDeferredNilFunctionCall(n, consume))
	}
}

func repeatedInAsyncNonreturnIIFE(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedInAsyncNonreturnIIFE creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func asyncNonreturnIIFELoop(groups, n int) {
	for range groups {
		go func(int) { select {} }(repeatedInAsyncNonreturnIIFE(n, consume))
	}
}

func repeatedInDeferredNonreturnIIFE(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedInDeferredNonreturnIIFE creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func deferredNonreturnIIFELoop(groups, n int) {
	for range groups {
		defer func(int) { select {} }(repeatedInDeferredNonreturnIIFE(n, consume))
	}
}

func repeatedOnlyBeforeLaterCloseNil(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func laterCloseNilLoop(groups, n int) {
	for range groups {
		repeatedOnlyBeforeLaterCloseNil(n, consume)
		close((chan int)(nil))
	}
}

func repeatedOnlyBeforeFollowingDirectPanic(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func followingDirectPanicLoop(groups, n int) {
	for range groups {
		repeatedOnlyBeforeFollowingDirectPanic(n, consume)
		_ = ([]int(nil))[1:]
	}
}

func repeatedOnlyAfterPriorDirectPanic(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func priorDirectPanicLoop(groups, n int) {
	for range groups {
		_ = ([]int(nil))[1:]
		repeatedOnlyAfterPriorDirectPanic(n, consume)
	}
}

func repeatedOnlyBeforeNonreturnIfCondition(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func followingNonreturnIfConditionLoop(groups, n int) {
	for range groups {
		repeatedOnlyBeforeNonreturnIfCondition(n, consume)
		if func() bool { select {} }() {
		}
	}
}

func repeatedOnlyAfterNonreturnIfCondition(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func priorNonreturnIfConditionLoop(groups, n int) {
	for range groups {
		if func() bool { select {} }() {
		}
		repeatedOnlyAfterNonreturnIfCondition(n, consume)
	}
}

func repeatedOnlyBeforeNonreturnSwitchTag(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func followingNonreturnSwitchTagLoop(groups, n int) {
	for range groups {
		repeatedOnlyBeforeNonreturnSwitchTag(n, consume)
		switch func() int { select {} }() {
		default:
		}
	}
}

func repeatedOnlyBeforeNilChannelRange(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func followingNilChannelRangeLoop(groups, n int) {
	for range groups {
		repeatedOnlyBeforeNilChannelRange(n, consume)
		for range (chan int)(nil) {
		}
	}
}

func repeatedOnlyBeforeNonreturnSelectHeader(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func followingNonreturnSelectHeaderLoop(groups, n int) {
	for range groups {
		repeatedOnlyBeforeNonreturnSelectHeader(n, consume)
		select {
		case <-func() chan int { select {} }():
		default:
		}
	}
}

func repeatedOnlyInAllNilSelectSendRHS(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func allNilSelectSendRHSLoop(groups, n int) {
	for range groups {
		select {
		case (chan int)(nil) <- repeatedOnlyInAllNilSelectSendRHS(n, consume):
		}
	}
}

func repeatedInNilSelectSendRHS(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedInNilSelectSendRHS creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func nilSelectSendRHSLoop(groups, n int) {
	for range groups {
		select {
		case (chan int)(nil) <- repeatedInNilSelectSendRHS(n, consume):
		default:
		}
	}
}

func repeatedOnlyInDisabledNilReceiveCase(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func disabledNilReceiveCaseLoop(groups, n int) {
	for range groups {
		select {
		case <-(chan int)(nil):
			repeatedOnlyInDisabledNilReceiveCase(n, consume)
		default:
		}
	}
}

func repeatedOnlyInDisabledNilSendCase(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func disabledNilSendCaseLoop(groups, n int) {
	for range groups {
		select {
		case (chan int)(nil) <- 1:
			repeatedOnlyInDisabledNilSendCase(n, consume)
		default:
		}
	}
}

func repeatedOnlyAfterNilPointerLvalue(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func nilPointerLvalueLoop(groups, n int) {
	for range groups {
		(*[1]int)(nil)[0] = repeatedOnlyAfterNilPointerLvalue(n, consume)
	}
}

func repeatedOnlyBeforeNilArraySliceSibling(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func nilArraySliceSiblingLoop(groups, n int) {
	for range groups {
		_ = []any{
			repeatedOnlyBeforeNilArraySliceSibling(n, consume),
			(*[2]int)(nil)[:],
		}
	}
}

func repeatedBeforeUnevaluatedPanicSiblings(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedBeforeUnevaluatedPanicSiblings creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func unevaluatedPanicSiblingLoop(groups, n int) {
	for range groups {
		_ = []any{
			repeatedBeforeUnevaluatedPanicSiblings(n, consume),
			unsafe.Sizeof((*[2]int)(make([]int, 1))),
			len(*(*[2]int)(nil)),
			cap(*(*[2]int)(nil)),
		}
	}
}

func repeatedBeforeReturningSibling(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedBeforeReturningSibling creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body; its launched body directly invokes callback parameter work`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func returningSiblingLoop(groups, n int) {
	for range groups {
		_ = []int{
			repeatedBeforeReturningSibling(n, consume),
			func() int { return 1 }(),
		}
	}
}

func repeatedWithShortCircuitPanicArgument(enabled bool, n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedWithShortCircuitPanicArgument creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			if enabled {
				work(index)
			}
		}()
	}
	wg.Wait()
}

func shortCircuitPanicArgumentCallers() {
	repeatedWithShortCircuitPanicArgument(true || (*[2]int)(make([]int, 1)) != nil, 2, consume)
	repeatedWithShortCircuitPanicArgument(false && (*[2]int)(make([]int, 1)) != nil, 3, consume)
}

func repeatedWithShortCircuitIIFEArgument(enabled bool, n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedWithShortCircuitIIFEArgument creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			if enabled {
				work(index)
			}
		}()
	}
	wg.Wait()
}

func shortCircuitIIFEArgumentCallers() {
	repeatedWithShortCircuitIIFEArgument(true || func() bool { select {} }(), 2, consume)
	repeatedWithShortCircuitIIFEArgument(false && func() bool { select {} }(), 3, consume)
}

func repeatedInsideCommaOkAssertion(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedInsideCommaOkAssertion creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func commaOkAssertionLoop(groups, n int) {
	for range groups {
		_, ok := any(repeatedInsideCommaOkAssertion(n, consume)).(string)
		_ = ok
	}
}

func repeatedOnlyAfterDeadArguments(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func firstDeadArgumentSite() {
	repeatedOnlyAfterDeadArguments(func() int { select {} }(), consume)
}

func secondDeadArgumentSite() {
	repeatedOnlyAfterDeadArguments(func() int { select {} }(), consume)
}

func repeatedOnlyAfterDeadEnclosingIIFEArguments(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func firstDeadEnclosingIIFEArgumentSite(n int) {
	func(int) { repeatedOnlyAfterDeadEnclosingIIFEArguments(n, consume) }(
		func() int { select {} }(),
	)
}

func secondDeadEnclosingIIFEArgumentSite(n int) {
	func(int) { repeatedOnlyAfterDeadEnclosingIIFEArguments(n, consume) }(
		func() int { select {} }(),
	)
}

func repeatedOnlyInAliasedIIFEParameter(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func aliasedIIFEParameter(bound *int, n int) {
	func(limit *int) {
		for index := 0; index < *limit; index++ {
			repeatedOnlyInAliasedIIFEParameter(n, consume)
		}
	}(bound)
}

func repeatedOnlyInTypeSwitchIIFELoop(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func typeSwitchIIFELoop(stored any, n int) {
	switch bound := stored.(type) {
	case *int:
		func() {
			for index := 0; index < *bound; index++ {
				repeatedOnlyInTypeSwitchIIFELoop(n, consume)
			}
		}()
	}
}

func repeatedOnlyInStoredBodyLiterals(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func storedBodyLiterals(n int) {
	first := func() { repeatedOnlyInStoredBodyLiterals(n, consume) }
	second := func() { repeatedOnlyInStoredBodyLiterals(n, consume) }
	_, _ = first, second
}

func repeatedOnlyInUnsafeBodies(n int, work func(int)) unsafeInitializerValue {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return unsafeInitializerValue{}
}

func unsafeBodyCalls() {
	_ = unsafe.Sizeof(repeatedOnlyInUnsafeBodies(2, consume))
	_ = unsafe.Alignof(repeatedOnlyInUnsafeBodies(3, consume))
	_ = unsafe.Offsetof(repeatedOnlyInUnsafeBodies(4, consume).field)
}

type unsafeInitializerValue struct {
	field int
}

func repeatedOnlyInUnsafeInitializers(n int, work func(int)) unsafeInitializerValue {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return unsafeInitializerValue{}
}

// The unsafe layout builtins type-check but never evaluate their operands.
var unevaluatedSizeInitializer = unsafe.Sizeof(repeatedOnlyInUnsafeInitializers(2, consume))
var unevaluatedAlignInitializer = unsafe.Alignof(repeatedOnlyInUnsafeInitializers(2, consume))
var unevaluatedOffsetInitializer = unsafe.Offsetof(repeatedOnlyInUnsafeInitializers(2, consume).field)

func repeatedInLoop(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedInLoop creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body; its launched body directly invokes callback parameter work`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func loopCaller(groups, n int) {
	for range groups {
		repeatedInLoop(n, consume)
	}
}

// A caller loop that unconditionally exits after the call supplies only one
// invocation, not repetition evidence.
func singleTripCallerEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func singleTripCaller(groups, n int) {
	for range groups {
		singleTripCallerEvidence(n, consume)
		break
	}
}

// Writing a canonical caller loop's bound after the call prevents that call
// site from supplying repetition evidence, even though the CFG has a
// syntactic backedge.
func overwrittenCallerBoundEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func overwrittenCallerBound(groups, n int) {
	for index := 0; index < groups; index++ {
		overwrittenCallerBoundEvidence(n, consume)
		groups = 0
	}
}

// Overwriting the iteration variable likewise makes the call site single
// trip, despite the remaining syntactic backedge.
func overwrittenCallerIterationEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func overwrittenCallerIteration(groups, n int) {
	for index := 0; index < groups; index++ {
		overwrittenCallerIterationEvidence(n, consume)
		index = groups
	}
}

// A noncanonical post clause can force the loop to stop after the first call.
func overwrittenCallerPostEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func overwrittenCallerPost(groups, n int) {
	for index := 0; index < groups; groups = 0 {
		overwrittenCallerPostEvidence(n, consume)
	}
}

func oneCallerCondition(groups *int) bool {
	ready := *groups > 0
	*groups = 0
	return ready
}

// Effectful current-condition evaluation is not stable repetition evidence.
func effectfulCallerConditionEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func effectfulCallerCondition(groups, n int) {
	for oneCallerCondition(&groups) {
		effectfulCallerConditionEvidence(n, consume)
	}
}

// Writes through aliases of the caller loop's control are equally unable to
// prove repeated invocation.
func aliasedCallerBoundEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func aliasedCallerBound(groups, n int) {
	bound := &groups
	for index := 0; index < groups; index++ {
		aliasedCallerBoundEvidence(n, consume)
		*bound = 0
	}
}

// Clearing a ranged map removes every not-yet-reached entry, so the call can
// execute at most once.
func clearedMapCallerEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func clearedMapCaller(groups map[int]bool, n int) {
	for range groups {
		clearedMapCallerEvidence(n, consume)
		clear(groups)
	}
}

// The candidate joins its workers before returning, so a callback capture
// that overwrites the caller bound also makes the caller loop single trip.
func callbackCallerControlEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func callbackCallerControl(groups, n int) {
	for index := 0; index < groups; index++ {
		callbackCallerControlEvidence(n, func(int) { groups = 0 })
	}
}

// Merely reading the caller bound from the joined callback does not alter the
// caller loop's repetition evidence.
func callbackCallerReadEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `callbackCallerReadEvidence creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func callbackCallerRead(groups, n int) {
	for index := 0; index < groups; index++ {
		callbackCallerReadEvidence(n, func(int) { _ = groups })
	}
}

// A function-valued parameter may close over the caller bound outside the
// analyzed function, so it cannot establish unconditional repetition.
func opaqueCallbackCallerEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func opaqueCallbackCaller(groups, n int, stop func(int)) {
	for index := 0; index < groups; index++ {
		opaqueCallbackCallerEvidence(n, stop)
	}
}

// Deferred callback effects finish before the callback returns and therefore
// before the candidate's joined worker completes.
func deferredCallbackCallerEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func deferredCallbackCaller(groups, n int) {
	for index := 0; index < groups; index++ {
		deferredCallbackCallerEvidence(n, func(int) {
			defer func() { groups = 0 }()
		})
	}
}

type callerControlHolder struct {
	bound *int
}

func (holder callerControlHolder) stop(int) { *holder.bound = 0 }

// Reference-containing aggregates passed by value can retain a pointer to the
// caller control and mutate it from a worker that is joined before return.
func aggregateCallerControlEvidence(n int, holder callerControlHolder) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			holder.stop(index)
		}()
	}
	wg.Wait()
}

func aggregateCallerControl(groups, n int) {
	holder := callerControlHolder{bound: &groups}
	for index := 0; index < groups; index++ {
		aggregateCallerControlEvidence(n, holder)
	}
}

// A reference-bearing aggregate parameter can alias the separate control
// parameter through values supplied by this function's caller.
func aggregateParameterCallerEvidence(n int, holder callerControlHolder) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			holder.stop(index)
		}()
	}
	wg.Wait()
}

func aggregateParameterCaller(groups *int, holder callerControlHolder, n int) {
	for index := 0; index < *groups; index++ {
		aggregateParameterCallerEvidence(n, holder)
	}
}

func invokeAggregateParameterCaller(n int) {
	groups := 2
	aggregateParameterCaller(&groups, callerControlHolder{bound: &groups}, n)
}

var escapedCallerControl *int

func stopEscapedCallerControl(int) { *escapedCallerControl = 0 }

// A declared callback can still mutate a local caller control after that
// control escapes into package-global reference storage.
func escapedDeclaredCallbackEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func escapedDeclaredCallback(groups, n int) {
	escapedCallerControl = &groups
	for index := 0; index < groups; index++ {
		escapedDeclaredCallbackEvidence(n, stopEscapedCallerControl)
	}
}

var sentCallerControls = make(chan *int, 2)

func stopSentCallerControl(int) { *<-sentCallerControls = 0 }

// Sending the control reference to package-visible storage is also an escape.
func sentDeclaredCallbackEvidence(work func(int)) {
	var wg sync.WaitGroup
	for index := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func sentDeclaredCallback(groups int) {
	sentCallerControls <- &groups
	sentCallerControls <- &groups
	for index := 0; index < groups; index++ {
		sentDeclaredCallbackEvidence(stopSentCallerControl)
	}
}

var keyedCallerControls = make(map[*int]struct{})

func stopKeyedCallerControl(int) {
	for control := range keyedCallerControls {
		*control = 0
	}
}

// Storing a control address as an externally reachable map key is an escape,
// even though the assignment value itself carries no reference.
func keyedDeclaredCallbackEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func keyedDeclaredCallback(groups, n int) {
	keyedCallerControls[&groups] = struct{}{}
	for index := 0; index < groups; index++ {
		keyedDeclaredCallbackEvidence(n, stopKeyedCallerControl)
	}
}

var incrementedCallerControls = make(map[*int]int)

func stopIncrementedCallerControl(int) {
	for control := range incrementedCallerControls {
		*control = 0
	}
}

// Inc/dec map updates store their reference-bearing keys just like ordinary
// assignments do.
func incrementedKeyCallbackEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func incrementedKeyCallback(groups, n int) {
	incrementedCallerControls[&groups]++
	for index := 0; index < groups; index++ {
		incrementedKeyCallbackEvidence(n, stopIncrementedCallerControl)
	}
}

var rangedCallerControl *int

func stopRangedCallerControl(int) { *rangedCallerControl = 0 }

// Range assignment targets inherit reference identity and package-global
// destinations make that identity externally reachable.
func rangedTargetCallbackEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func rangedTargetCallback(groups, n int) {
	controls := map[*int]struct{}{&groups: {}}
	for rangedCallerControl = range controls {
		break
	}
	for index := 0; index < groups; index++ {
		rangedTargetCallbackEvidence(n, stopRangedCallerControl)
	}
}

var nestedRangedCallerControl *int

func stopNestedRangedCallerControl(int) { *nestedRangedCallerControl = 0 }

// A range alias nested inside a preceding control statement must still make
// its package-global assignment visible to the following caller loop.
func nestedRangedTargetEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func nestedRangedTarget(groups, n int) {
	controls := map[*int]struct{}{&groups: {}}
	for once := 0; once < 1; once++ {
		for nestedRangedCallerControl = range controls {
			break
		}
	}
	for index := 0; index < groups; index++ {
		nestedRangedTargetEvidence(n, stopNestedRangedCallerControl)
	}
}

// A nested preceding range can also introduce a local alias that a joined
// callback later dereferences.
func nestedLocalRangeAliasEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func nestedLocalRangeAlias(groups, n int) {
	controls := map[*int]struct{}{&groups: {}}
	var control *int
	for once := 0; once < 1; once++ {
		for control = range controls {
			break
		}
	}
	for index := 0; index < groups; index++ {
		nestedLocalRangeAliasEvidence(n, func(int) { *control = 0 })
	}
}

// Comma-ok map lookups propagate a reference-bearing value into their first
// destination while leaving the boolean destination unrelated.
func mapLookupAliasEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func mapLookupAlias(groups, n int) {
	controls := map[string]*int{"groups": &groups}
	control, ok := controls["groups"]
	_ = ok
	for index := 0; index < groups; index++ {
		mapLookupAliasEvidence(n, func(int) { *control = 0 })
	}
}

// Comma-ok assertions propagate reference identity through the asserted
// value, not through the success flag.
func assertionAliasEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func assertionAlias(groups, n int) {
	var stored any = &groups
	control, ok := stored.(*int)
	_ = ok
	for index := 0; index < groups; index++ {
		assertionAliasEvidence(n, func(int) { *control = 0 })
	}
}

// Type-switch case bindings are implicit per-case objects. Calls nested in a
// type switch therefore do not use enclosing-loop repetition evidence.
func typeSwitchAliasEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func typeSwitchAlias(groups, n int) {
	var stored any = &groups
	switch control := stored.(type) {
	case *int:
		for index := 0; index < groups; index++ {
			typeSwitchAliasEvidence(n, func(int) { *control = 0 })
		}
	}
}

// Opaque multi-result calls conservatively propagate reference-bearing result
// slots because a local function value can return a captured control.
func multiCallAliasEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func multiCallAlias(groups, n int) {
	getter := func() (*int, bool) { return &groups, true }
	control, ok := getter()
	_ = ok
	for index := 0; index < groups; index++ {
		multiCallAliasEvidence(n, func(int) { *control = 0 })
	}
}

// Local map keys captured by an audited callback carry the same alias through
// the callback's range variable.
func keyedClosureCallbackEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func keyedClosureCallback(groups, n int) {
	controls := make(map[*int]struct{})
	controls[&groups] = struct{}{}
	for index := 0; index < groups; index++ {
		keyedClosureCallbackEvidence(n, func(int) {
			for control := range controls {
				*control = 0
			}
		})
	}
}

var conditionalCallerControl *int

func stopConditionalCallerControl(int) { *conditionalCallerControl = 0 }

// Escape state from either side of a dynamic branch must survive the merge.
func conditionalEscapeCallbackEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func conditionalEscapeCallback(enabled bool, groups, n int) {
	if enabled {
		conditionalCallerControl = &groups
	}
	for index := 0; index < groups; index++ {
		conditionalEscapeCallbackEvidence(n, stopConditionalCallerControl)
	}
}

var externalCallerControl *int

func stopExternalCallerControl(int) { *externalCallerControl = 0 }

// A reference-valued control parameter may already alias package state before
// this function begins, so declared callbacks are not transparent here.
func externalAliasCallbackEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func externalAliasCallback(groups *int, n int) {
	for index := 0; index < *groups; index++ {
		externalAliasCallbackEvidence(n, stopExternalCallerControl)
	}
}

func invokeExternalAliasCallback(n int) {
	groups := 2
	externalCallerControl = &groups
	externalAliasCallback(&groups, n)
}

// Receiving from the ranged channel can drain its only remaining element and
// leave just one candidate invocation.
func drainedChannelCallerEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func drainedChannelCaller(groups chan int, n int) {
	for range groups {
		drainedChannelCallerEvidence(n, consume)
		<-groups
	}
}

// Unrelated body writes do not suppress genuine caller-loop evidence.
func unrelatedCallerWriteEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `unrelatedCallerWriteEvidence creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func unrelatedCallerWrite(groups, n int) {
	unrelated := 1
	for index := 0; index < groups; index++ {
		unrelatedCallerWriteEvidence(n, consume)
		unrelated = 0
	}
	_ = unrelated
}

func singleTripTrueCallerEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func singleTripTrueCaller(groups, n int) {
	for range groups {
		singleTripTrueCallerEvidence(n, consume)
		if true {
			break
		}
	}
}

func conditionalCallerEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `conditionalCallerEvidence creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func conditionalLoopCaller(groups, n int, stop func() bool) {
	for range groups {
		conditionalCallerEvidence(n, consume)
		if stop() {
			break
		}
	}
}

func nestedSingleTripEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func nestedSingleTripCaller(n int) {
	for {
		for {
			nestedSingleTripEvidence(n, consume)
			break
		}
		break
	}
}

func nestedOuterRepeatEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `nestedOuterRepeatEvidence creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func nestedOuterRepeatCaller(groups, n int) {
	for range groups {
		for {
			nestedOuterRepeatEvidence(n, consume)
			break
		}
	}
}

func labeledSingleTripEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func labeledSingleTripCaller(n int) {
outer:
	for range 1 {
		for {
			labeledSingleTripEvidence(n, consume)
			continue outer
		}
	}
}

func labeledOuterRepeatEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `labeledOuterRepeatEvidence creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func labeledOuterRepeatCaller(groups, n int) {
outer:
	for range groups {
		for {
			labeledOuterRepeatEvidence(n, consume)
			continue outer
		}
	}
}

func panicSuffixEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func panicSuffixCaller(groups, n int) {
	for range groups {
		panicSuffixEvidence(n, consume)
		(panic("stop"))
	}
}

func emptySelectSuffixEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func emptySelectSuffixCaller(groups, n int) {
	for range groups {
		emptySelectSuffixEvidence(n, consume)
		select {}
	}
}

func osExitSuffixEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func osExitSuffixCaller(groups, n int) {
	for range groups {
		osExitSuffixEvidence(n, consume)
		(os.Exit(0))
	}
}

func barrierAfterParenthesizedExit(n int, work func(int)) {
	(os.Exit(0))
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func barrierAfterExitOne(n int) { barrierAfterParenthesizedExit(n, consume) }

func barrierAfterExitTwo(n int) { barrierAfterParenthesizedExit(n, consume) }

func callAfterExitEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func callAfterExitCaller(n int) {
	callAfterExitEvidence(n, consume)
	(os.Exit(0))
	callAfterExitEvidence(n, consume)
}

func goexitSuffixEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func goexitSuffixCaller(groups, n int) {
	for range groups {
		goexitSuffixEvidence(n, consume)
		(runtime.Goexit())
	}
}

// Parentheses around a typed non-returning call inside opaque control flow do
// not turn the post-call loop edge into repetition evidence.
func opaqueExitSuffixEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func opaqueExitSuffixCaller(groups, n int) {
	for range groups {
		opaqueExitSuffixEvidence(n, consume)
		switch {
		default:
			(os.Exit(0))
		}
	}
}

// A goto can bypass a lexically preceding non-returning expression. Both the
// barrier and caller remain live, and the caller's loop can reach its backedge.
func gotoBypassedExitEvidence(n int, work func(int)) {
	goto barrier
	(os.Exit(0))
barrier:
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `gotoBypassedExitEvidence creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func gotoBypassedExitCaller(groups, n int) {
	for range groups {
		goto call
		(runtime.Goexit())
	call:
		gotoBypassedExitEvidence(n, consume)
	}
}

func repeatedOnlyAfterDeadGotoAndPanic(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func deadGotoDoesNotBypassPanic(groups, n int) {
	for range groups {
		_ = ([]int(nil))[1:]
		if false {
			goto call
		}
	call:
		repeatedOnlyAfterDeadGotoAndPanic(n, consume)
	}
}

// Constant control flow after a nested call proves that the exact call block
// cannot cycle, even though the enclosing loop and outer condition can repeat.
func nestedConstantBreakEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func nestedConstantBreakCaller(groups, n int, enabled bool) {
outer:
	for range groups {
		if enabled {
			{
				nestedConstantBreakEvidence(n, consume)
				if true {
					break outer
				}
			}
		}
	}
}

func constantSwitchBreakEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func constantSwitchBreakCaller(groups, n int) {
outer:
	for range groups {
		constantSwitchBreakEvidence(n, consume)
		switch 1 {
		case 1:
			break outer
		default:
			consume(n)
		}
	}
}

func booleanSwitchBreakEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func booleanSwitchBreakCaller(groups, n int) {
outer:
	for range groups {
		booleanSwitchBreakEvidence(n, consume)
		switch false {
		case false:
			break outer
		}
	}
}

func booleanSwitchRepeatEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `booleanSwitchRepeatEvidence creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func booleanSwitchRepeatCaller(groups, n int) {
outer:
	for range groups {
		booleanSwitchRepeatEvidence(n, consume)
		switch false {
		case true:
			break outer
		}
	}
}

func shortCircuitBreakEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func shortCircuitBreakCaller(groups, n int, enabled bool) {
outer:
	for range groups {
		shortCircuitBreakEvidence(n, consume)
		if !(false && enabled) {
			break outer
		}
	}
}

func shortCircuitRepeatEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `shortCircuitRepeatEvidence creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func shortCircuitRepeatCaller(groups, n int, enabled bool) {
outer:
	for range groups {
		shortCircuitRepeatEvidence(n, consume)
		if false && enabled {
			break outer
		}
	}
}

func gotoCycleEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `gotoCycleEvidence creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func gotoCycleCaller(n int) {
	for range 1 {
	again:
		gotoCycleEvidence(n, consume)
		switch {
		default:
			goto again
		}
	}
}

func forwardGotoEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func forwardGotoCaller(groups, n int) {
	for range groups {
		forwardGotoEvidence(n, consume)
		goto done
	}
done:
	consume(n)
}

func loopThenCold(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `loopThenCold creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body and 2 direct production call sites; its launched body directly invokes callback parameter work`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func loopThenColdCallers(groups, n int) {
	for range groups {
		loopThenCold(n, consume)
	}
	loopThenCold(n, consume)
}

func fixedBody(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `fixedBody creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites.*lifecycle measurement candidate`
			defer wg.Done()
			consume(int(index))
		}()
	}
	wg.Wait()
}

func fixedCallerOne(n int) { fixedBody(n) }

func fixedCallerTwo(n int) { fixedBody(n) }

type waitGroupAlias = sync.WaitGroup

func aliasedBarrier(n int, work func(int)) {
	var wg waitGroupAlias
	for index := range n {
		wg.Add(1)
		go func() { // want `aliasedBarrier creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func aliasedCallerOne(n int) { aliasedBarrier(n, consume) }

func aliasedCallerTwo(n int) { aliasedBarrier(n, consume) }

type runner struct{}

func (*runner) repeatedMethod(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedMethod creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites; its launched body directly invokes callback parameter work`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func methodCallerOne(r *runner, n int) { r.repeatedMethod(n, consume) }

func methodCallerTwo(r *runner, n int) { (*runner).repeatedMethod(r, n, consume) }

// One cold direct production call does not establish repetition.
func singleCallOnly(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func oneCaller(n int) { singleCallOnly(n, consume) }

// An unreachable second site does not establish repeated callers.
func unreachableCallsOnly(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func oneLiveCaller(n int) {
	unreachableCallsOnly(n, consume)
	return
	unreachableCallsOnly(n, consume)
}

// An unreachable barrier shape is not a live lifecycle candidate.
func unreachableBarrier(n int, work func(int)) {
	return
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func unreachableBarrierOne(n int) { unreachableBarrier(n, consume) }

func unreachableBarrierTwo(n int) { unreachableBarrier(n, consume) }

// A compile-time false branch does not establish a second caller.
func falseBranchOnly(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func falseBranchCaller(n int) {
	falseBranchOnly(n, consume)
	if false {
		falseBranchOnly(n, consume)
	}
}

// A barrier in a statically dead branch is not a lifecycle candidate.
func falseBranchBarrier(n int, work func(int)) {
	if false {
		var wg sync.WaitGroup
		for index := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				work(index)
			}()
		}
		wg.Wait()
	}
}

func falseBarrierOne(n int) { falseBranchBarrier(n, consume) }

func falseBarrierTwo(n int) { falseBranchBarrier(n, consume) }

// A statically empty or single-iteration worker loop is not a fan-out.
func emptyFanout(work func(int)) {
	var wg sync.WaitGroup
	for index := range 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func emptyFanoutOne() { emptyFanout(consume) }

func emptyFanoutTwo() { emptyFanout(consume) }

func singleWorker(work func(int)) {
	var wg sync.WaitGroup
	for index := range 1 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func singleWorkerOne() { singleWorker(consume) }

func singleWorkerTwo() { singleWorker(consume) }

// An unconditional exit after the launch proves that only one iteration can
// reach it even when the range domain itself can be larger.
func singleTripBreak(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		break
	}
	wg.Wait()
}

func singleTripBreakOne(n int) { singleTripBreak(n, consume) }

func singleTripBreakTwo(n int) { singleTripBreak(n, consume) }

func singleTripTrueBreak(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		if true {
			break
		}
	}
	wg.Wait()
}

func singleTripTrueOne(n int) { singleTripTrueBreak(n, consume) }

func singleTripTrueTwo(n int) { singleTripTrueBreak(n, consume) }

// A conditional exit still permits multiple launches and remains a candidate.
func conditionalBreakAfterLaunch(n int, stop func(int) bool, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `conditionalBreakAfterLaunch creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			work(index)
		}()
		if stop(index) {
			break
		}
	}
	wg.Wait()
}

func conditionalBreakOne(n int) { conditionalBreakAfterLaunch(n, neverStop, consume) }

func conditionalBreakTwo(n int) { conditionalBreakAfterLaunch(n, neverStop, consume) }

func neverStop(int) bool { return false }

// A canonical loop that overwrites a condition object after launching cannot
// establish a multi-worker fan-out even though its CFG still has a backedge.
func boundWriteSingleTrip(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < n; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		n = 0
	}
	wg.Wait()
}

func boundWriteOne(n int) { boundWriteSingleTrip(n, consume) }

func boundWriteTwo(n int) { boundWriteSingleTrip(n, consume) }

func boundWriteBeforeLaunchSingleTrip(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < n; index++ {
		n = 0
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func boundWriteBeforeOne(n int) { boundWriteBeforeLaunchSingleTrip(n, consume) }

func boundWriteBeforeTwo(n int) { boundWriteBeforeLaunchSingleTrip(n, consume) }

func immediateBoundWriteSingleTrip(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < n; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		func() { n = 0 }()
	}
	wg.Wait()
}

func immediateBoundWriteOne(n int) { immediateBoundWriteSingleTrip(n, consume) }

func immediateBoundWriteTwo(n int) { immediateBoundWriteSingleTrip(n, consume) }

func parameterizedImmediateBoundWriteSingleTrip(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < n; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		func(bound *int) { *bound = 0 }(&n)
	}
	wg.Wait()
}

func parameterizedImmediateBoundWriteOne(n int) {
	parameterizedImmediateBoundWriteSingleTrip(n, consume)
}

func parameterizedImmediateBoundWriteTwo(n int) {
	parameterizedImmediateBoundWriteSingleTrip(n, consume)
}

func variadicImmediateBoundWriteSingleTrip(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < n; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		func(bounds ...*int) { *bounds[1] = 0 }(new(int), &n)
	}
	wg.Wait()
}

func variadicImmediateBoundWriteOne(n int) {
	variadicImmediateBoundWriteSingleTrip(n, consume)
}

func variadicImmediateBoundWriteTwo(n int) {
	variadicImmediateBoundWriteSingleTrip(n, consume)
}

func aliasedBoundWriteSingleTrip(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < n; index++ {
		bound := &n
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		*bound = 0
	}
	wg.Wait()
}

func aliasedBoundWriteOne(n int) { aliasedBoundWriteSingleTrip(n, consume) }

func aliasedBoundWriteTwo(n int) { aliasedBoundWriteSingleTrip(n, consume) }

func outerAliasedBoundWriteSingleTrip(n int, work func(int)) {
	bound := &n
	var wg sync.WaitGroup
	for index := 0; index < n; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		*bound = 0
	}
	wg.Wait()
}

func outerAliasedBoundWriteOne(n int) { outerAliasedBoundWriteSingleTrip(n, consume) }

func outerAliasedBoundWriteTwo(n int) { outerAliasedBoundWriteSingleTrip(n, consume) }

func initializedAliasBoundWriteSingleTrip(n int, enabled bool, work func(int)) {
	bound := new(int)
	if bound = &n; enabled {
		consume(n)
	}
	var wg sync.WaitGroup
	for index := 0; index < n; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		*bound = 0
	}
	wg.Wait()
}

func initializedAliasBoundWriteOne(n int, enabled bool) {
	initializedAliasBoundWriteSingleTrip(n, enabled, consume)
}

func initializedAliasBoundWriteTwo(n int, enabled bool) {
	initializedAliasBoundWriteSingleTrip(n, enabled, consume)
}

func ancestorInitializedAliasBoundWriteSingleTrip(n int, enabled bool, work func(int)) {
	if bound := &n; enabled {
		var wg sync.WaitGroup
		for index := 0; index < n; index++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				work(index)
			}()
			*bound = 0
		}
		wg.Wait()
	}
}

func ancestorInitializedAliasBoundWriteOne(n int, enabled bool) {
	ancestorInitializedAliasBoundWriteSingleTrip(n, enabled, consume)
}

func ancestorInitializedAliasBoundWriteTwo(n int, enabled bool) {
	ancestorInitializedAliasBoundWriteSingleTrip(n, enabled, consume)
}

// Using the iteration variable to index an unrelated write does not mutate the
// loop control object and still permits multiple launches.
func indexedWriteFanout(n int, output []int, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < n; index++ {
		wg.Add(1)
		go func() { // want `indexedWriteFanout creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			work(index)
		}()
		output[index] = index
	}
	wg.Wait()
}

func indexedWriteOne(n int, output []int) { indexedWriteFanout(n, output, consume) }

func indexedWriteTwo(n int, output []int) { indexedWriteFanout(n, output, consume) }

func pointedIndexedWriteFanout(n int, output []*int, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < n; index++ {
		wg.Add(1)
		go func() { // want `pointedIndexedWriteFanout creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			work(index)
		}()
		*output[index] = index
	}
	wg.Wait()
}

func pointedIndexedWriteOne(n int, output []*int) {
	pointedIndexedWriteFanout(n, output, consume)
}

func pointedIndexedWriteTwo(n int, output []*int) {
	pointedIndexedWriteFanout(n, output, consume)
}

type loopLimits struct {
	count int
}

type embeddedLoopLimits struct {
	*loopLimits
}

type loopLimitHolder struct {
	limit *loopLimits
}

type callLoopLimit struct {
	count int
}

type convertedCallLoopLimit struct {
	count int
}

func (limit *callLoopLimit) stop() { limit.count = 0 }

func stopCallLoopLimit(limit *callLoopLimit) { limit.count = 0 }

func stopCallLoopLimitAndReturn(limit *callLoopLimit) int {
	limit.count = 0
	return 0
}

func stopLoopLimitHolder(holder *loopLimitHolder) { holder.limit.count = 0 }

func stopUnsafeCallLoopLimit(pointer unsafe.Pointer) {
	(*callLoopLimit)(pointer).count = 0
}

func callLoopOne() int { return 1 }

func setOneAndTrue(limit *callLoopLimit) bool {
	limit.count = 1
	return true
}

func setOneRange(limit *callLoopLimit) []int {
	limit.count = 1
	return []int{0}
}

func setOneSwitch(limit *callLoopLimit) int {
	limit.count = 1
	return 1
}

var globalCallLoopLimit = 2

func stopGlobalCallLoopLimit() { globalCallLoopLimit = 0 }

func stopIntCallLoopLimit(limit *int) int {
	*limit = 0
	return 0
}

func indexDependencyWriteSingleTrip(work func(int)) {
	limits := [2]loopLimits{{count: 2}, {count: 0}}
	slot := 0
	var wg sync.WaitGroup
	for index := 0; index < limits[slot].count; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		slot = 1
	}
	wg.Wait()
}

func indexDependencyWriteOne() { indexDependencyWriteSingleTrip(consume) }

func indexDependencyWriteTwo() { indexDependencyWriteSingleTrip(consume) }

func dynamicIndexAliasWriteSingleTrip(work func(int)) {
	limits := [1]loopLimits{{count: 2}}
	slot := 0
	other := slot
	var wg sync.WaitGroup
	for index := 0; index < limits[slot].count; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		limits[other].count = 0
	}
	wg.Wait()
}

func dynamicIndexAliasWriteOne() { dynamicIndexAliasWriteSingleTrip(consume) }

func dynamicIndexAliasWriteTwo() { dynamicIndexAliasWriteSingleTrip(consume) }

func indirectSelectorWriteSingleTrip(limit *loopLimits, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < (*limit).count; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		limit.count = 0
	}
	wg.Wait()
}

func indirectSelectorWriteOne(limit *loopLimits) {
	indirectSelectorWriteSingleTrip(limit, consume)
}

func indirectSelectorWriteTwo(limit *loopLimits) {
	indirectSelectorWriteSingleTrip(limit, consume)
}

func indexedSelectorWriteSingleTrip(limits [1]loopLimits, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < limits[0].count; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		limits[0].count = 0
	}
	wg.Wait()
}

func indexedSelectorWriteOne(limits [1]loopLimits) {
	indexedSelectorWriteSingleTrip(limits, consume)
}

func indexedSelectorWriteTwo(limits [1]loopLimits) {
	indexedSelectorWriteSingleTrip(limits, consume)
}

func aliasedSelectorWriteSingleTrip(limit *loopLimits, work func(int)) {
	alias := limit
	var wg sync.WaitGroup
	for index := 0; index < limit.count; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		alias.count = 0
	}
	wg.Wait()
}

func aliasedSelectorWriteOne(limit *loopLimits) {
	aliasedSelectorWriteSingleTrip(limit, consume)
}

func aliasedSelectorWriteTwo(limit *loopLimits) {
	aliasedSelectorWriteSingleTrip(limit, consume)
}

func sliceAliasWriteSingleTrip(work func(int)) {
	limits := []loopLimits{{count: 2}}
	other := limits
	var wg sync.WaitGroup
	for index := 0; index < limits[0].count; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		other[0].count = 0
	}
	wg.Wait()
}

func sliceAliasWriteOne() { sliceAliasWriteSingleTrip(consume) }

func sliceAliasWriteTwo() { sliceAliasWriteSingleTrip(consume) }

func sliceParameterAliasWriteSingleTrip(limits, other []loopLimits, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < limits[0].count; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		other[0].count = 0
	}
	wg.Wait()
}

func sliceParameterAliasWriteOne(limits, other []loopLimits) {
	sliceParameterAliasWriteSingleTrip(limits, other, consume)
}

func sliceParameterAliasWriteTwo(limits, other []loopLimits) {
	sliceParameterAliasWriteSingleTrip(limits, other, consume)
}

func sliceAggregateAliasWriteSingleTrip(limits, other []loopLimits, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < limits[0].count; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		other[0] = loopLimits{}
	}
	wg.Wait()
}

func sliceAggregateAliasWriteOne(limits, other []loopLimits) {
	sliceAggregateAliasWriteSingleTrip(limits, other, consume)
}

func sliceAggregateAliasWriteTwo(limits, other []loopLimits) {
	sliceAggregateAliasWriteSingleTrip(limits, other, consume)
}

func mapAliasWriteSingleTrip(work func(int)) {
	limits := map[int]*loopLimits{0: {count: 2}}
	other := limits
	var wg sync.WaitGroup
	for index := 0; index < limits[0].count; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		other[0].count = 0
	}
	wg.Wait()
}

func mapAliasWriteOne() { mapAliasWriteSingleTrip(consume) }

func mapAliasWriteTwo() { mapAliasWriteSingleTrip(consume) }

func pointerAggregateAliasWriteSingleTrip(limit, other *loopLimits, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < limit.count; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		*other = loopLimits{}
	}
	wg.Wait()
}

func pointerAggregateAliasWriteOne(limit, other *loopLimits) {
	pointerAggregateAliasWriteSingleTrip(limit, other, consume)
}

func pointerAggregateAliasWriteTwo(limit, other *loopLimits) {
	pointerAggregateAliasWriteSingleTrip(limit, other, consume)
}

func indexedPointerConvergenceSingleTrip(work func(int)) {
	shared := &loopLimits{count: 2}
	limits := []*loopLimits{shared, shared}
	var wg sync.WaitGroup
	for index := 0; index < limits[0].count; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		limits[1].count = 0
	}
	wg.Wait()
}

func indexedPointerConvergenceOne() { indexedPointerConvergenceSingleTrip(consume) }

func indexedPointerConvergenceTwo() { indexedPointerConvergenceSingleTrip(consume) }

func holderPointerConvergenceSingleTrip(work func(int)) {
	shared := &loopLimits{count: 2}
	limit := loopLimitHolder{limit: shared}
	other := loopLimitHolder{limit: shared}
	var wg sync.WaitGroup
	for index := 0; index < limit.limit.count; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		other.limit.count = 0
	}
	wg.Wait()
}

func holderPointerConvergenceOne() { holderPointerConvergenceSingleTrip(consume) }

func holderPointerConvergenceTwo() { holderPointerConvergenceSingleTrip(consume) }

func genericSliceAliasWriteSingleTrip[S ~[]int](limits S, work func(int)) {
	other := limits
	var wg sync.WaitGroup
	for index := 0; index < limits[0]; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		other[0] = 0
	}
	wg.Wait()
}

func genericSliceAliasWriteOne(limits []int) {
	genericSliceAliasWriteSingleTrip(limits, consume)
}

func genericSliceAliasWriteTwo(limits []int) {
	genericSliceAliasWriteSingleTrip(limits, consume)
}

func methodCallBoundWriteSingleTrip(limit *callLoopLimit, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < limit.count; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		limit.stop()
	}
	wg.Wait()
}

func methodCallBoundWriteOne(limit *callLoopLimit) {
	methodCallBoundWriteSingleTrip(limit, consume)
}

func methodCallBoundWriteTwo(limit *callLoopLimit) {
	methodCallBoundWriteSingleTrip(limit, consume)
}

func namedCallBoundWriteSingleTrip(limit *callLoopLimit, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < limit.count; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		stopCallLoopLimit(limit)
	}
	wg.Wait()
}

func namedCallBoundWriteOne(limit *callLoopLimit) {
	namedCallBoundWriteSingleTrip(limit, consume)
}

func namedCallBoundWriteTwo(limit *callLoopLimit) {
	namedCallBoundWriteSingleTrip(limit, consume)
}

func clearCallBoundWriteSingleTrip(work func(int)) {
	limits := map[int]int{0: 1, 1: 2}
	var wg sync.WaitGroup
	for index := 0; index < len(limits); index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		clear(limits)
	}
	wg.Wait()
}

func clearCallBoundWriteOne() { clearCallBoundWriteSingleTrip(consume) }

func clearCallBoundWriteTwo() { clearCallBoundWriteSingleTrip(consume) }

func closureCallBoundWriteSingleTrip(n int, work func(int)) {
	stop := func() { n = 0 }
	var wg sync.WaitGroup
	for index := 0; index < n; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		stop()
	}
	wg.Wait()
}

func closureCallBoundWriteOne(n int) { closureCallBoundWriteSingleTrip(n, consume) }

func closureCallBoundWriteTwo(n int) { closureCallBoundWriteSingleTrip(n, consume) }

func globalCallBoundWriteSingleTrip(work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < globalCallLoopLimit; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		stopGlobalCallLoopLimit()
	}
	wg.Wait()
}

func globalCallBoundWriteOne() { globalCallBoundWriteSingleTrip(consume) }

func globalCallBoundWriteTwo() { globalCallBoundWriteSingleTrip(consume) }

func immediateArgumentCallBoundWriteSingleTrip(n int, work func(int)) {
	func(int) {}(stopIntCallLoopLimit(&n))
	var wg sync.WaitGroup
	for index := 0; index < n; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func immediateArgumentCallBoundWriteOne(n int) {
	immediateArgumentCallBoundWriteSingleTrip(n, consume)
}

func immediateArgumentCallBoundWriteTwo(n int) {
	immediateArgumentCallBoundWriteSingleTrip(n, consume)
}

func bodyArgumentCallBoundWriteSingleTrip(limit *callLoopLimit, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < limit.count; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		func(int) {}(stopCallLoopLimitAndReturn(limit))
	}
	wg.Wait()
}

func bodyArgumentCallBoundWriteOne(limit *callLoopLimit) {
	bodyArgumentCallBoundWriteSingleTrip(limit, consume)
}

func bodyArgumentCallBoundWriteTwo(limit *callLoopLimit) {
	bodyArgumentCallBoundWriteSingleTrip(limit, consume)
}

func nestedImmediateArgumentCallBoundWriteSingleTrip(limit *callLoopLimit, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < limit.count; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		func(int) {}(func() int {
			limit.count = 0
			return 0
		}())
	}
	wg.Wait()
}

func nestedImmediateArgumentCallBoundWriteOne(limit *callLoopLimit) {
	nestedImmediateArgumentCallBoundWriteSingleTrip(limit, consume)
}

func nestedImmediateArgumentCallBoundWriteTwo(limit *callLoopLimit) {
	nestedImmediateArgumentCallBoundWriteSingleTrip(limit, consume)
}

func holderAddressCallBoundWriteSingleTrip(limit *loopLimits, work func(int)) {
	holder := loopLimitHolder{limit: limit}
	var wg sync.WaitGroup
	for index := 0; index < limit.count; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		stopLoopLimitHolder(&holder)
	}
	wg.Wait()
}

func holderAddressCallBoundWriteOne(limit *loopLimits) {
	holderAddressCallBoundWriteSingleTrip(limit, consume)
}

func holderAddressCallBoundWriteTwo(limit *loopLimits) {
	holderAddressCallBoundWriteSingleTrip(limit, consume)
}

func importedGlobalCallBoundWriteSingleTrip(work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < ps6088dep.GlobalLimit; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		ps6088dep.Stop()
	}
	wg.Wait()
}

func importedGlobalCallBoundWriteOne() {
	importedGlobalCallBoundWriteSingleTrip(consume)
}

func importedGlobalCallBoundWriteTwo() {
	importedGlobalCallBoundWriteSingleTrip(consume)
}

func unsafeCallBoundWriteSingleTrip(limit *callLoopLimit, work func(int)) {
	pointer := unsafe.Pointer(limit)
	var wg sync.WaitGroup
	for index := 0; index < limit.count; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		stopUnsafeCallLoopLimit(pointer)
	}
	wg.Wait()
}

func unsafeCallBoundWriteOne(limit *callLoopLimit) {
	unsafeCallBoundWriteSingleTrip(limit, consume)
}

func unsafeCallBoundWriteTwo(limit *callLoopLimit) {
	unsafeCallBoundWriteSingleTrip(limit, consume)
}

func convertedUnsafeBoundWriteSingleTrip(limit *callLoopLimit, work func(int)) {
	other := (*convertedCallLoopLimit)(unsafe.Pointer(limit))
	var wg sync.WaitGroup
	for index := 0; index < limit.count; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		other.count = 0
	}
	wg.Wait()
}

func convertedUnsafeBoundWriteOne(limit *callLoopLimit) {
	convertedUnsafeBoundWriteSingleTrip(limit, consume)
}

func convertedUnsafeBoundWriteTwo(limit *callLoopLimit) {
	convertedUnsafeBoundWriteSingleTrip(limit, consume)
}

func arithmeticUnsafeBoundWriteSingleTrip(limit *callLoopLimit, work func(int)) {
	other := (*convertedCallLoopLimit)(unsafe.Pointer(uintptr(unsafe.Pointer(limit)) + 0))
	var wg sync.WaitGroup
	for index := 0; index < limit.count; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		other.count = 0
	}
	wg.Wait()
}

func arithmeticUnsafeBoundWriteOne(limit *callLoopLimit) {
	arithmeticUnsafeBoundWriteSingleTrip(limit, consume)
}

func arithmeticUnsafeBoundWriteTwo(limit *callLoopLimit) {
	arithmeticUnsafeBoundWriteSingleTrip(limit, consume)
}

func minOneFanout(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < min(n, 1); index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func minOneFanoutOne(n int) { minOneFanout(n, consume) }

func minOneFanoutTwo(n int) { minOneFanout(n, consume) }

func convertedMinOneFanout(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < int(min(n, 1)); index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func convertedMinOneFanoutOne(n int) { convertedMinOneFanout(n, consume) }

func convertedMinOneFanoutTwo(n int) { convertedMinOneFanout(n, consume) }

func namedOneFanout(work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < callLoopOne(); index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func namedOneFanoutOne() { namedOneFanout(consume) }

func namedOneFanoutTwo() { namedOneFanout(consume) }

func minTwoFanout(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < min(n, 2); index++ {
		wg.Add(1)
		go func() { // want `minTwoFanout creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func minTwoFanoutOne(n int) { minTwoFanout(n, consume) }

func minTwoFanoutTwo(n int) { minTwoFanout(n, consume) }

func enclosingIfCallBoundSingleTrip(limit *callLoopLimit, work func(int)) {
	if setOneAndTrue(limit) {
		var wg sync.WaitGroup
		for index := 0; index < limit.count; index++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				work(index)
			}()
		}
		wg.Wait()
	}
}

func enclosingIfCallBoundOne(limit *callLoopLimit) {
	enclosingIfCallBoundSingleTrip(limit, consume)
}

func enclosingIfCallBoundTwo(limit *callLoopLimit) {
	enclosingIfCallBoundSingleTrip(limit, consume)
}

func enclosingForCallBoundSingleTrip(limit *callLoopLimit, work func(int)) {
	for setOneAndTrue(limit) {
		var wg sync.WaitGroup
		for index := 0; index < limit.count; index++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				work(index)
			}()
		}
		wg.Wait()
		break
	}
}

func enclosingForCallBoundOne(limit *callLoopLimit) {
	enclosingForCallBoundSingleTrip(limit, consume)
}

func enclosingForCallBoundTwo(limit *callLoopLimit) {
	enclosingForCallBoundSingleTrip(limit, consume)
}

func enclosingRangeCallBoundSingleTrip(limit *callLoopLimit, work func(int)) {
	for range setOneRange(limit) {
		var wg sync.WaitGroup
		for index := 0; index < limit.count; index++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				work(index)
			}()
		}
		wg.Wait()
	}
}

func enclosingRangeCallBoundOne(limit *callLoopLimit) {
	enclosingRangeCallBoundSingleTrip(limit, consume)
}

func enclosingRangeCallBoundTwo(limit *callLoopLimit) {
	enclosingRangeCallBoundSingleTrip(limit, consume)
}

func enclosingSwitchCallBoundSingleTrip(limit *callLoopLimit, work func(int)) {
	switch setOneSwitch(limit) {
	case 1:
		var wg sync.WaitGroup
		for index := 0; index < limit.count; index++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				work(index)
			}()
		}
		wg.Wait()
	}
}

func enclosingSwitchCallBoundOne(limit *callLoopLimit) {
	enclosingSwitchCallBoundSingleTrip(limit, consume)
}

func enclosingSwitchCallBoundTwo(limit *callLoopLimit) {
	enclosingSwitchCallBoundSingleTrip(limit, consume)
}

func precedingWriteBoundSingleTrip(limit *callLoopLimit, work func(int)) {
	limit.count = 1
	var wg sync.WaitGroup
	for index := 0; index < limit.count; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func precedingWriteBoundOne(limit *callLoopLimit) {
	precedingWriteBoundSingleTrip(limit, consume)
}

func precedingWriteBoundTwo(limit *callLoopLimit) {
	precedingWriteBoundSingleTrip(limit, consume)
}

func remainderBoundSingleTrip(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < n%2; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func remainderBoundOne(n int) { remainderBoundSingleTrip(n, consume) }

func remainderBoundTwo(n int) { remainderBoundSingleTrip(n, consume) }

func maskedBoundSingleTrip(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < n&1; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func maskedBoundOne(n int) { maskedBoundSingleTrip(n, consume) }

func maskedBoundTwo(n int) { maskedBoundSingleTrip(n, consume) }

func canceledBoundSingleTrip(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < n-n+1; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func canceledBoundOne(n int) { canceledBoundSingleTrip(n, consume) }

func canceledBoundTwo(n int) { canceledBoundSingleTrip(n, consume) }

func arrayLiteralProjectionSingleTrip(work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < [...]int{1}[0]; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func arrayLiteralProjectionOne() { arrayLiteralProjectionSingleTrip(consume) }

func arrayLiteralProjectionTwo() { arrayLiteralProjectionSingleTrip(consume) }

func sliceLiteralProjectionSingleTrip(work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < []int{1}[0]; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func sliceLiteralProjectionOne() { sliceLiteralProjectionSingleTrip(consume) }

func sliceLiteralProjectionTwo() { sliceLiteralProjectionSingleTrip(consume) }

func mapLiteralProjectionSingleTrip(work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < map[int]int{0: 1}[0]; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func mapLiteralProjectionOne() { mapLiteralProjectionSingleTrip(consume) }

func mapLiteralProjectionTwo() { mapLiteralProjectionSingleTrip(consume) }

func gotoInitializedBoundSingleTrip(limit *callLoopLimit, work func(int)) {
	goto initialize
fanout:
	{
		var wg sync.WaitGroup
		for index := 0; index < limit.count; index++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				work(index)
			}()
		}
		wg.Wait()
	}
	return
initialize:
	limit.count = 1
	goto fanout
}

func gotoInitializedBoundOne(limit *callLoopLimit) {
	gotoInitializedBoundSingleTrip(limit, consume)
}

func gotoInitializedBoundTwo(limit *callLoopLimit) {
	gotoInitializedBoundSingleTrip(limit, consume)
}

func switchFallthroughBoundSingleTrip(limit *callLoopLimit, work func(int)) {
	switch 0 {
	case 0:
		limit.count = 1
		fallthrough
	default:
		{
			var wg sync.WaitGroup
			for index := 0; index < limit.count; index++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					work(index)
				}()
			}
			wg.Wait()
		}
	}
}

func switchFallthroughBoundOne(limit *callLoopLimit) {
	switchFallthroughBoundSingleTrip(limit, consume)
}

func switchFallthroughBoundTwo(limit *callLoopLimit) {
	switchFallthroughBoundSingleTrip(limit, consume)
}

func promotedSelectorWriteSingleTrip(work func(int)) {
	limit := embeddedLoopLimits{loopLimits: &loopLimits{count: 2}}
	var wg sync.WaitGroup
	for index := 0; index < limit.count; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		limit.loopLimits.count = 0
	}
	wg.Wait()
}

func promotedSelectorWriteOne() { promotedSelectorWriteSingleTrip(consume) }

func promotedSelectorWriteTwo() { promotedSelectorWriteSingleTrip(consume) }

func branchReboundAliasFanout(n int, enabled bool, work func(int)) {
	bound := &n
	if enabled {
		bound = new(int)
	} else {
		bound = new(int)
	}
	var wg sync.WaitGroup
	for index := 0; index < n; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		*bound = 0
	}
	wg.Wait()
}

func branchReboundAliasOne(n int, enabled bool) {
	branchReboundAliasFanout(n, enabled, consume)
}

func branchReboundAliasTwo(n int, enabled bool) {
	branchReboundAliasFanout(n, enabled, consume)
}

func reboundAliasFanout(n int, work func(int)) {
	bound := &n
	bound = new(int)
	var wg sync.WaitGroup
	for index := 0; index < n; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		*bound = 0
	}
	wg.Wait()
}

func reboundAliasOne(n int) { reboundAliasFanout(n, consume) }

func reboundAliasTwo(n int) { reboundAliasFanout(n, consume) }

func deadAliasFanout(n int, work func(int)) {
	bound := new(int)
	if false {
		bound = &n
	}
	var wg sync.WaitGroup
	for index := 0; index < n; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		*bound = 0
	}
	wg.Wait()
}

func deadAliasOne(n int) { deadAliasFanout(n, consume) }

func deadAliasTwo(n int) { deadAliasFanout(n, consume) }

func unrelatedSelectorWriteFanout(limit, other loopLimits, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < limit.count; index++ {
		wg.Add(1)
		go func() { // want `unrelatedSelectorWriteFanout creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			work(index)
		}()
		other.count = 0
	}
	wg.Wait()
}

func selectorWriteOne(limit, other loopLimits) {
	unrelatedSelectorWriteFanout(limit, other, consume)
}

func selectorWriteTwo(limit, other loopLimits) {
	unrelatedSelectorWriteFanout(limit, other, consume)
}

func assignedEmptyFanout(work func(int)) {
	var wg sync.WaitGroup
	var index int
	for index = range 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(0)
		}()
	}
	wg.Wait()
	_ = index
}

func assignedEmptyOne() { assignedEmptyFanout(consume) }

func assignedEmptyTwo() { assignedEmptyFanout(consume) }

// A single-iteration caller loop does not establish repeated invocation.
func onceLoopEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func onceLoopCaller(n int) {
	for range 1 {
		onceLoopEvidence(n, consume)
	}
}

// Statically empty integer, string, array, slice, and map ranges do not add
// dead call sites to the one live production call.
func emptyRangeEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func emptyRangeCaller(n int) {
	emptyRangeEvidence(n, consume)
	for range 0 {
		emptyRangeEvidence(n, consume)
	}
	for range "" {
		emptyRangeEvidence(n, consume)
	}
	for range [0]int{} {
		emptyRangeEvidence(n, consume)
	}
	for range []int{} {
		emptyRangeEvidence(n, consume)
	}
	for range map[int]int{} {
		emptyRangeEvidence(n, consume)
	}
	var index int
	for index = range 0 {
		emptyRangeEvidence(n, consume)
	}
	_ = index
}

func assignedOnceEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func assignedOnceCaller(n int) {
	var index int
	for index = range 1 {
		assignedOnceEvidence(n, consume)
	}
	_ = index
}

func falsePostEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func falsePostCaller(n int) {
	falsePostEvidence(n, consume)
	for ; false; falsePostEvidence(n, consume) {
	}
}

// An enclosing dynamic loop still supplies repetition through an inner
// single-iteration loop.
func nestedLoopEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `nestedLoopEvidence creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func nestedLoopCaller(groups, n int) {
	for range groups {
		for range 1 {
			nestedLoopEvidence(n, consume)
		}
	}
}

// Constant short-circuit and switch paths do not add dead call sites.
func booleanBarrier(n int, work func(int)) bool {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return true
}

func deadExpressionCall(n int) {
	booleanBarrier(n, consume)
	_ = false && booleanBarrier(n, consume)
	switch 1 {
	case 2:
		booleanBarrier(n, consume)
	}
	if true {
		consume(n)
	} else {
		booleanBarrier(n, consume)
	}
	switch 1 {
	case 1:
		consume(n)
	case dynamicSwitchValue():
		booleanBarrier(n, consume)
	}
	switch true {
	case true:
		consume(n)
	case booleanBarrier(n, consume):
		consume(n)
	}
}

func dynamicSwitchValue() int { return 2 }

func fallthroughEvidence(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `fallthroughEvidence creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func fallthroughCallerOne(n int) { fallthroughEvidence(n, consume) }

func fallthroughCallerTwo(n int) {
	switch 1 {
	case 1:
		fallthrough
	case 2:
		fallthroughEvidence(n, consume)
	}
}

// Same-spelled local callees do not count as calls to the typed helper.
func shadowCountOnly(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func shadowRealCaller(n int) { shadowCountOnly(n, consume) }

func shadowedCallees(n int) {
	shadowCountOnly := func(int, func(int)) {}
	shadowCountOnly(n, consume)
	shadowCountOnly(n, consume)
}

// A loop condition is not on the loop body's repeated execution path.
func conditionOnly(n int, work func(int)) bool {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return false
}

func conditionCaller(n int) {
	for conditionOnly(n, consume) {
	}
}

// A call inside a closure created in a loop is not itself on that loop path.
func closureOnly(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func closureFactory(groups, n int) []func() {
	functions := make([]func(), 0, groups)
	for range groups {
		functions = append(functions, func() { closureOnly(n, consume) })
	}
	return functions
}

// Calls made only by real benchmark declarations in _test.go are not
// production repetition evidence.
func benchmarkOnly(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

// A production function's name is not enough to make it a Go benchmark.
func BenchmarkDispatch(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `BenchmarkDispatch creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func benchmarkNamedProductionOne(n int) { BenchmarkDispatch(n, consume) }

func benchmarkNamedProductionTwo(n int) { BenchmarkDispatch(n, consume) }

// Calls from _test.go are also excluded from production caller evidence.
func testCallsOnly(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

// The barrier is caller-owned rather than fresh per helper invocation.
func sharedBarrier(wg *sync.WaitGroup, n int, work func(int)) {
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func sharedOne(wg *sync.WaitGroup, n int) { sharedBarrier(wg, n, consume) }

func sharedTwo(wg *sync.WaitGroup, n int) { sharedBarrier(wg, n, consume) }

// A local barrier handed to an unknown callee is not proven private.
func escapedBarrier(n int, work func(int)) {
	var wg sync.WaitGroup
	observeBarrier(&wg)
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func escapedOne(n int) { escapedBarrier(n, consume) }

func escapedTwo(n int) { escapedBarrier(n, consume) }

func observeBarrier(*sync.WaitGroup) {}

// Add must register exactly one completion per iteration.
func nonUnitAdd(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(2)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func nonUnitOne(n int) { nonUnitAdd(n, consume) }

func nonUnitTwo(n int) { nonUnitAdd(n, consume) }

// Wait must immediately follow the loop.
func delayedWait(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	consume(n)
	wg.Wait()
}

func delayedOne(n int) { delayedWait(n, consume) }

func delayedTwo(n int) { delayedWait(n, consume) }

// Channel control is outside the independent finite-domain proof.
func channelRange(values <-chan int, work func(int)) {
	var wg sync.WaitGroup
	for value := range values {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(value)
		}()
	}
	wg.Wait()
}

func channelOne(values <-chan int) { channelRange(values, consume) }

func channelTwo(values <-chan int) { channelRange(values, consume) }

// Completion must be the first deferred worker statement.
func lateCompletion(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			work(index)
			defer wg.Done()
		}()
	}
	wg.Wait()
}

func lateOne(n int) { lateCompletion(n, consume) }

func lateTwo(n int) { lateCompletion(n, consume) }

// Exactly one launch is required.
func twoLaunches(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(2)
		go func() {
			defer wg.Done()
			work(index)
		}()
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func twoLaunchOne(n int) { twoLaunches(n, consume) }

func twoLaunchTwo(n int) { twoLaunches(n, consume) }

// A launch behind a branch is not one unconditional launch per iteration.
func conditionalLaunch(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		if index > 0 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				work(index)
			}()
		}
	}
	wg.Wait()
}

func conditionalOne(n int) { conditionalLaunch(n, consume) }

func conditionalTwo(n int) { conditionalLaunch(n, consume) }

// An earlier continue means the direct launch is not reached every iteration.
func skippedLaunch(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		if index == 0 {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func skippedOne(n int) { skippedLaunch(n, consume) }

func skippedTwo(n int) { skippedLaunch(n, consume) }

func recursiveOnly(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	if n > 1 {
		recursiveOnly(n-1, work)
	}
}

type fakeWaitGroup struct{}

func (*fakeWaitGroup) Add(int) {}

func (*fakeWaitGroup) Done() {}

func (*fakeWaitGroup) Wait() {}

func lookalikeBarrier(n int, work func(int)) {
	var wg fakeWaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func lookalikeOne(n int) { lookalikeBarrier(n, consume) }

func lookalikeTwo(n int) { lookalikeBarrier(n, consume) }

type otherRunner struct{}

func (*runner) rareMethod(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func (*otherRunner) rareMethod(int, func(int)) {}

func rareRealCaller(r *runner, n int) { r.rareMethod(n, consume) }

func sameNamedMethodCallers(r *otherRunner, n int) {
	r.rareMethod(n, consume)
	r.rareMethod(n, consume)
}

// Constant-empty actual arguments do not establish lifecycle repetition.
func emptyActualDomain(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func emptyActualOne() { emptyActualDomain(0, consume) }

func emptyActualTwo() { emptyActualDomain(-1, consume) }

func emptyActualLoop(groups int) {
	for range groups {
		emptyActualDomain(0, consume)
	}
}

// Empty actuals are excluded without hiding independent nonempty call sites.
func mixedActualDomain(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `mixedActualDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func mixedActualCallers() {
	mixedActualDomain(0, consume)
	mixedActualDomain(-1, consume)
	mixedActualDomain(2, consume)
	mixedActualDomain(3, consume)
}

// Evidence is attributed per candidate when a helper owns multiple domains.
func splitActualDomains(first, second int, work func(int)) {
	var firstGroup sync.WaitGroup
	for index := range first {
		firstGroup.Add(1)
		go func() {
			defer firstGroup.Done()
			work(index)
		}()
	}
	firstGroup.Wait()

	var secondGroup sync.WaitGroup
	for index := range second {
		secondGroup.Add(1)
		go func() { // want `splitActualDomains creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer secondGroup.Done()
			work(index)
		}()
	}
	secondGroup.Wait()
}

func splitActualOne() { splitActualDomains(0, 2, consume) }

func splitActualTwo() { splitActualDomains(0, 3, consume) }

// An omitted variadic domain is statically empty at the call site.
func emptyVariadicActual(values ...int) {
	var wg sync.WaitGroup
	for index := range values {
		wg.Add(1)
		go func() { // want `emptyVariadicActual creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func emptyVariadicOne() { emptyVariadicActual() }

func emptyVariadicTwo() { emptyVariadicActual() }

func emptyVariadicEllipsisOne() { emptyVariadicActual(make([]int, 0)...) }

func emptyVariadicEllipsisTwo() { emptyVariadicActual([]int(nil)...) }

func nonemptyVariadicOne() { emptyVariadicActual(0) }

func nonemptyVariadicTwo() { emptyVariadicActual(0, 1) }

// Pure min/cap parameter domains are evaluated under actual substitution.
func minimumActualDomain(n int) {
	var wg sync.WaitGroup
	for index := range min(n, 2) {
		wg.Add(1)
		go func() { // want `minimumActualDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func minimumActualCallers() {
	minimumActualDomain(0)
	minimumActualDomain(0)
	minimumActualDomain(2)
	minimumActualDomain(3)
}

type actualSlice []int

func capacityActualDomain(values actualSlice) {
	var wg sync.WaitGroup
	for index := 0; index < cap(values); index++ {
		wg.Add(1)
		go func() { // want `capacityActualDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func capacityActualCallers() {
	capacityActualDomain(actualSlice(nil))
	capacityActualDomain(actualSlice([]int(nil)))
	capacityActualDomain(make(actualSlice, 0, 2))
	capacityActualDomain(make(actualSlice, 0, 3))
}

func sliceActualDomain(values actualSlice) {
	var wg sync.WaitGroup
	for index := range values {
		wg.Add(1)
		go func() { // want `sliceActualDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func sliceActualCallers() {
	sliceActualDomain(actualSlice(nil))
	sliceActualDomain(actualSlice([]int(nil)))
	sliceActualDomain(make(actualSlice, 0))
	sliceActualDomain(actualSlice{1, 2})
	sliceActualDomain(actualSlice{1, 2, 3})
}

type actualMethodRunner struct{}

func (*actualMethodRunner) expressionDomain(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `expressionDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func methodExpressionActualCallers(runner *actualMethodRunner) {
	(*actualMethodRunner).expressionDomain(runner, 0)
	(*actualMethodRunner).expressionDomain(runner, 0)
	(*actualMethodRunner).expressionDomain(runner, 2)
	(*actualMethodRunner).expressionDomain(runner, 3)
}

func (*actualMethodRunner) valueDomain(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `valueDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func methodValueActualCallers(runner *actualMethodRunner) {
	runner.valueDomain(0)
	runner.valueDomain(0)
	runner.valueDomain(2)
	runner.valueDomain(3)
}

type actualReceiverDomain int

func (n actualReceiverDomain) valueFanout() {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `valueFanout creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(int(index))
		}()
	}
	wg.Wait()
}

func receiverValueActualCallers() {
	actualReceiverDomain(0).valueFanout()
	actualReceiverDomain(0).valueFanout()
	actualReceiverDomain(2).valueFanout()
	actualReceiverDomain(3).valueFanout()
}

func (n actualReceiverDomain) expressionFanout() {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `expressionFanout creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(int(index))
		}()
	}
	wg.Wait()
}

func receiverExpressionActualCallers() {
	actualReceiverDomain.expressionFanout(0)
	actualReceiverDomain.expressionFanout(0)
	actualReceiverDomain.expressionFanout(2)
	actualReceiverDomain.expressionFanout(3)
}

func pointerArrayNilDomain(values *[2]int) {
	var wg sync.WaitGroup
	for index := range values {
		wg.Add(1)
		go func() { // want `pointerArrayNilDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func pointerArrayNilOne() { pointerArrayNilDomain(nil) }

func pointerArrayNilTwo() { pointerArrayNilDomain(nil) }

func genericPointerArrayNilDomain[T ~*[2]int](values T) {
	var wg sync.WaitGroup
	for index := 0; index < len(values); index++ {
		wg.Add(1)
		go func() { // want `genericPointerArrayNilDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func genericPointerArrayNilOne() { genericPointerArrayNilDomain[*[2]int](nil) }

func genericPointerArrayNilTwo() { genericPointerArrayNilDomain[*[2]int](nil) }

// Substitution is disabled when the formal domain is reassigned before use.
func reassignedActualDomain(n int) {
	n = 2
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

func reassignedActualOne() { reassignedActualDomain(0) }

func reassignedActualTwo() { reassignedActualDomain(0) }

func fixedZeroActualDomain(n int) {
	n = 0
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

func fixedZeroActualOne() { fixedZeroActualDomain(2) }

func fixedZeroActualTwo() { fixedZeroActualDomain(3) }

func branchZeroActualDomain(n int, choose bool) {
	if choose {
		n = 0
	} else {
		n = -1
	}
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

func branchZeroActualOne(choose bool) { branchZeroActualDomain(2, choose) }

func branchZeroActualTwo(choose bool) { branchZeroActualDomain(3, choose) }

func selectorActualDomain(limit loopLimits) {
	var wg sync.WaitGroup
	for index := 0; index < limit.count; index++ {
		wg.Add(1)
		go func() { // want `selectorActualDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func selectorActualCallers() {
	selectorActualDomain(loopLimits{})
	selectorActualDomain(loopLimits{count: 0})
	selectorActualDomain(loopLimits{count: 2})
	selectorActualDomain(loopLimits{count: 3})
}

type promotedValueDomain int

func (n promotedValueDomain) promotedValueFanout() {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `promotedValueFanout creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(int(index))
		}()
	}
	wg.Wait()
}

type promotedValueOuter struct{ promotedValueDomain }

func promotedValueActualCallers() {
	promotedValueOuter{}.promotedValueFanout()
	promotedValueOuter{promotedValueDomain: 0}.promotedValueFanout()
	promotedValueOuter{promotedValueDomain: 2}.promotedValueFanout()
	promotedValueOuter{promotedValueDomain: 3}.promotedValueFanout()
}

type promotedExpressionDomain int

func (n promotedExpressionDomain) promotedExpressionFanout() {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `promotedExpressionFanout creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(int(index))
		}()
	}
	wg.Wait()
}

type promotedExpressionOuter struct{ promotedExpressionDomain }

func promotedExpressionActualCallers() {
	promotedExpressionOuter.promotedExpressionFanout(promotedExpressionOuter{})
	promotedExpressionOuter.promotedExpressionFanout(promotedExpressionOuter{promotedExpressionDomain: 0})
	promotedExpressionOuter.promotedExpressionFanout(promotedExpressionOuter{promotedExpressionDomain: 2})
	promotedExpressionOuter.promotedExpressionFanout(promotedExpressionOuter{promotedExpressionDomain: 3})
}

func genericConversionActualDomain[T ~int](n T) {
	var wg sync.WaitGroup
	for index := range int(n) {
		wg.Add(1)
		go func() { // want `genericConversionActualDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func genericConversionActualCallers() {
	genericConversionActualDomain(0)
	genericConversionActualDomain(0)
	genericConversionActualDomain(2)
	genericConversionActualDomain(3)
}

func mapActualDomain(values map[int]int) {
	var wg sync.WaitGroup
	for key := range values {
		wg.Add(1)
		go func() { // want `mapActualDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(key)
		}()
	}
	wg.Wait()
}

func mapActualCallers() {
	mapActualDomain(make(map[int]int, 4))
	mapActualDomain(map[int]int(nil))
	mapActualDomain(map[int]int{1: 1, 2: 2})
	mapActualDomain(map[int]int{1: 1, 2: 2, 3: 3})
}

func channelLengthActualDomain(values chan int) {
	var wg sync.WaitGroup
	for index := range len(values) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func channelLengthActualCallers() {
	channelLengthActualDomain(nil)
	channelLengthActualDomain((chan int)(nil))
	channelLengthActualDomain(make(chan int, 0))
}

func variadicPair() (int, int) { return 0, 1 }

func tupleVariadicActualDomain(prefix int, values ...int) {
	var wg sync.WaitGroup
	for index := range values {
		wg.Add(1)
		go func() { // want `tupleVariadicActualDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(prefix + index)
		}()
	}
	wg.Wait()
}

func tupleVariadicActualCallers() {
	tupleVariadicActualDomain(variadicPair())
	tupleVariadicActualDomain(variadicPair())
}

type tupleMethodActualDomain struct{}

func (*tupleMethodActualDomain) fanout(values ...int) {
	var wg sync.WaitGroup
	for index := range values {
		wg.Add(1)
		go func() { // want `fanout creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func tupleMethodArguments() (*tupleMethodActualDomain, int) {
	return new(tupleMethodActualDomain), 0
}

func tupleMethodActualCallers() {
	(*tupleMethodActualDomain).fanout(tupleMethodArguments())
	(*tupleMethodActualDomain).fanout(tupleMethodArguments())
}

func indexActualDomain(limits []int) {
	var wg sync.WaitGroup
	for index := range limits[0] {
		wg.Add(1)
		go func() { // want `indexActualDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func indexActualCallers() {
	indexActualDomain([]int{0})
	indexActualDomain([]int{-1})
	indexActualDomain([]int{2})
	indexActualDomain([]int{3})
}

func switchSiblingActualDomain(n, branch int) {
	switch branch {
	case 0:
		n = 0
		var wg sync.WaitGroup
		for index := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				consume(index)
			}()
		}
		wg.Wait()
	case 1:
		n = 2
	}
}

func switchSiblingActualOne() { switchSiblingActualDomain(2, 0) }

func switchSiblingActualTwo() { switchSiblingActualDomain(3, 0) }

func enclosingPostActualDomain(n int) {
	for once := true; once; n = 2 {
		var wg sync.WaitGroup
		for index := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				consume(index)
			}()
		}
		wg.Wait()
		once = false
	}
}

func enclosingPostActualOne() { enclosingPostActualDomain(0) }

func enclosingPostActualTwo() { enclosingPostActualDomain(0) }

func enclosingRangeValueActualDomain(n int, values []int) {
	for _, n = range values {
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

func enclosingRangeValueActualOne() { enclosingRangeValueActualDomain(0, []int{2}) }

func enclosingRangeValueActualTwo() { enclosingRangeValueActualDomain(0, []int{3}) }

func enclosingRangeKeyActualDomain(n int, values []int) {
	for n = range values {
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

func enclosingRangeKeyActualOne() { enclosingRangeKeyActualDomain(0, []int{1, 2, 3}) }

func enclosingRangeKeyActualTwo() { enclosingRangeKeyActualDomain(0, []int{1, 2, 3, 4}) }

func mapIndexDynamicActualDomain(values map[int]int) {
	var wg sync.WaitGroup
	for index := range values[1] {
		wg.Add(1)
		go func() { // want `mapIndexDynamicActualDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func mapIndexDynamicActualCallers(key int) {
	mapIndexDynamicActualDomain(map[int]int{key: 2})
	mapIndexDynamicActualDomain(map[int]int{key: 3})
}

func formalIndexActualDomain(values []int, slot int) {
	var wg sync.WaitGroup
	for index := range values[slot] {
		wg.Add(1)
		go func() { // want `formalIndexActualDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func formalIndexActualCallers() {
	formalIndexActualDomain([]int{0}, 0)
	formalIndexActualDomain([]int{0}, 0)
	formalIndexActualDomain([]int{2}, 0)
	formalIndexActualDomain([]int{3}, 0)
}

func sliceBoundsActualDomain(values [][2]int) {
	var wg sync.WaitGroup
	for index := range values[0] {
		wg.Add(1)
		go func() { // want `sliceBoundsActualDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func sliceBoundsActualCallers() {
	sliceBoundsActualDomain(nil)
	sliceBoundsActualDomain([][2]int{})
	sliceBoundsActualDomain(make([][2]int, 0))
	sliceBoundsActualDomain([][2]int{{}})
	sliceBoundsActualDomain([][2]int{{}, {}})
}

func conditionEffectZeroActualDomain(n int, choose bool) {
	n = 2
	if func() bool {
		n = 0
		return choose
	}() {
	}
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

func conditionEffectZeroActualOne(choose bool) { conditionEffectZeroActualDomain(2, choose) }

func conditionEffectZeroActualTwo(choose bool) { conditionEffectZeroActualDomain(3, choose) }

func duplicateAssignmentZeroActualDomain(n int) {
	n, n = 2, 0
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

func duplicateAssignmentZeroActualOne() { duplicateAssignmentZeroActualDomain(2) }

func duplicateAssignmentZeroActualTwo() { duplicateAssignmentZeroActualDomain(3) }

func duplicateAssignmentPositiveActualDomain(n int) {
	n, n = 0, 2
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

func duplicateAssignmentPositiveActualOne() { duplicateAssignmentPositiveActualDomain(0) }

func duplicateAssignmentPositiveActualTwo() { duplicateAssignmentPositiveActualDomain(0) }

var aliasedActualMap map[int]int

func clearAliasedActualMap() { clear(aliasedActualMap) }

func globalAliasZeroActualDomain(values map[int]int) {
	aliasedActualMap = values
	clearAliasedActualMap()
	var wg sync.WaitGroup
	for key := range values {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(key)
		}()
	}
	wg.Wait()
}

func globalAliasZeroActualOne() { globalAliasZeroActualDomain(map[int]int{1: 1, 2: 2}) }

func globalAliasZeroActualTwo() { globalAliasZeroActualDomain(map[int]int{1: 1, 2: 2, 3: 3}) }

type missingPromotedDomain int

func (n missingPromotedDomain) missingPromotedFanout() {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(int(index))
		}()
	}
	wg.Wait()
}

type missingPromotedOuter struct{ *missingPromotedDomain }

func missingPromotedActualCallers() {
	missingPromotedOuter{}.missingPromotedFanout()
	missingPromotedOuter{}.missingPromotedFanout()
	missingPromotedOuter.missingPromotedFanout(missingPromotedOuter{})
	missingPromotedOuter.missingPromotedFanout(missingPromotedOuter{})
}

func dereferenceActualDomain(value *int) {
	var wg sync.WaitGroup
	for index := range *value {
		wg.Add(1)
		go func() { // want `dereferenceActualDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func actualIntPointer(value int) *int { return &value }

func dereferenceActualCallers() {
	dereferenceActualDomain(new(int))
	dereferenceActualDomain((*int)(nil))
	dereferenceActualDomain(actualIntPointer(2))
	dereferenceActualDomain(actualIntPointer(3))
}

func pointerArrayIndexActualDomain(values *[1]int) {
	var wg sync.WaitGroup
	for index := range values[0] {
		wg.Add(1)
		go func() { // want `pointerArrayIndexActualDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func pointerArrayIndexActualCallers() {
	pointerArrayIndexActualDomain(nil)
	pointerArrayIndexActualDomain(&[1]int{0})
	pointerArrayIndexActualDomain(&[1]int{2})
	pointerArrayIndexActualDomain(&[1]int{3})
}

func localAliasActualDomain(n int) {
	limit := n
	var wg sync.WaitGroup
	for index := range limit {
		wg.Add(1)
		go func() { // want `localAliasActualDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func localAliasActualCallers() {
	localAliasActualDomain(0)
	localAliasActualDomain(0)
	localAliasActualDomain(2)
	localAliasActualDomain(3)
}

func localFixedZeroActualDomain() {
	limit := 0
	var wg sync.WaitGroup
	for index := range limit {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func localFixedZeroActualOne() { localFixedZeroActualDomain() }

func localFixedZeroActualTwo() { localFixedZeroActualDomain() }

type actualMap map[int]int
type actualChannel chan int

func namedMakeActualCallers() {
	mapActualDomain(actualMap(make(map[int]int, 4)))
	mapActualDomain(actualMap(make(map[int]int)))
	channelLengthActualDomain(actualChannel(make(chan int, 0)))
	channelLengthActualDomain(actualChannel(make(chan int)))
}

type selectorCollections struct{ values []int }

func capSelectorActualDomain(collection selectorCollections) {
	var wg sync.WaitGroup
	for index := range cap(collection.values) {
		wg.Add(1)
		go func() { // want `capSelectorActualDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func capSelectorActualCallers() {
	capSelectorActualDomain(selectorCollections{})
	capSelectorActualDomain(selectorCollections{values: nil})
	capSelectorActualDomain(selectorCollections{values: make([]int, 0, 2)})
	capSelectorActualDomain(selectorCollections{values: make([]int, 0, 3)})
}

func nestedLenSelectorActualDomain(collection selectorCollections) {
	var wg sync.WaitGroup
	for index := range min(len(collection.values), 2) {
		wg.Add(1)
		go func() { // want `nestedLenSelectorActualDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func nestedLenSelectorActualCallers() {
	nestedLenSelectorActualDomain(selectorCollections{})
	nestedLenSelectorActualDomain(selectorCollections{values: nil})
	nestedLenSelectorActualDomain(selectorCollections{values: []int{1, 2}})
	nestedLenSelectorActualDomain(selectorCollections{values: []int{1, 2, 3}})
}

func zeroInitializerActualDomain(n int) int {
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

var zeroActualInitializerOne = zeroInitializerActualDomain(0)
var zeroActualInitializerTwo = zeroInitializerActualDomain(0)

func nonzeroInitializerActualDomain(n int) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `nonzeroInitializerActualDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
	return n
}

var nonzeroActualInitializerOne = nonzeroInitializerActualDomain(2)
var nonzeroActualInitializerTwo = nonzeroInitializerActualDomain(3)

func localSnapshotZeroDomain(n int) {
	limit := n
	n = 0
	var wg sync.WaitGroup
	for index := range limit {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func localSnapshotZeroCallers() {
	localSnapshotZeroDomain(0)
	localSnapshotZeroDomain(0)
}

func localPostAliasZeroDomain(n int) {
	limit := n
	pointer := &limit
	*pointer = 0
	var wg sync.WaitGroup
	for index := range limit {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func localPostAliasZeroCallers() {
	localPostAliasZeroDomain(2)
	localPostAliasZeroDomain(3)
}

func localCompoundZeroDomain(n int) {
	limit := n
	limit *= 0
	var wg sync.WaitGroup
	for index := range limit {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func localCompoundZeroCallers() {
	localCompoundZeroDomain(2)
	localCompoundZeroDomain(3)
}

func formalPostAliasZeroDomain(n int) {
	n = 2
	pointer := &n
	*pointer = 0
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

func formalPostAliasZeroCallers() {
	formalPostAliasZeroDomain(2)
	formalPostAliasZeroDomain(3)
}

func siblingAssignmentZeroDomain(n, other int) {
	n, other = other, 0
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

func siblingAssignmentZeroCallers() {
	siblingAssignmentZeroDomain(2, 0)
	siblingAssignmentZeroDomain(3, 0)
}

func localAtMostOneDomain() {
	limit := 1
	var wg sync.WaitGroup
	for index := range limit {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func localAtMostOneCallers() {
	localAtMostOneDomain()
	localAtMostOneDomain()
}

func namedResultZeroDomain() (limit int) {
	var wg sync.WaitGroup
	for index := range limit {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
	return
}

func namedResultZeroCallers() {
	namedResultZeroDomain()
	namedResultZeroDomain()
}

func negativeIndexActualDomain(values []int, slot int) {
	var wg sync.WaitGroup
	for index := range values[slot] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func negativeIndexActualCallers() {
	negativeIndexActualDomain([]int{2}, -1)
	negativeIndexActualDomain([]int{3}, -1)
}

func stringIndexActualDomain(values string) {
	var wg sync.WaitGroup
	for index := range values[0] {
		wg.Add(1)
		go func() { // want `stringIndexActualDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(int(index))
		}()
	}
	wg.Wait()
}

func stringIndexActualCallers() {
	stringIndexActualDomain("")
	stringIndexActualDomain("")
	stringIndexActualDomain("\x02")
	stringIndexActualDomain("\x03")
}

func variadicIndexActualDomain(values ...int) {
	var wg sync.WaitGroup
	for index := range values[0] {
		wg.Add(1)
		go func() { // want `variadicIndexActualDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func variadicIndexActualCallers() {
	variadicIndexActualDomain()
	variadicIndexActualDomain()
	variadicIndexActualDomain(0)
	variadicIndexActualDomain(0)
	variadicIndexActualDomain(2)
	variadicIndexActualDomain(3)
}

func mapIndexZeroActualDomain(values map[int]int) {
	var wg sync.WaitGroup
	for index := range values[0] {
		wg.Add(1)
		go func() { // want `mapIndexZeroActualDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func mapIndexZeroActualCallers() {
	mapIndexZeroActualDomain(nil)
	mapIndexZeroActualDomain(make(map[int]int))
	mapIndexZeroActualDomain(map[int]int{0: 2})
	mapIndexZeroActualDomain(map[int]int{0: 3})
}

func pointerArrayDereferenceOneDomain(values *[2]int) {
	var wg sync.WaitGroup
	for index := range *values {
		wg.Add(1)
		go func() { // want `pointerArrayDereferenceOneDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func pointerArrayDereferenceOneCallers() {
	pointerArrayDereferenceOneDomain(nil)
	pointerArrayDereferenceOneDomain((*[2]int)(nil))
}

func pointerArrayDereferenceTwoDomain(values *[2]int) {
	var wg sync.WaitGroup
	for index, value := range *values {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index + value)
		}()
	}
	wg.Wait()
}

func pointerArrayDereferenceTwoCallers() {
	pointerArrayDereferenceTwoDomain(nil)
	pointerArrayDereferenceTwoDomain((*[2]int)(nil))
}

type implicitReceiverDomain int

func (n implicitReceiverDomain) implicitReceiverFanout() {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `implicitReceiverFanout creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(int(index))
		}()
	}
	wg.Wait()
}

func implicitReceiverPointer(value implicitReceiverDomain) *implicitReceiverDomain { return &value }

func implicitReceiverCallers() {
	implicitReceiverPointer(2).implicitReceiverFanout()
	implicitReceiverPointer(3).implicitReceiverFanout()
	(*implicitReceiverDomain)(nil).implicitReceiverFanout()
	new(implicitReceiverDomain).implicitReceiverFanout()
}

func utf8LengthFanout() {
	var wg sync.WaitGroup
	for index := 0; index < len("é"); index++ {
		wg.Add(1)
		go func() { // want `utf8LengthFanout creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func utf8LengthCallers() {
	utf8LengthFanout()
	utf8LengthFanout()
}

func utf8RuneSingleIteration() {
	var wg sync.WaitGroup
	for index := range "é" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func utf8RuneSingleIterationCallers() {
	utf8RuneSingleIteration()
	utf8RuneSingleIteration()
}

func localModuloDomain(n int) {
	limit := n % 2
	var wg sync.WaitGroup
	for index := range limit {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func localModuloCallers() {
	localModuloDomain(2)
	localModuloDomain(3)
}

func localOpaqueLimit() int { return 2 }

func localOpaqueDomain() {
	limit := localOpaqueLimit()
	var wg sync.WaitGroup
	for index := range limit {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func localOpaqueCallers() {
	localOpaqueDomain()
	localOpaqueDomain()
}

type mutableLocalDomain int

func (n *mutableLocalDomain) clear() { *n = 0 }

func localPointerMethodDomain() {
	limit := mutableLocalDomain(2)
	limit.clear()
	var wg sync.WaitGroup
	for index := range limit {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(int(index))
		}()
	}
	wg.Wait()
}

func localPointerMethodCallers() {
	localPointerMethodDomain()
	localPointerMethodDomain()
}

func dereferencedPointerArrayIndexDomain(values *[1]int) {
	var wg sync.WaitGroup
	for index := range (*values)[0] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func dereferencedPointerArrayIndexCallers() {
	dereferencedPointerArrayIndexDomain(nil)
	dereferencedPointerArrayIndexDomain((*[1]int)(nil))
}

func convertedPointerArrayTwoDomain(values *[2]int) {
	var wg sync.WaitGroup
	for index, value := range ([2]int)(*values) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index + value)
		}()
	}
	wg.Wait()
}

func convertedPointerArrayTwoCallers() {
	convertedPointerArrayTwoDomain(nil)
	convertedPointerArrayTwoDomain((*[2]int)(nil))
}

func compositePointerArrayDomain(value *int) {
	var wg sync.WaitGroup
	for index, item := range [2]int{*value, 0} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index + item)
		}()
	}
	wg.Wait()
}

func compositePointerArrayCallers() {
	compositePointerArrayDomain(nil)
	compositePointerArrayDomain((*int)(nil))
}

func compositePointerSliceDomain(value *int) {
	var wg sync.WaitGroup
	for index, item := range []int{*value, *value} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index + item)
		}()
	}
	wg.Wait()
}

func compositePointerSliceCallers() {
	compositePointerSliceDomain(nil)
	compositePointerSliceDomain((*int)(nil))
}

type genericValueReceiverDomain[T any] struct{ limit int }

func (value genericValueReceiverDomain[T]) fanout() {
	var wg sync.WaitGroup
	for index := range value.limit {
		wg.Add(1)
		go func() { // want `fanout creates a fresh function-local sync.WaitGroup generation.*4 direct production call sites`
			defer wg.Done()
			consume(int(index))
		}()
	}
	wg.Wait()
}

func genericValueReceiverCallers() {
	genericValueReceiverDomain[int]{}.fanout()
	genericValueReceiverDomain[int]{limit: 0}.fanout()
	genericValueReceiverDomain[int]{limit: 2}.fanout()
	genericValueReceiverDomain[int]{limit: 3}.fanout()
	genericValueReceiverDomain[int].fanout(genericValueReceiverDomain[int]{})
	genericValueReceiverDomain[int].fanout(genericValueReceiverDomain[int]{limit: 0})
	genericValueReceiverDomain[int].fanout(genericValueReceiverDomain[int]{limit: 2})
	genericValueReceiverDomain[int].fanout(genericValueReceiverDomain[int]{limit: 3})
}

type genericPointerReceiverDomain[T any] struct{ limit int }

func (value *genericPointerReceiverDomain[T]) fanout() {
	var wg sync.WaitGroup
	for index := range value.limit {
		wg.Add(1)
		go func() { // want `fanout creates a fresh function-local sync.WaitGroup generation.*4 direct production call sites`
			defer wg.Done()
			consume(int(index))
		}()
	}
	wg.Wait()
}

func genericPointerReceiverCallers() {
	(*genericPointerReceiverDomain[int])(nil).fanout()
	new(genericPointerReceiverDomain[int]).fanout()
	(&genericPointerReceiverDomain[int]{limit: 2}).fanout()
	(&genericPointerReceiverDomain[int]{limit: 3}).fanout()
	(*genericPointerReceiverDomain[int]).fanout(nil)
	(*genericPointerReceiverDomain[int]).fanout(new(genericPointerReceiverDomain[int]))
	(*genericPointerReceiverDomain[int]).fanout(&genericPointerReceiverDomain[int]{limit: 2})
	(*genericPointerReceiverDomain[int]).fanout(&genericPointerReceiverDomain[int]{limit: 3})
}

type evaluatedSelectorHolder struct{ values [2]int }

func evaluatedSelectorTwoDomain(holder *evaluatedSelectorHolder) {
	var wg sync.WaitGroup
	for index, value := range holder.values {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index + value)
		}()
	}
	wg.Wait()
}

func evaluatedSelectorTwoCallers() {
	evaluatedSelectorTwoDomain(nil)
	evaluatedSelectorTwoDomain((*evaluatedSelectorHolder)(nil))
}

func unevaluatedSelectorOneDomain(holder *evaluatedSelectorHolder) {
	var wg sync.WaitGroup
	for index := range holder.values {
		wg.Add(1)
		go func() { // want `unevaluatedSelectorOneDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func unevaluatedSelectorOneCallers() {
	unevaluatedSelectorOneDomain(nil)
	unevaluatedSelectorOneDomain((*evaluatedSelectorHolder)(nil))
}

func sliceArrayConversionTwoDomain(values []int) {
	var wg sync.WaitGroup
	for index, value := range ([2]int)(values) {
		wg.Add(1)
		go func() { // want `sliceArrayConversionTwoDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index + value)
		}()
	}
	wg.Wait()
}

func sliceArrayConversionTwoCallers() {
	sliceArrayConversionTwoDomain(nil)
	sliceArrayConversionTwoDomain([]int{})
	sliceArrayConversionTwoDomain([]int{1, 2})
	sliceArrayConversionTwoDomain(make([]int, 3))
}

func slicePointerArrayConversionTwoDomain(values []int) {
	var wg sync.WaitGroup
	for index, value := range (*[2]int)(values) {
		wg.Add(1)
		go func() { // want `slicePointerArrayConversionTwoDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index + value)
		}()
	}
	wg.Wait()
}

func slicePointerArrayConversionTwoCallers() {
	slicePointerArrayConversionTwoDomain(nil)
	slicePointerArrayConversionTwoDomain([]int{})
	slicePointerArrayConversionTwoDomain([]int{1, 2})
	slicePointerArrayConversionTwoDomain(make([]int, 3))
}

func sliceArrayConversionOneDomain(values []int) {
	var wg sync.WaitGroup
	for index := range ([2]int)(values) {
		wg.Add(1)
		go func() { // want `sliceArrayConversionOneDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func sliceArrayConversionOneCallers() {
	sliceArrayConversionOneDomain(nil)
	sliceArrayConversionOneDomain([]int{})
}

func directNilSelectorTwoDomain() {
	var wg sync.WaitGroup
	for index, value := range ((*evaluatedSelectorHolder)(nil)).values {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index + value)
		}()
	}
	wg.Wait()
}

func directNilSelectorTwoCallers() {
	directNilSelectorTwoDomain()
	directNilSelectorTwoDomain()
}

func directNilPointerArrayTwoDomain() {
	var wg sync.WaitGroup
	for index, value := range (*[2]int)(nil) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index + value)
		}()
	}
	wg.Wait()
}

func directNilPointerArrayTwoCallers() {
	directNilPointerArrayTwoDomain()
	directNilPointerArrayTwoDomain()
}

func directNilDereferencedArrayTwoDomain() {
	var wg sync.WaitGroup
	for index, value := range *(*[2]int)(nil) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index + value)
		}()
	}
	wg.Wait()
}

func directNilDereferencedArrayTwoCallers() {
	directNilDereferencedArrayTwoDomain()
	directNilDereferencedArrayTwoDomain()
}

func directNilSliceIndexDomain() {
	var wg sync.WaitGroup
	for index := range (([]int)(nil))[0] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func directNilSliceIndexCallers() {
	directNilSliceIndexDomain()
	directNilSliceIndexDomain()
}

func directNilPointerArrayIndexDomain() {
	var wg sync.WaitGroup
	for index := range ((*[1]int)(nil))[0] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func directNilPointerArrayIndexCallers() {
	directNilPointerArrayIndexDomain()
	directNilPointerArrayIndexDomain()
}

func directNilMapIndexDomain() {
	var wg sync.WaitGroup
	for index := range ((map[int]int)(nil))[0] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func directNilMapIndexCallers() {
	directNilMapIndexDomain()
	directNilMapIndexDomain()
}

func panickingCallerArgumentDomain(limit int) {
	var wg sync.WaitGroup
	for index := range limit {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func panickingCallerArgumentCallers() {
	panickingCallerArgumentDomain(([]int(nil))[0])
	panickingCallerArgumentDomain(([]int(nil))[0])
}

func panickingCallerSelectorDomain(values [2]int) {
	var wg sync.WaitGroup
	for index := range values {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func panickingCallerSelectorCallers() {
	panickingCallerSelectorDomain(((*evaluatedSelectorHolder)(nil)).values)
	panickingCallerSelectorDomain(((*evaluatedSelectorHolder)(nil)).values)
}

func panickingCallerConversionCallers() {
	panickingCallerSelectorDomain(([2]int)(([]int)(nil)))
	panickingCallerSelectorDomain(([2]int)([]int{}))
}

func panickingLocalInitializerDomain() {
	limit := ((*evaluatedSelectorHolder)(nil)).values[0]
	var wg sync.WaitGroup
	for index := range limit {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func panickingLocalInitializerCallers() {
	panickingLocalInitializerDomain()
	panickingLocalInitializerDomain()
}

func panickingLocalIndexInitializerDomain() {
	limit := ([]int(nil))[0]
	var wg sync.WaitGroup
	for index := range limit {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func panickingLocalIndexInitializerCallers() {
	panickingLocalIndexInitializerDomain()
	panickingLocalIndexInitializerDomain()
}

var gomaxIndexedArrays [1][2]int

func gomaxIndexedArrayDomain() {
	var wg sync.WaitGroup
	for index := range gomaxIndexedArrays[runtime.GOMAXPROCS(0)] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func gomaxIndexedArrayCallers() {
	gomaxIndexedArrayDomain()
	gomaxIndexedArrayDomain()
}

func gomaxIndexedArrayLengthDomain() {
	var wg sync.WaitGroup
	for index := 0; index < len(gomaxIndexedArrays[runtime.GOMAXPROCS(0)]); index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func gomaxIndexedArrayLengthCallers() {
	gomaxIndexedArrayLengthDomain()
	gomaxIndexedArrayLengthDomain()
}

func convertedGomaxIndexedArrayDomain() {
	var wg sync.WaitGroup
	for index := range gomaxIndexedArrays[int(runtime.GOMAXPROCS(0))] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func convertedGomaxIndexedArrayCallers() {
	convertedGomaxIndexedArrayDomain()
	convertedGomaxIndexedArrayDomain()
}

func wrappedGomaxIndexedArrayDomain() {
	var wg sync.WaitGroup
	for index := range gomaxIndexedArrays[min(runtime.GOMAXPROCS(0), 1)] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func wrappedGomaxIndexedArrayCallers() {
	wrappedGomaxIndexedArrayDomain()
	wrappedGomaxIndexedArrayDomain()
}

type promotedReceiverViabilityInner struct{}

func (promotedReceiverViabilityInner) promotedReceiverFanout(limit int) {
	var wg sync.WaitGroup
	for index := range limit {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

type promotedReceiverViabilityOuter struct {
	*promotedReceiverViabilityInner
}

func promotedReceiverViabilityCallers() {
	promotedReceiverViabilityOuter{}.promotedReceiverFanout(2)
	promotedReceiverViabilityOuter{}.promotedReceiverFanout(3)
}

func promotedReceiverLocalAliasCallers() {
	outer := promotedReceiverViabilityOuter{}
	outer.promotedReceiverFanout(2)
	promotedReceiverViabilityOuter.promotedReceiverFanout(outer, 3)
}

type localReceiverScalar int

func (limit localReceiverScalar) localReceiverAliasFanout() {
	var wg sync.WaitGroup
	for index := range limit {
		wg.Add(1)
		go func() { // want `localReceiverAliasFanout creates a fresh function-local sync.WaitGroup generation.*4 direct production call sites`
			defer wg.Done()
			consume(int(index))
		}()
	}
	wg.Wait()
}

func localReceiverScalarAliasCallers() {
	zero := localReceiverScalar(0)
	two := localReceiverScalar(2)
	three := localReceiverScalar(3)
	zero.localReceiverAliasFanout()
	localReceiverScalar.localReceiverAliasFanout(zero)
	two.localReceiverAliasFanout()
	three.localReceiverAliasFanout()
	localReceiverScalar.localReceiverAliasFanout(two)
	localReceiverScalar.localReceiverAliasFanout(three)
}

func directScalarActualDomain(limit int) {
	var wg sync.WaitGroup
	for index := range limit {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func directScalarZeroActualCallers() {
	directScalarActualDomain(*new(int))
	directScalarActualDomain(struct{ limit int }{}.limit)
	directScalarActualDomain([1]int{}[0])
}

func callerLocalActualDomain(limit int) {
	var wg sync.WaitGroup
	for index := range limit {
		wg.Add(1)
		go func() { // want `callerLocalActualDomain creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func callerLocalZeroActualCallers() {
	zero := 0
	callerLocalActualDomain(zero)
	callerLocalActualDomain(zero)
}

func callerLocalPositiveActualCallers() {
	two := 2
	three := 3
	callerLocalActualDomain(two)
	callerLocalActualDomain(three)
}

func callerLocalWrittenActualCallers() {
	value := 0
	value = 2
	callerLocalActualDomain(value)
	callerLocalActualDomain(value)
}

func loopIIFEZeroActualDomain(limit int) {
	var wg sync.WaitGroup
	for index := range limit {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func loopIIFEZeroActualCallers(groups int) {
	for range groups {
		func(limit int) { loopIIFEZeroActualDomain(limit) }(0)
	}
}

func loopIIFEPositiveActualDomain(limit int) {
	var wg sync.WaitGroup
	for index := range limit {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
}

func loopIIFEPositiveActualCallers(groups int) {
	for range groups {
		func(limit int) { loopIIFEPositiveActualDomain(limit) }(2)
	}
}

func initializerIIFEZeroActualDomain(limit int) int {
	var wg sync.WaitGroup
	for index := range limit {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
	return limit
}

var initializerIIFEZeroActualOne = func(limit int) int {
	return initializerIIFEZeroActualDomain(limit)
}(0)

var initializerIIFEZeroActualTwo = func(limit int) int {
	return initializerIIFEZeroActualDomain(limit)
}(0)

func initializerIIFEPositiveActualDomain(limit int) int {
	var wg sync.WaitGroup
	for index := range limit {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consume(index)
		}()
	}
	wg.Wait()
	return limit
}

var initializerIIFEPositiveActualOne = func(limit int) int {
	return initializerIIFEPositiveActualDomain(limit)
}(2)

var initializerIIFEPositiveActualTwo = func(limit int) int {
	return initializerIIFEPositiveActualDomain(limit)
}(3)

func repeatedOnlyAfterNestedUnreachableGoto(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func nestedUnreachableGotoCaller(groups, n int) {
	for range groups {
		{
			_ = ([]int(nil))[1:]
			goto call
		}
	call:
		repeatedOnlyAfterNestedUnreachableGoto(n, consume)
	}
}

func repeatedOnlyAfterHeaderBlockedGoto(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func headerBlockedGotoCaller(groups, n int) {
	for range groups {
		if func() bool { select {} }() {
			goto call
		}
		_ = ([]int(nil))[1:]
	call:
		repeatedOnlyAfterHeaderBlockedGoto(n, consume)
	}
}

func repeatedOnlyAroundNonreturnSwitchCase(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func nonreturnSwitchCaseCaller(groups, n int) {
	for range groups {
		repeatedOnlyAroundNonreturnSwitchCase(n, consume)
		switch 0 {
		case func() int { select {} }():
		}
		repeatedOnlyAroundNonreturnSwitchCase(n, consume)
	}
}

func repeatedOnlyInDisabledReceiveLHS(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func disabledReceiveLHSCaller(groups, n int) {
	values := make(map[int]int)
	for range groups {
		select {
		case values[repeatedOnlyInDisabledReceiveLHS(n, consume)] = <-(chan int)(nil):
		default:
		}
	}
}

func repeatedOnlyBeforeFiniteLoopPost(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func finiteLoopPostCaller(groups, n int) {
	for range groups {
		repeatedOnlyBeforeFiniteLoopPost(n, consume)
		for index := 0; index < 1; func() { select {} }() {
			_ = index
		}
	}
}

func repeatedOnlyBeforeNonreturnFiniteBody(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func nonreturnFiniteBodyCaller(groups, n int) {
	for range groups {
		repeatedOnlyBeforeNonreturnFiniteBody(n, consume)
		for range 1 {
			select {}
		}
	}
}

func repeatedOnlyBeforeNestedSwitchBreakPost(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func nestedSwitchBreakPostCaller(groups, n int) {
	for range groups {
		repeatedOnlyBeforeNestedSwitchBreakPost(n, consume)
		for index := 0; index < 1; func() { select {} }() {
			switch {
			default:
				break
			}
			_ = index
		}
	}
}

func repeatedOnlyBeforeDeadBreakPost(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func deadBreakPostCaller(groups, n int) {
	for range groups {
		repeatedOnlyBeforeDeadBreakPost(n, consume)
		for index := 0; index < 1; func() { select {} }() {
			if false {
				break
			}
			_ = index
		}
	}
}

func repeatedOnlyBeforeEvaluatedNilPointerRange(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func evaluatedNilPointerRangeCaller(groups, n int) {
	for range groups {
		repeatedOnlyBeforeEvaluatedNilPointerRange(n, consume)
		for _, value := range (*[1]int)(nil) {
			_ = value
		}
	}
}

func repeatedBeforeUnevaluatedNilPointerRange(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedBeforeUnevaluatedNilPointerRange creates a fresh function-local sync.WaitGroup generation.*a direct call in a syntactic loop body`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func unevaluatedNilPointerRangeCaller(groups, n int) {
	for range groups {
		repeatedBeforeUnevaluatedNilPointerRange(n, consume)
		for range (*[1]int)(nil) {
		}
	}
}

func repeatedOnlyInNestedAllNilSelect(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func nestedAllNilSelectCaller(groups, n int) {
	for range groups {
		{
			select {
			case (chan int)(nil) <- repeatedOnlyInNestedAllNilSelect(n, consume):
			}
		}
	}
}

func repeatedOnlyAfterLabeledNestedBreak(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func labeledNestedBreakBeforeCallOne(n int) {
	for index := 0; index < 1; func() { select {} }() {
	nested:
		switch {
		default:
			break nested
		}
		_ = index
	}
	repeatedOnlyAfterLabeledNestedBreak(n, consume)
}

func labeledNestedBreakBeforeCallTwo(n int) {
	for index := 0; index < 1; func() { select {} }() {
	nested:
		switch {
		default:
			break nested
		}
		_ = index
	}
	repeatedOnlyAfterLabeledNestedBreak(n, consume)
}

func repeatedAfterNilSliceRange(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedAfterNilSliceRange creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func nilSliceRangeBeforeCallOne(n int) {
	for _, value := range ([]int)(nil) {
		_ = value
	}
	repeatedAfterNilSliceRange(n, consume)
}

func nilSliceRangeBeforeCallTwo(n int) {
	for _, value := range ([]int)(nil) {
		_ = value
	}
	repeatedAfterNilSliceRange(n, consume)
}

func repeatedOnlyInDeadSelectSendValue(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func deadSelectSendValueOne(n int) {
	select {
	case func() chan int { select {} }() <- repeatedOnlyInDeadSelectSendValue(n, consume):
	default:
	}
}

func deadSelectSendValueTwo(n int) {
	select {
	case func() chan int { select {} }() <- repeatedOnlyInDeadSelectSendValue(n, consume):
	default:
	}
}

func repeatedOnlyInDeadSelectReceiveLHS(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func deadSelectReceiveLHSOne(n int) {
	values := make(map[int]int)
	select {
	case values[repeatedOnlyInDeadSelectReceiveLHS(n, consume)] = <-func() chan int { select {} }():
	default:
	}
}

func deadSelectReceiveLHSTwo(n int) {
	values := make(map[int]int)
	select {
	case values[repeatedOnlyInDeadSelectReceiveLHS(n, consume)] = <-func() chan int { select {} }():
	default:
	}
}

func repeatedAfterOtherSelectClausePanicLHS(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedAfterOtherSelectClausePanicLHS creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func otherSelectClausePanicLHSOne(n int, ready <-chan int) {
	values := make(map[int]int)
	select {
	case ([]int(nil))[1] = <-ready:
	case values[repeatedAfterOtherSelectClausePanicLHS(n, consume)] = <-ready:
	}
}

func otherSelectClausePanicLHSTwo(n int, ready <-chan int) {
	values := make(map[int]int)
	select {
	case ([]int(nil))[1] = <-ready:
	case values[repeatedAfterOtherSelectClausePanicLHS(n, consume)] = <-ready:
	}
}

func repeatedOnlyUnderNonreturnIfCondition(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func nonreturnIfConditionSiteOne(n int) {
	if func() bool { select {} }() {
		repeatedOnlyUnderNonreturnIfCondition(n, consume)
	}
}

func nonreturnIfConditionSiteTwo(n int) {
	if func() bool { select {} }() {
		repeatedOnlyUnderNonreturnIfCondition(n, consume)
	}
}

func repeatedOnlyInUnreachableForPost(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func unreachableForPostOne(n int) {
	for index := 0; index < 1; repeatedOnlyInUnreachableForPost(n, consume) {
		func() { select {} }()
	}
}

func unreachableForPostTwo(n int) {
	for index := 0; index < 1; repeatedOnlyInUnreachableForPost(n, consume) {
		func() { select {} }()
	}
}

func repeatedInLabeledContinueForPost(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedInLabeledContinueForPost creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func labeledContinueForPostOne(n int) {
loop:
	for index := 0; index < 1; repeatedInLabeledContinueForPost(n, consume) {
		_ = index
		continue loop
	}
}

func labeledContinueForPostTwo(n int) {
loop:
	for index := 0; index < 1; repeatedInLabeledContinueForPost(n, consume) {
		_ = index
		continue loop
	}
}

func repeatedOnlyInPanickingRangeBody(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func panickingRangeBodyOne(n int) {
	for _, value := range (*[1]int)(nil) {
		_ = value
		repeatedOnlyInPanickingRangeBody(n, consume)
	}
}

func panickingRangeBodyTwo(n int) {
	for _, value := range (*[1]int)(nil) {
		_ = value
		repeatedOnlyInPanickingRangeBody(n, consume)
	}
}

func panickingBlankValueRangeBodyOne(n int) {
	for _, _ = range (*[1]int)(nil) {
		repeatedOnlyInPanickingRangeBody(n, consume)
	}
}

func panickingBlankValueRangeBodyTwo(n int) {
	for _, _ = range (*[1]int)(nil) {
		repeatedOnlyInPanickingRangeBody(n, consume)
	}
}

func repeatedOnlyInEmptyRangeLHS(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func emptyRangeLHSOne(n int) {
	values := make(map[int]int)
	for values[repeatedOnlyInEmptyRangeLHS(n, consume)] = range ([]int)(nil) {
	}
}

func emptyRangeLHSTwo(n int) {
	values := make(map[int]int)
	for values[repeatedOnlyInEmptyRangeLHS(n, consume)] = range ([]int)(nil) {
	}
}

func repeatedInNonemptyRangeLHS(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `repeatedInNonemptyRangeLHS creates a fresh function-local sync.WaitGroup generation.*2 direct production call sites`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

func nonemptyRangeLHSOne(n int) {
	values := make(map[int]int)
	for values[repeatedInNonemptyRangeLHS(n, consume)] = range []int{1} {
	}
}

func nonemptyRangeLHSTwo(n int) {
	values := make(map[int]int)
	for values[repeatedInNonemptyRangeLHS(n, consume)] = range []int{1} {
	}
}

func consume(int) {}

func repeatedOnlyInDeadPackageInitializers(n int, work func(int)) int {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	return n
}

var deadNestedPackageInitializer, deadSiblingPackageInitializer = (*[2]int)([]int{repeatedOnlyInDeadPackageInitializers(2, consume)}),
	repeatedOnlyInDeadPackageInitializers(3, consume)

var deadFollowingPackageInitializer = repeatedOnlyInDeadPackageInitializers(4, consume)

// Keep deliberately nonreturning package initializers after all production
// initializer evidence: once either initializer is reached, package
// initialization cannot proceed to later declarations.
var firstDeadDeferredInitializer = func() int {
	defer repeatedOnlyInDeadInitializerDefers(2, consume)
	select {}
}()

var secondDeadDeferredInitializer = func() int {
	defer repeatedOnlyInDeadInitializerDefers(3, consume)
	select {}
}()
