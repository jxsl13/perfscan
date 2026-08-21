package ps3037

import (
	"slices"
	"sort"
)

// Keepers so the source compiles and neither import is orphaned by the rewrites.
var _ = slices.Contains([]int{}, 0)
var _ = sort.IntsAreSorted

// --- POSITIVES ---

func defineInt(a []int, x int) int {
	i := sort.SearchInts(a, x) // want `sort\.Search\* runs a binary search through a per-probe closure; slices\.BinarySearch`
	return i
}

func assignString(s []string, x string) int {
	var i int
	i = sort.SearchStrings(s, x) // want `sort\.Search\* runs a binary search through a per-probe closure; slices\.BinarySearch`
	return i
}

// --- ADVISORY: reported, no fix ---

// Bare expression position — a two-result call cannot be spliced.
func exprPos(a []int, x int) bool {
	return sort.SearchInts(a, x) < len(a) // want `sort\.Search\* runs a binary search through a per-probe closure; slices\.BinarySearch`
}

// Index position.
func indexPos(a []int, x int) int {
	return a[sort.SearchInts(a, x)%len(a)] // want `sort\.Search\* runs a binary search through a per-probe closure; slices\.BinarySearch`
}

// A comment in the renamed selector withholds the fix.
func commentSel(a []int, x int) int {
	i := sort. /*keep*/ SearchInts(a, x) // want `sort\.Search\* runs a binary search through a per-probe closure; slices\.BinarySearch`
	return i
}

// --- NEGATIVES: silent ---

// SearchFloat64s disagrees with slices.BinarySearch on NaN.
func floatSearch(a []float64, x float64) int {
	i := sort.SearchFloat64s(a, x)
	return i
}

// sort.Search (the generic closure form) is not this pattern.
func genericSearch(n int, f func(int) bool) int {
	return sort.Search(n, f)
}
