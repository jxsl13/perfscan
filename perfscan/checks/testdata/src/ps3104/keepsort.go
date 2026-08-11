package ps3104

import (
	"sort"
)

// The file keeps OTHER sort references (sort.Slice, sort.Float64s), so the
// sort import stays; the fix only inserts the slices import.
func keepSort(xs []int, fs []float64, pairs [][2]int) {
	sort.Ints(xs) // want `sort\.Ints is the legacy spelling of slices\.Sort \(an interface-dispatch sort on go1\.21, a one-line wrapper since go1\.22\); slices\.Sort sorts the concrete \[\]int directly with the identical ascending order`

	// NEVER flagged: float64 has distinguishable ties (-0.0/+0.0, NaN
	// payloads) that an unstable sort may arrange differently — the
	// rewrite is not guaranteed bit-identical, so PS3104 excludes it.
	sort.Float64s(fs)

	// NEVER flagged by PS3104: a different sort function entirely.
	sort.Slice(pairs, func(i, j int) bool { return pairs[i][1] < pairs[j][1] })
}
