package ps2103

// ALIASED-IMPORT positive: the fixable in-loop Sprintf is written through
// an fmt alias (`import f "fmt"`), pinning that the stdlib-call detector
// matches aliased imports. The concatenation rewrite replaces the whole
// call, orphans the alias, and the fix pipeline prunes it.

import f "fmt"

func aliasedKeys(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, f.Sprintf("k:%s", n)) // want `fmt\.Sprintf in a loop parses its format and boxes every argument per iteration; this format only splices simple verbs — build the string with concatenation or strconv instead`
	}
	return out
}
