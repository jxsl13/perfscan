package ps6010

import "unsafe"

var ps6010Round17GlobalSlice []float64
var ps6010Round17GlobalSlices map[int][]float64
var ps6010Round17GlobalAny any
var ps6010Round17GlobalChannel <-chan []float64
var ps6010Round17GlobalInts []int

func ps6010Round17OpaquePair() ([]float64, bool) {
	return ps6010Round17GlobalSlice, true
}

func ps6010Round17OpaqueSecond() (bool, []float64) {
	return true, ps6010Round17GlobalSlice
}

func ps6010Round17OpaqueIntPair() ([]int, bool) {
	return ps6010Round17GlobalInts, true
}

func ps6010Round17MutateGlobalLeaf(output int) {
	ps6010Round17GlobalSlice[0] = float64(output)
}

func ps6010Round17MutateGlobalMiddle(output int) {
	ps6010Round17MutateGlobalLeaf(output)
}

func ps6010Round17RecursiveA(output int) {
	if output < 0 {
		ps6010Round17RecursiveB(output)
	}
}

func ps6010Round17RecursiveB(output int) {
	ps6010Round17MutateGlobalLeaf(output)
	if output < 0 {
		ps6010Round17RecursiveA(output)
	}
}

func ps6010Round17MutateRaw(raw uintptr, output int) {
	*(*float64)(unsafe.Pointer(raw)) = float64(output)
}

func ps6010Round17PureScalar(value int) int { return value + 1 }

func ps6010Round17MutateIncompatibleGlobal(output int) {
	ps6010Round17GlobalInts[0] = output
}

func ps6010Round17MutateGeneric[T ~float64](output T) {
	ps6010Round17GlobalSlice[0] = float64(output)
}

func ps6010Round17AssemblyLike(output int)
