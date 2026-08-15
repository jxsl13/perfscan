package ps2036

// ALIASED-IMPORT positive: the fixable Append is written through an fmt
// alias (`import f "fmt"`), pinning that the stdlib-call detector matches
// aliased imports. The rewrite replaces the fmt.Append selector, orphans
// the alias, and the fix pipeline prunes it while adding the strconv
// import.

import f "fmt"

func aliasedAppend(buf []byte, n int) []byte {
	return f.Append(buf, n) // want `fmt\.Append with a single int/uint/bool/float operand`
}
