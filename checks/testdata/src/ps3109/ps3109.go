package ps3109

import (
	"cmp"
	"slices"
)

type Version int

// The bare cmp.Compare comparator is slices.BinarySearch spelled the slow way;
// the rewrites delete this file's ONLY cmp references, so the fix also drops the
// orphaned cmp import. The slice and target expressions are kept verbatim.

func searchInts(xs []int, target int) (int, bool) {
	return slices.BinarySearchFunc(xs, target, func(a, b int) int { return cmp.Compare(a, b) }) // want `slices\.BinarySearchFunc with a bare cmp\.Compare comparator`
}

// cmp.Compare passed directly (one fewer call layer) is matched too.
func searchStrings(ys []string, target string) (int, bool) {
	return slices.BinarySearchFunc(ys, target, cmp.Compare) // want `slices\.BinarySearchFunc with a bare cmp\.Compare comparator`
}

// FLOATS are fixable here, unlike the unstable-sort family (PS3107): a binary
// search is a deterministic function of the comparator, with no tie-arrangement
// freedom to make -0.0/+0.0 or NaN payloads observable.
func searchFloats(fs []float64, target float64) (int, bool) {
	return slices.BinarySearchFunc(fs, target, func(a, b float64) int { return cmp.Compare(a, b) }) // want `slices\.BinarySearchFunc with a bare cmp\.Compare comparator`
}

// A named ordered element is fixed too.
func searchNamed(vs []Version, target Version) (int, bool) {
	return slices.BinarySearchFunc(vs, target, func(a, b Version) int { return cmp.Compare(a, b) }) // want `slices\.BinarySearchFunc with a bare cmp\.Compare comparator`
}
