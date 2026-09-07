package ps6101round22

import (
	"math/rand"
	"os"
	"testing"
)

var sink float64
var choose bool

func returnedNested() map[any]map[string]*float64 {
	p := new(float64)
	*p = rand.NormFloat64()
	return map[any]map[string]*float64{any("outer"): {"weight": p}}
}

func returnedNestedStringKey() map[string]map[string]*float64 {
	p := new(float64)
	*p = rand.NormFloat64()
	return map[string]map[string]*float64{"outer": {"weight": p}}
}

func returnedNestedValues() map[any]map[string]float64 {
	return map[any]map[string]float64{any("outer"): {"weight": rand.NormFloat64()}}
}

func BenchmarkNestedMapValueAndAliases(b *testing.B) {
	m := returnedNested()
	key := any("outer")
	aliasKey := key
	alias := m
	for rangedKey, inner := range alias {
		if rangedKey == aliasKey {
			weight := *inner["weight"]
			if weight > 0 { // want `benchmark feeds symmetric signed random inputs`
				sink = weight
			}
		}
	}
}

func BenchmarkNestedMapDirectLookup(b *testing.B) {
	m := returnedNested()
	weight := *m[any("outer")]["weight"]
	if weight > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = weight
	}
}

func BenchmarkNestedStringMapDirectLookup(b *testing.B) {
	m := returnedNestedStringKey()
	weight := *m["outer"]["weight"]
	if weight > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = weight
	}
}

func BenchmarkNestedMapScalarValueRange(b *testing.B) {
	m := returnedNestedValues()
	for _, inner := range m {
		weight := inner["weight"]
		if weight > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = weight
		}
	}
}

func BenchmarkNestedMapScalarValueLookup(b *testing.B) {
	weight := returnedNestedValues()[any("outer")]["weight"]
	if weight > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = weight
	}
}

func returnedScalar() map[string]float64 {
	return map[string]float64{"weight": rand.NormFloat64()}
}

func BenchmarkScalarDeleteThenReinsert(b *testing.B) {
	m := returnedScalar()
	alias := m
	delete(alias, "weight")
	alias["weight"] = rand.NormFloat64()
	weight := m["weight"]
	if weight > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = weight
	}
}

func BenchmarkScalarClearThenAssign(b *testing.B) {
	m := returnedScalar()
	alias := m
	clear(alias)
	m["weight"] = rand.NormFloat64()
	weight := alias["weight"]
	if weight > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = weight
	}
}

func BenchmarkDeleteThenReinsert(b *testing.B) {
	m := returnedNested()
	alias := m
	delete(alias, any("outer"))
	p := new(float64)
	*p = rand.NormFloat64()
	alias[any("outer")] = map[string]*float64{"weight": p}
	weight := *m[any("outer")]["weight"]
	if weight > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = weight
	}
}

func BenchmarkClearThenAssign(b *testing.B) {
	m := returnedNested()
	alias := m
	clear(alias)
	p := new(float64)
	*p = rand.NormFloat64()
	m[any("outer")] = map[string]*float64{"weight": p}
	weight := *alias[any("outer")]["weight"]
	if weight > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = weight
	}
}

func BenchmarkMultiEntryRangeConservative(b *testing.B) {
	m := map[string]float64{"random": rand.NormFloat64(), "safe": 1}
	for _, weight := range m {
		if weight > 0 {
			sink = weight
		}
	}
}

func BenchmarkStoredOSExit(b *testing.B) {
	weight := rand.NormFloat64()
	exit := os.Exit
	exit(1)
	if weight > 0 {
		sink = weight
	}
}

func invokeExit(exit func(int)) { exit(1) }

func BenchmarkArgumentOSExit(b *testing.B) {
	weight := rand.NormFloat64()
	invokeExit(os.Exit)
	if weight > 0 {
		sink = weight
	}
}

func BenchmarkInterfaceStoredOSExit(b *testing.B) {
	weight := rand.NormFloat64()
	exit := any(os.Exit).(func(int))
	exit(1)
	if weight > 0 {
		sink = weight
	}
}

type fataler interface{ Fatal(...any) }
type fatalfer interface{ Fatalf(string, ...any) }
type failNower interface{ FailNow() }
type errorer interface{ Error(...any) }
type errorfer interface{ Errorf(string, ...any) }
type failer interface{ Fail() }

func BenchmarkTestingInterfaceMethodValue(b *testing.B) {
	weight := rand.NormFloat64()
	var terminal fataler = b
	fatal := terminal.Fatal
	fatal("stop")
	if weight > 0 {
		sink = weight
	}
}

func BenchmarkTestingInterfaceClosure(b *testing.B) {
	weight := rand.NormFloat64()
	var terminal fataler = b
	fatal := terminal.Fatal
	call := func() { fatal("stop") }
	call()
	if weight > 0 {
		sink = weight
	}
}

func BenchmarkTestingInterfaceFatalfMethodValue(b *testing.B) {
	weight := rand.NormFloat64()
	var terminal fatalfer = b
	call := terminal.Fatalf
	call("%s", "stop")
	if weight > 0 {
		sink = weight
	}
}

func BenchmarkTestingInterfaceFailNowClosure(b *testing.B) {
	weight := rand.NormFloat64()
	var terminal failNower = b
	call := func() { terminal.FailNow() }
	call()
	if weight > 0 {
		sink = weight
	}
}

func BenchmarkTestingInterfaceErrorClosure(b *testing.B) {
	weight := rand.NormFloat64()
	var nonterminal errorer = b
	call := func() { nonterminal.Error("continue") }
	call()
	if weight > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = weight
	}
}

func BenchmarkTestingInterfaceErrorMethodValue(b *testing.B) {
	weight := rand.NormFloat64()
	var nonterminal errorer = b
	call := nonterminal.Error
	call("continue")
	if weight > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = weight
	}
}

func BenchmarkTestingInterfaceErrorfMethodValue(b *testing.B) {
	weight := rand.NormFloat64()
	var nonterminal errorfer = b
	call := nonterminal.Errorf
	call("%s", "continue")
	if weight > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = weight
	}
}

func BenchmarkTestingInterfaceFailClosure(b *testing.B) {
	weight := rand.NormFloat64()
	var nonterminal failer = b
	call := func() { nonterminal.Fail() }
	call()
	if weight > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = weight
	}
}

func BenchmarkDirectTestingErrorClosure(b *testing.B) {
	weight := rand.NormFloat64()
	call := func() { b.Error("continue") }
	call()
	if weight > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = weight
	}
}

func BenchmarkDirectTestingErrorfMethodValue(b *testing.B) {
	weight := rand.NormFloat64()
	call := b.Errorf
	call("%s", "continue")
	if weight > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = weight
	}
}

func BenchmarkDirectTestingFailMethodValue(b *testing.B) {
	weight := rand.NormFloat64()
	call := b.Fail
	call()
	if weight > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = weight
	}
}

func recoverRandom(weights []float64) {
	if recover() != nil {
		weights[0] = rand.NormFloat64()
	}
}

func noRecover([]float64) {}

func deferredSnapshot(weights []float64) {
	call := recoverRandom
	defer call(weights)
	call = noRecover
	other := []float64{1}
	weights = other
	panic("stop")
}

func deferredCallableReassignment(weights []float64) {
	call := recoverRandom
	defer call(weights)
	call = noRecover
	panic("stop")
}

func deferredArgumentReassignment(weights []float64) {
	defer recoverRandom(weights)
	weights = []float64{1}
	panic("stop")
}

func BenchmarkDeferredCallableAndArgumentSnapshot(b *testing.B) {
	weights := []float64{1}
	deferredSnapshot(weights)
	weight := weights[0]
	if weight > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = weight
	}
}

func BenchmarkDeferredCallableReassignment(b *testing.B) {
	weights := []float64{1}
	deferredCallableReassignment(weights)
	weight := weights[0]
	if weight > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = weight
	}
}

func BenchmarkDeferredArgumentReassignment(b *testing.B) {
	weights := []float64{1}
	deferredArgumentReassignment(weights)
	weight := weights[0]
	if weight > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = weight
	}
}

func BenchmarkCallableCycleNoCrash(b *testing.B) {
	weight := rand.NormFloat64()
	var first, second func()
	first = func() { second() }
	second = func() { first() }
	if choose {
		first()
	}
	if weight > 0 {
		sink = weight
	}
}
