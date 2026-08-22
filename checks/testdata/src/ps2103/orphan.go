package ps2103

// The only fmt reference in this FILE is the fixable Sprintf itself: the
// concatenation rewrite orphans the import, and the fix pipeline prunes
// the now-unused fmt import afterwards, so the fix applies (non-cgo file).

import "fmt"

func orphanKeys(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, fmt.Sprintf("k:%s", n)) // want `fmt\.Sprintf in a loop parses its format and boxes every argument per iteration; this format only splices simple verbs — build the string with concatenation or strconv instead`
	}
	return out
}
