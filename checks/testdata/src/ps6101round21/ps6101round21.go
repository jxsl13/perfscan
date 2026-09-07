package ps6101round21

import (
	"math/rand"
	"os"
	"runtime"
	"testing"
)

var sink float64
var choose bool
var opaqueFatal fataler

func deferredGoexit()              { defer runtime.Goexit() }
func deferredFailNow(b *testing.B) { defer b.FailNow() }
func deferredError(b *testing.B)   { defer b.Error("continue") }
func deferredFail(b *testing.B)    { defer b.Fail() }
func deferredExit()                { defer os.Exit(1) }
func directExit()                  { os.Exit(1) }

func BenchmarkDeferredGoexit(b *testing.B) {
	weight := rand.NormFloat64()
	deferredGoexit()
	if weight > 0 {
		sink = weight
	}
}

func BenchmarkDeferredFailNow(b *testing.B) {
	weight := rand.NormFloat64()
	deferredFailNow(b)
	if weight > 0 {
		sink = weight
	}
}

func BenchmarkDeferredError(b *testing.B) {
	weight := rand.NormFloat64()
	deferredError(b)
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkDeferredFail(b *testing.B) {
	weight := rand.NormFloat64()
	deferredFail(b)
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkDeferredExit(b *testing.B) {
	weight := rand.NormFloat64()
	deferredExit()
	if weight > 0 {
		sink = weight
	}
}

func BenchmarkDirectExit(b *testing.B) {
	weight := rand.NormFloat64()
	directExit()
	if weight > 0 {
		sink = weight
	}
}

func fatal3(b *testing.B) { b.Fatalf("%s", "stop") }
func fatal2(b *testing.B) { fatal3(b) }
func fatal1(b *testing.B) { fatal2(b) }
func error3(b *testing.B) { b.Errorf("%s", "continue") }
func error2(b *testing.B) { error3(b) }
func error1(b *testing.B) { error2(b) }

func BenchmarkNestedFatal(b *testing.B) {
	weight := rand.NormFloat64()
	fatal1(b)
	if weight > 0 {
		sink = weight
	}
}

func BenchmarkNestedError(b *testing.B) {
	weight := rand.NormFloat64()
	error1(b)
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

type fataler interface{ Fatal(...any) }
type failNower interface{ FailNow() }
type errorer interface{ Error(...any) }

func interfaceFatal(value fataler)           { value.Fatal("stop") }
func interfaceFailNow(value failNower)       { value.FailNow() }
func interfaceError(value errorer)           { value.Error("continue") }
func interfaceFatalExpression(value fataler) { fataler.Fatal(value, "stop") }
func interfaceErrorExpression(value errorer) { errorer.Error(value, "continue") }
func invokeTestingMethod(call func(...any))  { call("stop") }

func BenchmarkInterfaceFatal(b *testing.B) {
	weight := rand.NormFloat64()
	interfaceFatal(b)
	if weight > 0 {
		sink = weight
	}
}

func BenchmarkInterfaceFailNow(b *testing.B) {
	weight := rand.NormFloat64()
	interfaceFailNow(b)
	if weight > 0 {
		sink = weight
	}
}

func BenchmarkInterfaceError(b *testing.B) {
	weight := rand.NormFloat64()
	interfaceError(b)
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkInterfaceFatalExpression(b *testing.B) {
	weight := rand.NormFloat64()
	interfaceFatalExpression(b)
	total := weight
	if total > 0 {
		sink = total
	}
}

func BenchmarkInterfaceErrorExpression(b *testing.B) {
	weight := rand.NormFloat64()
	interfaceErrorExpression(b)
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkOpaqueFatalInterface(b *testing.B) {
	weight := rand.NormFloat64()
	interfaceFatal(opaqueFatal)
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkStoredFatal(b *testing.B) {
	weight := rand.NormFloat64()
	invokeTestingMethod(b.Fatal)
	if weight > 0 {
		sink = weight
	}
}

func BenchmarkStoredError(b *testing.B) {
	weight := rand.NormFloat64()
	invokeTestingMethod(b.Error)
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func branchSelectedRecover(weights []float64) {
	call := func() {}
	if choose {
		call = func() {
			if recover() != nil {
				weights[0] = rand.NormFloat64()
			}
		}
	}
	defer call()
	panic("stop")
}

func branchRecoverVariants(weights []float64) {
	call := func() { _ = recover() }
	if choose {
		call = func() {
			_ = recover()
			weights[0] = rand.NormFloat64()
		}
	}
	defer call()
	panic("stop")
}

func branchDeferredRecover(weights []float64) {
	if choose {
		defer func() {
			_ = recover()
			weights[0] = rand.NormFloat64()
		}()
	} else {
		defer func() { _ = recover() }()
	}
	panic("stop")
}

func branchRecoverOnlyRandom(weights []float64) {
	if choose {
		defer func() {
			_ = recover()
			weights[0] = rand.NormFloat64()
		}()
	}
	panic("stop")
}

func branchRecoverOnlySafe(weights []float64) {
	if choose {
		defer func() { _ = recover() }()
	}
	panic("stop")
}

func namedRecover()   { _ = recover() }
func namedNoRecover() {}

func branchNamedRecoverCorrelation(weights []float64) {
	call := namedNoRecover
	weights[0] = 1
	if choose {
		weights[0] = rand.NormFloat64()
		call = namedRecover
	}
	defer call()
	panic("stop")
}

func namedRecoverArgument([]float64)   { _ = recover() }
func namedNoRecoverArgument([]float64) {}

func branchNamedArgumentCorrelation(weights []float64) {
	call := namedNoRecoverArgument
	weights[0] = 1
	if choose {
		weights[0] = rand.NormFloat64()
		call = namedRecoverArgument
	}
	defer call(weights)
	panic("stop")
}

func returnedBranchRecover(weights []float64) func() {
	call := func() {}
	if choose {
		call = func() {
			_ = recover()
			weights[0] = rand.NormFloat64()
		}
	}
	return call
}

func useReturnedBranchRecover(weights []float64) {
	call := returnedBranchRecover(weights)
	defer call()
	panic("stop")
}

func BenchmarkBranchSelectedRecover(b *testing.B) {
	weights := []float64{1}
	branchSelectedRecover(weights)
	weight := weights[0]
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkBranchRecoverVariants(b *testing.B) {
	weights := []float64{1}
	branchRecoverVariants(weights)
	weight := weights[0]
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkBranchDeferredRecover(b *testing.B) {
	weights := []float64{1}
	branchDeferredRecover(weights)
	weight := weights[0]
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkBranchRecoverOnlyRandom(b *testing.B) {
	weights := []float64{1}
	branchRecoverOnlyRandom(weights)
	weight := weights[0]
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkBranchRecoverOnlySafe(b *testing.B) {
	weights := []float64{1}
	branchRecoverOnlySafe(weights)
	if weights[0] > 0 {
		sink = weights[0]
	}
}

func BenchmarkBranchNamedRecoverCorrelation(b *testing.B) {
	weights := []float64{1}
	branchNamedRecoverCorrelation(weights)
	weight := weights[0]
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkBranchNamedArgumentCorrelation(b *testing.B) {
	weights := []float64{1}
	branchNamedArgumentCorrelation(weights)
	weight := weights[0]
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkReturnedBranchRecover(b *testing.B) {
	weights := []float64{1}
	useReturnedBranchRecover(weights)
	weight := weights[0]
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

type namedKey string
type structKey struct{ tag string }

func returnedNamedMap() map[namedKey]*float64 {
	weights := map[namedKey]*float64{"weight": new(float64)}
	*weights["weight"] = rand.NormFloat64()
	return weights
}

func returnedScalarMap() map[namedKey]float64 {
	return map[namedKey]float64{"weight": rand.NormFloat64()}
}

func returnedInterfaceMap() map[any]*float64 {
	weights := map[any]*float64{"weight": new(float64)}
	*weights["weight"] = rand.NormFloat64()
	return weights
}

func returnedWrappedMap() any {
	weights := map[string]*float64{"weight": new(float64)}
	*weights["weight"] = rand.NormFloat64()
	return weights
}

func returnedStructMap() map[structKey]*float64 {
	key := structKey{tag: "weight"}
	weights := map[structKey]*float64{key: new(float64)}
	*weights[key] = rand.NormFloat64()
	return weights
}

func BenchmarkInterfaceMapKey(b *testing.B) {
	weights := returnedInterfaceMap()
	weight := *weights[any("weight")]
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkWrappedMap(b *testing.B) {
	weights := returnedWrappedMap().(map[string]*float64)
	weight := *weights["weight"]
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkComparableStructMapNoCrash(b *testing.B) {
	weights := returnedStructMap()
	weight := *weights[structKey{tag: "weight"}]
	if weight > 0 {
		sink = weight
	}
}

func BenchmarkMapAliasRead(b *testing.B) {
	weights := returnedNamedMap()
	alias := weights
	weight := *alias["weight"]
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkMapAliasDelete(b *testing.B) {
	weights := returnedScalarMap()
	alias := weights
	delete(alias, "weight")
	if weights["weight"] > 0 {
		sink = weights["weight"]
	}
}

func BenchmarkMapAliasClear(b *testing.B) {
	weights := returnedScalarMap()
	alias := weights
	clear(alias)
	if weights["weight"] > 0 {
		sink = weights["weight"]
	}
}

func BenchmarkMapDeleteOtherKey(b *testing.B) {
	weights := returnedScalarMap()
	alias := weights
	delete(alias, "other")
	weight := weights["weight"]
	total := weight
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkMapAliasRangeKey(b *testing.B) {
	weights := returnedNamedMap()
	alias := weights
	for key := range alias {
		weight := *weights[key]
		total := weight
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func BenchmarkMapRangeValue(b *testing.B) {
	weights := returnedNamedMap()
	for _, pointer := range weights {
		weight := *pointer
		total := weight
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func BenchmarkWrappedMapRange(b *testing.B) {
	weights := returnedWrappedMap().(map[string]*float64)
	for _, pointer := range weights {
		weight := *pointer
		total := weight
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func BenchmarkInterfaceMapRangeKey(b *testing.B) {
	weights := returnedInterfaceMap()
	for key := range weights {
		weight := *weights[key]
		total := weight
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func BenchmarkInterfaceMapRangeValue(b *testing.B) {
	weights := returnedInterfaceMap()
	for _, pointer := range weights {
		weight := *pointer
		total := weight
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func BenchmarkUnknownMapRangeOrder(b *testing.B) {
	weights := map[string]float64{"weight": rand.NormFloat64(), "safe": 1}
	for _, weight := range weights {
		if weight > 0 {
			sink = weight
		}
	}
}
