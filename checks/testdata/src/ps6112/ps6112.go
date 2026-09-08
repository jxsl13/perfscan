package ps6112

import "sync/atomic"

func schedule(rows int) { // want "schedule selects grain 30 for target darwin/arm64\\+ps6112simd and a 4-row microkernel tile \\(30%4=2\\); every repeated full band can enter ps6112.scalarTail"
	bands := (rows + bandRows - 1) / bandRows
	for task := 0; task < bands; task++ {
		start := task * bandRows
		count := min(bandRows, rows-start)
		band(start, count)
	}
}

func ownerSchedule(rows, heads int) { // want "ownerSchedule selects grain 30 for target darwin/arm64\\+ps6112simd"
	bands := (rows + bandRows - 1) / bandRows
	tasks := heads * bands
	var next atomic.Int64
	parallel(func() {
		for {
			task := int(next.Add(1)) - 1
			if task >= tasks {
				return
			}
			bandIndex := bands - 1 - task/heads
			start := bandIndex * bandRows
			count := min(bandRows, rows-start)
			band(start, count)
		}
	})
}

func arbitraryTaskSchedule(rows, heads int) {
	bands := (rows + bandRows - 1) / bandRows
	tasks := heads * bands
	parallel(func() {
		for {
			task := nextTask()
			if task >= tasks {
				return
			}
			bandIndex := bands - 1 - task/heads
			start := bandIndex * bandRows
			count := min(bandRows, rows-start)
			band(start, count)
		}
	})
}

func typedIntSchedule(rows int) { // want "typedIntSchedule selects grain 30 for target darwin/arm64\\+ps6112simd"
	bands := (rows + typedBandRows - 1) / typedBandRows
	for task := range bands {
		start := task * typedBandRows
		count := min(typedBandRows, rows-start)
		band(start, count)
	}
}

func localGrainShadow(rows int) {
	const bandRows = 32
	bands := (rows + bandRows - 1) / bandRows
	for task := range bands {
		start := task * bandRows
		count := min(bandRows, rows-start)
		band(start, count)
	}
}

func shiftedStart(rows int) {
	bands := (rows + bandRows - 1) / bandRows
	for task := range bands {
		start := rows + task*bandRows
		count := min(bandRows, rows-start)
		band(start, count)
	}
}

func zeroTripStart(rows int) {
	bands := (rows + bandRows - 1) / bandRows
	for task := bands + 1; task < bands; task++ {
		start := task * bandRows
		count := min(bandRows, rows-start)
		band(start, count)
	}
}

func staticallyPartialOnly() {
	rows := 12
	bands := (rows + bandRows - 1) / bandRows
	for task := range bands {
		start := task * bandRows
		count := min(bandRows, rows-start)
		band(start, count)
	}
}

func skippingPost(rows int) {
	bands := (rows + bandRows - 1) / bandRows
	for task := 0; task < bands; task += 2 {
		start := task * bandRows
		count := min(bandRows, rows-start)
		band(start, count)
	}
}

func inclusiveCondition(rows int) {
	bands := (rows + bandRows - 1) / bandRows
	for task := 0; task <= bands; task++ {
		start := task * bandRows
		count := min(bandRows, rows-start)
		band(start, count)
	}
}

func narrowSchedule(rows uint8) {
	bands := (rows + narrowBandRows - 1) / narrowBandRows
	for task := range bands {
		start := task * narrowBandRows
		count := min(narrowBandRows, rows-start)
		narrowBand(start, count)
	}
}

func incompleteVariantSchedule(rows int) {
	bands := (rows + bandRows - 1) / bandRows
	for task := range bands {
		start := task * bandRows
		count := min(bandRows, rows-start)
		band(start, count)
	}
}

func storedForwardSchedule(rows int) {
	bands := (rows + bandRows - 1) / bandRows
	for task := range bands {
		start := task * bandRows
		count := min(bandRows, rows-start)
		storedBand(start, count)
	}
}

func storedSchedulerClosure(rows int) {
	bands := (rows + bandRows - 1) / bandRows
	work := func() {
		for task := range bands {
			start := task * bandRows
			count := min(bandRows, rows-start)
			band(start, count)
		}
	}
	_ = work
}

func returnedSchedulerClosure(rows int) func() {
	bands := (rows + bandRows - 1) / bandRows
	return func() {
		for task := range bands {
			start := task * bandRows
			count := min(bandRows, rows-start)
			band(start, count)
		}
	}
}

func retainedSchedulerClosure(rows int) {
	bands := (rows + bandRows - 1) / bandRows
	retain(func() {
		for task := range bands {
			start := task * bandRows
			count := min(bandRows, rows-start)
			band(start, count)
		}
	})
}

func appendedSchedulerClosure(rows int) {
	bands := (rows + bandRows - 1) / bandRows
	callbacks = append(callbacks, func() {
		for task := range bands {
			start := task * bandRows
			count := min(bandRows, rows-start)
			band(start, count)
		}
	})
}

func immediateSchedulerClosure(rows int) { // want "immediateSchedulerClosure selects grain 30 for target darwin/arm64\\+ps6112simd"
	bands := (rows + bandRows - 1) / bandRows
	func() {
		for task := range bands {
			start := task * bandRows
			count := min(bandRows, rows-start)
			band(start, count)
		}
	}()
}

func wrongRunnerSlot(rows int) {
	bands := (rows + bandRows - 1) / bandRows
	parallelLabeled("bands", func() {
		for task := range bands {
			start := task * bandRows
			count := min(bandRows, rows-start)
			band(start, count)
		}
	})
}

func reboundExtent(rows, replacement int) {
	bands := (rows + bandRows - 1) / bandRows
	rows = replacement
	for task := range bands {
		start := task * bandRows
		count := min(bandRows, rows-start)
		band(start, count)
	}
}

func addressedExtent(rows int) {
	bands := (rows + bandRows - 1) / bandRows
	mutateRows(&rows)
	for task := range bands {
		start := task * bandRows
		count := min(bandRows, rows-start)
		band(start, count)
	}
}

func aliasedExtent(rows int) {
	pointer := &rows
	bands := (rows + bandRows - 1) / bandRows
	*pointer = 96
	for task := range bands {
		start := task * bandRows
		count := min(bandRows, rows-start)
		band(start, count)
	}
}

func closureMutatedExtent(rows int) {
	bands := (rows + bandRows - 1) / bandRows
	mutate := func() { rows = 96 }
	mutate()
	for task := range bands {
		start := task * bandRows
		count := min(bandRows, rows-start)
		band(start, count)
	}
}

func rangeAssignedExtent(rows int, sequence []int) {
	bands := (rows + bandRows - 1) / bandRows
	for rows = range sequence {
	}
	for task := range bands {
		start := task * bandRows
		count := min(bandRows, rows-start)
		band(start, count)
	}
}

func snapshotExtent(rows, replacement int) { // want "snapshotExtent selects grain 30 for target darwin/arm64\\+ps6112simd"
	snapshot := rows
	bands := (snapshot + bandRows - 1) / bandRows
	rows = replacement
	mutateRows(&rows)
	for task := range bands {
		start := task * bandRows
		count := min(bandRows, snapshot-start)
		band(start, count)
	}
}

func loopCarriedExtent(rows, replacement int) {
	bands := (rows + bandRows - 1) / bandRows
	for task := range bands {
		start := task * bandRows
		count := min(bandRows, rows-start)
		band(start, count)
		rows = replacement
	}
}

func stableLoopCarriedExtent(rows, replacement int) { // want "stableLoopCarriedExtent selects grain 30 for target darwin/arm64\\+ps6112simd"
	bands := (rows + bandRows - 1) / bandRows
	for task := range bands {
		start := task * bandRows
		count := min(bandRows, rows-start)
		band(start, count)
		replacement++
	}
}

//perfscan:tile-grain-intentional measured asymmetric-core load balance wins
func intentional(rows int) {
	bands := (rows + bandRows - 1) / bandRows
	for task := range bands {
		start := task * bandRows
		count := min(bandRows, rows-start)
		band(start, count)
	}
}

func profiled(rows int) {
	//perfscan:tile-grain-profiled retained after full routed AB/BA matrix
	bands := (rows + bandRows - 1) / bandRows
	for task := range bands {
		start := task * bandRows
		count := min(bandRows, rows-start)
		band(start, count)
	}
}

//perfscan:tile-grain-intentional
func bareMarker(rows int) { // want "bareMarker selects grain 30 for target darwin/arm64\\+ps6112simd"
	bands := (rows + bandRows - 1) / bandRows
	for task := range bands {
		start := task * bandRows
		count := min(bandRows, rows-start)
		band(start, count)
	}
}

func deadSchedule(rows int) {
	if false {
		bands := (rows + bandRows - 1) / bandRows
		for task := range bands {
			start := task * bandRows
			count := min(bandRows, rows-start)
			band(start, count)
		}
	}
}

func noCeiling(rows int) {
	bands := rows / bandRows
	for task := range bands {
		start := task * bandRows
		count := min(bandRows, rows-start)
		band(start, count)
	}
}

func falseCeiling(rows int) {
	bands := rows * bandRows / bandRows
	for task := range bands {
		start := task * bandRows
		count := min(bandRows, rows-start)
		band(start, count)
	}
}

func noFinalClamp(rows int) {
	bands := (rows + bandRows - 1) / bandRows
	for task := range bands {
		start := task * bandRows
		band(start, bandRows)
	}
}

func falseClamp(rows int) {
	bands := (rows + bandRows - 1) / bandRows
	for task := range bands {
		start := task * bandRows
		count := min(start, rows)
		band(start, count)
	}
}

func fakeBand(rows int) {
	band := func(_, _ int) {}
	bands := (rows + bandRows - 1) / bandRows
	for task := range bands {
		start := task * bandRows
		count := min(bandRows, rows-start)
		band(start, count)
	}
}

func fakeMin(rows int) {
	min := func(left, right int) int {
		if left < right {
			return left
		}
		return right
	}
	bands := (rows + bandRows - 1) / bandRows
	for task := range bands {
		start := task * bandRows
		count := min(bandRows, rows-start)
		band(start, count)
	}
}

func aliasedBandRows(rows int) {
	bands := (rows + bandRows - 1) / bandRows
	for task := range bands {
		start := task * bandRows
		count := min(bandRows, rows-start)
		forwarded := count
		band(start, forwarded)
	}
}

func aliasedKernelRows(rows int) {
	bands := (rows + bandRows - 1) / bandRows
	for task := range bands {
		start := task * bandRows
		count := min(bandRows, rows-start)
		bandAlias(start, count)
	}
}

func missingFallback(rows int) {
	bands := (rows + bandRows - 1) / bandRows
	for task := range bands {
		start := task * bandRows
		count := min(bandRows, rows-start)
		band(start, count)
	}
}

func dynamicGrain(rows int) {
	bands := (rows + dynamicBandRows - 1) / dynamicBandRows
	for task := range bands {
		start := task * dynamicBandRows
		count := min(dynamicBandRows, rows-start)
		band(start, count)
	}
}

func overlapSchedule(rows int) {
	bands := (rows + overlapRows - 1) / overlapRows
	for task := range bands {
		start := task * overlapRows
		count := min(overlapRows, rows-start)
		band(start, count)
	}
}

func band(start, count int) {
	kernelEntry(nil, start, count)
}

func bandAlias(start, count int) {
	forwarded := count
	kernelEntry(nil, start, forwarded)
}

func narrowBand(start, count uint8) {
	narrowKernelEntry(nil, start, count)
}

func narrowKernelEntry(_ []float32, _, _ uint8) {}

func storedBand(start, count int) {
	_ = func() {
		kernelEntry(nil, start, count)
	}
}

var dynamicBandRows = 30
var callbacks []func()

func scalarTail(_ []float32, _, _ int) {}

func parallel(work func()) { work() }
func nextTask() int        { return 0 }
func retain(func())        {}
func mutateRows(rows *int) { *rows = 96 }
func parallelLabeled(_ string, work func()) {
	work()
}
