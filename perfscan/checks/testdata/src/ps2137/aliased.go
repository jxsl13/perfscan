package ps2137

// ALIASED-IMPORT positive: the fixable Sprint is written through an fmt
// alias (`import f "fmt"`), pinning that the stdlib-call detector matches
// aliased imports. The rewrite replaces the whole call, orphans the alias,
// and the fix pipeline prunes it while adding the strconv import.

import f "fmt"

func aliasedSprint(n int) string {
	return f.Sprint(n) // want `fmt\.Sprint\(i\) / fmt\.Sprintf\("%v", i\) on an integer pays fmt's reflection and boxing for a plain decimal; strconv\.Itoa/FormatInt/FormatUint converts it directly`
}
