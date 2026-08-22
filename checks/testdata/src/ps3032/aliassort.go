package ps3032

import st "sort"

// sort is imported under an ALIAS: type info still resolves st.IsSorted
// and st.IntSlice to the sort package's objects; the orphaned aliased spec
// (name included) is swapped for "slices".
func aliasedSort(xs []int) bool {
	return st.IsSorted(st.IntSlice(xs)) // want `sort\.IsSorted\(sort\.IntSlice\(\.\.\.\)\) scans through the sort\.Interface adapter \(an interface Len plus a Less dispatch per adjacent pair\); slices\.IsSorted checks the concrete \[\]int directly with the identical boolean result`
}
