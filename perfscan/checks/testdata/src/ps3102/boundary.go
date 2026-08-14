//go:build go1.21

package ps3102

// VERSION BOUNDARY: an explicit //go:build go1.21 pins this file's effective
// language version (pass.TypesInfo.FileVersions) EXACTLY at go1.21, where the
// clear builtin first exists. The gate is inclusive (>= go1.21), so PS3102 MUST
// fire here — a regression narrowing it to `> go1.21` would skip go1.21-pinned
// files, leaving an uncompilable clear() out of reach. The clear-builtin checks
// otherwise have no file-version testdata.
func clearAtBoundary(m map[string]int) {
	for k := range m { delete(m, k) } // want `m is emptied with a range-delete loop; clear\(m\) empties the map in one call`
}
