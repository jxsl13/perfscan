package ps6101round20

import (
	"math/rand"
	"runtime"
	"testing"
)

var sink float64
var choose bool

func setRandom(weights []float64) { weights[0] = rand.NormFloat64() }

func storedRecover(weights []float64) {
	recoverer := func() {
		if recover() != nil {
			setRandom(weights)
		}
	}
	defer recoverer()
	panic("stop")
}

func namedRecoverRandom(weights []float64) {
	if recover() != nil {
		setRandom(weights)
	}
}

func storedNamedRecover(weights []float64) {
	recoverer := namedRecoverRandom
	defer recoverer(weights)
	panic("stop")
}

func returnedRecoverer(weights []float64) func() {
	return func() {
		if recover() != nil {
			setRandom(weights)
		}
	}
}

func returnedClosureRecover(weights []float64) {
	defer returnedRecoverer(weights)()
	panic("stop")
}

func conditionalRecoverWrites(weights []float64) {
	defer func() {
		if choose {
			_ = recover()
			setRandom(weights)
		}
	}()
	panic("stop")
}

func conditionalRecoverSafeNormal(weights []float64) {
	defer func() {
		if choose {
			_ = recover()
		} else {
			setRandom(weights)
		}
	}()
	panic("stop")
}

func sequentialConditionalRecover(weights []float64) {
	defer func() {
		if choose {
			_ = recover()
			return
		}
		if !choose {
			_ = recover()
			setRandom(weights)
		}
	}()
	panic("stop")
}

func nestedIneffectiveRecover(weights []float64) {
	defer func() {
		func() {
			_ = recover()
			setRandom(weights)
		}()
	}()
	panic("stop")
}

func storedRecoverAfterDeferredPanic(weights []float64) {
	recoverer := func() {
		_ = recover()
		setRandom(weights)
	}
	defer recoverer()
	defer func() { panic("deferred") }()
}

func repanic(weights []float64) {
	defer func() {
		_ = recover()
		setRandom(weights)
		panic("again")
	}()
	panic("first")
}

func fatalHelper(b *testing.B)   { b.Fatal("stop") }
func fatalfHelper(b *testing.B)  { b.Fatalf("%s", "stop") }
func failNowHelper(b *testing.B) { b.FailNow() }
func errorHelper(b *testing.B)   { b.Error("continue") }
func failHelper(b *testing.B)    { b.Fail() }
func goexitHelper()              { runtime.Goexit() }

func goexitWithDeferredWrite(weights []float64) {
	defer setRandom(weights)
	runtime.Goexit()
}

func returnedStringMapDeferred() map[string]*float64 {
	weight := new(float64)
	weights := map[string]*float64{"weight": weight}
	defer func() { *weights["weight"] = rand.NormFloat64() }()
	return weights
}

func returnedBoolMapDeferred() map[bool]*float64 {
	weight := new(float64)
	weights := map[bool]*float64{true: weight}
	defer func() { *weights[true] = rand.NormFloat64() }()
	return weights
}

func returnedIntMap() map[int]*float64 {
	weights := map[int]*float64{0: new(float64)}
	*weights[0] = rand.NormFloat64()
	return weights
}

func returnedMapUnrelatedRoot() map[int]*float64 {
	weights := map[int]*float64{0: new(float64), 1: new(float64)}
	*weights[0] = rand.NormFloat64()
	return weights
}

func BenchmarkStoredRecover(b *testing.B) {
	weights := []float64{1}
	storedRecover(weights)
	weight := weights[0]
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkStoredNamedRecover(b *testing.B) {
	weights := []float64{1}
	storedNamedRecover(weights)
	weight := weights[0]
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkReturnedClosureRecover(b *testing.B) {
	weights := []float64{1}
	returnedClosureRecover(weights)
	weight := weights[0]
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkConditionalRecoverWrites(b *testing.B) {
	weights := []float64{1}
	conditionalRecoverWrites(weights)
	weight := weights[0]
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkConditionalRecoverSafeNormal(b *testing.B) {
	weights := []float64{1}
	conditionalRecoverSafeNormal(weights)
	weight := weights[0]
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkSequentialConditionalRecover(b *testing.B) {
	weights := []float64{1}
	sequentialConditionalRecover(weights)
	weight := weights[0]
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkNestedIneffectiveRecover(b *testing.B) {
	weights := []float64{1}
	nestedIneffectiveRecover(weights)
	weight := weights[0]
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkStoredRecoverAfterDeferredPanic(b *testing.B) {
	weights := []float64{1}
	storedRecoverAfterDeferredPanic(weights)
	weight := weights[0]
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkRepanicEscapes(b *testing.B) {
	weights := []float64{1}
	repanic(weights)
	weight := weights[0]
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkDirectGoexit(b *testing.B) {
	weight := rand.NormFloat64()
	runtime.Goexit()
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkHelperGoexit(b *testing.B) {
	weight := rand.NormFloat64()
	goexitHelper()
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkGoexitRunsDefersButDoesNotReturn(b *testing.B) {
	weights := []float64{1}
	goexitWithDeferredWrite(weights)
	weight := weights[0]
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkDirectFatal(b *testing.B) {
	weight := rand.NormFloat64()
	b.Fatal("stop")
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkHelperFatal(b *testing.B) {
	weight := rand.NormFloat64()
	fatalHelper(b)
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkHelperFatalf(b *testing.B) {
	weight := rand.NormFloat64()
	fatalfHelper(b)
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkHelperFailNow(b *testing.B) {
	weight := rand.NormFloat64()
	failNowHelper(b)
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkHelperErrorContinues(b *testing.B) {
	weight := rand.NormFloat64()
	errorHelper(b)
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkHelperFailContinues(b *testing.B) {
	weight := rand.NormFloat64()
	failHelper(b)
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkReturnedStringMapDeferred(b *testing.B) {
	weights := returnedStringMapDeferred()
	weight := *weights["weight"]
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkReturnedBoolMapDeferred(b *testing.B) {
	weights := returnedBoolMapDeferred()
	weight := *weights[true]
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkReturnedIntMap(b *testing.B) {
	weights := returnedIntMap()
	weight := *weights[0]
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkReturnedMapUnrelatedRoot(b *testing.B) {
	weights := returnedMapUnrelatedRoot()
	weight := *weights[1]
	total := weight
	if total > 0 {
		sink = total
	}
}
