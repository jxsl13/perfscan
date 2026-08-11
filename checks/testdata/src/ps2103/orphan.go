package ps2103

// The only fmt reference in this FILE is the fixable Sprintf itself: the
// concatenation rewrite would orphan the import, so advisory only.

import "fmt"

func orphanKeys(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, fmt.Sprintf("k:%s", n)) // want `fmt\.Sprintf in a loop parses its format and boxes every argument per iteration; this format only splices simple verbs — build the string with concatenation or strconv instead`
	}
	return out
}
