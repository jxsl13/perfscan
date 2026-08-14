package ps3008

import (
	"sort"
)

// The file keeps OTHER sort references (sort.Float64sAreSorted, sort.Ints),
// so the sort import stays; the fix only inserts the slices import.
func keepSort(xs []int, fs []float64) bool {
	ok := sort.IntsAreSorted(xs) // want `sort\.IntsAreSorted is the legacy spelling of slices\.IsSorted \(an interface-dispatch scan on go1\.21, a one-line wrapper since go1\.22\); slices\.IsSorted checks the concrete \[\]int directly with the identical boolean result`

	// NEVER flagged by PS3008: sort.Float64sAreSorted would be
	// bool-identical too (nothing is reordered), but the sort family keeps
	// a bright no-float line, so it is deferred.
	ok = ok && sort.Float64sAreSorted(fs)

	// NEVER flagged by PS3008: a different sort function entirely.
	sort.Ints(xs)
	return ok
}
