package ps3016

import (
	"cmp"
	"maps"
	"slices"
)

type Celsius int

// The bare cmp.Compare comparator is slices.Sorted spelled the slow, stable
// way; the rewrites delete this file's ONLY cmp references, so the fix drops
// the orphaned cmp import. The seq expression is kept verbatim.

// cmp.Compare passed directly.
func sortedVals(m map[string]int) []int {
	return slices.SortedStableFunc(maps.Values(m), cmp.Compare) // want `slices\.SortedStableFunc with a bare cmp\.Compare comparator pays an indirect comparator call per comparison plus the stable sort's merge overhead; slices\.Sorted collects and sorts the int elements`
}

// A func literal wrapping cmp.Compare.
func sortedKeys(m map[string]int) []string {
	return slices.SortedStableFunc(maps.Keys(m), func(a, b string) int { return cmp.Compare(a, b) }) // want `slices\.Sorted collects and sorts the string elements`
}

// A named ordered element is fixed too: sorting orders by the ordered value
// and never consults a method on the element.
func sortedNamed(cs []Celsius) []Celsius {
	return slices.SortedStableFunc(slices.Values(cs), func(a, b Celsius) int { return cmp.Compare(a, b) }) // want `slices\.Sorted collects and sorts the Celsius elements`
}
