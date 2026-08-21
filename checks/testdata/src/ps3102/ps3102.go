package ps3102

// The exact idiom over a local map is rewritten to clear(m).
func clearLocal(m map[string]int) {
	for k := range m { delete(m, k) } // want `m is emptied with a range-delete loop; clear\(m\) empties the map in one call`
}

// Any key/value shapes work as long as the key cannot hold NaN; only the
// loop statement is replaced, surrounding statements stay.
func clearSet(seen map[int]struct{}) int {
	before := len(seen)
	for k := range seen { delete(seen, k) } // want `seen is emptied with a range-delete loop; clear\(seen\) empties the map in one call`
	return before
}

var cache = map[string][]byte{}

// A package-level map variable is matched by object identity too.
func resetCache() {
	for k := range cache { delete(cache, k) } // want `cache is emptied with a range-delete loop; clear\(cache\) empties the map in one call`
}

// --- advisory: reported but never rewritten ---

// A float key can be NaN; delete(m, k) cannot remove a NaN key (NaN != NaN)
// but clear does, so the rewrite is not bit-identical.
func floatKeys(m map[float64]int) {
	for k := range m { // want `m is emptied with a range-delete loop; clear\(m\) empties the map in one call — advisory: the key type can hold NaN, which delete cannot remove but clear does, so the rewrite is not bit-identical`
		delete(m, k)
	}
}

// A struct key embedding a float can hold NaN just the same.
type coord struct{ x, y float64 }

func structFloatKeys(m map[coord]bool) {
	for k := range m { // want `m is emptied with a range-delete loop; clear\(m\) empties the map in one call — advisory: the key type can hold NaN, which delete cannot remove but clear does, so the rewrite is not bit-identical`
		delete(m, k)
	}
}

// An interface key may dynamically hold a float NaN.
func anyKeys(m map[any]int) {
	for k := range m { // want `m is emptied with a range-delete loop; clear\(m\) empties the map in one call — advisory: the key type can hold NaN, which delete cannot remove but clear does, so the rewrite is not bit-identical`
		delete(m, k)
	}
}

// A complex key can hold a NaN in either component (complex(NaN, 0) != itself),
// so the IsComplex exclusion keeps this advisory, like floats.
func complexKeys(m map[complex128]int) {
	for k := range m { // want `m is emptied with a range-delete loop; clear\(m\) empties the map in one call — advisory: the key type can hold NaN, which delete cannot remove but clear does, so the rewrite is not bit-identical`
		delete(m, k)
	}
}

// An array-of-floats key can hold a NaN element; the guard recurses into the
// array element type, so this stays advisory too.
func arrayFloatKeys(m map[[2]float64]int) {
	for k := range m { // want `m is emptied with a range-delete loop; clear\(m\) empties the map in one call — advisory: the key type can hold NaN, which delete cannot remove but clear does, so the rewrite is not bit-identical`
		delete(m, k)
	}
}

// A POINTER key is always reflexive (a pointer equals itself), so the NaN hazard
// cannot arise and the rewrite IS applied — a positive exercising the Pointer
// branch of the key-type guard.
func pointerKeys(m map[*int]int) {
	for k := range m { delete(m, k) } // want `m is emptied with a range-delete loop; clear\(m\) empties the map in one call`
}

// A user declaration named clear captures the rewritten call: the loop is
// reported but the fix is withheld.
func shadowedClear(m map[string]int) {
	clear := 0
	_ = clear
	for k := range m { delete(m, k) } // want `m is emptied with a range-delete loop; clear\(m\) empties the map in one call`
}

// --- guards: none of the following may be reported or rewritten ---

// The body does more than the delete.
func countAndClear(m map[string]int) int {
	n := 0
	for k := range m {
		n++
		delete(m, k)
	}
	return n
}

// A value variable is bound: not the bare key-only idiom.
func withValue(m map[string]int, sink func(int)) {
	for k, v := range m {
		sink(v)
		delete(m, k)
	}
}

// Even a blank value variable is left alone (strict shape only).
func blankValue(m map[string]int) {
	for k, _ := range m {
		delete(m, k)
	}
}

// delete targets a DIFFERENT map than the one ranged over.
func deleteOther(m, other map[string]int) {
	for k := range m {
		delete(other, k)
	}
}

// The range is over a slice, not a map.
func sliceRange(s []string, m map[string]int) {
	for i := range s {
		delete(m, s[i])
	}
}

// A shadowed delete is not the builtin.
func shadowedDelete(m map[string]int) {
	delete := func(mm map[string]int, k string) {}
	for k := range m {
		delete(m, k)
	}
}

// `for k = range m` assigns the final key to an outer variable, which
// clear(m) would not; not matched.
func assignedKey(m map[string]int) string {
	var k string
	for k = range m {
		delete(m, k)
	}
	return k
}

// An interior comment would be DELETED by the clear(m) rewrite, which replaces
// the whole range-delete loop, so PS3102 reports it WITHOUT a fix (the
// ps3102CommentsOverlap guard). Sibling of PS2116's interiorComment negative.
func interiorComment(m map[string]int) {
	for k := range m { // want `m is emptied with a range-delete loop; clear\(m\) empties the map in one call`
		// drop every entry so the map's backing store can be reused
		delete(m, k)
	}
}
