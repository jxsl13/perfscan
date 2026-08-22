package ps2103cgo

// The only fmt reference in this cgo FILE is the fixable Sprintf itself:
// the concatenation rewrite would orphan the import, and a cgo file's
// import block is never pruned, so the fix is withheld — the report
// stays advisory and the golden is identical.

// #include <stdlib.h>
import "C"

import "fmt"

func cgoKeys(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, fmt.Sprintf("k:%s", n)) // want `fmt\.Sprintf in a loop parses its format and boxes every argument per iteration; this format only splices simple verbs — build the string with concatenation or strconv instead`
	}
	return out
}
