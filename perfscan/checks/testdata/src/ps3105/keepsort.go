package ps3105

import (
	"sort"
)

// The file keeps OTHER sort references (sort.Slice, a Float64Slice sort),
// so the sort import stays; the fix only inserts the slices import.
func keepSort(xs []int, fs []float64, pairs [][2]int) {
	sort.Sort(sort.IntSlice(xs)) // want `sort\.Sort\(sort\.IntSlice\(\.\.\.\)\) sorts through the sort\.Interface adapter \(an interface dispatch per comparison and swap\); slices\.Sort sorts the concrete \[\]int directly with the identical ascending order`

	// NEVER flagged: float64 has distinguishable ties (-0.0/+0.0, NaN
	// payloads) that an unstable sort may arrange differently, and
	// Float64Slice.Less orders NaNs first — the rewrite is not
	// guaranteed bit-identical, so PS3105 excludes it.
	sort.Sort(sort.Float64Slice(fs))

	// NEVER flagged by PS3105: a different sort function entirely.
	sort.Slice(pairs, func(i, j int) bool { return pairs[i][1] < pairs[j][1] })
}
