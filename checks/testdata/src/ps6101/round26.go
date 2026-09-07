package ps6101

import (
	"math/rand"
	"testing"
)

var round26Sink float64

type round26State struct{ weight float64 }

func BenchmarkRound26RangeIndexStoreSafe(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	for _, weights[0] = range []float64{1} {
	}
	if weights[0] > 0 {
		round26Sink = weights[0]
	}
}

func BenchmarkRound26RangePointerStoreSafe(b *testing.B) {
	weight := rand.NormFloat64()
	p := &weight
	for _, *p = range []float64{1} {
	}
	if weight > 0 {
		round26Sink = weight
	}
}

func BenchmarkRound26RangeFieldStoreSafe(b *testing.B) {
	state := round26State{weight: rand.NormFloat64()}
	for _, state.weight = range []float64{1} {
	}
	if state.weight > 0 {
		round26Sink = state.weight
	}
}

func BenchmarkRound26RangeIndexStoreRandom(b *testing.B) {
	weights := []float64{1}
	for _, weights[0] = range []float64{rand.NormFloat64()} {
	}
	if weights[0] > 0 { // want `benchmark feeds symmetric signed random inputs`
		round26Sink = weights[0]
	}
}

func BenchmarkRound26RangePointerStoreRandom(b *testing.B) {
	weight := 1.0
	p := &weight
	for _, *p = range []float64{rand.NormFloat64()} {
	}
	if weight > 0 { // want `benchmark feeds symmetric signed random inputs`
		round26Sink = weight
	}
}

func BenchmarkRound26RangeFieldStoreRandom(b *testing.B) {
	state := round26State{weight: 1}
	for _, state.weight = range []float64{rand.NormFloat64()} {
	}
	if state.weight > 0 { // want `benchmark feeds symmetric signed random inputs`
		round26Sink = state.weight
	}
}

func BenchmarkRound26RangeMapTargetsSafe(b *testing.B) {
	weights := []float64{rand.NormFloat64()}
	other := []float64{1}
	for weights[0], other[0] = range map[float64]float64{1: 1} {
	}
	if weights[0] > 0 {
		round26Sink = weights[0]
	}
}

func round26First(a, b float64) float64 { return a }
func round26Add(a, b float64) float64   { return a + b }

func BenchmarkRound26NestedSameHelperSafe(b *testing.B) {
	weight := rand.NormFloat64()
	total := round26First(1, round26First(weight, 0))
	if total > 0 {
		round26Sink = total
	}
}

func BenchmarkRound26NestedSameHelperRandom(b *testing.B) {
	weight := rand.NormFloat64()
	total := round26Add(weight, round26Add(1, 2))
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		round26Sink = total
	}
}

func BenchmarkRound26NestedSameHelperRandomReverse(b *testing.B) {
	weight := rand.NormFloat64()
	total := round26First(weight, round26First(1, 2))
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		round26Sink = total
	}
}

type round26MixedBox struct{ weight, noise float64 }

func BenchmarkRound26UnrelatedSiblingNoise(b *testing.B) {
	box := round26MixedBox{weight: rand.NormFloat64(), noise: rand.NormFloat64()}
	if box.noise > 0 {
		round26Sink = box.noise
	}
}

func round26ReturnedMixedBox() round26MixedBox {
	return round26MixedBox{weight: rand.NormFloat64(), noise: rand.NormFloat64()}
}

func BenchmarkRound26UnrelatedReturnedSiblingNoise(b *testing.B) {
	if noise := round26ReturnedMixedBox().noise; noise > 0 {
		round26Sink = noise
	}
}

func BenchmarkRound26UnrelatedWrittenSiblingNoise(b *testing.B) {
	box := round26MixedBox{}
	box.weight = rand.NormFloat64()
	box.noise = rand.NormFloat64()
	if box.noise > 0 {
		round26Sink = box.noise
	}
}

func round26InterfaceSlice() any { return []float64{rand.NormFloat64()} }
func round26InterfaceMap() any   { return map[string]float64{"x": rand.NormFloat64()} }

type round26SliceBox struct{ data []float64 }
type round26MapBox struct{ data map[string]float64 }

func round26StructSlice() round26SliceBox {
	return round26SliceBox{data: []float64{rand.NormFloat64()}}
}

func round26StructMap() round26MapBox {
	return round26MapBox{data: map[string]float64{"x": rand.NormFloat64()}}
}

func round26GenericSlice[T ~[]float64]() T { return T{rand.NormFloat64()} }
func round26GenericMap[T ~map[string]float64]() T {
	return T{"x": rand.NormFloat64()}
}

func BenchmarkRound26InterfaceSlice(b *testing.B) {
	total := round26InterfaceSlice().([]float64)[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		round26Sink = total
	}
}

func BenchmarkRound26InterfaceMap(b *testing.B) {
	total := round26InterfaceMap().(map[string]float64)["x"]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		round26Sink = total
	}
}

func BenchmarkRound26StructSlice(b *testing.B) {
	total := round26StructSlice().data[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		round26Sink = total
	}
}

func BenchmarkRound26StructMap(b *testing.B) {
	total := round26StructMap().data["x"]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		round26Sink = total
	}
}

func BenchmarkRound26GenericSlice(b *testing.B) {
	total := round26GenericSlice[[]float64]()[0]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		round26Sink = total
	}
}

func BenchmarkRound26GenericMap(b *testing.B) {
	total := round26GenericMap[map[string]float64]()["x"]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		round26Sink = total
	}
}

func round26NestedPointerMapInterface() any {
	value := new(float64)
	*value = rand.NormFloat64()
	return map[string]*float64{"x": value}
}

func BenchmarkRound26NestedPointerMapInterface(b *testing.B) {
	total := *round26NestedPointerMapInterface().(map[string]*float64)["x"]
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		round26Sink = total
	}
}

func round26InterfaceNoise() any { return rand.NormFloat64() }

func BenchmarkRound26InterfaceNoiseSafe(b *testing.B) {
	total := round26InterfaceNoise().(float64)
	if total > 0 {
		round26Sink = total
	}
}

func round26WeightPointer() *float64 {
	weight := rand.NormFloat64()
	return &weight
}

func round26NoisePointer() *float64 {
	noise := rand.NormFloat64()
	return &noise
}

func BenchmarkRound26WeightPointer(b *testing.B) {
	total := *round26WeightPointer()
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		round26Sink = total
	}
}

func BenchmarkRound26NoisePointerSafe(b *testing.B) {
	total := *round26NoisePointer()
	if total > 0 {
		round26Sink = total
	}
}

type round26EscapedBox struct{ weight, noise float64 }

func round26WeightFieldPointer() *float64 {
	box := round26EscapedBox{weight: rand.NormFloat64()}
	return &box.weight
}

func round26NoiseFieldPointer() *float64 {
	box := round26EscapedBox{noise: rand.NormFloat64()}
	return &box.noise
}

func BenchmarkRound26WeightFieldPointer(b *testing.B) {
	total := *round26WeightFieldPointer()
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		round26Sink = total
	}
}

func BenchmarkRound26NoiseFieldPointerSafe(b *testing.B) {
	total := *round26NoiseFieldPointer()
	if total > 0 {
		round26Sink = total
	}
}

func round26NamedWeightResult() (weight float64) { return rand.NormFloat64() }
func round26NamedWeightResultBare() (weight float64) {
	weight = rand.NormFloat64()
	return
}

func BenchmarkRound26NamedWeightResult(b *testing.B) {
	total := round26NamedWeightResult()
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		round26Sink = total
	}
}

func BenchmarkRound26NamedWeightResultBare(b *testing.B) {
	total := round26NamedWeightResultBare()
	if total > 0 { // want `benchmark feeds symmetric signed random inputs`
		round26Sink = total
	}
}

type round26PositiveMath struct{}

func (round26PositiveMath) Abs(float64) float64 { return 1 }

type round26RandomMath struct{}

func (round26RandomMath) Abs(float64) float64 { return rand.NormFloat64() }

type round26NegativeMath struct{}

func (round26NegativeMath) Abs(float64) float64 { return -1 }

func BenchmarkRound26LookalikeAbsSafe(b *testing.B) {
	weight := (round26PositiveMath{}).Abs(rand.NormFloat64())
	if weight > 0 {
		round26Sink = weight
	}
}

func BenchmarkRound26LookalikeAbsRandom(b *testing.B) {
	weight := (round26RandomMath{}).Abs(1)
	if weight > 0 { // want `benchmark feeds symmetric signed random inputs`
		round26Sink = weight
	}
}

func BenchmarkRound26LookalikeAbsStoredSafe(b *testing.B) {
	abs := (round26PositiveMath{}).Abs
	weight := abs(rand.NormFloat64())
	if weight > 0 {
		round26Sink = weight
	}
}

func BenchmarkRound26LookalikeAbsExpressionSafe(b *testing.B) {
	weight := round26PositiveMath.Abs(round26PositiveMath{}, rand.NormFloat64())
	if weight > 0 {
		round26Sink = weight
	}
}

func BenchmarkRound26LookalikeAbsAlwaysHotSafe(b *testing.B) {
	weight := (round26NegativeMath{}).Abs(rand.NormFloat64())
	if weight < 0 {
		round26Sink = weight
	}
}

func BenchmarkRound26LookalikeAbsAlwaysSkipped(b *testing.B) {
	weight := (round26NegativeMath{}).Abs(rand.NormFloat64())
	if weight > 0 { // want `benchmark feeds symmetric signed random inputs`
		round26Sink = weight
	}
}

func BenchmarkRound26ParallelStructSwapSafe(b *testing.B) {
	x, y := round26State{weight: 1}, round26State{weight: rand.NormFloat64()}
	x, y = y, x
	if y.weight > 0 {
		round26Sink = y.weight
	}
}

func BenchmarkRound26ParallelStructSwapRandom(b *testing.B) {
	x, y := round26State{weight: rand.NormFloat64()}, round26State{weight: 1}
	x, y = y, x
	if y.weight > 0 { // want `benchmark feeds symmetric signed random inputs`
		round26Sink = y.weight
	}
}

func BenchmarkRound26ParallelArraySwapSafe(b *testing.B) {
	weights, safe := [1]float64{rand.NormFloat64()}, [1]float64{1}
	safe, weights = weights, safe
	if weights[0] > 0 {
		round26Sink = weights[0]
	}
}

func BenchmarkRound26ParallelArraySwapRandom(b *testing.B) {
	weights, safe := [1]float64{1}, [1]float64{rand.NormFloat64()}
	safe, weights = weights, safe
	if weights[0] > 0 { // want `benchmark feeds symmetric signed random inputs`
		round26Sink = weights[0]
	}
}
