package ps6090

import (
	"ps6090dep"
	"runtime"
	"testing"
)

type Tensor struct {
	Values []float64
}

func QMatMul() (*Tensor, error) { return &Tensor{Values: []float64{1}}, nil }
func ComputeOnly() int          { return 1 }
func OnlyError() error          { return nil }
func Unconfigured() (*Tensor, error) {
	return &Tensor{}, nil
}

type engine struct{}

func (engine) Compute() (*Tensor, error) { return &Tensor{}, nil }

func GenericCompute[T any](value T) (T, error) { return value, nil }

type benchmarkError struct{}

func (benchmarkError) Error() string { return "benchmark" }

func ErrorFirst() (benchmarkError, *Tensor) { return benchmarkError{}, &Tensor{} }

var tensorSink *Tensor
var intSink int

func BenchmarkDirectBlank(b *testing.B) {
	for range b.N {
		_, err := QMatMul() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.QMatMul`
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCanonicalIndexLoop(b *testing.B) {
	for index := 0; index < b.N; index++ {
		ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
	}
}

func BenchmarkLoopExpressionStatement(b *testing.B) {
	for b.Loop() {
		ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
	}
}

func BenchmarkGoAndDeferStatements(b *testing.B) {
	for range b.N {
		go ComputeOnly()    // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		defer ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
	}
}

func BenchmarkBoundThenBlank(b *testing.B) {
	for b.Loop() {
		result, err := QMatMul() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.QMatMul`
		_ = result
		if err != nil {
			b.Fatal(err)
		}
	}
}

func discardInHelper() {
	_, _ = QMatMul() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.QMatMul`
}

func BenchmarkCallsHelperInLoop(b *testing.B) {
	for b.Loop() {
		discardInHelper()
	}
}

func helperOwnsBenchmarkLoop(b *testing.B) {
	for range b.N {
		ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
	}
}

func BenchmarkHelperOwnsLoop(b *testing.B) {
	helperOwnsBenchmarkLoop(b)
}

func BenchmarkMethodAndGeneric(b *testing.B) {
	e := engine{}
	for b.Loop() {
		_, _ = e.Compute()              // want `benchmark repetition discards the primary result of configured pure compute call ps6090.engine.Compute`
		_, _ = GenericCompute[int](123) // want `benchmark repetition discards the primary result of configured pure compute call ps6090.GenericCompute`
		_, _ = engine.Compute(e)        // want `benchmark repetition discards the primary result of configured pure compute call ps6090.engine.Compute`
		_, _ = (QMatMul)()              // want `benchmark repetition discards the primary result of configured pure compute call ps6090.QMatMul`
	}
}

func BenchmarkValueSpec(b *testing.B) {
	for b.Loop() {
		var result, err = QMatMul() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.QMatMul`
		_ = result
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkErrorFirst(b *testing.B) {
	for b.Loop() {
		_, _ = ErrorFirst() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ErrorFirst`
	}
}

func subBenchmark(b *testing.B) {
	for b.Loop() {
		ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
	}
}

func BenchmarkSubtests(b *testing.B) {
	b.Run("literal", func(b *testing.B) {
		for b.Loop() {
			_, _ = QMatMul() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.QMatMul`
		}
	})
	b.Run("named", subBenchmark)
}

func BenchmarkParallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		}
	})
}

func BenchmarkImmediatelyInvoked(b *testing.B) {
	for b.Loop() {
		func() {
			ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		}()
	}
}

func liveReturn() (*Tensor, error) {
	return QMatMul()
}

func consume(*Tensor) {}

func BenchmarkLiveResults(b *testing.B) {
	for b.Loop() {
		result, err := QMatMul()
		tensorSink = result
		if err != nil {
			b.Fatal(err)
		}

		result, err = QMatMul()
		consume(result)
		if err != nil {
			b.Fatal(err)
		}

		result, err = liveReturn()
		if result == nil || err != nil {
			b.Fatal(result, err)
		}

		value := ComputeOnly()
		intSink = value

		result, err = QMatMul()
		runtime.KeepAlive(result)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSetupIsNotTimedRepetition(b *testing.B) {
	_, _ = QMatMul()
	for range 10 {
		_, _ = QMatMul()
	}
	for b.Loop() {
		result, err := QMatMul()
		tensorSink = result
		if err != nil {
			b.Fatal(err)
		}
	}
}

func helperOutsideLoopOnly() {
	_, _ = QMatMul()
}

func BenchmarkHelperOutsideLoopOnly(b *testing.B) {
	helperOutsideLoopOnly()
	for b.Loop() {
		intSink = ComputeOnly()
	}
}

func BenchmarkLoopInitIsNotRepetition(b *testing.B) {
	for ComputeOnly(); b.Loop(); {
	}
	for helperOutsideLoopOnly(); b.Loop(); {
	}
}

func loopPostHelper() {
	ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
}

func BenchmarkLoopPostIsRepetition(b *testing.B) {
	for ; b.Loop(); ComputeOnly() { // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
	}
	for ; b.Loop(); loopPostHelper() {
	}
}

func getError() error { return nil }

func BenchmarkMultiExpressionAssignments(b *testing.B) {
	for b.Loop() {
		_, err := ComputeOnly(), getError() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		if err != nil {
			b.Fatal(err)
		}
		{
			var _, otherErr = ComputeOnly(), getError() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
			if otherErr != nil {
				b.Fatal(otherErr)
			}
		}
		{
			err, _ := getError(), ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
			if err != nil {
				b.Fatal(err)
			}
		}
		intSink, _ = ComputeOnly(), getError()
		_, intSink = getError(), ComputeOnly()
	}
}

func consumeInt(int) {}

func BenchmarkStaticallyDeadResultRead(b *testing.B) {
	for b.Loop() {
		result := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = result
		if false {
			consumeInt(result)
		}

		gotoResult := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = gotoResult
		goto afterDeadRead
		consumeInt(gotoResult)
	afterDeadRead:

		liveResult := ComputeOnly()
		_ = liveResult
		if b.N >= 0 {
			consumeInt(liveResult)
		}
	}
}

func BenchmarkReassignedFreshResults(b *testing.B) {
	for b.Loop() {
		result := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = result
		result = 0
		consumeInt(result)

		compound := ComputeOnly()
		_ = compound
		compound += 1
		consumeInt(compound)

		maybeOverwritten := ComputeOnly()
		_ = maybeOverwritten
		if b.N < 0 {
			maybeOverwritten = 0
		}
		consumeInt(maybeOverwritten)

		constantIf := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = constantIf
		if true {
			constantIf = 0
		} else {
			consumeInt(constantIf)
		}
		consumeInt(constantIf)

		constantSwitch := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = constantSwitch
		switch 1 {
		case 0:
			consumeInt(constantSwitch)
		case 1:
			constantSwitch = 0
		default:
			consumeInt(constantSwitch)
		}
		consumeInt(constantSwitch)

		forInit := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = forInit
		for forInit = 0; false; {
		}
		consumeInt(forInit)

		forPost := ComputeOnly()
		_ = forPost
		for ; b.N < 0; forPost = 0 {
		}
		consumeInt(forPost)

		{
			jumped := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
			_ = jumped
			goto overwritten
			consumeInt(jumped)
		overwritten:
			jumped = 0
			consumeInt(jumped)
		}

		{
			nested := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
			_ = nested
			{
				nested = 0
			}
			consumeInt(nested)
		}

		{
			switchValue := ComputeOnly()
			_ = switchValue
			switch b.N {
			case 0:
				switchValue = 0
			default:
				consumeInt(switchValue)
			}
		}

		{
			selectValue := ComputeOnly()
			_ = selectValue
			values := make(chan int)
			select {
			case selectValue = <-values:
			default:
				consumeInt(selectValue)
			}
		}

		rangeBodyValue := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = rangeBodyValue
		for rangeBodyValue = range []int{1} {
			consumeInt(rangeBodyValue)
		}

		rangeZeroValue := ComputeOnly()
		_ = rangeZeroValue
		for rangeZeroValue = range []int{} {
		}
		consumeInt(rangeZeroValue)

		rangeIndexValues := make([]int, 1)
		rangeEmptyIndex := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		for rangeIndexValues[rangeEmptyIndex] = range []int{} {
		}
		_ = rangeEmptyIndex

		var zeroLengthArray [0]int
		rangeZeroArrayIndex := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		for rangeIndexValues[rangeZeroArrayIndex] = range zeroLengthArray {
		}
		_ = rangeZeroArrayIndex

		rangeZeroArrayPointerIndex := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		for rangeIndexValues[rangeZeroArrayPointerIndex] = range &zeroLengthArray {
		}
		_ = rangeZeroArrayPointerIndex

		rangeMadeEmptySliceIndex := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		for rangeIndexValues[rangeMadeEmptySliceIndex] = range make([]int, 0) {
		}
		_ = rangeMadeEmptySliceIndex

		rangeNilSliceIndex := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		for rangeIndexValues[rangeNilSliceIndex] = range []int(nil) {
		}
		_ = rangeNilSliceIndex

		const zeroRangeCount = 0
		rangeNamedZeroIndex := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		for rangeIndexValues[rangeNamedZeroIndex] = range zeroRangeCount {
		}
		_ = rangeNamedZeroIndex

		const emptyRangeString = ""
		rangeNamedEmptyStringIndex := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		for rangeIndexValues[rangeNamedEmptyStringIndex] = range emptyRangeString {
		}
		_ = rangeNamedEmptyStringIndex

		rangeMadeEmptyMapIndex := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		for rangeIndexValues[rangeMadeEmptyMapIndex] = range make(map[int]int) {
		}
		_ = rangeMadeEmptyMapIndex

		rangeReservedEmptyMapIndex := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		for rangeIndexValues[rangeReservedEmptyMapIndex] = range make(map[int]int, b.N) {
		}
		_ = rangeReservedEmptyMapIndex

		nonemptyArrayTarget := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = nonemptyArrayTarget
		for nonemptyArrayTarget = range [1]int{} {
		}
		consumeInt(nonemptyArrayTarget)

		const positiveRangeCount = 1
		nonemptyIntegerTarget := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = nonemptyIntegerTarget
		for nonemptyIntegerTarget = range positiveRangeCount {
		}
		consumeInt(nonemptyIntegerTarget)

		const nonemptyRangeString = "x"
		nonemptyStringTarget := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = nonemptyStringTarget
		for nonemptyStringTarget = range nonemptyRangeString {
		}
		consumeInt(nonemptyStringTarget)

		nonemptySliceTarget := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = nonemptySliceTarget
		for nonemptySliceTarget = range []int{1} {
		}
		consumeInt(nonemptySliceTarget)

		nonemptyMadeSliceTarget := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = nonemptyMadeSliceTarget
		for nonemptyMadeSliceTarget = range make([]int, 1) {
		}
		consumeInt(nonemptyMadeSliceTarget)

		nilArrayPointerValueTarget := ComputeOnly()
		_ = nilArrayPointerValueTarget
		func() {
			defer func() { _ = recover() }()
			var value int
			for nilArrayPointerValueTarget, value = range (*[1]int)(nil) {
			}
			_ = value
		}()
		consumeInt(nilArrayPointerValueTarget)

		nilArrayPointerBlankTarget := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = nilArrayPointerBlankTarget
		for nilArrayPointerBlankTarget, _ = range (*[1]int)(nil) {
		}
		consumeInt(nilArrayPointerBlankTarget)

		nonNilArrayPointerValueTarget := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = nonNilArrayPointerValueTarget
		var nonNilArrayPointerValue int
		for nonNilArrayPointerValueTarget, nonNilArrayPointerValue = range &[1]int{1} {
		}
		consumeInt(nonNilArrayPointerValueTarget)
		_ = nonNilArrayPointerValue

		addressOfNilDereferenceTarget := ComputeOnly()
		_ = addressOfNilDereferenceTarget
		func() {
			defer func() { _ = recover() }()
			pointer := (*[1]int)(nil)
			var value int
			for addressOfNilDereferenceTarget, value = range &*pointer {
			}
			_ = value
		}()
		consumeInt(addressOfNilDereferenceTarget)

		rangeNonemptyIndex := ComputeOnly()
		for rangeIndexValues[rangeNonemptyIndex] = range []int{1} {
		}
		_ = rangeNonemptyIndex

		rangeArrayIndex := ComputeOnly()
		for rangeIndexValues[rangeArrayIndex] = range [1]int{} {
		}
		_ = rangeArrayIndex

		receiveValues := make(chan int)
		receiveBodyValue := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = receiveBodyValue
		select {
		case receiveBodyValue = <-receiveValues:
			consumeInt(receiveBodyValue)
		default:
		}

		receiveUnselectedValue := ComputeOnly()
		_ = receiveUnselectedValue
		select {
		case receiveUnselectedValue = <-receiveValues:
		default:
			consumeInt(receiveUnselectedValue)
		}

		nilReceiveIndex := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		select {
		case rangeIndexValues[nilReceiveIndex] = <-(chan int)(nil):
		default:
		}
		_ = nilReceiveIndex

		selectableIndex := ComputeOnly()
		readyValues := make(chan int, 1)
		readyValues <- 1
		select {
		case rangeIndexValues[selectableIndex] = <-readyValues:
		default:
		}
		_ = selectableIndex

		{
			nil := make(chan int, 1)
			nil <- 1
			shadowedNilReceiveIndex := ComputeOnly()
			select {
			case rangeIndexValues[shadowedNilReceiveIndex] = <-(chan int)(nil):
			default:
			}
			_ = shadowedNilReceiveIndex

			shadowedNilSendBody := ComputeOnly()
			select {
			case (chan int)(nil) <- 1:
				consumeInt(shadowedNilSendBody)
			default:
			}
			_ = shadowedNilSendBody
		}

		nilSendBody := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		select {
		case (chan int)(nil) <- 1:
			consumeInt(nilSendBody)
		default:
		}
		_ = nilSendBody

		capturedBeforeOverwrite := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = capturedBeforeOverwrite
		captureOldValue := func() {
			consumeInt(capturedBeforeOverwrite)
		}
		capturedBeforeOverwrite = 0
		_ = captureOldValue

		capturedAfterOverwrite := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = capturedAfterOverwrite
		capturedAfterOverwrite = 0
		captureNewValue := func() {
			consumeInt(capturedAfterOverwrite)
		}
		_ = captureNewValue

		invokedBeforeOverwrite := ComputeOnly()
		_ = invokedBeforeOverwrite
		readOldValue := func() {
			consumeInt(invokedBeforeOverwrite)
		}
		readOldValue()
		invokedBeforeOverwrite = 0

		invokedAfterOverwrite := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = invokedAfterOverwrite
		readNewValue := func() {
			consumeInt(invokedAfterOverwrite)
		}
		invokedAfterOverwrite = 0
		readNewValue()

		invokedWriter := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = invokedWriter
		overwriteCapturedValue := func() {
			invokedWriter = 0
		}
		overwriteCapturedValue()
		consumeInt(invokedWriter)

		nonreturningClosurePath := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = nonreturningClosurePath
		func() {
			if b.N < 0 {
				panic("stop")
			}
			nonreturningClosurePath = 0
		}()
		consumeInt(nonreturningClosurePath)

		testingTerminatorClosurePath := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = testingTerminatorClosurePath
		func() {
			if b.N < 0 {
				b.Fatal("stop")
			}
			testingTerminatorClosurePath = 0
		}()
		consumeInt(testingTerminatorClosurePath)

		recoveringClosurePath := ComputeOnly()
		_ = recoveringClosurePath
		func() {
			defer func() { _ = recover() }()
			if b.N < 0 {
				panic("stop")
			}
			recoveringClosurePath = 0
		}()
		consumeInt(recoveringClosurePath)

		correlatedRecoverPath := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = correlatedRecoverPath
		func() {
			if b.N < 0 {
				correlatedRecoverPath = 0
				defer func() { _ = recover() }()
				panic("recovered")
			}
			panic("not recovered")
		}()
		consumeInt(correlatedRecoverPath)

		goexitWithDeferPath := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = goexitWithDeferPath
		func() {
			defer func() { _ = recover() }()
			if b.N < 0 {
				runtime.Goexit()
			}
			goexitWithDeferPath = 0
		}()
		consumeInt(goexitWithDeferPath)

		shortCircuitAndPath := ComputeOnly()
		_ = shortCircuitAndPath
		_ = false && func() bool {
			shortCircuitAndPath = 0
			return true
		}()
		consumeInt(shortCircuitAndPath)

		shortCircuitOrPath := ComputeOnly()
		_ = shortCircuitOrPath
		_ = true || func() bool {
			shortCircuitOrPath = 0
			return false
		}()
		consumeInt(shortCircuitOrPath)

		dynamicShortCircuitAndPath := ComputeOnly()
		_ = dynamicShortCircuitAndPath
		_ = b.N < 0 && func() bool {
			dynamicShortCircuitAndPath = 0
			return true
		}()
		consumeInt(dynamicShortCircuitAndPath)

		dynamicShortCircuitOrPath := ComputeOnly()
		_ = dynamicShortCircuitOrPath
		_ = b.N < 0 || func() bool {
			dynamicShortCircuitOrPath = 0
			return false
		}()
		consumeInt(dynamicShortCircuitOrPath)

		staticallyDeadWriterPath := ComputeOnly()
		_ = staticallyDeadWriterPath
		if false {
			func() { staticallyDeadWriterPath = 0 }()
		}
		consumeInt(staticallyDeadWriterPath)

		lhsReadBeforeRHSWriter := ComputeOnly()
		_ = lhsReadBeforeRHSWriter
		rangeIndexValues[lhsReadBeforeRHSWriter] = func() int {
			lhsReadBeforeRHSWriter = 0
			return 1
		}()
		consumeInt(lhsReadBeforeRHSWriter)

		lhsWriterBeforeRHSRead := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = lhsWriterBeforeRHSRead
		rangeIndexValues[func() int {
			lhsWriterBeforeRHSRead = 0
			return 0
		}()] = lhsWriterBeforeRHSRead
		consumeInt(lhsWriterBeforeRHSRead)

		argumentWriter := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = argumentWriter
		readAfterArgument := func(int) {
			consumeInt(argumentWriter)
		}
		readAfterArgument(func() int {
			argumentWriter = 0
			return 0
		}())

		deferredCapture := ComputeOnly()
		_ = deferredCapture
		deferredRead := func() {
			consumeInt(deferredCapture)
		}
		defer deferredRead()

		deferredImmediateCapture := ComputeOnly()
		_ = deferredImmediateCapture
		defer func() {
			consumeInt(deferredImmediateCapture)
		}()

		nestedInvokedCapture := ComputeOnly()
		_ = nestedInvokedCapture
		nestedRead := func() {
			consumeInt(nestedInvokedCapture)
		}
		invokeNestedRead := func() {
			nestedRead()
		}
		invokeNestedRead()

		iifeBlankDiscard := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		func() {
			_ = iifeBlankDiscard
		}()

		writeOnlyCapture := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = writeOnlyCapture
		writeOnlyClosure := func() {
			writeOnlyCapture = 0
		}
		writeOnlyClosure()

		readBeforeWrite := ComputeOnly()
		func() {
			consumeInt(readBeforeWrite)
			readBeforeWrite = 0
		}()

		readAfterWrite := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = readAfterWrite
		func() {
			readAfterWrite = 0
			consumeInt(readAfterWrite)
		}()

		deadCapture := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = deadCapture
		deadClosure := func() {
			return
			consumeInt(deadCapture)
		}
		_ = deadClosure

		deadNestedCapture := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = deadNestedCapture
		deadOuterClosure := func() {
			deadInnerClosure := func() {
				panic("stop")
				consumeInt(deadNestedCapture)
			}
			_ = deadInnerClosure
		}
		_ = deadOuterClosure

		uncreatedNestedCapture := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = uncreatedNestedCapture
		uncreatedOuterClosure := func() {
			return
			uncreatedInnerClosure := func() {
				consumeInt(uncreatedNestedCapture)
			}
			_ = uncreatedInnerClosure
		}
		_ = uncreatedOuterClosure

		deadTestingCapture := ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
		_ = deadTestingCapture
		deadTestingClosure := func(tb testing.TB) {
			tb.Fatal("stop")
			consumeInt(deadTestingCapture)
		}
		_ = deadTestingClosure

		liveLabeledCapture := ComputeOnly()
		_ = liveLabeledCapture
		liveLabeledClosure := func() {
			goto liveCapture
			return
		liveCapture:
			consumeInt(liveLabeledCapture)
		}
		liveLabeledClosure()
	}
}

func TestNotABenchmark(t *testing.T) {
	for range 10 {
		_, _ = QMatMul()
	}
}

func BenchmarkUnrelatedCalls(b *testing.B) {
	for b.Loop() {
		_, _ = Unconfigured()
		_, _ = ps6090dep.QMatMul()
		_ = OnlyError()
	}
}

func BenchmarkDynamicCallableIsOpaque(b *testing.B) {
	compute := QMatMul
	for b.Loop() {
		_, _ = compute()
	}
}

func BenchmarkNonInvokedClosure(b *testing.B) {
	for b.Loop() {
		closure := func() {
			_, _ = QMatMul()
		}
		_ = closure
	}
}

func deadHelper() bool {
	ComputeOnly()
	return true
}

func dynamicSwitchCase() int { return 2 }

func BenchmarkStaticallyDeadPaths(b *testing.B) {
	for b.Loop() {
		if false {
			_, _ = QMatMul()
		}
		if true {
			intSink = ComputeOnly()
		} else {
			discardInHelper()
		}
		if false && deadHelper() {
			b.Fatal("unreachable")
		}
		switch 1 {
		case 0:
			discardInHelper()
		case 1:
			intSink = ComputeOnly()
		case 2:
			_, _ = QMatMul()
		case dynamicSwitchCase():
			ComputeOnly()
		}
	}
}

func switchFallthroughHelper() {
	ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
}

func BenchmarkConstantSwitchFallthrough(b *testing.B) {
	for b.Loop() {
		switch 1 {
		case 1:
			fallthrough
		case 2:
			switchFallthroughHelper()
		}
	}
}

func BenchmarkIncidentalBNUseIsNotALoopProof(b *testing.B) {
	for index := 0; index < 1 && b.N >= 0; index++ {
		_, _ = QMatMul()
	}
}

func lowercaseBenchmarkHelper() {
	ComputeOnly()
}

func BenchmarklowercaseIsNotRegistered(b *testing.B) {
	for b.Loop() {
		ComputeOnly()
		lowercaseBenchmarkHelper()
	}
}

func BenchmarkéUnicodeLowercaseIsNotRegistered(b *testing.B) {
	for b.Loop() {
		ComputeOnly()
	}
}

func Benchmark(b *testing.B) {
	for b.Loop() {
		ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
	}
}

func unreachableAfterReturn() {
	return
	ComputeOnly()
}

func unreachableHelperAfterReturn() {
	return
	discardInHelper()
}

func unreachableAfterPanic() {
	panic("stop")
	ComputeOnly()
}

func unreachableAfterTestingTerminators(b *testing.B, which int) {
	switch which {
	case 0:
		b.Fatal("stop")
		ComputeOnly()
	case 1:
		b.Fatalf("%s", "stop")
		ComputeOnly()
	case 2:
		b.FailNow()
		ComputeOnly()
	case 3:
		b.Skip("stop")
		ComputeOnly()
	case 4:
		b.Skipf("%s", "stop")
		ComputeOnly()
	case 5:
		b.SkipNow()
		ComputeOnly()
	}
}

func unreachableAfterTestingTB(tb testing.TB) {
	tb.Fatal("stop")
	ComputeOnly()
}

func unreachableAfterGoto() {
	goto done
	ComputeOnly()
done:
	intSink = 0
}

func unreachableGotoOutOfNestedBlock() {
	{
		goto done
		ComputeOnly()
	}
done:
	intSink = 0
}

func reachableGotoWithinNestedBlock() {
	{
		goto live
		return
	live:
		ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
	}
}

func unreachableNestedReturn() {
	{
		return
		ComputeOnly()
	}
}

func unreachableSwitchBreak(value int) {
	switch value {
	case 0:
		break
		ComputeOnly()
	}
}

func unreachableSelectContinue() {
	for range 1 {
		select {
		default:
			continue
			ComputeOnly()
		}
	}
}

func reachableGotoLabel(takeLabel bool) {
	if takeLabel {
		goto live
	}
	return
live:
	ComputeOnly() // want `benchmark repetition discards the primary result of configured pure compute call ps6090.ComputeOnly`
}

func BenchmarkCFGReachability(b *testing.B) {
	for b.Loop() {
		unreachableAfterReturn()
		unreachableHelperAfterReturn()
		unreachableAfterPanic()
		unreachableAfterTestingTerminators(b, b.N)
		unreachableAfterTestingTB(b)
		unreachableAfterGoto()
		unreachableGotoOutOfNestedBlock()
		unreachableNestedReturn()
		unreachableSwitchBreak(0)
		unreachableSelectContinue()
		reachableGotoLabel(b.N >= 0)
		reachableGotoWithinNestedBlock()
	}
	for b.Loop() {
		break
		ComputeOnly()
	}
	for b.Loop() {
		continue
		ComputeOnly()
	}
}
