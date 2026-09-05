package ps6101round23

import (
	"math/rand"
	"testing"
)

var sink float64

func returnedDeepMap() map[string]map[string]map[string]map[string]*float64 {
	p := new(float64)
	*p = rand.NormFloat64()
	return map[string]map[string]map[string]map[string]*float64{
		"outer]key": {"middle[part]": {"inner].key": {"weight]": p}}},
	}
}

func returnedEscapedScalarMap() map[string]float64 {
	return map[string]float64{"outer]key[part].tail": rand.NormFloat64()}
}

func returnedScalar() map[string]float64 {
	return map[string]float64{"weight": rand.NormFloat64()}
}

func BenchmarkDeepReturnedMapLookup(b *testing.B) {
	m := returnedDeepMap()
	value := *m["outer]key"]["middle[part]"]["inner].key"]["weight]"]
	total := value
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkDeepReturnedMapRange(b *testing.B) {
	m := returnedDeepMap()
	for key, second := range m {
		if key == "outer]key" {
			value := *second["middle[part]"]["inner].key"]["weight]"]
			total := value
			if total > 0 { // want `benchmark feeds symmetric signed random inputs`
				sink = total
			}
		}
	}
}

func BenchmarkDeepReturnedMapRangeWithoutKey(b *testing.B) {
	m := returnedDeepMap()
	for _, second := range m {
		value := *second["middle[part]"]["inner].key"]["weight]"]
		total := value
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func BenchmarkEscapedScalarLookup(b *testing.B) {
	m := returnedEscapedScalarMap()
	value := m["outer]key[part].tail"]
	total := value
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkEscapedScalarRange(b *testing.B) {
	m := returnedEscapedScalarMap()
	for _, value := range m {
		total := value
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func BenchmarkEscapedScalarReinsertRange(b *testing.B) {
	m := returnedEscapedScalarMap()
	alias := m
	delete(alias, "outer]key[part].tail")
	m["outer]key[part].tail"] = rand.NormFloat64()
	for _, value := range alias {
		total := value
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

func BenchmarkFourHopAliasRead(b *testing.B) {
	m := returnedScalar()
	first := m
	second := first
	third := second
	fourth := third
	value := fourth["weight"]
	total := value
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkFourHopDeleteReinsert(b *testing.B) {
	m := returnedScalar()
	first := m
	second := first
	third := second
	fourth := third
	delete(fourth, "weight")
	first["weight"] = rand.NormFloat64()
	value := m["weight"]
	total := value
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkFourHopClearThenAliasAndAssign(b *testing.B) {
	m := returnedScalar()
	first := m
	second := first
	third := second
	fourth := third
	clear(fourth)
	afterClear := second
	afterClear["weight"] = rand.NormFloat64()
	value := m["weight"]
	total := value
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkFourHopReinsertOneEntryRange(b *testing.B) {
	m := returnedScalar()
	first := m
	second := first
	third := second
	fourth := third
	delete(second, "weight")
	fourth["weight"] = rand.NormFloat64()
	for _, value := range first {
		total := value
		if total > 0 { // want `benchmark feeds symmetric signed random inputs`
			sink = total
		}
	}
}

type errorer interface{ Error(...any) }
type errorfer interface{ Errorf(string, ...any) }
type failer interface{ Fail() }
type fataler interface{ Fatal(...any) }
type fatalfer interface{ Fatalf(string, ...any) }
type failNower interface{ FailNow() }

func BenchmarkDirectErrorTotal(b *testing.B) {
	value := rand.NormFloat64()
	b.Error("continue")
	total := value
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkDirectErrorClosureTotal(b *testing.B) {
	value := rand.NormFloat64()
	call := func() { b.Error("continue") }
	call()
	total := value
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkDirectErrorfMethodValueTotal(b *testing.B) {
	value := rand.NormFloat64()
	call := b.Errorf
	call("%s", "continue")
	total := value
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkDirectFailMethodValueTotal(b *testing.B) {
	value := rand.NormFloat64()
	call := b.Fail
	call()
	total := value
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkProvenInterfaceErrorTotal(b *testing.B) {
	value := rand.NormFloat64()
	var nonterminal errorer = b
	nonterminal.Error("continue")
	total := value
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkProvenInterfaceErrorfMethodValueTotal(b *testing.B) {
	value := rand.NormFloat64()
	var nonterminal errorfer = b
	call := nonterminal.Errorf
	call("%s", "continue")
	total := value
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkProvenInterfaceFailClosureTotal(b *testing.B) {
	value := rand.NormFloat64()
	var nonterminal failer = b
	call := func() { nonterminal.Fail() }
	call()
	total := value
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkProvenFatalStops(b *testing.B) {
	value := rand.NormFloat64()
	b.Fatal("stop")
	total := value
	if total > 0 {
		sink = total
	}
}

func BenchmarkProvenFatalfMethodValueStops(b *testing.B) {
	value := rand.NormFloat64()
	var terminal fatalfer = b
	call := terminal.Fatalf
	call("%s", "stop")
	total := value
	if total > 0 {
		sink = total
	}
}

func BenchmarkProvenFailNowClosureStops(b *testing.B) {
	value := rand.NormFloat64()
	var terminal failNower = b
	call := func() { terminal.FailNow() }
	call()
	total := value
	if total > 0 {
		sink = total
	}
}

var opaqueFatal fataler
var opaqueFatalf fatalfer
var opaqueFailNow failNower

func BenchmarkOpaqueFatalContinues(b *testing.B) {
	value := rand.NormFloat64()
	opaqueFatal.Fatal("unknown")
	total := value
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkOpaqueFatalfContinues(b *testing.B) {
	value := rand.NormFloat64()
	opaqueFatalf.Fatalf("%s", "unknown")
	total := value
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkOpaqueFailNowContinues(b *testing.B) {
	value := rand.NormFloat64()
	opaqueFailNow.FailNow()
	total := value
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		sink = total
	}
}

func BenchmarkMultiEntryMapRangeIsConservative(b *testing.B) {
	m := map[string]float64{"random": rand.NormFloat64(), "safe": 1}
	for _, value := range m {
		total := value
		if total > 0 {
			sink = total
		}
	}
}
