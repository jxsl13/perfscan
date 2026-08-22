//go:build go1.21

package ps2116

// VERSION BOUNDARY: an explicit //go:build go1.21 pins this file's effective
// language version (pass.TypesInfo.FileVersions) EXACTLY at go1.21, where the
// clear builtin first exists. The gate is inclusive (>= go1.21), so PS2116 MUST
// fire here — a regression narrowing it to `> go1.21` would skip go1.21-pinned
// files, leaving an uncompilable clear() out of reach. Sibling of
// ps3102/boundary.go; the clear-builtin checks otherwise had no version testdata.
func zeroAtBoundary(s []int) {
	for i := range s { // want `the loop writes the zero value to every element of s; clear\(s\) states that directly \(Go 1\.21\)`
		s[i] = 0
	}
}
