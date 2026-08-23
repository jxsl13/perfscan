//go:build go1.22

package ps6086

import (
	"fmt"
	"sync"
)

func allWorkers(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < n; index++ {
		wg.Add(1)
		go func(index int) { // want `all-goroutine fan-out launches every chunk and the caller only waits on a function-local sync.WaitGroup; caller participation can avoid one goroutine launch per dispatch.*allocation reduction does not prove a latency win.*advisory, no automatic fix`
			defer wg.Done()
			work(index)
		}(index)
	}
	wg.Wait()
}

func addBeforeRange(chunks []int, work func(int)) {
	var wg sync.WaitGroup
	for _, chunk := range chunks {
		wg.Add(1)
		go func() { // want `all-goroutine fan-out launches every chunk and the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			work(chunk)
		}()
	}
	wg.Wait()
}

func channelFanout(ch <-chan int, work func(int)) {
	var wg sync.WaitGroup
	for item := range ch {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(item)
		}()
	}
	wg.Wait()
}

func classicReceiveControl(ch <-chan int, work func(int)) {
	var wg sync.WaitGroup
	for item := <-ch; item >= 0; item = <-ch {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(item)
		}()
	}
	wg.Wait()
}

func classicReceiveBody(n int, ch <-chan int, work func(int)) {
	var wg sync.WaitGroup
	for range n {
		item := <-ch
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(item)
		}()
	}
	wg.Wait()
}

func classicSendBody(n int, acknowledgements chan<- int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		acknowledgements <- index
	}
	wg.Wait()
}

func genericNestedChannel[T ~<-chan int](n int, ch T, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		for range ch {
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func concreteNestedChannel(n int, ch <-chan int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		for range ch {
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func closeAfterLaunch(done chan int, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < 1; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		close(done)
	}
	wg.Wait()
}

func parenthesizedCloseAfterLaunch(done chan int, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < 1; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		(close)(done)
	}
	wg.Wait()
}

func shadowedClose(n int, work func(int)) {
	close := func(int) {}
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			work(index)
		}()
		(close)(index)
	}
	wg.Wait()
}

func reusedAfterWait(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
	wg = sync.WaitGroup{}
}

func pointerBarrier(n int, work func(int)) {
	wg := new(sync.WaitGroup)
	for index := range n {
		wg.Add(1)
		go func() { // want `caller participation can avoid one goroutine launch per dispatch`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func compositeBarrier(n int, work func(int)) {
	wg := &sync.WaitGroup{}
	for index := range n {
		wg.Add(1)
		go func() { // want `function-local sync.WaitGroup`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func parenthesizedWorker(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go (func() { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			work(index)
		})()
	}
	wg.Wait()
}

func labeledFanout(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

// The caller already runs one equivalent chunk before it waits.
func callerParticipates(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index+1 < n; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			work(index)
		}(index)
	}
	work(n - 1)
	wg.Wait()
}

// Deferred work runs only when this function returns, after Wait; it is not
// caller participation in the joined fan-out.
func deferredCallerWork(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		defer work(index)
		go func() { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

// An unreachable call is not guaranteed caller participation.
func unreachableCallerWork(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		if false {
			work(index)
		}
		go func() { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func tracedCallerOnly(n int, trace, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		trace(index)
		go func() { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			trace(index)
			work(index)
		}()
	}
	wg.Wait()
}

type runner struct{}

func (*runner) Work(int) {}

type workerFn func(int)

func (worker *workerFn) Replace(replacement workerFn) { *worker = replacement }

type counter int

func (value *counter) Advance() { (*value)++ }
func (value *counter) Work()    {}

type thresholdCount int

func (value *thresholdCount) Reset() { *value = 128 }

func differentMethodReceiver(n int, asynchronous, synchronous *runner) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		synchronous.Work(index)
		go func() { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			asynchronous.Work(index)
		}()
	}
	wg.Wait()
}

func sameLocalMethodReceiver(n int) {
	local := &runner{}
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		local.Work(index)
		go func() {
			defer wg.Done()
			local.Work(index)
		}()
	}
	wg.Wait()
}

func implicitCallableMutation(n int, work, replacement workerFn) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		work(index)
		work.Replace(replacement)
		go func() { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func implicitLoopVariableMutation(n int, work func(counter)) {
	var wg sync.WaitGroup
	for index := counter(0); index < counter(n); {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		index.Advance()
	}
	wg.Wait()
}

func stableValueMethodReceiver(n int) {
	var local runner
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		local.Work(index)
		go func() {
			defer wg.Done()
			local.Work(index)
		}()
	}
	wg.Wait()
}

type runnerContainer struct{ nested runner }

func stableSelectorMethodReceiver(n int, container runnerContainer) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		container.nested.Work(index)
		go func() {
			defer wg.Done()
			container.nested.Work(index)
		}()
	}
	wg.Wait()
}

func stableIndexMethodReceiver(n int, runners []runner) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		runners[index].Work(index)
		go func() {
			defer wg.Done()
			runners[index].Work(index)
		}()
	}
	wg.Wait()
}

func substitutedIndexMethodReceiver(n int, runners []runner) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		runners[index].Work(index)
		go func(workerIndex int) {
			defer wg.Done()
			runners[workerIndex].Work(workerIndex)
		}(index)
	}
	wg.Wait()
}

func substitutedIndexWithSafeCapture(n int, runners []runner) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		runners[index].Work(index)
		go func(workerIndex int) {
			defer wg.Done()
			runners[workerIndex].Work(index)
		}(index)
	}
	wg.Wait()
}

func parenthesizedSubstitutedIndexWithSafeCapture(n int, runners []runner) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		runners[index].Work(index)
		go (func(workerIndex int) {
			defer wg.Done()
			runners[workerIndex].Work(index)
		})(index)
	}
	wg.Wait()
}

type statefulRunner struct{ calls int }

func (value *statefulRunner) Work(int) { value.calls++ }

func copiedValueReceiverIsDifferent(values []*statefulRunner) {
	var wg sync.WaitGroup
	for index, value := range values {
		wg.Add(1)
		(*value).Work(index)
		go func(local statefulRunner, workerIndex int) { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			local.Work(workerIndex)
		}(*value, index)
	}
	wg.Wait()
}

func substitutedPointerSnapshot(values []*statefulRunner) {
	var wg sync.WaitGroup
	for index, value := range values {
		wg.Add(1)
		value.Work(index)
		go func(local *statefulRunner, workerIndex int) {
			defer wg.Done()
			local.Work(workerIndex)
		}(value, index)
		value = nil
	}
	wg.Wait()
}

func substitutedPointerDereference(values []*statefulRunner) {
	var wg sync.WaitGroup
	for index, value := range values {
		wg.Add(1)
		(*value).Work(index)
		go func(local *statefulRunner, workerIndex int) {
			defer wg.Done()
			(*local).Work(workerIndex)
		}(value, index)
		value = nil
	}
	wg.Wait()
}

type statefulRunnerContainer struct{ nested statefulRunner }

func substitutedPointerField(values []*statefulRunnerContainer) {
	var wg sync.WaitGroup
	for index, value := range values {
		wg.Add(1)
		value.nested.Work(index)
		go func(local *statefulRunnerContainer, workerIndex int) {
			defer wg.Done()
			local.nested.Work(workerIndex)
		}(value, index)
		value = nil
	}
	wg.Wait()
}

func substitutedSliceElement(groups [][]statefulRunner) {
	var wg sync.WaitGroup
	for index, values := range groups {
		wg.Add(1)
		values[0].Work(index)
		go func(local []statefulRunner, workerIndex int) {
			defer wg.Done()
			local[0].Work(workerIndex)
		}(values, index)
		values = nil
	}
	wg.Wait()
}

type receiverSlot struct{ slot int }

func mutateReceiverSlot(value *receiverSlot) { value.slot++ }

func derivedReceiverSnapshotMutation(slots []*receiverSlot, runners []runner) {
	var wg sync.WaitGroup
	for index, slot := range slots {
		wg.Add(1)
		runners[slot.slot].Work(index)
		mutateReceiverSlot(slot)
		go func(workerSlot, workerIndex int) { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			runners[workerSlot].Work(workerIndex)
		}(slot.slot, index)
	}
	wg.Wait()
}

type scalarSlot int

func (value *scalarSlot) Advance() { (*value)++ }

func implicitSnapshotBindingMutation(slots []scalarSlot, runners []runner) {
	var wg sync.WaitGroup
	for index, slot := range slots {
		wg.Add(1)
		runners[slot].Work(index)
		slot.Advance()
		go func(workerSlot scalarSlot, workerIndex int) { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			runners[workerSlot].Work(workerIndex)
		}(slot, index)
	}
	wg.Wait()
}

func aliasedSnapshotBindingMutation(slots []scalarSlot, runners []runner) {
	var wg sync.WaitGroup
	for index, slot := range slots {
		alias := &slot
		wg.Add(1)
		runners[slot].Work(index)
		(*alias)++
		go func(workerSlot scalarSlot, workerIndex int) { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			runners[workerSlot].Work(workerIndex)
		}(slot, index)
	}
	wg.Wait()
}

func closureSnapshotBindingMutation(slots []scalarSlot, runners []runner) {
	var wg sync.WaitGroup
	for index, slot := range slots {
		mutate := func() { slot++ }
		wg.Add(1)
		runners[slot].Work(index)
		mutate()
		go func(workerSlot scalarSlot, workerIndex int) { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			runners[workerSlot].Work(workerIndex)
		}(slot, index)
	}
	wg.Wait()
}

func methodValueSnapshotBindingMutation(slots []scalarSlot, runners []runner) {
	var wg sync.WaitGroup
	for index, slot := range slots {
		advance := slot.Advance
		wg.Add(1)
		runners[slot].Work(index)
		advance()
		go func(workerSlot scalarSlot, workerIndex int) { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			runners[workerSlot].Work(workerIndex)
		}(slot, index)
	}
	wg.Wait()
}

func genericPointerReceiverSnapshot[T ~*statefulRunner](values []T) {
	var wg sync.WaitGroup
	for index, value := range values {
		wg.Add(1)
		(*value).Work(index)
		go func(local T, workerIndex int) { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			(*local).Work(workerIndex)
		}(value, index)
	}
	wg.Wait()
}

func genericSliceReceiverSnapshot[T ~[]statefulRunner](groups []T) {
	var wg sync.WaitGroup
	for index, values := range groups {
		wg.Add(1)
		values[0].Work(index)
		go func(local T, workerIndex int) { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			local[0].Work(workerIndex)
		}(values, index)
	}
	wg.Wait()
}

func genericPointerArrayReceiverSnapshot[T ~*[1]statefulRunner](groups []T) {
	var wg sync.WaitGroup
	for index, values := range groups {
		wg.Add(1)
		values[0].Work(index)
		go func(local T, workerIndex int) { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			local[0].Work(workerIndex)
		}(values, index)
	}
	wg.Wait()
}

func effectfulReceiveReceiver(n int, receivers chan *runner) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		(<-receivers).Work(index)
		go func() {
			defer wg.Done()
			(<-receivers).Work(index)
		}()
	}
	wg.Wait()
}

func unrelatedIndexWrite(n int, runners []runner, other []int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		runners[index].Work(index)
		other[index] = 1
		go func() {
			defer wg.Done()
			runners[index].Work(index)
		}()
	}
	wg.Wait()
}

func assignedCallerResult(n int, work func(int) int) {
	results := make([]int, n)
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		results[index] = work(index)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func discardedCallerResult(n int, work func(int) int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		_ = work(index)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func declaredCallerResult(n int, work func(int) int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		var result = work(index)
		_ = result
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func zeroTripCallerLoop(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		for range 0 {
			work(index)
		}
		go func() { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func wrongCallerArguments(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		work(-1)
		go func() { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func callerModeMismatch(n int, work func(int, bool)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		work(index, true)
		go func() { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			work(index, false)
		}()
	}
	wg.Wait()
}

func callerOffsetMismatch(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		work(index + 1)
		go func() { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func variadicMismatch(batches [][]any, work func(int, ...any)) {
	var wg sync.WaitGroup
	for index, args := range batches {
		wg.Add(1)
		work(index, args)
		go func() { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			work(index, args...)
		}()
	}
	wg.Wait()
}

func sameVariadicExpansion(batches [][]any, work func(int, ...any)) {
	var wg sync.WaitGroup
	for index, args := range batches {
		wg.Add(1)
		work(index, args...)
		go func() {
			defer wg.Done()
			work(index, args...)
		}()
	}
	wg.Wait()
}

func variadicLiteralPackingMismatch(batches [][]any, work func(int, ...any)) {
	var wg sync.WaitGroup
	for index, args := range batches {
		wg.Add(1)
		work(index, args...)
		go func(packed ...any) { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			work(index, packed...)
		}(args)
	}
	wg.Wait()
}

func genericWork[T any](int) {}

func genericInstantiationMismatch(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		genericWork[string](index)
		go func() { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			genericWork[int](index)
		}()
	}
	wg.Wait()
}

func sameGenericInstantiation(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		genericWork[int](index)
		go func() {
			defer wg.Done()
			genericWork[int](index)
		}()
	}
	wg.Wait()
}

func callerAfterConditionalContinue(n int, skip func(int) bool, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			work(index)
		}()
		if skip(index) {
			continue
		}
		work(index)
	}
	wg.Wait()
}

func callerAfterEmptySelect(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		select {}
		work(index)
	}
	wg.Wait()
}

func callerAfterInfiniteLoop(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		for {
		}
		work(index)
	}
	wg.Wait()
}

func callerAfterConstantTrueLoop(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		for true {
		}
		work(index)
	}
	wg.Wait()
}

func callerAfterBreakingLoop(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		for {
			break
		}
		work(index)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func unreachableBreakInFanout(work func(int)) {
	var wg sync.WaitGroup
	for {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(0)
		}()
		continue
		break
	}
	wg.Wait()
}

func panicBeforeBreak(work func(int)) {
	var wg sync.WaitGroup
	for {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(0)
		}()
		panic("stop")
		break
	}
	wg.Wait()
}

func panicBeforeLaunch(work func(int)) {
	var wg sync.WaitGroup
	for {
		wg.Add(1)
		panic("stop")
		go func() {
			defer wg.Done()
			work(0)
		}()
		break
	}
	wg.Wait()
}

func mutableCallerArgument(n int, mode bool, work func(int, bool)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		work(index, mode)
		go func() { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			work(index, mode)
		}()
	}
	wg.Wait()
}

// A terminating serial path behind a work threshold means the crossover is
// already tuned; the generic caller-participation advisory stays silent.
func tunedWorkThreshold(n int, work func(int)) {
	if n < 64 {
		for index := range n {
			work(index)
		}
		return
	}
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

func tunedParallelismThreshold(workers int, work func(int)) {
	if workers <= 1 {
		for index := range workers {
			work(index)
		}
		return
	}
	var wg sync.WaitGroup
	for index := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

// Integer validation is not a tuned serial fallback when it performs no
// equivalent worker call.
func validationExit(n int, work func(int)) {
	if n < 0 {
		fmt.Println("invalid")
		return
	}
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `allocation reduction does not prove a latency win`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

// A nested return and a later worker call do not make the threshold branch
// terminate on the path that performed the work.
func nonTerminatingThreshold(n int, disabled bool, work func(int)) {
	if n < 64 {
		if disabled {
			return
		}
		work(0)
	}
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func elseIfThreshold(n int, work func(int)) {
	if n == 0 {
		return
	} else if n < 64 {
		for index := range n {
			work(index)
		}
		return
	}
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

func gotoThreshold(n int, work func(int)) {
	if n < 64 {
		for index := range n {
			work(index)
		}
		goto parallel
		return
	}
parallel:
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func bypassedSerialWork(n int, skip func(int) bool, work func(int)) {
	if n < 64 {
		for index := range n {
			if skip(index) {
				continue
			}
			work(index)
		}
		return
	}
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func mutatedThresholdDomain(n, replacement int, work func(int)) {
	if n < 64 {
		for index := range n {
			work(index)
		}
		return
	}
	n = replacement
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func mutatedThresholdWorker(n int, work, replacement func(int)) {
	if n < 64 {
		for index := range n {
			work(index)
		}
		return
	}
	work = replacement
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func gotoBypassedThreshold(n int, work func(int)) {
	var wg sync.WaitGroup
	goto parallel
	if n < 64 {
		for index := range n {
			work(index)
		}
		return
	}
parallel:
	for index := range n {
		wg.Add(1)
		go func() { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func mutateDomain(value *domain) { value.N++ }

func referentialThresholdDomain(value *domain, work func(int)) {
	if value.N < 64 {
		for index := range value.N {
			work(index)
		}
		return
	}
	mutateDomain(value)
	var wg sync.WaitGroup
	for index := range value.N {
		wg.Add(1)
		go func() { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func mapThresholdDomain(values map[int]bool, work func(int)) {
	if len(values) < 64 {
		for value := range values {
			work(value)
		}
		return
	}
	clear(values)
	var wg sync.WaitGroup
	for value := range values {
		wg.Add(1)
		go func() { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			work(value)
		}()
	}
	wg.Wait()
}

func implicitThresholdMutation(n thresholdCount, work func(thresholdCount)) {
	if n < 64 {
		for index := range n {
			work(index)
		}
		return
	}
	n.Reset()
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

type domainMap struct{ items map[int]bool }

type selectorDomain struct{ count thresholdCount }

type pointerDomain struct{ count *thresholdCount }

type mixedDomain struct {
	count int
	cache map[string]int
}

func safeMixedThreshold(value mixedDomain, work func(int)) {
	if value.count < 64 {
		for index := range value.count {
			work(index)
		}
		return
	}
	var wg sync.WaitGroup
	for index := range value.count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func mutateMixedDomains(values []mixedDomain) { values[0].count++ }

func arraySliceThreshold(values [1]mixedDomain, work func(int)) {
	if values[0].count < 64 {
		for index := range values[0].count {
			work(index)
		}
		return
	}
	mutateMixedDomains(values[:])
	var wg sync.WaitGroup
	for index := range values[0].count {
		wg.Add(1)
		go func() { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func assignedResultThreshold(n int, work func(int) int) {
	if n < 64 {
		for index := range n {
			_ = work(index)
		}
		return
	}
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

func nestedImplicitThresholdMutation(value selectorDomain, work func(thresholdCount)) {
	if value.count < 64 {
		for index := range value.count {
			work(index)
		}
		return
	}
	value.count.Reset()
	var wg sync.WaitGroup
	for index := range value.count {
		wg.Add(1)
		go func() { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func nestedPointerThresholdDomain(value pointerDomain, work func(thresholdCount)) {
	if *value.count < 64 {
		for index := range *value.count {
			work(index)
		}
		return
	}
	value.count.Reset()
	var wg sync.WaitGroup
	for index := range *value.count {
		wg.Add(1)
		go func() { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func nestedReferentialThresholdDomain(value domainMap, work func(int)) {
	if len(value.items) < 64 {
		for item := range value.items {
			work(item)
		}
		return
	}
	clear(value.items)
	var wg sync.WaitGroup
	for item := range value.items {
		wg.Add(1)
		go func() { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			work(item)
		}()
	}
	wg.Wait()
}

func oneUnrelatedThresholdCall(n int, work func(int)) {
	if n < 64 {
		work(-1)
		return
	}
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func wrongSerialArguments(n int, work func(int)) {
	if n < 64 {
		for range n {
			work(-1)
		}
		return
	}
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func unrelatedLenThreshold(chunks, other []int, work func(int)) {
	if len(other) < 64 {
		for _, value := range other {
			work(value)
		}
		return
	}
	var wg sync.WaitGroup
	for index := 0; index < len(chunks); index++ {
		wg.Add(1)
		go func() { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

type domain struct{ N int }

func unrelatedFieldThreshold(selected, other domain, work func(int)) {
	if other.N < 64 {
		for index := range other.N {
			work(index)
		}
		return
	}
	var wg sync.WaitGroup
	for index := 0; index < selected.N; index++ {
		wg.Add(1)
		go func() { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func nonDominatingElseIf(bypass bool, n int, observe func(), work func(int)) {
	if bypass {
		observe()
	} else if n < 64 {
		for index := range n {
			work(index)
		}
		return
	}
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func splitDomainThreshold(n, m int, work func(int)) {
	if n < 64 {
		for index := range m {
			work(index)
		}
		return
	}
	var wg sync.WaitGroup
	for index := range n + m {
		wg.Add(1)
		go func() { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

const sharedDomain = 8

func constantDomainCollision(limit, n int, work func(int)) {
	if limit < sharedDomain {
		for index := range sharedDomain {
			work(index)
		}
		return
	}
	var wg sync.WaitGroup
	for index := range n + sharedDomain {
		wg.Add(1)
		go func() { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

type fakeGroup struct{}

func (*fakeGroup) Add(int) {}
func (*fakeGroup) Done()   {}
func (*fakeGroup) Wait()   {}

// Same spelling is not the standard library WaitGroup.
func lookalike(n int, work func(int)) {
	var group fakeGroup
	for index := range n {
		group.Add(1)
		go func() {
			defer group.Done()
			work(index)
		}()
	}
	group.Wait()
}

// A caller-owned barrier is shared state, not the function-local fork/join
// shape this rule proves.
func callerOwnedBarrier(n int, group *sync.WaitGroup, work func(int)) {
	for index := range n {
		group.Add(1)
		go func() {
			defer group.Done()
			work(index)
		}()
	}
	group.Wait()
}

func callerOwnedAlias(n int, group *sync.WaitGroup, work func(int)) {
	local := group
	for index := range n {
		local.Add(1)
		go func() {
			defer local.Done()
			work(index)
		}()
	}
	local.Wait()
}

var packageGroup sync.WaitGroup

var escapedPackagePtr *sync.WaitGroup

func publishGroup(*sync.WaitGroup) {}

func escapedBarrier(n int, work func(int)) {
	wg := new(sync.WaitGroup)
	escapedPackagePtr = wg
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func publishedBarrier(n int, work func(int)) {
	wg := new(sync.WaitGroup)
	publishGroup(wg)
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func copiedBarrier(n int, work func(int)) {
	wg := new(sync.WaitGroup)
	copied := *wg
	_ = copied
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func packageOwnedAlias(n int, work func(int)) {
	local := &packageGroup
	for index := range n {
		local.Add(1)
		go func() {
			defer local.Done()
			work(index)
		}()
	}
	local.Wait()
}

func indirectCallerAlias(n int, caller *sync.WaitGroup, work func(int)) {
	local := new(sync.WaitGroup)
	alias := &local
	*alias = caller
	for index := range n {
		local.Add(1)
		go func() {
			defer local.Done()
			work(index)
		}()
	}
	local.Wait()
}

// No Add means this is not a complete WaitGroup fan-out proof.
func missingAdd(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

// Add after launch is not the safe fan-out protocol and must not be treated as
// evidence for the caller-participation shape.
func addAfterLaunch(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		go func() {
			defer wg.Done()
			work(index)
		}()
		wg.Add(1)
	}
	wg.Wait()
}

func returnAfterLaunch(n int, stop bool, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		if stop {
			return
		}
	}
	wg.Wait()
}

func gotoAfterLaunch(n int, stop bool, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		if stop {
			goto escaped
		}
	}
	wg.Wait()
escaped:
}

func assignedLoopCapture(n int, work func(int)) {
	var wg sync.WaitGroup
	index := 0
	for index = 0; index < n; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func noInitAssignedLoopCapture(n int, work func(int)) {
	var wg sync.WaitGroup
	index := 0
	for ; index < n; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func addressSnapshotIsNotValueSnapshot(n int, work func(int)) {
	var wg sync.WaitGroup
	index := 0
	for index = 0; index < n; index++ {
		wg.Add(1)
		go func(pointer *int) {
			defer wg.Done()
			work(*pointer)
		}(&index)
	}
	wg.Wait()
}

func implicitMethodValueIsNotSnapshot(n int) {
	var wg sync.WaitGroup
	index := counter(0)
	for index = 0; index < counter(n); index++ {
		wg.Add(1)
		go func(callback func()) {
			defer wg.Done()
			callback()
		}(index.Work)
	}
	wg.Wait()
}

type loopItem struct{ value int }

func mutatedFreshLoopField(items []loopItem, work func(int)) {
	var wg sync.WaitGroup
	for _, item := range items {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(item.value)
		}()
		item.value++
	}
	wg.Wait()
}

func arraySliceIsNotValueSnapshot(items [][1]int, work func(int)) {
	var wg sync.WaitGroup
	var item [1]int
	for _, item = range items {
		wg.Add(1)
		go func(value []int) {
			defer wg.Done()
			work(value[0])
		}(item[:])
	}
	wg.Wait()
}

func mutateArray(values []int) { values[0]++ }

func arraySliceMutatesFreshCapture(items [][1]int, work func(int)) {
	var wg sync.WaitGroup
	for _, item := range items {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(item[0])
		}()
		mutateArray(item[:])
	}
	wg.Wait()
}

func reassignedFreshLoopVariable(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := 0; index < n; index++ {
		index = n - index
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func assignedRangeCapture(values []int, work func(int)) {
	var wg sync.WaitGroup
	var value int
	for _, value = range values {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(value)
		}()
	}
	wg.Wait()
}

func assignedLoopSnapshot(n int, work func(int)) {
	var wg sync.WaitGroup
	index := 0
	for index = 0; index < n; index++ {
		wg.Add(1)
		go func(index int) { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			work(index)
		}(index)
	}
	wg.Wait()
}

func addInForInitializer(n int, work func(int)) {
	var wg sync.WaitGroup
	index := 0
	for wg.Add(1); index < n; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			work(index)
		}(index)
	}
	wg.Wait()
}

func activeOuterGeneration(n int, enabled bool, other func(), work func(int)) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		other()
	}()
	if enabled {
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

func backedgeActiveGeneration(n int, other func(int), work func(int)) {
	var wg sync.WaitGroup
	for batch := range 2 {
		for index := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				work(index)
			}()
		}
		wg.Wait()
		wg.Add(1)
		go func() {
			defer wg.Done()
			other(batch)
		}()
	}
}

func callableBackedgeMutation(n int, work, replacement func(int)) {
	for range 2 {
		var wg sync.WaitGroup
		for index := range n {
			wg.Add(1)
			work(index)
			go func() { // want `the caller only waits on a function-local sync.WaitGroup`
				defer wg.Done()
				work(index)
			}()
		}
		wg.Wait()
		go func() { work = replacement }()
	}
}

func stableValueReceiverBackedge(n int) {
	var local runner
	for range 2 {
		var wg sync.WaitGroup
		for index := range n {
			wg.Add(1)
			local.Work(index)
			go func() {
				defer wg.Done()
				local.Work(index)
			}()
		}
		wg.Wait()
		local.Work(0)
	}
}

func thresholdBackedgeMutation(n, replacement int, work func(int)) {
	for range 2 {
		if n < 64 {
			for index := range n {
				work(index)
			}
			return
		}
		var wg sync.WaitGroup
		for index := range n {
			wg.Add(1)
			go func() { // want `the caller only waits on a function-local sync.WaitGroup`
				defer wg.Done()
				work(index)
			}()
		}
		wg.Wait()
		go func() { n = replacement }()
	}
}

func touchGroup(*sync.WaitGroup) {}

func backedgeBarrierEscape(n int, work func(int)) {
	var wg sync.WaitGroup
	for range 2 {
		for index := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				work(index)
			}()
		}
		wg.Wait()
		go touchGroup(&wg)
	}
}

func doubleAdd(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func inlineExtraDone(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		func() { wg.Done() }()
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func preLoopAndPerIterationAdd(n int, work func(int)) {
	var wg sync.WaitGroup
	wg.Add(1)
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

// The rule is deliberately limited to an immediate fork/join boundary.
func nonImmediateWait(n int, work func(int), observe func()) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	observe()
	wg.Wait()
}

// Conditional launch does not prove that every chunk becomes a goroutine.
func conditionalLaunch(n int, enabled bool, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		if enabled {
			go func() {
				defer wg.Done()
				work(index)
			}()
		} else {
			wg.Done()
		}
	}
	wg.Wait()
}

func skippedChunks(n int, skip func(int) bool, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		if skip(index) {
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

// Conditional completion is not a proven fork/join protocol.
func conditionalCompletion(n int, enabled bool, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			if enabled {
				defer wg.Done()
			}
			work(index)
		}()
	}
	wg.Wait()
}

// Conditional Add likewise does not dominate every launch.
func conditionalAdd(n int, enabled bool, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		if enabled {
			wg.Add(1)
		}
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

// Done before the worker body means Wait does not join the actual work.
func earlyCompletion(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func completionAfterEarlyExit(n int, skip bool, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			if skip {
				return
			}
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func terminalCompletionAfterEarlyExit(n int, skip bool, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			if skip {
				return
			}
			work(index)
			wg.Done()
		}()
	}
	wg.Wait()
}

func workerChangesBarrier(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wg.Add(1)
			work(index)
		}()
	}
	wg.Wait()
}

func multipleLaunches(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work(index)
		}()
		go func() { work(index) }()
	}
	wg.Wait()
}

func reassignedWorker(n int, work, replacement func(int)) {
	worker := work
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		go func() { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			worker(index)
		}()
		worker = replacement
		worker(index)
	}
	wg.Wait()
}

func workerReassignedAfterWait(n int, work, replacement func(int)) {
	worker := work
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		worker(index)
		go func() {
			defer wg.Done()
			worker(index)
		}()
	}
	wg.Wait()
	worker = replacement
}

func selfMutatingWorker(n int, replacement func(int)) {
	var worker func(int)
	worker = func(int) { worker = replacement }
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		worker(index)
		go func() { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			worker(index)
		}()
	}
	wg.Wait()
}

func indirectlyMutatedWorker(n int, work, replacement func(int)) {
	pointer := &work
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		work(index)
		*pointer = replacement
		go func() { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func closureMutatedWorker(n int, work, replacement func(int)) {
	replace := func() { work = replacement }
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		work(index)
		replace()
		go func() { // want `all-goroutine fan-out launches every chunk`
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

var currentWorker func(int)

func init() { currentWorker = replaceCurrentWorker }

func replaceCurrentWorker(int) { currentWorker = func(int) {} }

func mutablePackageWorker(n int) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(1)
		currentWorker(index)
		go func() { // want `the caller only waits on a function-local sync.WaitGroup`
			defer wg.Done()
			currentWorker(index)
		}()
	}
	wg.Wait()
}

func zeroAdd(n int, work func(int)) {
	var wg sync.WaitGroup
	for index := range n {
		wg.Add(0)
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

func bulkAdd(n int, work func(int)) {
	var wg sync.WaitGroup
	wg.Add(n)
	for index := range n {
		go func() {
			defer wg.Done()
			work(index)
		}()
	}
	wg.Wait()
}

// Completion and wait refer to different barriers.
func mismatchedBarrier(n int, work func(int)) {
	var launched sync.WaitGroup
	var waited sync.WaitGroup
	for index := range n {
		launched.Add(1)
		go func() {
			defer launched.Done()
			work(index)
		}()
	}
	waited.Wait()
}

// An indirect worker may own a different completion protocol; decline it.
func indirectWorker(n int, worker func()) {
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		go worker()
	}
	wg.Wait()
}

// A one-off goroutine is not a loop fan-out.
func oneGoroutine(work func()) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		work()
	}()
	wg.Wait()
}

// Benchmark harness orchestration is deliberately out of scope.
func BenchmarkSynthetic(n int, work func(int)) {
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
